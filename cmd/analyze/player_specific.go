package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// PlayerHistory tracks a player's recent performance
type PlayerHistory struct {
	PlayerID     int
	GamesPlayed  int
	TotalTOI     float64
	TotalSOG     int
	TotalGoals   int
	TotalAssists int
	TotalPoints  int
	TotalHits    int
	TotalBlocks  int
	TotalPPTOI   float64
}

// RunPlayerSpecificAnalysis uses player-specific baselines instead of league averages
func RunPlayerSpecificAnalysis() {
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("         PLAYER-SPECIFIC ANALYSIS (Using Historical Performance)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("This analysis uses each player's last 10 games as their baseline,")
	fmt.Println("simulating how sportsbooks actually set player prop lines.")
	fmt.Println()

	client := nhl.New()

	// Analysis period: last 7 days
	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -6)

	// Training period: 10 games before analysis period
	trainingEnd := startDate.AddDate(0, 0, -1)
	trainingStart := trainingEnd.AddDate(0, 0, -20) // ~20 days to get 10 games per player

	fmt.Printf("Training Period: %s to %s (building player baselines)\n",
		trainingStart.Format("2006-01-02"), trainingEnd.Format("2006-01-02"))
	fmt.Printf("Analysis Period: %s to %s (evaluating predictions)\n\n",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// Build player histories from training period
	playerHistory := buildPlayerHistory(client, trainingStart, trainingEnd)
	fmt.Printf("Built baselines for %d players\n\n", len(playerHistory))

	// Now evaluate on analysis period
	var picks []RealisticPick
	gamesAnalyzed := 0
	playersEvaluated := 0

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

			for playerID, actual := range boxscore.PlayerStats {
				if actual.TOI < 10 {
					continue
				}

				// Get player's historical baseline
				hist, hasHistory := playerHistory[playerID]
				if !hasHistory || hist.GamesPlayed < 5 {
					continue // Skip players without enough history
				}

				playersEvaluated++

				// Calculate player-specific expected values
				avgTOI := hist.TotalTOI / float64(hist.GamesPlayed)
				toiRatio := actual.TOI / avgTOI // How much more/less TOI than usual

				// Player's per-game averages
				expSOG := float64(hist.TotalSOG) / float64(hist.GamesPlayed) * toiRatio
				expGoals := float64(hist.TotalGoals) / float64(hist.GamesPlayed) * toiRatio
				expAssists := float64(hist.TotalAssists) / float64(hist.GamesPlayed) * toiRatio
				expPoints := float64(hist.TotalPoints) / float64(hist.GamesPlayed) * toiRatio
				expHits := float64(hist.TotalHits) / float64(hist.GamesPlayed) * toiRatio
				expBlocks := float64(hist.TotalBlocks) / float64(hist.GamesPlayed) * toiRatio

				// Evaluate picks
				playerPicks := evaluatePlayerSpecificPicks(playerID, game.ID, actual,
					expSOG, expGoals, expAssists, expPoints, expHits, expBlocks)
				picks = append(picks, playerPicks...)
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	fmt.Printf("Games: %d | Players Evaluated: %d | Total Picks: %d\n\n",
		gamesAnalyzed, playersEvaluated, len(picks))

	// Analyze results
	analyzePlayerSpecificResults(picks)
}

func buildPlayerHistory(client *nhl.Client, startDate, endDate time.Time) map[int]*PlayerHistory {
	history := make(map[int]*PlayerHistory)

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

				if history[playerID] == nil {
					history[playerID] = &PlayerHistory{PlayerID: playerID}
				}

				h := history[playerID]
				h.GamesPlayed++
				h.TotalTOI += stats.TOI
				h.TotalSOG += stats.SOG
				h.TotalGoals += stats.Goals
				h.TotalAssists += stats.Assists
				h.TotalPoints += stats.Points
				h.TotalHits += stats.Hits
				h.TotalBlocks += stats.Blocks
				h.TotalPPTOI += stats.PPTOI
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return history
}

func evaluatePlayerSpecificPicks(playerID, gameID int, actual types.BoxscorePlayerStats,
	expSOG, expGoals, expAssists, expPoints, expHits, expBlocks float64) []RealisticPick {

	var picks []RealisticPick
	impliedProb := 0.524

	// Points - Core market
	{
		line := getRealisticLine("Points", expPoints)
		dist := distributions.NewPoissonBinomialMixture(expPoints*2, expPoints*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Points", Line: line,
			Expected: expPoints, Actual: float64(actual.Points),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Points) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Points", Line: line,
			Expected: expPoints, Actual: float64(actual.Points),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Points) < line,
		})
	}

	// Goals
	{
		line := getRealisticLine("Goals", expGoals)
		dist := distributions.NewPoissonBinomialMixture(expGoals*2, expGoals*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Goals", Line: line,
			Expected: expGoals, Actual: float64(actual.Goals),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Goals) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Goals", Line: line,
			Expected: expGoals, Actual: float64(actual.Goals),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Goals) < line,
		})
	}

	// Assists
	{
		line := getRealisticLine("Assists", expAssists)
		dist := distributions.NewPoissonBinomialMixture(expAssists*2, expAssists*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Assists", Line: line,
			Expected: expAssists, Actual: float64(actual.Assists),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Assists) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Assists", Line: line,
			Expected: expAssists, Actual: float64(actual.Assists),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Assists) < line,
		})
	}

	// SOG
	{
		line := getRealisticLine("SOG", expSOG)
		dist := distributions.NewHierarchicalPoissonGamma(expSOG, expSOG)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "SOG", Line: line,
			Expected: expSOG, Actual: float64(actual.SOG),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.SOG) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "SOG", Line: line,
			Expected: expSOG, Actual: float64(actual.SOG),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.SOG) < line,
		})
	}

	// Hits
	{
		line := getRealisticLine("Hits", expHits)
		dist := distributions.NewZeroInflatedNegBin(expHits, expHits)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Hits", Line: line,
			Expected: expHits, Actual: float64(actual.Hits),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Hits) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Hits", Line: line,
			Expected: expHits, Actual: float64(actual.Hits),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Hits) < line,
		})
	}

	// Blocks
	{
		line := getRealisticLine("Blocks", expBlocks)
		dist := distributions.NewZeroInflatedNegBin(expBlocks, expBlocks)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Blocks", Line: line,
			Expected: expBlocks, Actual: float64(actual.Blocks),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Blocks) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Blocks", Line: line,
			Expected: expBlocks, Actual: float64(actual.Blocks),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Blocks) < line,
		})
	}

	return picks
}

