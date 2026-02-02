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

type MarketPick struct {
	Player    string
	Team      string
	Market    string
	Line      float64
	Side      string
	Proj      float64
	ProbHit   float64
	Edge      float64
	PlayerAvg float64
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("           MULTI-MARKET EDGE ANALYSIS - ALL PROP TYPES")
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

	// Collect picks by market
	marketPicks := make(map[string][]MarketPick)

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

		ctx := distributions.DistributionContext{
			TeamTotal:     expectedTotal / 2,
			OppGA60:       awayStats.GAPerGame, // Goals against per game, not multiplied
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
			if err != nil || pStats == nil || pStats.TOI < 10 {
				continue
			}

			ctx.ExpectedTOI = pStats.TOI
			ctx.PP1Unit = pStats.PPTOI >= 1.5

			// ===== POINTS O0.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
				proj := dist.Mean()
				line := 0.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.535) * 100

				marketPicks["Points O0.5"] = append(marketPicks["Points O0.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "Points O0.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: pStats.PointsPerGame,
				})
			}

			// ===== POINTS U0.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
				proj := dist.Mean()
				line := 0.5
				probUnder := 1 - dist.ProbOver(line)
				edge := (probUnder - 0.535) * 100

				marketPicks["Points U0.5"] = append(marketPicks["Points U0.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "Points U0.5",
					Line: line, Side: "under", Proj: proj, ProbHit: probUnder,
					Edge: edge, PlayerAvg: pStats.PointsPerGame,
				})
			}

			// ===== SOG O1.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()
				line := 1.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.524) * 100

				marketPicks["SOG O1.5"] = append(marketPicks["SOG O1.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "SOG O1.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: pStats.SOG,
				})
			}

			// ===== SOG U1.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()
				line := 1.5
				probUnder := 1 - dist.ProbOver(line)
				edge := (probUnder - 0.524) * 100

				marketPicks["SOG U1.5"] = append(marketPicks["SOG U1.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "SOG U1.5",
					Line: line, Side: "under", Proj: proj, ProbHit: probUnder,
					Edge: edge, PlayerAvg: pStats.SOG,
				})
			}

			// ===== SOG O2.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()
				line := 2.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.524) * 100

				marketPicks["SOG O2.5"] = append(marketPicks["SOG O2.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "SOG O2.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: pStats.SOG,
				})
			}

			// ===== SOG U2.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()
				line := 2.5
				probUnder := 1 - dist.ProbOver(line)
				edge := (probUnder - 0.524) * 100

				marketPicks["SOG U2.5"] = append(marketPicks["SOG U2.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "SOG U2.5",
					Line: line, Side: "under", Proj: proj, ProbHit: probUnder,
					Edge: edge, PlayerAvg: pStats.SOG,
				})
			}

			// ===== SOG O3.5 =====
			if pStats.SOG >= 2.5 {
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()
				line := 3.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.524) * 100

				marketPicks["SOG O3.5"] = append(marketPicks["SOG O3.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "SOG O3.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: pStats.SOG,
				})
			}

			// ===== GOALS O0.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketGoalsOver, pStats, ctx)
				proj := dist.Mean()
				line := 0.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.55) * 100 // Goals typically juiced more

				marketPicks["Goals O0.5"] = append(marketPicks["Goals O0.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "Goals O0.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: float64(pStats.Goals) / float64(max(pStats.GamesPlayed, 1)),
				})
			}

			// ===== ASSISTS O0.5 =====
			{
				dist := factory.CreateDistribution(types.PropMarketAssistsOver, pStats, ctx)
				proj := dist.Mean()
				line := 0.5
				probOver := dist.ProbOver(line)
				edge := (probOver - 0.54) * 100

				marketPicks["Assists O0.5"] = append(marketPicks["Assists O0.5"], MarketPick{
					Player: p.Name, Team: p.TeamAbbr, Market: "Assists O0.5",
					Line: line, Side: "over", Proj: proj, ProbHit: probOver,
					Edge: edge, PlayerAvg: float64(pStats.Assists) / float64(max(pStats.GamesPlayed, 1)),
				})
			}
		}
	}

	fmt.Printf("\r%s\n\n", strings.Repeat(" ", 50))

	// Analyze each market
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    MARKET-BY-MARKET EDGE SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("%-15s %6s %8s %8s %8s %8s\n", "Market", "Total", "Edge>0", "Edge>5", "Edge>10", "AvgEdge")
	fmt.Println(strings.Repeat("-", 65))

	marketOrder := []string{
		"Points O0.5", "Points U0.5",
		"SOG O1.5", "SOG U1.5", "SOG O2.5", "SOG U2.5", "SOG O3.5",
		"Goals O0.5", "Assists O0.5",
	}

	for _, market := range marketOrder {
		picks := marketPicks[market]
		if len(picks) == 0 {
			continue
		}

		positive := 0
		edge5 := 0
		edge10 := 0
		totalEdge := 0.0

		for _, p := range picks {
			if p.Edge > 0 {
				positive++
				totalEdge += p.Edge
			}
			if p.Edge > 5 {
				edge5++
			}
			if p.Edge > 10 {
				edge10++
			}
		}

		avgEdge := 0.0
		if positive > 0 {
			avgEdge = totalEdge / float64(positive)
		}

		fmt.Printf("%-15s %6d %8d %8d %8d %+7.1f%%\n",
			market, len(picks), positive, edge5, edge10, avgEdge)
	}

	// Show top picks for each market with positive edge
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    TOP PICKS BY MARKET (Edge > 5%)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	for _, market := range marketOrder {
		picks := marketPicks[market]

		// Filter and sort
		var goodPicks []MarketPick
		for _, p := range picks {
			if p.Edge > 5 {
				goodPicks = append(goodPicks, p)
			}
		}

		if len(goodPicks) == 0 {
			continue
		}

		sort.Slice(goodPicks, func(i, j int) bool {
			return goodPicks[i].Edge > goodPicks[j].Edge
		})

		fmt.Printf("\n%s:\n", market)
		fmt.Println(strings.Repeat("-", 80))

		limit := 8
		if len(goodPicks) < limit {
			limit = len(goodPicks)
		}

		for i := 0; i < limit; i++ {
			p := goodPicks[i]
			fmt.Printf("  %-22s %-5s  Proj: %.2f  Avg: %.2f  Edge: %+.1f%%\n",
				p.Player, p.Team, p.Proj, p.PlayerAvg, p.Edge)
		}

		if len(goodPicks) > limit {
			fmt.Printf("  ... and %d more\n", len(goodPicks)-limit)
		}
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
}

func getCurrentSeason(date time.Time) string {
	year := date.Year()
	month := date.Month()
	if month >= time.October {
		return fmt.Sprintf("%d%d", year, year+1)
	}
	return fmt.Sprintf("%d%d", year-1, year)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
