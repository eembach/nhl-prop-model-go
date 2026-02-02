// +build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

type Pick struct {
	Player     string
	Team       string
	Opponent   string
	Game       string
	Market     string
	Line       float64
	Side       string
	Projection float64
	SeasonAvg  float64
	ProbHit    float64
	Edge       float64
	Confidence string
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("              TODAY'S HIGH-CONVICTION PICKS - BACKTEST-VALIDATED MODEL")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Date: %s\n", time.Now().Format("Monday, January 2, 2006"))
	fmt.Println()
	fmt.Println("Model Performance (Past Week Backtest):")
	fmt.Println("  • SOG U2.5:     84.9% hit rate")
	fmt.Println("  • Points U0.5:  80.6% hit rate")
	fmt.Println("  • Points O0.5:  67.5% hit rate")
	fmt.Println("  • SOG O2.5:     60.5% hit rate")
	fmt.Println()

	client := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	games, err := client.GetSchedule(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Games Today: %d\n\n", len(games))

	teamStats, _ := client.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var allPicks []Pick

	for i, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats, ok1 := teamStatsMap[homeTeam]
		awayStats, ok2 := teamStatsMap[awayTeam]
		if !ok1 || !ok2 {
			continue
		}

		fmt.Printf("\r[%d/%d] Analyzing %s @ %s...", i+1, len(games), awayTeam, homeTeam)

		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame
		gameStr := fmt.Sprintf("%s@%s", awayTeam, homeTeam)

		// Process home team
		homeRoster, _ := client.GetRoster(homeTeam)
		for _, p := range homeRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 10 {
				continue
			}

			ctx := distributions.DistributionContext{
				TeamTotal:     expectedTotal / 2,
				OppGA60:       awayStats.GAPerGame,
				HomeGame:      true,
				ArenaSOGBias:  1.0,
				FatigueFactor: 0.0,
				ExpectedTOI:   pStats.TOI,
				PP1Unit:       pStats.PPTOI >= 1.5,
			}

			picks := generatePicks(p, pStats, factory, ctx, homeTeam, awayTeam, gameStr)
			allPicks = append(allPicks, picks...)
		}

		// Process away team
		awayRoster, _ := client.GetRoster(awayTeam)
		for _, p := range awayRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 10 {
				continue
			}

			ctx := distributions.DistributionContext{
				TeamTotal:     expectedTotal / 2,
				OppGA60:       homeStats.GAPerGame,
				HomeGame:      false,
				ArenaSOGBias:  1.0,
				FatigueFactor: 0.0,
				ExpectedTOI:   pStats.TOI,
				PP1Unit:       pStats.PPTOI >= 1.5,
			}

			picks := generatePicks(p, pStats, factory, ctx, awayTeam, homeTeam, gameStr)
			allPicks = append(allPicks, picks...)
		}
	}

	fmt.Printf("\r%s\n\n", strings.Repeat(" ", 60))

	// Filter by minimum edge
	var qualified []Pick
	for _, p := range allPicks {
		if p.Edge >= 10.0 {
			qualified = append(qualified, p)
		}
	}

	// Sort by edge
	sort.Slice(qualified, func(i, j int) bool {
		return qualified[i].Edge > qualified[j].Edge
	})

	// Group by market
	markets := map[string][]Pick{
		"SOG U2.5":     {},
		"Points U0.5":  {},
		"Points O0.5":  {},
		"SOG O2.5":     {},
	}

	for _, p := range qualified {
		if picks, ok := markets[p.Market]; ok {
			markets[p.Market] = append(picks, p)
		}
	}

	// Display picks by market (in order of backtest performance)
	marketOrder := []string{"SOG U2.5", "Points U0.5", "Points O0.5", "SOG O2.5"}
	marketHitRates := map[string]string{
		"SOG U2.5":    "84.9%",
		"Points U0.5": "80.6%",
		"Points O0.5": "67.5%",
		"SOG O2.5":    "60.5%",
	}

	for _, market := range marketOrder {
		picks := markets[market]
		if len(picks) == 0 {
			continue
		}

		fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
		fmt.Printf("  %s (Backtest: %s hit rate)\n", market, marketHitRates[market])
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
		fmt.Println()

		fmt.Printf("%-22s %-5s %-9s %6s %6s %6s %7s %-12s\n",
			"Player", "Team", "Game", "Avg", "Proj", "Line", "Edge", "Confidence")
		fmt.Println(strings.Repeat("─", 90))

		limit := 15
		if len(picks) < limit {
			limit = len(picks)
		}

		for i := 0; i < limit; i++ {
			p := picks[i]
			fmt.Printf("%-22s %-5s %-9s %6.2f %6.2f %6.1f %+6.1f%% %-12s\n",
				truncate(p.Player, 22),
				p.Team,
				p.Game,
				p.SeasonAvg,
				p.Projection,
				p.Line,
				p.Edge,
				p.Confidence,
			)
		}

		if len(picks) > limit {
			fmt.Printf("\n  ... and %d more picks available\n", len(picks)-limit)
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                                    PICK SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	totalPicks := 0
	for _, market := range marketOrder {
		count := len(markets[market])
		if count > 0 {
			fmt.Printf("  %-15s %4d picks (Edge >= 10%%)\n", market+":", count)
			totalPicks += count
		}
	}
	fmt.Printf("\n  %-15s %4d picks\n", "TOTAL:", totalPicks)

	// Top 10 overall
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              TOP 10 HIGHEST-CONVICTION PLAYS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("%-3s %-22s %-5s %-15s %6s %6s %7s\n",
		"#", "Player", "Team", "Market", "Avg", "Proj", "Edge")
	fmt.Println(strings.Repeat("─", 75))

	for i := 0; i < 10 && i < len(qualified); i++ {
		p := qualified[i]
		fmt.Printf("%-3d %-22s %-5s %-15s %6.2f %6.2f %+6.1f%%\n",
			i+1,
			truncate(p.Player, 22),
			p.Team,
			p.Market,
			p.SeasonAvg,
			p.Projection,
			p.Edge,
		)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  Note: All picks based on model with 82.1% overall hit rate in blind backtest")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

func generatePicks(p types.Player, stats *types.PlayerStats, factory *distributions.Factory, ctx distributions.DistributionContext, team, opponent, game string) []Pick {
	var picks []Pick

	gp := float64(max(stats.GamesPlayed, 1))

	// SOG U2.5 (Best performer: 84.9%)
	if stats.SOG <= 2.0 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(2.5)
		edge := (probUnder - 0.524) * 100

		if edge >= 5 {
			confidence := "Medium"
			if edge >= 30 {
				confidence = "Very High"
			} else if edge >= 20 {
				confidence = "High"
			}

			picks = append(picks, Pick{
				Player:     p.Name,
				Team:       team,
				Opponent:   opponent,
				Game:       game,
				Market:     "SOG U2.5",
				Line:       2.5,
				Side:       "under",
				Projection: proj,
				SeasonAvg:  stats.SOG,
				ProbHit:    probUnder,
				Edge:       edge,
				Confidence: confidence,
			})
		}
	}

	// Points U0.5 (Second best: 80.6%)
	if stats.PointsPerGame <= 0.35 {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(0.5)
		edge := (probUnder - 0.535) * 100

		if edge >= 5 {
			confidence := "Medium"
			if edge >= 30 {
				confidence = "Very High"
			} else if edge >= 20 {
				confidence = "High"
			}

			picks = append(picks, Pick{
				Player:     p.Name,
				Team:       team,
				Opponent:   opponent,
				Game:       game,
				Market:     "Points U0.5",
				Line:       0.5,
				Side:       "under",
				Projection: proj,
				SeasonAvg:  stats.PointsPerGame,
				ProbHit:    probUnder,
				Edge:       edge,
				Confidence: confidence,
			})
		}
	}

	// Points O0.5 (67.5%)
	if stats.PointsPerGame >= 0.6 {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(0.5)
		edge := (probOver - 0.535) * 100

		if edge >= 5 {
			confidence := "Medium"
			if edge >= 15 {
				confidence = "High"
			} else if edge >= 10 {
				confidence = "Medium-High"
			}

			picks = append(picks, Pick{
				Player:     p.Name,
				Team:       team,
				Opponent:   opponent,
				Game:       game,
				Market:     "Points O0.5",
				Line:       0.5,
				Side:       "over",
				Projection: proj,
				SeasonAvg:  stats.PointsPerGame,
				ProbHit:    probOver,
				Edge:       edge,
				Confidence: confidence,
			})
		}
	}

	// SOG O2.5 (60.5%)
	if stats.SOG >= 2.5 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(2.5)
		edge := (probOver - 0.524) * 100

		if edge >= 5 {
			confidence := "Medium"
			if edge >= 20 {
				confidence = "High"
			} else if edge >= 15 {
				confidence = "Medium-High"
			}

			picks = append(picks, Pick{
				Player:     p.Name,
				Team:       team,
				Opponent:   opponent,
				Game:       game,
				Market:     "SOG O2.5",
				Line:       2.5,
				Side:       "over",
				Projection: proj,
				SeasonAvg:  stats.SOG,
				ProbHit:    probOver,
				Edge:       edge,
				Confidence: confidence,
			})
		}
	}

	_ = gp // Silence unused variable warning

	return picks
}

func getCurrentSeason(date time.Time) string {
	year := date.Year()
	month := date.Month()
	if month >= time.October {
		return fmt.Sprintf("%d%d", year, year+1)
	}
	return fmt.Sprintf("%d%d", year-1, year)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
