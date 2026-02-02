// +build ignore

package main

import (
	"fmt"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

func main() {
	client := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	// Fetch MacKinnon's actual stats
	roster, err := client.GetRoster("COL")
	if err != nil {
		fmt.Printf("Error getting roster: %v\n", err)
		return
	}

	var mackinnonID int
	for _, p := range roster {
		if p.Name == "Nathan MacKinnon" {
			mackinnonID = p.ID
			break
		}
	}

	if mackinnonID == 0 {
		fmt.Println("MacKinnon not found")
		return
	}

	stats, err := client.GetPlayerStats(mackinnonID, season)
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		return
	}

	fmt.Println("=== Nathan MacKinnon Actual Stats ===")
	fmt.Printf("GamesPlayed: %d\n", stats.GamesPlayed)
	fmt.Printf("Goals: %d\n", stats.Goals)
	fmt.Printf("Assists: %d\n", stats.Assists)
	fmt.Printf("Points: %d\n", stats.Points)
	fmt.Printf("PointsPerGame: %.3f\n", stats.PointsPerGame)
	fmt.Printf("SOG: %.2f (per game)\n", stats.SOG)
	fmt.Printf("RecentSOG: %.2f\n", stats.RecentSOG)
	fmt.Printf("TOI: %.1f\n", stats.TOI)
	fmt.Printf("PPTOI: %.1f\n", stats.PPTOI)
	fmt.Printf("ShootingPct: %.1f%%\n", stats.ShootingPct)
	fmt.Printf("XGF60: %.2f\n", stats.XGF60)
	fmt.Printf("RecentPPG: %.3f\n", stats.RecentPPG)

	ctx := distributions.DistributionContext{
		TeamTotal:     3.5,
		OppGA60:       3.0,
		HomeGame:      true,
		ArenaSOGBias:  1.0,
		FatigueFactor: 0.0,
		ExpectedTOI:   stats.TOI,
		ExpectedPPTOI: stats.PPTOI,
		PP1Unit:       stats.PPTOI >= 1.5,
	}

	fmt.Println("\n=== Distribution Context ===")
	fmt.Printf("ExpectedTOI: %.1f\n", ctx.ExpectedTOI)
	fmt.Printf("PP1Unit: %v\n", ctx.PP1Unit)
	fmt.Printf("TeamTotal: %.1f\n", ctx.TeamTotal)

	// Test Points distribution
	fmt.Println("\n=== Points Distribution ===")
	pointsDist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
	fmt.Printf("Mean: %.4f\n", pointsDist.Mean())
	fmt.Printf("P(Over 0.5): %.1f%%\n", pointsDist.ProbOver(0.5)*100)

	// Test SOG distribution
	fmt.Println("\n=== SOG Distribution ===")
	sogDist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
	fmt.Printf("Mean: %.4f\n", sogDist.Mean())
	fmt.Printf("P(Over 1.5): %.1f%%\n", sogDist.ProbOver(1.5)*100)

	// Manual calculation to trace through
	fmt.Println("\n=== Manual Points Trace ===")
	oppRate := stats.PointsPerGame * 2.0
	fmt.Printf("oppRate from PPG: %.3f * 2.0 = %.3f\n", stats.PointsPerGame, oppRate)

	if stats.XGF60 > 0 {
		oppRateXGF := stats.XGF60 / 60.0 * stats.TOI * 2.0
		fmt.Printf("oppRate from XGF60: %.2f / 60 * %.1f * 2.0 = %.3f\n", stats.XGF60, stats.TOI, oppRateXGF)
		oppRate = oppRateXGF
	}

	recentOppRate := stats.RecentPPG * 2.0
	fmt.Printf("recentOppRate: %.3f * 2.0 = %.3f\n", stats.RecentPPG, recentOppRate)

	if recentOppRate == 0 {
		recentOppRate = oppRate
		fmt.Printf("recentOppRate (fallback): %.3f\n", recentOppRate)
	}

	convProb := 0.5
	if stats.ShootingPct > 0 {
		convProb = 0.3 + (stats.ShootingPct/100.0)*0.4
		fmt.Printf("convProb: 0.3 + (%.1f/100) * 0.4 = %.3f\n", stats.ShootingPct, convProb)
	}

	blendedOpp := 0.4*oppRate + 0.6*recentOppRate
	fmt.Printf("blendedOpp: 0.4*%.3f + 0.6*%.3f = %.3f\n", oppRate, recentOppRate, blendedOpp)

	baseMean := blendedOpp * convProb
	fmt.Printf("baseMean (before covariates): %.3f * %.3f = %.3f\n", blendedOpp, convProb, baseMean)

	// Check covariates
	fmt.Println("\n=== Covariates ===")
	xgf60Ratio := stats.XGF60 / 2.5
	fmt.Printf("XGF60Ratio: %.2f / 2.5 = %.3f\n", stats.XGF60, xgf60Ratio)

	finishingSkill := 1.0 + (stats.ShootingPct-10.0)/100.0
	fmt.Printf("FinishingSkill: 1.0 + (%.1f-10.0)/100.0 = %.3f\n", stats.ShootingPct, finishingSkill)

	pp1Share := 0.0
	if ctx.PP1Unit {
		pp1Share = ctx.ExpectedPPTOI / stats.TOI
		fmt.Printf("PP1Share: %.1f / %.1f = %.3f\n", ctx.ExpectedPPTOI, stats.TOI, pp1Share)
	}
}

func getCurrentSeason(date time.Time) string {
	year := date.Year()
	month := date.Month()
	if month >= time.October {
		return fmt.Sprintf("%d%d", year, year+1)
	}
	return fmt.Sprintf("%d%d", year-1, year)
}
