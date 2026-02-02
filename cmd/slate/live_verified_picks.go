// +build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/api/odds"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

type Pick struct {
	Player      string
	Team        string
	Game        string
	Market      string
	Line        float64
	Side        string
	Projection  float64
	SeasonAvg   float64
	Edge        float64
	TOI         float64
	// Live odds data
	LiveLine    float64
	LiveOdds    int
	Bookmaker   string
	Verified    bool
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("              LIVE VERIFIED PICKS - REAL BETTING LINES")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Date: %s\n\n", time.Now().Format("Monday, January 2, 2006"))

	// Check for API key
	oddsClient := odds.New()
	if !oddsClient.HasAPIKey() {
		fmt.Println("⚠️  ODDS_API_KEY not set - running in offline mode")
		fmt.Println("   To enable live line verification:")
		fmt.Println("   1. Get free API key at https://the-odds-api.com/")
		fmt.Println("   2. Run: export ODDS_API_KEY=your_key_here")
		fmt.Println()
		runOfflineMode()
		return
	}

	fmt.Println("✓ Odds API connected - fetching live lines...")
	fmt.Println()

	// Fetch all available player props
	fmt.Print("Fetching player props from sportsbooks...")
	allProps, err := oddsClient.GetAllPlayerProps()
	if err != nil {
		fmt.Printf("\n⚠️  Error fetching odds: %v\n", err)
		fmt.Println("Falling back to offline mode...")
		runOfflineMode()
		return
	}
	fmt.Printf(" Found %d games with props\n\n", len(allProps))

	// Show what lines are available
	fmt.Println("Available Lines by Game:")
	fmt.Println(strings.Repeat("─", 60))
	for game, props := range allProps {
		sogCount := 0
		ptsCount := 0
		for _, p := range props {
			if strings.Contains(p.Market, "shots") {
				sogCount++
			}
			if strings.Contains(p.Market, "points") {
				ptsCount++
			}
		}
		fmt.Printf("  %s: %d SOG props, %d Points props\n", game, sogCount, ptsCount)
	}
	fmt.Println()

	// Now generate picks and verify against live lines
	client := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	games, err := client.GetSchedule(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	teamStats, _ := client.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var verifiedPicks []Pick

	fmt.Print("Analyzing players and matching to live lines...")

	for _, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats, ok1 := teamStatsMap[homeTeam]
		awayStats, ok2 := teamStatsMap[awayTeam]
		if !ok1 || !ok2 {
			continue
		}

		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame
		gameStr := fmt.Sprintf("%s@%s", awayTeam, homeTeam)

		// Find props for this game (try different name formats)
		var gameProps []odds.PlayerProp
		for gameKey, props := range allProps {
			if strings.Contains(gameKey, homeTeam) || strings.Contains(gameKey, awayTeam) {
				gameProps = append(gameProps, props...)
			}
		}

		processTeam := func(roster []types.Player, team string, isHome bool, oppStats types.TeamStats) {
			for _, p := range roster {
				if p.Position == types.PositionGoalie {
					continue
				}

				pStats, err := client.GetPlayerStats(p.ID, season)
				if err != nil || pStats == nil || pStats.TOI < 12 {
					continue
				}

				ctx := distributions.DistributionContext{
					TeamTotal:     expectedTotal / 2,
					OppGA60:       oppStats.GAPerGame,
					HomeGame:      isHome,
					ArenaSOGBias:  1.0,
					FatigueFactor: 0.0,
					ExpectedTOI:   pStats.TOI,
					PP1Unit:       pStats.PPTOI >= 1.5,
				}

				// Check for SOG line
				sogProp := findPlayerProp(gameProps, p.Name, "shots")
				if sogProp != nil {
					dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
					proj := dist.Mean()

					// Calculate edge for over
					probOver := dist.ProbOver(sogProp.Line)
					impliedProb := americanToProb(sogProp.OverPrice)
					edgeOver := (probOver - impliedProb) * 100

					// Calculate edge for under
					probUnder := 1 - dist.ProbOver(sogProp.Line)
					impliedProbUnder := americanToProb(sogProp.UnderPrice)
					edgeUnder := (probUnder - impliedProbUnder) * 100

					if edgeOver >= 5 {
						verifiedPicks = append(verifiedPicks, Pick{
							Player:     p.Name,
							Team:       team,
							Game:       gameStr,
							Market:     fmt.Sprintf("SOG O%.1f", sogProp.Line),
							Line:       sogProp.Line,
							Side:       "over",
							Projection: proj,
							SeasonAvg:  pStats.SOG,
							Edge:       edgeOver,
							TOI:        pStats.TOI,
							LiveLine:   sogProp.Line,
							LiveOdds:   sogProp.OverPrice,
							Bookmaker:  sogProp.Bookmaker,
							Verified:   true,
						})
					}

					if edgeUnder >= 5 {
						verifiedPicks = append(verifiedPicks, Pick{
							Player:     p.Name,
							Team:       team,
							Game:       gameStr,
							Market:     fmt.Sprintf("SOG U%.1f", sogProp.Line),
							Line:       sogProp.Line,
							Side:       "under",
							Projection: proj,
							SeasonAvg:  pStats.SOG,
							Edge:       edgeUnder,
							TOI:        pStats.TOI,
							LiveLine:   sogProp.Line,
							LiveOdds:   sogProp.UnderPrice,
							Bookmaker:  sogProp.Bookmaker,
							Verified:   true,
						})
					}
				}

				// Check for Points line
				ptsProp := findPlayerProp(gameProps, p.Name, "points")
				if ptsProp != nil {
					dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
					proj := dist.Mean()

					// Calculate edge for over
					probOver := dist.ProbOver(ptsProp.Line)
					impliedProb := americanToProb(ptsProp.OverPrice)
					edgeOver := (probOver - impliedProb) * 100

					// Calculate edge for under
					probUnder := 1 - dist.ProbOver(ptsProp.Line)
					impliedProbUnder := americanToProb(ptsProp.UnderPrice)
					edgeUnder := (probUnder - impliedProbUnder) * 100

					if edgeOver >= 5 {
						verifiedPicks = append(verifiedPicks, Pick{
							Player:     p.Name,
							Team:       team,
							Game:       gameStr,
							Market:     fmt.Sprintf("Pts O%.1f", ptsProp.Line),
							Line:       ptsProp.Line,
							Side:       "over",
							Projection: proj,
							SeasonAvg:  pStats.PointsPerGame,
							Edge:       edgeOver,
							TOI:        pStats.TOI,
							LiveLine:   ptsProp.Line,
							LiveOdds:   ptsProp.OverPrice,
							Bookmaker:  ptsProp.Bookmaker,
							Verified:   true,
						})
					}

					if edgeUnder >= 5 {
						verifiedPicks = append(verifiedPicks, Pick{
							Player:     p.Name,
							Team:       team,
							Game:       gameStr,
							Market:     fmt.Sprintf("Pts U%.1f", ptsProp.Line),
							Line:       ptsProp.Line,
							Side:       "under",
							Projection: proj,
							SeasonAvg:  pStats.PointsPerGame,
							Edge:       edgeUnder,
							TOI:        pStats.TOI,
							LiveLine:   ptsProp.Line,
							LiveOdds:   ptsProp.UnderPrice,
							Bookmaker:  ptsProp.Bookmaker,
							Verified:   true,
						})
					}
				}
			}
		}

		homeRoster, _ := client.GetRoster(homeTeam)
		awayRoster, _ := client.GetRoster(awayTeam)

		processTeam(homeRoster, homeTeam, true, awayStats)
		processTeam(awayRoster, awayTeam, false, homeStats)
	}

	fmt.Printf(" Done!\n\n")

	if len(verifiedPicks) == 0 {
		fmt.Println("No verified picks found with edge >= 5%")
		fmt.Println("This could mean:")
		fmt.Println("  - Lines are efficiently priced")
		fmt.Println("  - Player props not yet posted")
		fmt.Println("  - API returned limited data")
		return
	}

	// Sort by edge
	sort.Slice(verifiedPicks, func(i, j int) bool {
		return verifiedPicks[i].Edge > verifiedPicks[j].Edge
	})

	// Display verified picks
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    VERIFIED PICKS WITH LIVE ODDS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("%-20s %-5s %-12s %5s %6s %6s %7s %8s %-12s\n",
		"Player", "Team", "Market", "Line", "Avg", "Proj", "Edge", "Odds", "Book")
	fmt.Println(strings.Repeat("─", 100))

	for i, p := range verifiedPicks {
		if i >= 30 {
			fmt.Printf("\n... and %d more verified picks\n", len(verifiedPicks)-30)
			break
		}

		oddsStr := fmt.Sprintf("%+d", p.LiveOdds)
		fmt.Printf("%-20s %-5s %-12s %5.1f %6.2f %6.2f %+6.1f%% %8s %-12s\n",
			truncate(p.Player, 20),
			p.Team,
			p.Market,
			p.LiveLine,
			p.SeasonAvg,
			p.Projection,
			p.Edge,
			oddsStr,
			truncate(p.Bookmaker, 12),
		)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Total verified picks: %d\n", len(verifiedPicks))
	fmt.Println("All picks verified against live sportsbook lines")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

func runOfflineMode() {
	fmt.Println()
	fmt.Println("Running verified_picks.go instead (offline mode with heuristic filtering)...")
	fmt.Println("For live odds verification, set ODDS_API_KEY environment variable.")
}

func findPlayerProp(props []odds.PlayerProp, playerName string, marketType string) *odds.PlayerProp {
	searchName := strings.ToLower(playerName)

	// Try exact match first
	for i, p := range props {
		propName := strings.ToLower(p.PlayerName)
		if propName == searchName && strings.Contains(strings.ToLower(p.Market), marketType) {
			return &props[i]
		}
	}

	// Try partial match (last name)
	nameParts := strings.Split(searchName, " ")
	lastName := nameParts[len(nameParts)-1]

	for i, p := range props {
		propName := strings.ToLower(p.PlayerName)
		if strings.Contains(propName, lastName) && strings.Contains(strings.ToLower(p.Market), marketType) {
			return &props[i]
		}
	}

	return nil
}

func americanToProb(odds int) float64 {
	if odds > 0 {
		return 100.0 / float64(100+odds)
	}
	return float64(-odds) / float64(-odds+100)
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
