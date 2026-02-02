// +build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/correlation"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/props"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// FullPick represents a comprehensive pick with all data
type FullPick struct {
	GameTime     string
	Matchup      string
	Scenario     types.ScenarioType
	Player       string
	Team         string
	Position     types.Position
	TOI          float64
	Market       types.PropMarket
	Line         float64
	Side         types.PropSide
	OurLine      float64
	ProbHit      float64
	Edge         float64
	EV           float64
	Stars        int
	Tier         types.PropTier
	IsUsageAnch  bool
	IsVolShoot   bool
	IsPP1        bool
	DistType     string
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("      NHL PROP MODEL - COMPREHENSIVE SLATE ANALYSIS (ALL GAMES)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Date: %s\n\n", time.Now().Format("Monday, January 2, 2006"))

	client := nhl.New()
	evaluator := props.NewEvaluator()
	corrMatrix := correlation.NewMatrix()
	distFactory := distributions.NewFactory()

	season := getCurrentSeason(time.Now())

	// Fetch schedule
	games, err := client.GetSchedule(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Games: %d\n", len(games))

	// Fetch standings
	teamStats, _ := client.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var allPicks []FullPick
	gamesAnalyzed := 0

	// Process ALL games
	for i, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats, okHome := teamStatsMap[homeTeam]
		awayStats, okAway := teamStatsMap[awayTeam]
		if !okHome || !okAway {
			continue
		}

		scenario := determineScenario(homeStats, awayStats)
		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame
		matchup := fmt.Sprintf("%s@%s", awayTeam, homeTeam)
		gameTime := game.StartTime.Local().Format("3:04PM")

		fmt.Printf("\r[%d/%d] Processing %s...", i+1, len(games), matchup)

		gameScenario := &types.GameScenario{
			GameID: game.ID,
			Primary: types.ScenarioScore{
				Scenario: scenario,
				Score:    70,
				Rank:     1,
			},
		}

		// Fetch rosters
		homeRoster, err := client.GetRoster(homeTeam)
		if err != nil {
			continue
		}
		awayRoster, err := client.GetRoster(awayTeam)
		if err != nil {
			continue
		}

		allRoster := append(homeRoster, awayRoster...)

		ctx := distributions.DistributionContext{
			TeamTotal:     expectedTotal / 2,
			OppGA60:       awayStats.GAPerGame * 20,
			HomeGame:      true,
			ArenaSOGBias:  1.0,
			FatigueFactor: 0.0,
		}

		for _, p := range allRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 10 {
				continue
			}

			// Role flags
			if p.Position == types.PositionDefense {
				pStats.IsTopFour = pStats.TOI >= 18.0
			} else {
				pStats.IsTopSix = pStats.TOI >= 14.0
			}
			pStats.PP1Unit = pStats.PPTOI >= 1.5

			ctx.ExpectedTOI = pStats.TOI
			ctx.PP1Unit = pStats.PP1Unit

			player := &types.PlayerWithStats{
				Player: p,
				Stats:  *pStats,
			}

			// Points O0.5
			if pStats.TOI >= 12 {
				pick := analyzePointsPick(player, ctx, gameScenario, evaluator, distFactory, gameTime, matchup)
				if pick.Edge > 0 {
					allPicks = append(allPicks, pick)
				}
			}

			// SOG
			if pStats.SOG >= 1.5 {
				pick := analyzeSOGPick(player, ctx, gameScenario, evaluator, distFactory, gameTime, matchup)
				if pick.Edge > 0 {
					allPicks = append(allPicks, pick)
				}
			}
		}
		gamesAnalyzed++
	}

	fmt.Printf("\r%s\n", strings.Repeat(" ", 60))
	fmt.Printf("Games Analyzed: %d | Total Positive-Edge Picks: %d\n\n", gamesAnalyzed, len(allPicks))

	// Sort by edge
	sort.Slice(allPicks, func(i, j int) bool {
		return allPicks[i].Edge > allPicks[j].Edge
	})

	// TIER 1: Points O0.5 for Usage Anchors
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  TIER 1: POINTS O0.5 - USAGE ANCHORS (TOI≥16 or PP1 or Top-6/4)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	tier1 := filterPicks(allPicks, func(p FullPick) bool {
		return p.Market == types.PropMarketPointsOver && p.IsUsageAnch
	})

	printPicksTable(tier1, 15)

	// TIER 1.5: SOG Overs for Volume Shooters
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  TIER 1.5: SOG OVERS - VOLUME SHOOTERS (≥2.5 SOG/G)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	tier15 := filterPicks(allPicks, func(p FullPick) bool {
		return p.Market == types.PropMarketSOGOver && p.IsVolShoot
	})

	printPicksTable(tier15, 15)

	// TIER 2: Points for non-usage anchors with high edge
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  TIER 2: POINTS O0.5 - HIGH EDGE NON-ANCHORS (Edge≥10%)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	tier2 := filterPicks(allPicks, func(p FullPick) bool {
		return p.Market == types.PropMarketPointsOver && !p.IsUsageAnch && p.Edge >= 10
	})

	printPicksTable(tier2, 10)

	// Best parlays
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  CORRELATION-AWARE PARLAY OPPORTUNITIES")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	// Find same-player SOG+Points combos
	playerPicks := make(map[string][]FullPick)
	for _, p := range allPicks {
		key := p.Player
		playerPicks[key] = append(playerPicks[key], p)
	}

	parlayCount := 0
	for player, picks := range playerPicks {
		if len(picks) < 2 {
			continue
		}

		var sogPick, ptsPick *FullPick
		for i := range picks {
			if picks[i].Market == types.PropMarketSOGOver && picks[i].Edge >= 3 {
				sogPick = &picks[i]
			}
			if picks[i].Market == types.PropMarketPointsOver && picks[i].Edge >= 3 {
				ptsPick = &picks[i]
			}
		}

		if sogPick != nil && ptsPick != nil {
			corr := corrMatrix.GetCorrelation(types.PropMarketSOGOver, types.PropMarketPointsOver, types.ScenarioRunAndGun)
			combinedEV := (1+sogPick.EV)*(1+ptsPick.EV) - 1

			if parlayCount < 5 {
				fmt.Printf("\n%s (%s) - %s\n", player, sogPick.Team, sogPick.Matchup)
				fmt.Printf("  Leg 1: Points O0.5 (Edge: %+.1f%%, EV: %+.2f)\n", ptsPick.Edge, ptsPick.EV)
				fmt.Printf("  Leg 2: SOG O%.1f   (Edge: %+.1f%%, EV: %+.2f)\n", sogPick.Line, sogPick.Edge, sogPick.EV)
				fmt.Printf("  Correlation: %.2f | Combined EV: %+.2f\n", corr, combinedEV)
				parlayCount++
			}
		}
	}

	if parlayCount == 0 {
		fmt.Println("\nNo high-correlation parlay opportunities found.")
	}

	// Summary
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  SLATE SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	fmt.Printf("\nTotal Positive-Edge Picks: %d\n", len(allPicks))
	fmt.Printf("Tier 1 (Usage Anchor Points): %d\n", len(tier1))
	fmt.Printf("Tier 1.5 (Volume Shooter SOG): %d\n", len(tier15))
	fmt.Printf("Tier 2 (High-Edge Non-Anchor): %d\n", len(tier2))

	// Average edges
	if len(tier1) > 0 {
		avgEdge := 0.0
		for _, p := range tier1 {
			avgEdge += p.Edge
		}
		fmt.Printf("\nTier 1 Average Edge: %.1f%%\n", avgEdge/float64(len(tier1)))
	}

	// Scenario breakdown
	scenarioCounts := make(map[types.ScenarioType]int)
	for _, p := range allPicks {
		scenarioCounts[p.Scenario]++
	}
	fmt.Println("\nBy Scenario:")
	for scenario, count := range scenarioCounts {
		fmt.Printf("  %s: %d picks\n", scenario, count)
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
}

