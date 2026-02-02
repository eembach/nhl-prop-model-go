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

type PlayerProjection struct {
	Name     string
	Team     string
	Position string
	Game     string

	// Actual averages
	AvgPts  float64
	AvgSOG  float64
	AvgGoals float64
	AvgAsts float64
	TOI     float64

	// Model projections
	ProjPts  float64
	ProjSOG  float64
	ProjGoals float64
	ProjAsts float64

	// Context
	IsHome   bool
	OppTeam  string
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              NHL PLAYER PROJECTIONS - FULL NUMERICAL OUTPUT")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
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

	var projections []PlayerProjection

	for i, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats, ok1 := teamStatsMap[homeTeam]
		awayStats, ok2 := teamStatsMap[awayTeam]
		if !ok1 || !ok2 {
			continue
		}

		fmt.Printf("\r[%d/%d] Processing %s @ %s...", i+1, len(games), awayTeam, homeTeam)

		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame

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

			proj := buildProjection(p, pStats, factory, ctx, homeTeam, awayTeam, true, game)
			projections = append(projections, proj)
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

			proj := buildProjection(p, pStats, factory, ctx, awayTeam, homeTeam, false, game)
			projections = append(projections, proj)
		}
	}

	fmt.Printf("\r%s\n\n", strings.Repeat(" ", 60))

	// Sort by projected points descending
	sort.Slice(projections, func(i, j int) bool {
		return projections[i].ProjPts > projections[j].ProjPts
	})

	// Print all projections grouped by game
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              TOP 50 PLAYERS BY PROJECTED POINTS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("%-22s %-5s %-3s %-11s  %5s │ %6s %6s │ %6s %6s │ %6s %6s │ %6s %6s\n",
		"Player", "Team", "Pos", "Game", "TOI",
		"AvgPts", "PROJ", "AvgSOG", "PROJ", "AvgG", "PROJ", "AvgA", "PROJ")
	fmt.Println(strings.Repeat("─", 115))

	limit := 50
	if len(projections) < limit {
		limit = len(projections)
	}

	for i := 0; i < limit; i++ {
		p := projections[i]
		loc := "vs"
		if !p.IsHome {
			loc = "@"
		}
		gameStr := fmt.Sprintf("%s %s", loc, p.OppTeam)

		fmt.Printf("%-22s %-5s %-3s %-11s  %5.1f │ %6.2f %6.2f │ %6.2f %6.2f │ %6.2f %6.2f │ %6.2f %6.2f\n",
			truncate(p.Name, 22),
			p.Team,
			p.Position,
			gameStr,
			p.TOI,
			p.AvgPts, p.ProjPts,
			p.AvgSOG, p.ProjSOG,
			p.AvgGoals, p.ProjGoals,
			p.AvgAsts, p.ProjAsts,
		)
	}

	// SOG focused view - sorted by projected SOG
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              TOP 30 PLAYERS BY PROJECTED SOG")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	sort.Slice(projections, func(i, j int) bool {
		return projections[i].ProjSOG > projections[j].ProjSOG
	})

	fmt.Printf("%-22s %-5s %-11s  %5s │ %6s %6s │ Line Analysis\n",
		"Player", "Team", "Game", "TOI", "Avg", "PROJ")
	fmt.Println(strings.Repeat("─", 90))

	for i := 0; i < 30 && i < len(projections); i++ {
		p := projections[i]
		loc := "vs"
		if !p.IsHome {
			loc = "@"
		}
		gameStr := fmt.Sprintf("%s %s", loc, p.OppTeam)

		// Line analysis
		lineAnalysis := ""
		if p.ProjSOG >= 3.5 {
			lineAnalysis = "O3.5 playable"
		} else if p.ProjSOG >= 2.5 {
			lineAnalysis = "O2.5 playable"
		} else if p.ProjSOG >= 1.5 {
			lineAnalysis = "O1.5 playable"
		} else {
			lineAnalysis = "U1.5 lean"
		}

		fmt.Printf("%-22s %-5s %-11s  %5.1f │ %6.2f %6.2f │ %s\n",
			truncate(p.Name, 22),
			p.Team,
			gameStr,
			p.TOI,
			p.AvgSOG, p.ProjSOG,
			lineAnalysis,
		)
	}

	// Low projection players (Under candidates)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              UNDER CANDIDATES (Low Projections)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Sort by projected points ascending for under candidates
	sort.Slice(projections, func(i, j int) bool {
		return projections[i].ProjPts < projections[j].ProjPts
	})

	fmt.Printf("%-22s %-5s %-11s  %5s │ %6s %6s │ %6s %6s │ Under Play\n",
		"Player", "Team", "Game", "TOI", "AvgPts", "PROJ", "AvgSOG", "PROJ")
	fmt.Println(strings.Repeat("─", 95))

	for i := 0; i < 30 && i < len(projections); i++ {
		p := projections[i]
		loc := "vs"
		if !p.IsHome {
			loc = "@"
		}
		gameStr := fmt.Sprintf("%s %s", loc, p.OppTeam)

		// Under play analysis
		underPlay := ""
		if p.ProjPts < 0.15 && p.ProjSOG < 1.0 {
			underPlay = "Pts U0.5 + SOG U1.5"
		} else if p.ProjPts < 0.20 {
			underPlay = "Pts U0.5"
		} else if p.ProjSOG < 1.0 {
			underPlay = "SOG U1.5"
		} else {
			underPlay = "Marginal"
		}

		fmt.Printf("%-22s %-5s %-11s  %5.1f │ %6.2f %6.2f │ %6.2f %6.2f │ %s\n",
			truncate(p.Name, 22),
			p.Team,
			gameStr,
			p.TOI,
			p.AvgPts, p.ProjPts,
			p.AvgSOG, p.ProjSOG,
			underPlay,
		)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Total players projected: %d\n", len(projections))
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

func buildProjection(p types.Player, stats *types.PlayerStats, factory *distributions.Factory, ctx distributions.DistributionContext, team, opp string, isHome bool, game types.Game) PlayerProjection {
	pos := "F"
	if p.Position == types.PositionDefense {
		pos = "D"
	}

	gp := float64(max(stats.GamesPlayed, 1))

	// Get projections from each distribution
	ptsDist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
	sogDist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
	goalsDist := factory.CreateDistribution(types.PropMarketGoalsOver, stats, ctx)
	astsDist := factory.CreateDistribution(types.PropMarketAssistsOver, stats, ctx)

	return PlayerProjection{
		Name:      p.Name,
		Team:      team,
		Position:  pos,
		Game:      fmt.Sprintf("%s@%s", game.AwayTeamAbbr, game.HomeTeamAbbr),
		AvgPts:    stats.PointsPerGame,
		AvgSOG:    stats.SOG,
		AvgGoals:  float64(stats.Goals) / gp,
		AvgAsts:   float64(stats.Assists) / gp,
		TOI:       stats.TOI,
		ProjPts:   ptsDist.Mean(),
		ProjSOG:   sogDist.Mean(),
		ProjGoals: goalsDist.Mean(),
		ProjAsts:  astsDist.Mean(),
		IsHome:    isHome,
		OppTeam:   opp,
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