func analyzePlayerSpecificResults(picks []RealisticPick) {
	// Market performance
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                MARKET PERFORMANCE (Player-Specific Baselines)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market/Line/Side    Picks    Wins  Losses  HitRate  AvgEdge      ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	type MarketKey struct {
		Market string
		Line   float64
		Side   string
	}
	marketStats := make(map[MarketKey]*struct {
		Total   int
		Wins    int
		Losses  int
		AvgEdge float64
	})

	for _, p := range picks {
		key := MarketKey{p.Market, p.Line, p.Side}
		if marketStats[key] == nil {
			marketStats[key] = &struct {
				Total   int
				Wins    int
				Losses  int
				AvgEdge float64
			}{}
		}
		stats := marketStats[key]
		stats.Total++
		stats.AvgEdge += p.Edge
		if p.Won {
			stats.Wins++
		} else {
			stats.Losses++
		}
	}

	var keys []MarketKey
	for k := range marketStats {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Market != keys[j].Market {
			return keys[i].Market < keys[j].Market
		}
		if keys[i].Line != keys[j].Line {
			return keys[i].Line < keys[j].Line
		}
		return keys[i].Side < keys[j].Side
	})

	for _, key := range keys {
		stats := marketStats[key]
		if stats.Total > 0 {
			hitRate := float64(stats.Wins) / float64(stats.Wins+stats.Losses) * 100
			avgEdge := stats.AvgEdge / float64(stats.Total)
			roi := (float64(stats.Wins)*0.909 - float64(stats.Losses)) / float64(stats.Total) * 100

			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}

			fmt.Printf("%-8s O%.1f %-5s %6d %7d %7d %7.1f%% %7.1f%% %10s\n",
				key.Market, key.Line, key.Side, stats.Total, stats.Wins, stats.Losses,
				hitRate, avgEdge, roiStr)
		}
	}

	// Edge-based ROI
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    EDGE-BASED ROI (Player-Specific)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Edge Range      Picks    Wins  Losses  HitRate      ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	edgeBuckets := []struct {
		name string
		min  float64
		max  float64
	}{
		{"<-10%", -100, -10},
		{"-10% to -5%", -10, -5},
		{"-5% to 0%", -5, 0},
		{"0% to 2%", 0, 2},
		{"2% to 5%", 2, 5},
		{"5% to 10%", 5, 10},
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
		if total > 0 && (wins+losses) > 0 {
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

	// Points O0.5 focused analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("              POINTS O0.5 PERFORMANCE (Player-Specific)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	var pointsOver []RealisticPick
	for _, p := range picks {
		if p.Market == "Points" && p.Line == 0.5 && p.Side == "over" {
			pointsOver = append(pointsOver, p)
		}
	}

	fmt.Println("\nBy Expected Points (player's recent avg):")
	fmt.Println("Expected Pts    Picks    Wins  Losses  HitRate      ROI")

	expBuckets := []struct {
		name string
		min  float64
		max  float64
	}{
		{"0.0-0.3", 0.0, 0.3},
		{"0.3-0.5", 0.3, 0.5},
		{"0.5-0.7", 0.5, 0.7},
		{"0.7-1.0", 0.7, 1.0},
		{">1.0", 1.0, 100},
	}

	for _, bucket := range expBuckets {
		var total, wins, losses int
		for _, p := range pointsOver {
			if p.Expected >= bucket.min && p.Expected < bucket.max {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
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

	// Compare to under
	fmt.Println("\nPoints O0.5 Over vs Under comparison:")
	var overWins, overLosses, underWins, underLosses int
	for _, p := range picks {
		if p.Market == "Points" && p.Line == 0.5 {
			if p.Side == "over" {
				if p.Won {
					overWins++
				} else {
					overLosses++
				}
			} else {
				if p.Won {
					underWins++
				} else {
					underLosses++
				}
			}
		}
	}

	if overWins+overLosses > 0 {
		overHitRate := float64(overWins) / float64(overWins+overLosses) * 100
		overROI := (float64(overWins)*0.909 - float64(overLosses)) / float64(overWins+overLosses) * 100
		fmt.Printf("  Points O0.5 OVER:  %5d picks, %.1f%% hit rate, %+.1f%% ROI\n",
			overWins+overLosses, overHitRate, overROI)
	}
	if underWins+underLosses > 0 {
		underHitRate := float64(underWins) / float64(underWins+underLosses) * 100
		underROI := (float64(underWins)*0.909 - float64(underLosses)) / float64(underWins+underLosses) * 100
		fmt.Printf("  Points O0.5 UNDER: %5d picks, %.1f%% hit rate, %+.1f%% ROI\n",
			underWins+underLosses, underHitRate, underROI)
	}

	// Calibration check
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                         CALIBRATION CHECK")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Predicted Prob    Picks      Hits    Actual%   Calibration")

	calBins := []struct {
		name string
		min  float64
		max  float64
		mid  float64
	}{
		{"0-20%", 0.0, 0.2, 10},
		{"20-40%", 0.2, 0.4, 30},
		{"40-60%", 0.4, 0.6, 50},
		{"60-80%", 0.6, 0.8, 70},
		{"80-100%", 0.8, 1.0, 90},
	}

	for _, bin := range calBins {
		var total, hits int
		for _, p := range picks {
			var prob float64
			if p.Side == "over" {
				prob = p.ProbOver
			} else {
				prob = p.ProbUnder
			}
			if prob >= bin.min && prob < bin.max {
				total++
				if p.Won {
					hits++
				}
			}
		}
		if total > 0 {
			actualRate := float64(hits) / float64(total) * 100
			diff := actualRate - bin.mid
			status := "✓"
			if math.Abs(diff) > 10 {
				status = "OFF"
			}
			fmt.Printf("%-14s %8d %9d %8.1f%% %8s (%+.1f%%)\n",
				bin.name, total, hits, actualRate, status, diff)
		}
	}

	// Final summary
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    PLAYER-SPECIFIC MODEL SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	// Find profitable opportunities
	fmt.Println("\nProfitable Opportunities (ROI > 0%):")
	type OppResult struct {
		market  string
		line    float64
		side    string
		picks   int
		hitRate float64
		roi     float64
	}
	var profitable []OppResult

	for key, stats := range marketStats {
		if stats.Total >= 50 {
			hitRate := float64(stats.Wins) / float64(stats.Wins+stats.Losses) * 100
			roi := (float64(stats.Wins)*0.909 - float64(stats.Losses)) / float64(stats.Total) * 100
			if roi > 0 {
				profitable = append(profitable, OppResult{
					key.Market, key.Line, key.Side, stats.Total, hitRate, roi,
				})
			}
		}
	}

	sort.Slice(profitable, func(i, j int) bool {
		return profitable[i].roi > profitable[j].roi
	})

	for _, opp := range profitable {
		fmt.Printf("  • %s %s %.1f: %d picks, %.1f%% hit rate, +%.1f%% ROI\n",
			opp.market, opp.side, opp.line, opp.picks, opp.hitRate, opp.roi)
	}

	if len(profitable) == 0 {
		fmt.Println("  No markets showing consistent positive ROI")
	}

	fmt.Println()
}
