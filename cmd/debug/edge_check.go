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

type EdgePick struct {
	Player      string
	Team        string
	Market      string
	Line        float64
	Side        string
	Projection  float64
	SeasonAvg   float64
	ModelProb   float64
	ImpliedProb float64
	Edge        float64
	Odds        int
}

func main() {
	apiKey := os.Getenv("ODDS_API_KEY")
	if apiKey == "" {
		fmt.Println("ODDS_API_KEY not set")
		return
	}

	oddsClient := odds.NewWithKey(apiKey)
	nhlClient := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	fmt.Println("Fetching live odds and comparing to model...")
	fmt.Println()

	events, err := oddsClient.GetEvents()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var allPicks []EdgePick

	// Just check first 3 games to save API calls
	limit := 3
	if len(events) < limit {
		limit = len(events)
	}

	for i := 0; i < limit; i++ {
		event := events[i]

		// Find team abbrevs
		homeTeam := teamNameToAbbr(event.HomeTeam)
		awayTeam := teamNameToAbbr(event.AwayTeam)

		if homeTeam == "" || awayTeam == "" {
			continue
		}

		fmt.Printf("Analyzing: %s @ %s\n", awayTeam, homeTeam)

		props, err := oddsClient.GetPlayerProps(event.ID, []string{"player_shots_on_goal", "player_points"})
		if err != nil {
			continue
		}

		// Get rosters
		homeRoster, _ := nhlClient.GetRoster(homeTeam)
		awayRoster, _ := nhlClient.GetRoster(awayTeam)
		allRoster := append(homeRoster, awayRoster...)

		for _, p := range allRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := nhlClient.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 12 {
				continue
			}

			ctx := distributions.DistributionContext{
				TeamTotal:     3.0,
				OppGA60:       3.0,
				HomeGame:      true,
				ArenaSOGBias:  1.0,
				FatigueFactor: 0.0,
				ExpectedTOI:   pStats.TOI,
				PP1Unit:       pStats.PPTOI >= 1.5,
			}

			// Check SOG props
			sogProp := findProp(props, p.Name, "shots")
			if sogProp != nil {
				dist := factory.CreateDistribution(types.PropMarketSOGOver, pStats, ctx)
				proj := dist.Mean()

				// Over
				probOver := dist.ProbOver(sogProp.Line)
				impliedOver := americanToProb(sogProp.OverPrice)
				edgeOver := (probOver - impliedOver) * 100

				allPicks = append(allPicks, EdgePick{
					Player: p.Name, Team: p.TeamAbbr,
					Market: fmt.Sprintf("SOG O%.1f", sogProp.Line),
					Line: sogProp.Line, Side: "over",
					Projection: proj, SeasonAvg: pStats.SOG,
					ModelProb: probOver, ImpliedProb: impliedOver,
					Edge: edgeOver, Odds: sogProp.OverPrice,
				})

				// Under
				probUnder := 1 - probOver
				impliedUnder := americanToProb(sogProp.UnderPrice)
				edgeUnder := (probUnder - impliedUnder) * 100

				allPicks = append(allPicks, EdgePick{
					Player: p.Name, Team: p.TeamAbbr,
					Market: fmt.Sprintf("SOG U%.1f", sogProp.Line),
					Line: sogProp.Line, Side: "under",
					Projection: proj, SeasonAvg: pStats.SOG,
					ModelProb: probUnder, ImpliedProb: impliedUnder,
					Edge: edgeUnder, Odds: sogProp.UnderPrice,
				})
			}

			// Check Points props
			ptsProp := findProp(props, p.Name, "points")
			if ptsProp != nil {
				dist := factory.CreateDistribution(types.PropMarketPointsOver, pStats, ctx)
				proj := dist.Mean()

				// Over
				probOver := dist.ProbOver(ptsProp.Line)
				impliedOver := americanToProb(ptsProp.OverPrice)
				edgeOver := (probOver - impliedOver) * 100

				allPicks = append(allPicks, EdgePick{
					Player: p.Name, Team: p.TeamAbbr,
					Market: fmt.Sprintf("Pts O%.1f", ptsProp.Line),
					Line: ptsProp.Line, Side: "over",
					Projection: proj, SeasonAvg: pStats.PointsPerGame,
					ModelProb: probOver, ImpliedProb: impliedOver,
					Edge: edgeOver, Odds: ptsProp.OverPrice,
				})

				// Under
				probUnder := 1 - probOver
				impliedUnder := americanToProb(ptsProp.UnderPrice)
				edgeUnder := (probUnder - impliedUnder) * 100

				allPicks = append(allPicks, EdgePick{
					Player: p.Name, Team: p.TeamAbbr,
					Market: fmt.Sprintf("Pts U%.1f", ptsProp.Line),
					Line: ptsProp.Line, Side: "under",
					Projection: proj, SeasonAvg: pStats.PointsPerGame,
					ModelProb: probUnder, ImpliedProb: impliedUnder,
					Edge: edgeUnder, Odds: ptsProp.UnderPrice,
				})
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Sort by edge
	sort.Slice(allPicks, func(i, j int) bool {
		return allPicks[i].Edge > allPicks[j].Edge
	})

	// Show edge distribution
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    EDGE DISTRIBUTION vs REAL ODDS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Count by edge bucket
	buckets := map[string]int{
		"Edge >= 10%":  0,
		"Edge 5-10%":   0,
		"Edge 2-5%":    0,
		"Edge 0-2%":    0,
		"Edge -2-0%":   0,
		"Edge < -2%":   0,
	}

	for _, p := range allPicks {
		switch {
		case p.Edge >= 10:
			buckets["Edge >= 10%"]++
		case p.Edge >= 5:
			buckets["Edge 5-10%"]++
		case p.Edge >= 2:
			buckets["Edge 2-5%"]++
		case p.Edge >= 0:
			buckets["Edge 0-2%"]++
		case p.Edge >= -2:
			buckets["Edge -2-0%"]++
		default:
			buckets["Edge < -2%"]++
		}
	}

	fmt.Printf("Total picks analyzed: %d\n\n", len(allPicks))
	for _, bucket := range []string{"Edge >= 10%", "Edge 5-10%", "Edge 2-5%", "Edge 0-2%", "Edge -2-0%", "Edge < -2%"} {
		fmt.Printf("  %-15s %4d picks\n", bucket+":", buckets[bucket])
	}

	// Show top 20 picks by edge
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    TOP 20 PICKS BY EDGE")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("%-20s %-5s %-12s %5s %6s %6s %6s %7s %7s\n",
		"Player", "Team", "Market", "Line", "Avg", "Model%", "Impl%", "Edge", "Odds")
	fmt.Println(strings.Repeat("─", 95))

	for i := 0; i < 20 && i < len(allPicks); i++ {
		p := allPicks[i]
		fmt.Printf("%-20s %-5s %-12s %5.1f %6.2f %5.1f%% %5.1f%% %+6.1f%% %+6d\n",
			truncate(p.Player, 20),
			p.Team,
			p.Market,
			p.Line,
			p.SeasonAvg,
			p.ModelProb*100,
			p.ImpliedProb*100,
			p.Edge,
			p.Odds,
		)
	}

	// Show any profitable picks
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    ACTIONABLE PICKS (Edge >= 3%)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	actionable := 0
	for _, p := range allPicks {
		if p.Edge >= 3 {
			if actionable == 0 {
				fmt.Printf("%-20s %-5s %-12s %6s %6s %7s %7s\n",
					"Player", "Team", "Market", "Model%", "Impl%", "Edge", "Odds")
				fmt.Println(strings.Repeat("─", 80))
			}
			fmt.Printf("%-20s %-5s %-12s %5.1f%% %5.1f%% %+6.1f%% %+6d\n",
				truncate(p.Player, 20),
				p.Team,
				p.Market,
				p.ModelProb*100,
				p.ImpliedProb*100,
				p.Edge,
				p.Odds,
			)
			actionable++
			if actionable >= 20 {
				break
			}
		}
	}

	if actionable == 0 {
		fmt.Println("No picks with >= 3% edge found against current lines")
	}
}

func findProp(props []odds.PlayerProp, playerName string, marketType string) *odds.PlayerProp {
	searchName := strings.ToLower(playerName)
	nameParts := strings.Split(searchName, " ")
	lastName := nameParts[len(nameParts)-1]

	for i, p := range props {
		propName := strings.ToLower(p.PlayerName)
		if (propName == searchName || strings.Contains(propName, lastName)) &&
			strings.Contains(strings.ToLower(p.Market), marketType) {
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

func teamNameToAbbr(name string) string {
	abbrs := map[string]string{
		"Florida Panthers": "FLA", "Buffalo Sabres": "BUF",
		"Pittsburgh Penguins": "PIT", "Ottawa Senators": "OTT",
		"Washington Capitals": "WSH", "New York Islanders": "NYI",
		"Minnesota Wild": "MIN", "Montréal Canadiens": "MTL", "Montreal Canadiens": "MTL",
		"Nashville Predators": "NSH", "St Louis Blues": "STL", "St. Louis Blues": "STL",
		"Chicago Blackhawks": "CHI", "San Jose Sharks": "SJS",
		"Dallas Stars": "DAL", "Winnipeg Jets": "WPG",
		"Colorado Avalanche": "COL", "Detroit Red Wings": "DET",
		"Utah Hockey Club": "UTA", "Utah Mammoth": "UTA", "Vancouver Canucks": "VAN",
		"Calgary Flames": "CGY", "Toronto Maple Leafs": "TOR",
		"Edmonton Oilers": "EDM", "Vegas Golden Knights": "VGK",
		"Los Angeles Kings": "LAK", "Anaheim Ducks": "ANA",
		"Seattle Kraken": "SEA", "New York Rangers": "NYR",
		"New Jersey Devils": "NJD", "Philadelphia Flyers": "PHI",
		"Boston Bruins": "BOS", "Tampa Bay Lightning": "TBL",
		"Carolina Hurricanes": "CAR", "Columbus Blue Jackets": "CBJ",
	}
	return abbrs[name]
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
