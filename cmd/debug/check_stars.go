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

	// Check stars from different teams
	teams := []string{"COL", "TOR", "EDM"}

	ctx := distributions.DistributionContext{
		TeamTotal:     3.5,
		OppGA60:       3.0,
		HomeGame:      true,
		ArenaSOGBias:  1.0,
	}

	for _, team := range teams {
		fmt.Printf("\n=== %s ===\n", team)
		roster, _ := client.GetRoster(team)

		for _, p := range roster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil {
				continue
			}

			// Only show high-TOI players (stars)
			if pStats.TOI < 18 {
				continue
			}

			ctx.ExpectedTOI = pStats.TOI
			ctx.PP1Unit = pStats.PPTOI >= 1.5

			dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
			proj := dist.Mean()

			fmt.Printf("%s:\n", p.Name)
			fmt.Printf("  TOI=%.1f, GP=%d, Goals=%d, Assists=%d, Points=%d\n",
				pStats.TOI, pStats.GamesPlayed, pStats.Goals, pStats.Assists, pStats.Points)
			fmt.Printf("  PPG=%.3f, SOG=%.1f, ShPct=%.1f%%\n",
				pStats.PointsPerGame, pStats.SOG, pStats.ShootingPct)
			fmt.Printf("  Model Proj=%.3f\n\n", proj)
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
