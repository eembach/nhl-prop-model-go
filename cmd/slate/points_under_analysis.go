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

type PointsUnderPick struct {
	Player      string
	Team        string
	Position    string
	TOI         float64
	PPTOI       float64
	PointsAvg   float64
	GoalsAvg    float64
	AssistsAvg  float64
	Proj        float64
	ProbUnder   float64
	Edge        float64
	Tier        string // "Elite Under", "Strong Under", "Value Under"
	Factors     []string
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("           POINTS UNDER 0.5 - DEEP ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Date: %s\n\n", time.Now().Format("Monday, January 2, 2006"))

	client := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	games, err := client.GetSchedule(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Games: %d\n\n", len(games))

	teamStats, _ := client.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var allPicks []PointsUnderPick

	for i, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats, ok1 := teamStatsMap[homeTeam]
		awayStats, ok2 := teamStatsMap[awayTeam]
		if !ok1 || !ok2 {
			continue
		}

		fmt.Printf("\r[%d/%d] Processing %s@%s...", i+1, len(games), awayTeam, homeTeam)

		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame

		// Determine game environment
		isLowScoring := expectedTotal < 5.5

		ctx := distributions.DistributionContext{
			TeamTotal:     expectedTotal / 2,
			OppGA60:       3.0, // Neutral
			HomeGame:      true,
			ArenaSOGBias:  1.0,
			FatigueFactor: 0.0,
		}

		homeRoster, _ := client.GetRoster(homeTeam)
		awayRoster, _ := client.GetRoster(awayTeam)
		allRoster := append(homeRoster, awayRoster...)

		for _, p := range allRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil {
				continue
			}

			// Skip high-TOI players for unders
			if pStats.TOI > 18 {
				continue
			}

			ctx.ExpectedTOI = pStats.TOI
			ctx.PP1Unit = pStats.PPTOI >= 1.5

			// Calculate points distribution
			dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
			proj := dist.Mean()
			probUnder := 1 - dist.ProbOver(0.5)

			// Edge vs implied 53.5% (-115 juice)
			edge := (probUnder - 0.535) * 100

			if edge <= 0 {
				continue
			}

			// Calculate per-game averages
			gp := float64(max(pStats.GamesPlayed, 1))
			pointsAvg := pStats.PointsPerGame
			goalsAvg := float64(pStats.Goals) / gp
			assistsAvg := float64(pStats.Assists) / gp

			// Determine tier and factors
			var tier string
			var factors []string

			// Elite Under: Very low production + low TOI + no PP
			if pointsAvg < 0.15 && pStats.TOI < 12 && pStats.PPTOI < 0.5 {
				tier = "Elite Under"
				factors = append(factors, "Ultra-low production")
			} else if pointsAvg < 0.25 && pStats.TOI < 14 && pStats.PPTOI < 1.0 {
				tier = "Strong Under"
			} else if pointsAvg < 0.35 && pStats.TOI < 16 {
				tier = "Value Under"
			} else {
				tier = "Speculative"
			}

			// Add factors
			if pStats.TOI < 10 {
				factors = append(factors, "Very low TOI (<10)")
			} else if pStats.TOI < 12 {
				factors = append(factors, "Low TOI (<12)")
			}

			if pStats.PPTOI < 0.5 {
				factors = append(factors, "No PP time")
			} else if pStats.PPTOI < 1.0 {
				factors = append(factors, "Minimal PP")
			}

			if pointsAvg < 0.10 {
				factors = append(factors, "Near-zero production")
			} else if pointsAvg < 0.20 {
				factors = append(factors, "Very low production")
			}

			if isLowScoring {
				factors = append(factors, "Low-scoring game")
			}

			if goalsAvg < 0.05 {
				factors = append(factors, "Non-scorer")
			}

			pos := "F"
			if p.Position == types.PositionDefense {
				pos = "D"
			}

			allPicks = append(allPicks, PointsUnderPick{
				Player:     p.Name,
				Team:       p.TeamAbbr,
				Position:   pos,
				TOI:        pStats.TOI,
				PPTOI:      pStats.PPTOI,
				PointsAvg:  pointsAvg,
				GoalsAvg:   goalsAvg,
				AssistsAvg: assistsAvg,
				Proj:       proj,
				ProbUnder:  probUnder,
				Edge:       edge,
				Tier:       tier,
				Factors:    factors,
			})
		}
	}

	fmt.Printf("\r%s\n\n", strings.Repeat(" ", 50))

	// Sort by probability of hitting under (highest first)
	sort.Slice(allPicks, func(i, j int) bool {
		return allPicks[i].ProbUnder > allPicks[j].ProbUnder
	})

	// Analysis by tier
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    POINTS UNDER TIER ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	tiers := []string{"Elite Under", "Strong Under", "Value Under", "Speculative"}
	for _, tier := range tiers {
		var tierPicks []PointsUnderPick
		for _, p := range allPicks {
			if p.Tier == tier {
				tierPicks = append(tierPicks, p)
			}
		}

		if len(tierPicks) == 0 {
			continue
		}

		avgProb := 0.0
		avgEdge := 0.0
		avgTOI := 0.0
		avgPPTOI := 0.0
		avgPtsAvg := 0.0
		for _, p := range tierPicks {
			avgProb += p.ProbUnder
			avgEdge += p.Edge
			avgTOI += p.TOI
			avgPPTOI += p.PPTOI
			avgPtsAvg += p.PointsAvg
		}
		n := float64(len(tierPicks))
		avgProb /= n
		avgEdge /= n
		avgTOI /= n
		avgPPTOI /= n
		avgPtsAvg /= n

		fmt.Printf("\n%s (%d picks)\n", tier, len(tierPicks))
		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("  Avg Hit Prob: %.1f%% | Avg Edge: +%.1f%%\n", avgProb*100, avgEdge)
		fmt.Printf("  Avg TOI: %.1f | Avg PPTOI: %.2f | Avg Pts/G: %.3f\n", avgTOI, avgPPTOI, avgPtsAvg)
	}

	// Top picks by hit probability
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    TOP POINTS UNDER PICKS (By Hit Probability)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("%-22s %-5s %-3s %5s %5s %6s %6s %6s %-15s\n",
		"Player", "Team", "Pos", "TOI", "PPTOI", "Pts/G", "P(U)", "Edge", "Tier")
	fmt.Println(strings.Repeat("-", 100))

	// Show top 30
	limit := 30
	if len(allPicks) < limit {
		limit = len(allPicks)
	}

	for i := 0; i < limit; i++ {
		p := allPicks[i]
		fmt.Printf("%-22s %-5s %-3s %5.1f %5.2f %6.3f %5.1f%% %+5.1f%% %-15s\n",
			truncate(p.Player, 22),
			p.Team,
			p.Position,
			p.TOI,
			p.PPTOI,
			p.PointsAvg,
			p.ProbUnder*100,
			p.Edge,
			p.Tier,
		)
	}

	// Profile of ideal Points Under candidate
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    IDEAL POINTS UNDER PROFILE")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Based on analysis, highest hit rate Points U0.5 candidates have:")
	fmt.Println()
	fmt.Println("  ✓ TOI < 12 minutes (4th line / 3rd pair)")
	fmt.Println("  ✓ PPTOI < 0.5 minutes (no power play role)")
	fmt.Println("  ✓ Points/G < 0.15 (historical non-producers)")
	fmt.Println("  ✓ Goals/G < 0.05 (non-scorers)")
	fmt.Println("  ✓ Low-scoring game environment (Exp Total < 5.5)")
	fmt.Println()

	// Filter for "lock" candidates
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    'LOCK' CANDIDATES (>85% Hit Probability)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	lockCount := 0
	for _, p := range allPicks {
		if p.ProbUnder >= 0.85 {
			if lockCount == 0 {
				fmt.Printf("%-22s %-5s %5s %5s %6s %6s %s\n",
					"Player", "Team", "TOI", "PPTOI", "Pts/G", "P(U)", "Factors")
				fmt.Println(strings.Repeat("-", 90))
			}
			fmt.Printf("%-22s %-5s %5.1f %5.2f %6.3f %5.1f%% %s\n",
				truncate(p.Player, 22),
				p.Team,
				p.TOI,
				p.PPTOI,
				p.PointsAvg,
				p.ProbUnder*100,
				strings.Join(p.Factors, ", "),
			)
			lockCount++
			if lockCount >= 20 {
				break
			}
		}
	}

	if lockCount == 0 {
		fmt.Println("No 'lock' candidates found (>85% hit probability)")
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
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
