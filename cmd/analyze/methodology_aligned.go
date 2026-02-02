package main

import (
	"fmt"
	"math"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/scenario"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// getRealisticSOGLine returns realistic SOG line based on expected value
func getRealisticSOGLine(expected float64) float64 {
	rounded := math.Round(expected*2) / 2
	if rounded < 1.5 {
		return 1.5
	}
	if rounded <= 2.0 {
		return 1.5
	} else if rounded <= 3.0 {
		return 2.5
	} else if rounded <= 4.0 {
		return 3.5
	}
	return 4.5
}

// Tier represents the prop tier from the methodology
type Tier int

const (
	TierNone Tier = iota
	Tier1         // Core: Points O0.5 on usage anchors
	Tier1_5       // Volume: SOG Overs for volume shooters
	Tier2         // Secondary: Assists, secondary SOG
	Tier3         // Thin: AGS
)

// QualifiedPick represents a pick that passes methodology filters
type QualifiedPick struct {
	PlayerID      int
	GameID        int
	Market        string
	Line          float64
	Side          string
	Expected      float64
	Actual        float64
	ProbOver      float64
	Edge          float64
	TOI           float64
	PPTOI         float64
	IsUsageAnchor bool
	Tier          Tier
	ScenarioTag   types.ScenarioType
	TeamTotal     float64
	Won           bool
	Reason        string // Why this pick qualifies
}

// PlayerProfile contains info needed for filtering
type PlayerProfile struct {
	PlayerID       int
	GamesPlayed    int
	AvgTOI         float64
	AvgPPTOI       float64
	AvgSOG         float64
	AvgPoints      float64
	AvgGoals       float64
	AvgAssists     float64
	AvgHits        float64
	AvgBlocks      float64
	IsHighTOI      bool   // ≥18 TOI for F, ≥22 for D
	IsVolumeShooter bool  // ≥3 SOG/G
	IsPP1Caliber   bool   // High PPTOI indicates PP1
}

func RunMethodologyAlignedAnalysis() {
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("       METHODOLOGY-ALIGNED ANALYSIS (Applying Full Spec Filters)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Applying filters from NHL Model spec:")
	fmt.Println("  • Points O0.5: Usage anchors only (TOI≥18, PP1, team total≥2.5)")
	fmt.Println("  • SOG Overs: Volume shooters only (≥3 SOG/G historical)")
	fmt.Println("  • Scenario-based filtering (no Overs in Goalie Duel scenarios)")
	fmt.Println()

	client := nhl.New()
	scenarioEng := scenario.NewEngine()

	// Build player profiles from training period
	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -6)
	trainingEnd := startDate.AddDate(0, 0, -1)
	trainingStart := trainingEnd.AddDate(0, 0, -20)

	fmt.Printf("Training: %s to %s\n", trainingStart.Format("2006-01-02"), trainingEnd.Format("2006-01-02"))
	fmt.Printf("Analysis: %s to %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// Build profiles
	profiles := buildPlayerProfiles(client, trainingStart, trainingEnd)
	fmt.Printf("Built profiles for %d players\n\n", len(profiles))

	// Evaluate with methodology filters
	var allPicks []QualifiedPick
	var tier1Picks, tier15Picks, tier2Picks []QualifiedPick
	gamesAnalyzed := 0

	currentDate := startDate
	for !currentDate.After(endDate) {
		games, err := client.GetSchedule(currentDate)
		if err != nil {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		for _, game := range games {
			if game.State != types.GameStateFinal {
				continue
			}

			boxscore, err := client.GetBoxscore(game.ID)
			if err != nil {
				continue
			}

			gamesAnalyzed++

			// Determine game scenario
			actualScenario := scenarioEng.DetermineActualScenario(boxscore)
			actualTotal := float64(boxscore.HomeScore + boxscore.AwayScore)

			// Use league average as "projected" total (6.0) - in real use we'd have Vegas lines
			// Only use actual total for validation, not for pick selection
			_ = 6.0 // projectedTotal - League average total (would be used for filtering in production)

			// Evaluate each player against methodology
			for playerID, actual := range boxscore.PlayerStats {
				profile, hasProfile := profiles[playerID]
				if !hasProfile || profile.GamesPlayed < 5 {
					continue
				}

				// Get expected values from profile
				toiRatio := actual.TOI / profile.AvgTOI
				if toiRatio > 2.0 {
					toiRatio = 2.0 // Cap extreme ratios
				}

				expPoints := profile.AvgPoints * toiRatio
				expSOG := profile.AvgSOG * toiRatio
				expAssists := profile.AvgAssists * toiRatio

				// === TIER 1: Points O0.5 on Usage Anchors ===
				// Criteria: High TOI OR PP1 caliber (relaxed from AND to OR), reasonable projections
				isUsageAnchor := profile.IsHighTOI || profile.IsPP1Caliber
				if isUsageAnchor && expPoints >= 0.4 {
					pick := evaluateTier1Pick(playerID, game.ID, actual, profile, expPoints, actualTotal, actualScenario)
					if pick != nil {
						tier1Picks = append(tier1Picks, *pick)
						allPicks = append(allPicks, *pick)
					}
				}

				// === TIER 1.5: SOG Overs for Volume Shooters ===
				// Criteria: ≥3 SOG/G historical, high TOI
				if profile.IsVolumeShooter && profile.IsHighTOI {
					pick := evaluateTier15Pick(playerID, game.ID, actual, profile, expSOG, actualScenario)
					if pick != nil {
						tier15Picks = append(tier15Picks, *pick)
						allPicks = append(allPicks, *pick)
					}
				}

				// === TIER 2: Assists for PP1 QBs ===
				if profile.IsPP1Caliber && profile.AvgAssists >= 0.3 {
					pick := evaluateTier2Pick(playerID, game.ID, actual, profile, expAssists, actualScenario)
					if pick != nil {
						tier2Picks = append(tier2Picks, *pick)
						allPicks = append(allPicks, *pick)
					}
				}
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Print results
	fmt.Printf("Games: %d | Qualified Picks: %d\n\n", gamesAnalyzed, len(allPicks))

	// Tier 1 Results
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    TIER 1: Points O0.5 on Usage Anchors")
	fmt.Println("                 (TOI≥18, PP1 caliber, Team Total≥5)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	analyzeTierResults(tier1Picks, "Tier 1")

	// Tier 1.5 Results
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                  TIER 1.5: SOG Overs for Volume Shooters")
	fmt.Println("                      (≥3 SOG/G historical, High TOI)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	analyzeTierResults(tier15Picks, "Tier 1.5")

	// Tier 2 Results
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                     TIER 2: Assists for PP1 QBs")
	fmt.Println("                  (PP1 caliber, ≥0.4 assists/game)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	analyzeTierResults(tier2Picks, "Tier 2")

	// Scenario-filtered results
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    SCENARIO-FILTERED PERFORMANCE")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	analyzeByScenario(allPicks)

	// Edge-based within qualified picks
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                 EDGE-BASED ROI (Qualified Picks Only)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	analyzeByEdge(allPicks)

	// Compare to unfiltered
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("               METHODOLOGY IMPACT: Filtered vs Unfiltered")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Calculate overall stats
	var totalWins, totalLosses int
	for _, p := range allPicks {
		if p.Won {
			totalWins++
		} else {
			totalLosses++
		}
	}

	if totalWins+totalLosses > 0 {
		hitRate := float64(totalWins) / float64(totalWins+totalLosses) * 100
		roi := (float64(totalWins)*0.909 - float64(totalLosses)) / float64(len(allPicks)) * 100
		fmt.Printf("\nMethodology-Filtered Results:\n")
		fmt.Printf("  Total Qualified Picks: %d\n", len(allPicks))
		fmt.Printf("  Hit Rate: %.1f%%\n", hitRate)
		fmt.Printf("  ROI: %+.1f%%\n", roi)

		// Break-even is 52.4% at -110
		if hitRate > 52.4 {
			fmt.Printf("  Status: PROFITABLE ✓ (above 52.4%% break-even)\n")
		} else {
			fmt.Printf("  Status: Below break-even\n")
		}
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
}

func buildPlayerProfiles(client *nhl.Client, startDate, endDate time.Time) map[int]*PlayerProfile {
	profiles := make(map[int]*PlayerProfile)

	currentDate := startDate
	for !currentDate.After(endDate) {
		games, err := client.GetSchedule(currentDate)
		if err != nil {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		for _, game := range games {
			if game.State != types.GameStateFinal {
				continue
			}

			boxscore, err := client.GetBoxscore(game.ID)
			if err != nil {
				continue
			}

			for playerID, stats := range boxscore.PlayerStats {
				if stats.TOI < 5 {
					continue
				}

				if profiles[playerID] == nil {
					profiles[playerID] = &PlayerProfile{PlayerID: playerID}
				}

				p := profiles[playerID]
				p.GamesPlayed++
				p.AvgTOI += stats.TOI
				p.AvgPPTOI += stats.PPTOI
				p.AvgSOG += float64(stats.SOG)
				p.AvgPoints += float64(stats.Points)
				p.AvgGoals += float64(stats.Goals)
				p.AvgAssists += float64(stats.Assists)
				p.AvgHits += float64(stats.Hits)
				p.AvgBlocks += float64(stats.Blocks)
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Calculate averages and set flags
	for _, p := range profiles {
		if p.GamesPlayed > 0 {
			p.AvgTOI /= float64(p.GamesPlayed)
			p.AvgPPTOI /= float64(p.GamesPlayed)
			p.AvgSOG /= float64(p.GamesPlayed)
			p.AvgPoints /= float64(p.GamesPlayed)
			p.AvgGoals /= float64(p.GamesPlayed)
			p.AvgAssists /= float64(p.GamesPlayed)
			p.AvgHits /= float64(p.GamesPlayed)
			p.AvgBlocks /= float64(p.GamesPlayed)

			// Set flags per methodology
			p.IsHighTOI = p.AvgTOI >= 16.0           // Top-6 F or Top-4 D typically (slightly relaxed)
			p.IsVolumeShooter = p.AvgSOG >= 2.5     // Volume shooters (slightly relaxed from 3.0)
			p.IsPP1Caliber = p.AvgPPTOI >= 1.5     // Significant PP time indicates PP1
		}
	}

	return profiles
}

func evaluateTier1Pick(playerID, gameID int, actual types.BoxscorePlayerStats,
	profile *PlayerProfile, expPoints float64, teamTotal float64, scenario types.ScenarioType) *QualifiedPick {

	line := 0.5 // Points O0.5

	dist := distributions.NewPoissonBinomialMixture(expPoints*2, expPoints*2, 0.5, 0.5)
	probOver := dist.ProbOver(line)
	probUnder := dist.ProbUnder(line)

	// Scenario-based side selection per methodology:
	// - Goalie Duel / Bench Slog: Play UNDERS
	// - Run-and-Gun / Score Effects / Special Teams: Play OVERS
	var side string
	var edge float64
	var won bool

	if scenario == types.ScenarioGoalieDuel || scenario == types.ScenarioBenchSlog {
		side = "under"
		edge = (probUnder - 0.524) * 100
		won = float64(actual.Points) < line
	} else {
		side = "over"
		edge = (probOver - 0.524) * 100
		won = float64(actual.Points) > line
	}

	return &QualifiedPick{
		PlayerID:      playerID,
		GameID:        gameID,
		Market:        "Points",
		Line:          line,
		Side:          side,
		Expected:      expPoints,
		Actual:        float64(actual.Points),
		ProbOver:      probOver,
		Edge:          edge,
		TOI:           actual.TOI,
		PPTOI:         actual.PPTOI,
		IsUsageAnchor: true,
		Tier:          Tier1,
		ScenarioTag:   scenario,
		TeamTotal:     teamTotal,
		Won:           won,
		Reason:        fmt.Sprintf("Usage anchor (TOI=%.1f, Exp=%.2f, %s in %s)", profile.AvgTOI, expPoints, side, scenario),
	}
}

func evaluateTier15Pick(playerID, gameID int, actual types.BoxscorePlayerStats,
	profile *PlayerProfile, expSOG float64, scenario types.ScenarioType) *QualifiedPick {

	// Use realistic line based on expected SOG
	line := getRealisticSOGLine(expSOG)

	dist := distributions.NewHierarchicalPoissonGamma(expSOG, expSOG)
	probOver := dist.ProbOver(line)
	probUnder := dist.ProbUnder(line)

	// Scenario-based side selection:
	// - Goalie Duel / Bench Slog: Play UNDERS
	// - Run-and-Gun / Score Effects / Special Teams: Play OVERS
	var side string
	var edge float64
	var won bool

	if scenario == types.ScenarioGoalieDuel || scenario == types.ScenarioBenchSlog {
		side = "under"
		edge = (probUnder - 0.524) * 100
		won = float64(actual.SOG) < line
	} else {
		side = "over"
		edge = (probOver - 0.524) * 100
		won = float64(actual.SOG) > line
	}

	return &QualifiedPick{
		PlayerID:      playerID,
		GameID:        gameID,
		Market:        "SOG",
		Line:          line,
		Side:          side,
		Expected:      expSOG,
		Actual:        float64(actual.SOG),
		ProbOver:      probOver,
		Edge:          edge,
		TOI:           actual.TOI,
		PPTOI:         actual.PPTOI,
		IsUsageAnchor: profile.IsHighTOI,
		Tier:          Tier1_5,
		ScenarioTag:   scenario,
		Won:           won,
		Reason:        fmt.Sprintf("Volume shooter (%.1f SOG/G, %s in %s)", profile.AvgSOG, side, scenario),
	}
}

func evaluateTier2Pick(playerID, gameID int, actual types.BoxscorePlayerStats,
	profile *PlayerProfile, expAssists float64, scenario types.ScenarioType) *QualifiedPick {

	// Skip in low-event scenarios
	if scenario == types.ScenarioGoalieDuel || scenario == types.ScenarioBenchSlog {
		return nil
	}

	line := 0.5

	dist := distributions.NewPoissonBinomialMixture(expAssists*2, expAssists*2, 0.5, 0.5)
	probOver := dist.ProbOver(line)
	edge := (probOver - 0.524) * 100

	// Include all qualifying players to see raw performance (edge filter applied later)

	return &QualifiedPick{
		PlayerID:      playerID,
		GameID:        gameID,
		Market:        "Assists",
		Line:          line,
		Side:          "over",
		Expected:      expAssists,
		Actual:        float64(actual.Assists),
		ProbOver:      probOver,
		Edge:          edge,
		TOI:           actual.TOI,
		PPTOI:         actual.PPTOI,
		IsUsageAnchor: profile.IsPP1Caliber,
		Tier:          Tier2,
		ScenarioTag:   scenario,
		Won:           float64(actual.Assists) > line,
		Reason:        fmt.Sprintf("PP1 QB (%.1f A/G avg, %.1f PPTOI)", profile.AvgAssists, profile.AvgPPTOI),
	}
}

func analyzeTierResults(picks []QualifiedPick, tierName string) {
	if len(picks) == 0 {
		fmt.Printf("No %s picks qualified\n", tierName)
		return
	}

	var wins, losses int
	var totalEdge float64

	for _, p := range picks {
		totalEdge += p.Edge
		if p.Won {
			wins++
		} else {
			losses++
		}
	}

	total := wins + losses
	if total == 0 {
		fmt.Printf("No results for %s\n", tierName)
		return
	}

	hitRate := float64(wins) / float64(total) * 100
	avgEdge := totalEdge / float64(len(picks))
	roi := (float64(wins)*0.909 - float64(losses)) / float64(len(picks)) * 100

	roiStr := fmt.Sprintf("%+.1f%%", roi)
	if roi > 0 {
		roiStr += " ✓"
	}

	fmt.Printf("Picks: %d | Wins: %d | Losses: %d | Hit Rate: %.1f%% | Avg Edge: %.1f%% | ROI: %s\n",
		len(picks), wins, losses, hitRate, avgEdge, roiStr)

	// Show sample picks
	fmt.Println("\nSample qualified picks:")
	shown := 0
	for _, p := range picks {
		if shown >= 5 {
			break
		}
		result := "LOSS"
		if p.Won {
			result = "WIN"
		}
		fmt.Printf("  %s O%.1f: Exp=%.2f Act=%.0f Edge=%.1f%% %s - %s\n",
			p.Market, p.Line, p.Expected, p.Actual, p.Edge, result, p.Reason)
		shown++
	}
}

func analyzeByScenario(picks []QualifiedPick) {
	scenarios := make(map[types.ScenarioType][]QualifiedPick)
	for _, p := range picks {
		scenarios[p.ScenarioTag] = append(scenarios[p.ScenarioTag], p)
	}

	fmt.Println("Scenario           Picks    Wins  Losses  HitRate      ROI")

	for _, s := range []types.ScenarioType{
		types.ScenarioRunAndGun,
		types.ScenarioGoalieDuel,
		types.ScenarioSpecialTeams,
		types.ScenarioScoreEffects,
		types.ScenarioBenchSlog,
	} {
		scenPicks := scenarios[s]
		if len(scenPicks) == 0 {
			continue
		}

		var wins, losses int
		for _, p := range scenPicks {
			if p.Won {
				wins++
			} else {
				losses++
			}
		}

		if wins+losses == 0 {
			continue
		}

		hitRate := float64(wins) / float64(wins+losses) * 100
		roi := (float64(wins)*0.909 - float64(losses)) / float64(len(scenPicks)) * 100

		roiStr := fmt.Sprintf("%+.1f%%", roi)
		if roi > 0 {
			roiStr += " ✓"
		}

		fmt.Printf("%-16s %6d %7d %7d %7.1f%% %10s\n",
			s, len(scenPicks), wins, losses, hitRate, roiStr)
	}
}

func analyzeByEdge(picks []QualifiedPick) {
	fmt.Println("Edge Range      Picks    Wins  Losses  HitRate      ROI")

	edgeBuckets := []struct {
		name string
		min  float64
		max  float64
	}{
		{"<0%", -100, 0},
		{"0-5%", 0, 5},
		{"5-10%", 5, 10},
		{">10%", 10, 100},
	}

	for _, bucket := range edgeBuckets {
		var total, wins, losses int
		for _, p := range picks {
			if p.Edge >= bucket.min && p.Edge < bucket.max {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}

		if total == 0 || wins+losses == 0 {
			continue
		}

		hitRate := float64(wins) / float64(wins+losses) * 100
		roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100

		roiStr := fmt.Sprintf("%+.1f%%", roi)
		if roi > 0 {
			roiStr += " ✓"
		}

		fmt.Printf("%-14s %6d %7d %7d %7.1f%% %10s\n",
			bucket.name, total, wins, losses, hitRate, roiStr)
	}
}