func filterPicks(picks []FullPick, filter func(FullPick) bool) []FullPick {
	var result []FullPick
	for _, p := range picks {
		if filter(p) {
			result = append(result, p)
		}
	}
	return result
}

func printPicksTable(picks []FullPick, limit int) {
	if len(picks) == 0 {
		fmt.Println("\nNo picks in this tier.")
		return
	}

	fmt.Printf("\n%-7s %-12s %-22s %-5s %-8s %-5s %-6s %-7s %-5s %-4s\n",
		"Time", "Matchup", "Player", "Team", "Market", "Line", "Proj", "Edge", "EV", "★")
	fmt.Println(strings.Repeat("-", 95))

	count := 0
	for _, p := range picks {
		if count >= limit {
			if len(picks) > limit {
				fmt.Printf("... and %d more\n", len(picks)-limit)
			}
			break
		}

		stars := strings.Repeat("★", p.Stars)
		marketShort := "PTS"
		if p.Market == types.PropMarketSOGOver {
			marketShort = "SOG"
		}

		fmt.Printf("%-7s %-12s %-22s %-5s %-8s %-5.1f %-6.2f %+6.1f%% %+5.2f %-4s\n",
			p.GameTime,
			p.Matchup,
			truncateName(p.Player, 22),
			p.Team,
			marketShort+" O",
			p.Line,
			p.OurLine,
			p.Edge,
			p.EV,
			stars,
		)
		count++
	}
}

