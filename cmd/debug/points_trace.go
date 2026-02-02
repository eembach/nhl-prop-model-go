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

	// Get one roster to test
	roster, _ := client.GetRoster("CHI")

	ctx := distributions.DistributionContext{
		TeamTotal:     3.0,
		OppGA60:       3.0,
		HomeGame:      true,
		ArenaSOGBias:  1.0,
		FatigueFactor: 0.0,
	}

	count := 0
	for _, p := range roster {
		if p.Position == types.PositionGoalie {
			continue
		}

		pStats, err := client.GetPlayerStats(p.ID, season)
		if err != nil || pStats == nil {
			continue
		}

		// Only show low-TOI players
		if pStats.TOI > 14 {
			continue
		}

		ctx.ExpectedTOI = pStats.TOI
		ctx.PP1Unit = pStats.PPTOI >= 1.5

		dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(0.5)
		probUnder := 1 - probOver

		fmt.Printf("%s:\n", p.Name)
		fmt.Printf("  Stats: TOI=%.1f, PPG=%.3f, RecentPPG=%.3f, ShPct=%.1f%%, XGF60=%.2f\n",
			pStats.TOI, pStats.PointsPerGame, pStats.RecentPPG, pStats.ShootingPct, pStats.XGF60)
		fmt.Printf("  Model: Proj=%.4f, P(Over)=%.2f%%, P(Under)=%.2f%%\n\n",
			proj, probOver*100, probUnder*100)

		count++
		if count >= 5 {
			break
		}
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
