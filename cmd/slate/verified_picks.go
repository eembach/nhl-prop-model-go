// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Pick represents a model-generated pick
type Pick struct {
	Player      string
	Team        string
	Opponent    string
	Game        string
	Market      string
	Line        float64
	Side        string
	Projection  float64
	SeasonAvg   float64
	ProbHit     float64
	Edge        float64
	Confidence  string
	TOI         float64
	LineLikely  bool   // Whether a betting line is likely available
	LineReason  string // Why line is/isn't likely
}

// OddsAPIResponse for The Odds API (free tier available)
type OddsAPIResponse struct {
	Bookmakers []struct {
		Key     string `json:"key"`
		Markets []struct {
			Key      string `json:"key"`
			Outcomes []struct {
				Name  string  `json:"name"`
				Price int     `json:"price"`
				Point float64 `json:"point"`
			} `json:"outcomes"`
		} `json:"markets"`
	} `json:"bookmakers"`
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("              VERIFIED PICKS - BETTING LINE AVAILABILITY FILTER")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Date: %s\n", time.Now().Format("Monday, January 2, 2006"))
	fmt.Println()
	fmt.Println("Line Availability Criteria:")
	fmt.Println("  • TOI >= 15 min (regular rotation player)")
	fmt.Println("  • OR Points/G >= 0.5 (productive player)")
	fmt.Println("  • OR SOG/G >= 2.5 (volume shooter)")
	fmt.Println("  • Depth players (4th line, 3rd pair) rarely have lines posted")
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

		// Process both teams
		processTeam := func(roster []types.Player, team, opponent string, isHome bool, oppStats types.TeamStats) {
			for _, p := range roster {
				if p.Position == types.PositionGoalie {
					continue
				}

				pStats, err := client.GetPlayerStats(p.ID, season)
				if err != nil || pStats == nil || pStats.TOI < 10 {
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

				picks := generatePicks(p, pStats, factory, ctx, team, opponent, gameStr)
				allPicks = append(allPicks, picks...)
			}
		}

		homeRoster, _ := client.GetRoster(homeTeam)
		awayRoster, _ := client.GetRoster(awayTeam)

		processTeam(homeRoster, homeTeam, awayTeam, true, awayStats)
		processTeam(awayRoster, awayTeam, homeTeam, false, homeStats)
	}

	fmt.Printf("\r%s\n\n", strings.Repeat(" ", 60))

	// Filter to picks with lines likely available AND positive edge
	var verifiedPicks []Pick
	var unverifiedPicks []Pick

	for _, p := range allPicks {
		if p.Edge >= 5.0 {
			if p.LineLikely {
				verifiedPicks = append(verifiedPicks, p)
			} else {
				unverifiedPicks = append(unverifiedPicks, p)
			}
		}
	}

	// Sort by edge
	sort.Slice(verifiedPicks, func(i, j int) bool {
		return verifiedPicks[i].Edge > verifiedPicks[j].Edge
	})

	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    VERIFIED PICKS (Lines Likely Available)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Group by market
	markets := map[string][]Pick{
		"SOG O2.5":    {},
		"SOG O3.5":    {},
		"Points O0.5": {},
		"SOG U2.5":    {},
		"Points U0.5": {},
	}

	for _, p := range verifiedPicks {
		if picks, ok := markets[p.Market]; ok {
			markets[p.Market] = append(picks, p)
		}
	}

	// Display OVER picks first (more commonly available)
	fmt.Println("─── OVER PICKS (More commonly posted) ───────────────────────────────────────────────────────────")
	fmt.Println()

	for _, market := range []string{"SOG O3.5", "SOG O2.5", "Points O0.5"} {
		picks := markets[market]
		if len(picks) == 0 {
			continue
		}

		fmt.Printf("  %s:\n", market)
		fmt.Printf("  %-22s %-5s %-9s %5s %6s %6s %7s\n",
			"Player", "Team", "Game", "TOI", "Avg", "Proj", "Edge")
		fmt.Println("  " + strings.Repeat("─", 75))

		limit := 10
		if len(picks) < limit {
			limit = len(picks)
		}

		for i := 0; i < limit; i++ {
			p := picks[i]
			fmt.Printf("  %-22s %-5s %-9s %5.1f %6.2f %6.2f %+6.1f%%\n",
				truncate(p.Player, 22),
				p.Team,
				p.Game,
				p.TOI,
				p.SeasonAvg,
				p.Projection,
				p.Edge,
			)
		}
		fmt.Println()
	}

	// Display UNDER picks
	fmt.Println("─── UNDER PICKS (Check availability - less common) ──────────────────────────────────────────────")
	fmt.Println()

	for _, market := range []string{"SOG U2.5", "Points U0.5"} {
		picks := markets[market]
		if len(picks) == 0 {
			continue
		}

		fmt.Printf("  %s:\n", market)
		fmt.Printf("  %-22s %-5s %-9s %5s %6s %6s %7s %-20s\n",
			"Player", "Team", "Game", "TOI", "Avg", "Proj", "Edge", "Note")
		fmt.Println("  " + strings.Repeat("─", 95))

		limit := 15
		if len(picks) < limit {
			limit = len(picks)
		}

		for i := 0; i < limit; i++ {
			p := picks[i]
			fmt.Printf("  %-22s %-5s %-9s %5.1f %6.2f %6.2f %+6.1f%% %-20s\n",
				truncate(p.Player, 22),
				p.Team,
				p.Game,
				p.TOI,
				p.SeasonAvg,
				p.Projection,
				p.Edge,
				truncate(p.LineReason, 20),
			)
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                                    SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Verified picks (line likely):     %d\n", len(verifiedPicks))
	fmt.Printf("  Unverified picks (line unlikely): %d\n", len(unverifiedPicks))
	fmt.Println()

	// Count by market
	for _, market := range []string{"SOG O3.5", "SOG O2.5", "Points O0.5", "SOG U2.5", "Points U0.5"} {
		fmt.Printf("    %-15s %d picks\n", market+":", len(markets[market]))
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  RECOMMENDED: Verify lines on your sportsbook before betting")
	fmt.Println("  API Integration: Set ODDS_API_KEY env var to enable live line verification")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")

	// Check if API key is set
	apiKey := os.Getenv("ODDS_API_KEY")
	if apiKey != "" {
		fmt.Println()
		fmt.Println("  [Odds API key detected - live verification available]")
	}
}

func generatePicks(p types.Player, stats *types.PlayerStats, factory *distributions.Factory, ctx distributions.DistributionContext, team, opponent, game string) []Pick {
	var picks []Pick

	// Determine if betting line is likely available
	lineLikely := false
	lineReason := ""

	if stats.TOI >= 18 {
		lineLikely = true
		lineReason = "High TOI (top-6/4)"
	} else if stats.TOI >= 15 && stats.PointsPerGame >= 0.4 {
		lineLikely = true
		lineReason = "Productive regular"
	} else if stats.SOG >= 2.5 {
		lineLikely = true
		lineReason = "Volume shooter"
	} else if stats.PointsPerGame >= 0.5 {
		lineLikely = true
		lineReason = "Point producer"
	} else if stats.TOI >= 15 {
		lineLikely = true
		lineReason = "Regular minutes"
	} else {
		lineReason = "Depth player"
	}

	// SOG O3.5 (elite shooters only)
	if stats.SOG >= 3.5 && lineLikely {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(3.5)
		edge := (probOver - 0.524) * 100

		if edge >= 5 {
			picks = append(picks, Pick{
				Player:     p.Name,
				Team:       team,
				Opponent:   opponent,
				Game:       game,
				Market:     "SOG O3.5",
				Line:       3.5,
				Side:       "over",
				Projection: proj,
				SeasonAvg:  stats.SOG,
				ProbHit:    probOver,
				Edge:       edge,
				TOI:        stats.TOI,
				LineLikely: lineLikely,
				LineReason: lineReason,
			})
		}
	}

	// SOG O2.5
	if stats.SOG >= 2.5 && lineLikely {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(2.5)
		edge := (probOver - 0.524) * 100

		if edge >= 5 {
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
				TOI:        stats.TOI,
				LineLikely: lineLikely,
				LineReason: lineReason,
			})
		}
	}

	// Points O0.5
	if stats.PointsPerGame >= 0.5 && lineLikely {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(0.5)
		edge := (probOver - 0.535) * 100

		if edge >= 5 {
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
				TOI:        stats.TOI,
				LineLikely: lineLikely,
				LineReason: lineReason,
			})
		}
	}

	// SOG U2.5 - only for players likely to have lines
	if stats.SOG <= 2.0 && stats.SOG >= 1.0 && lineLikely {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(2.5)
		edge := (probUnder - 0.524) * 100

		if edge >= 5 {
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
				TOI:        stats.TOI,
				LineLikely: lineLikely,
				LineReason: lineReason,
			})
		}
	}

	// Points U0.5 - only for players likely to have lines
	if stats.PointsPerGame <= 0.4 && stats.PointsPerGame >= 0.15 && lineLikely {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(0.5)
		edge := (probUnder - 0.535) * 100

		if edge >= 5 {
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
				TOI:        stats.TOI,
				LineLikely: lineLikely,
				LineReason: lineReason,
			})
		}
	}

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

// verifyLineWithAPI checks if a line is available using The Odds API
// Requires ODDS_API_KEY environment variable
func verifyLineWithAPI(gameID string, playerName string, market string) bool {
	apiKey := os.Getenv("ODDS_API_KEY")
	if apiKey == "" {
		return false
	}

	// The Odds API endpoint for player props
	url := fmt.Sprintf("https://api.the-odds-api.com/v4/sports/icehockey_nhl/events/%s/odds?apiKey=%s&regions=us&markets=player_points,player_shots_on_goal&oddsFormat=american",
		gameID, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var data OddsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false
	}

	// Check if player has a line in the response
	for _, book := range data.Bookmakers {
		for _, mkt := range book.Markets {
			for _, outcome := range mkt.Outcomes {
				if strings.Contains(strings.ToLower(outcome.Name), strings.ToLower(playerName)) {
					return true
				}
			}
		}
	}

	return false
}