func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func getCurrentSeason(date time.Time) string {
	year := date.Year()
	month := date.Month()
	if month >= time.October {
		return fmt.Sprintf("%d%d", year, year+1)
	}
	return fmt.Sprintf("%d%d", year-1, year)
}

func determineScenario(homeStats, awayStats types.TeamStats) types.ScenarioType {
	combinedGF := homeStats.GFPerGame + awayStats.GFPerGame
	if combinedGF >= 6.5 {
		return types.ScenarioRunAndGun
	}
	if combinedGF <= 5.0 {
		return types.ScenarioGoalieDuel
	}
	return types.ScenarioRunAndGun
}

func analyzePointsPick(player *types.PlayerWithStats, ctx distributions.DistributionContext, scenario *types.GameScenario, evaluator *props.Evaluator, factory *distributions.Factory, gameTime, matchup string) FullPick {
	stats := player.Stats
	dist := factory.CreateDistribution(types.PropMarketPointsOver, &stats, ctx)
	mean := dist.Mean()
	line := 0.5

	probOver := dist.ProbOver(line)
	probUnder := 1.0 - probOver

	var side types.PropSide
	var probHit float64
	if scenario.Primary.Scenario == types.ScenarioGoalieDuel || scenario.Primary.Scenario == types.ScenarioBenchSlog {
		side = types.PropSideUnder
		probHit = probUnder
	} else {
		side = types.PropSideOver
		probHit = probOver
	}

	impliedProb := 0.535
	edge := (probHit - impliedProb) * 100
	ev := probHit*0.87 - (1-probHit)*1.0

	stars := getStars(edge)

	return FullPick{
		GameTime:    gameTime,
		Matchup:     matchup,
		Scenario:    scenario.Primary.Scenario,
		Player:      player.Player.Name,
		Team:        player.Player.TeamAbbr,
		Position:    player.Player.Position,
		TOI:         stats.TOI,
		Market:      types.PropMarketPointsOver,
		Line:        line,
		Side:        side,
		OurLine:     mean,
		ProbHit:     probHit,
		Edge:        edge,
		EV:          ev,
		Stars:       stars,
		Tier:        types.PropTier1,
		IsUsageAnch: evaluator.IsUsageAnchor(&stats),
		IsVolShoot:  evaluator.IsVolumeShooter(&stats),
		IsPP1:       stats.PP1Unit,
		DistType:    "PoissBinom",
	}
}

func analyzeSOGPick(player *types.PlayerWithStats, ctx distributions.DistributionContext, scenario *types.GameScenario, evaluator *props.Evaluator, factory *distributions.Factory, gameTime, matchup string) FullPick {
	stats := player.Stats
	dist := factory.CreateDistribution(types.PropMarketSOGOver, &stats, ctx)
	mean := dist.Mean()
	line := getRealisticSOGLine(stats.SOG)

	probOver := dist.ProbOver(line)
	probUnder := 1.0 - probOver

	var side types.PropSide
	var probHit float64
	if scenario.Primary.Scenario == types.ScenarioGoalieDuel || scenario.Primary.Scenario == types.ScenarioBenchSlog {
		side = types.PropSideUnder
		probHit = probUnder
	} else {
		side = types.PropSideOver
		probHit = probOver
	}

	impliedProb := 0.524
	edge := (probHit - impliedProb) * 100
	ev := probHit*0.91 - (1-probHit)*1.0

	stars := getStars(edge)

	return FullPick{
		GameTime:    gameTime,
		Matchup:     matchup,
		Scenario:    scenario.Primary.Scenario,
		Player:      player.Player.Name,
		Team:        player.Player.TeamAbbr,
		Position:    player.Player.Position,
		TOI:         stats.TOI,
		Market:      types.PropMarketSOGOver,
		Line:        line,
		Side:        side,
		OurLine:     mean,
		ProbHit:     probHit,
		Edge:        edge,
		EV:          ev,
		Stars:       stars,
		Tier:        types.PropTier1_5,
		IsUsageAnch: evaluator.IsUsageAnchor(&stats),
		IsVolShoot:  evaluator.IsVolumeShooter(&stats),
		IsPP1:       stats.PP1Unit,
		DistType:    "HierPoiss",
	}
}

func getRealisticSOGLine(avgSOG float64) float64 {
	if avgSOG < 2.0 {
		return 1.5
	} else if avgSOG < 3.0 {
		return 2.5
	} else if avgSOG < 4.0 {
		return 3.5
	}
	return 4.5
}

func getStars(edge float64) int {
	if edge >= 15 {
		return 5
	} else if edge >= 10 {
		return 4
	} else if edge >= 7 {
		return 3
	} else if edge >= 4 {
		return 2
	} else if edge > 0 {
		return 1
	}
	return 0
}
