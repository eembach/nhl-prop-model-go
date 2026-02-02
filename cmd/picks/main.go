package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/props"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// QualifiedPick represents a methodology-qualified pick for output
type QualifiedPick struct {
	GameTime       string  `json:"gameTime"`
	Matchup        string  `json:"matchup"`
	Scenario       string  `json:"scenario"`
	PlayerName     string  `json:"playerName"`
	Team           string  `json:"team"`
	Market         string  `json:"market"`
	Line           float64 `json:"line"`
	Side           string  `json:"side"`
	OurProjection  float64 `json:"ourProjection"`
	Edge           float64 `json:"edge"`
	Stars          int     `json:"stars"`
	Tier           string  `json:"tier"`
	Determinants   string  `json:"determinants"`
	RiskFlags      string  `json:"riskFlags"`
	Qualification  string  `json:"qualification"`
}

func main() {
	// Parse command line flags
	dateStr := flag.String("date", "", "Date to analyze (YYYY-MM-DD), defaults to today")
	minEdge := flag.Float64("min-edge", 3.0, "Minimum edge to show picks")
	maxPicks := flag.Int("max-picks", 20, "Maximum picks to show")
	outputFormat := flag.String("format", "table", "Output format: table, json, csv")
	tierFilter := flag.String("tier", "all", "Filter by tier: 1, 1.5, 2, 3, or all")
	showAll := flag.Bool("all", false, "Show all picks including non-qualified")
	flag.Parse()

	// Determine date
	var targetDate time.Time
	if *dateStr != "" {
		var err error
		targetDate, err = time.Parse("2006-01-02", *dateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid date format: %s (use YYYY-MM-DD)\n", *dateStr)
			os.Exit(1)
		}
	} else {
		targetDate = time.Now()
	}

	fmt.Printf("NHL Prop Picks Generator - %s\n", targetDate.Format("Monday, January 2, 2006"))
	fmt.Println(strings.Repeat("=", 70))

	// Initialize components
	client := nhl.New()
	evaluator := props.NewEvaluator()

	// Set methodology parameters
	evaluator.MinEdgeForPlay = *minEdge

	// Determine current season
	season := getCurrentSeason(targetDate)

	// Fetch schedule
	games, err := client.GetSchedule(targetDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching schedule: %v\n", err)
		os.Exit(1)
	}

	if len(games) == 0 {
		fmt.Println("No games scheduled for this date.")
		return
	}

	fmt.Printf("\nFound %d games\n\n", len(games))

	// Fetch standings for team stats
	teamStats, err := client.GetStandings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching standings: %v\n", err)
		os.Exit(1)
	}

	// Build team stats map
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var allPicks []QualifiedPick

	// Process each game
	for _, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr
		matchup := fmt.Sprintf("%s @ %s", awayTeam, homeTeam)
		gameTime := game.StartTime.Local().Format("3:04 PM")

		fmt.Printf("Processing: %s (%s)\n", matchup, gameTime)

		// Get team stats from standings
		homeStats, okHome := teamStatsMap[homeTeam]
		awayStats, okAway := teamStatsMap[awayTeam]
		if !okHome || !okAway {
			fmt.Printf("  Warning: Could not find team stats\n")
			continue
		}

		// Use default scenario based on team stats
		// High-scoring teams -> Run-and-Gun, Low-scoring -> Goalie Duel
		scenarioType := determineScenario(homeStats, awayStats)
		fmt.Printf("  Scenario: %s (based on team totals)\n", scenarioType)

		// Build game scenario
		gameScenario := &types.GameScenario{
			GameID: game.ID,
			Primary: types.ScenarioScore{
				Scenario: scenarioType,
				Score:    70, // Default confidence
				Rank:     1,
			},
		}

		// Fetch rosters
		homeRoster, err := client.GetRoster(homeTeam)
		if err != nil {
			fmt.Printf("  Warning: Could not fetch %s roster: %v\n", homeTeam, err)
			continue
		}
		awayRoster, err := client.GetRoster(awayTeam)
		if err != nil {
			fmt.Printf("  Warning: Could not fetch %s roster: %v\n", awayTeam, err)
			continue
		}

		// Build player map
		players := make(map[int]*types.PlayerWithStats)
		allRoster := append(homeRoster, awayRoster...)

		for _, p := range allRoster {
			// Skip goalies for prop analysis
			if p.Position == types.PositionGoalie {
				continue
			}

			// Fetch individual player stats
			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil {
				continue
			}

			// Fetch recent game logs for form
			logs, _ := client.GetPlayerGameLog(p.ID, season, 5)
			if len(logs) > 0 {
				recentStats := nhl.CalculateRecentStats(logs)
				pStats.RecentSOG = recentStats.RecentSOG
				pStats.RecentPPG = recentStats.RecentPPG
				pStats.RecentTOI = recentStats.RecentTOI
			}

			// Estimate top-6/top-4 and PP1 from TOI
			if p.Position == types.PositionDefense {
				pStats.IsTopFour = pStats.TOI >= 18.0
			} else {
				pStats.IsTopSix = pStats.TOI >= 14.0
			}
			pStats.PP1Unit = pStats.PPTOI >= 1.5

			players[p.ID] = &types.PlayerWithStats{
				Player: p,
				Stats:  *pStats,
			}
		}

		fmt.Printf("  Loaded stats for %d players\n", len(players))

		// Generate prop lines (in production, these would come from odds feeds)
		propLines := generateRealisticPropLines(players)

		// Build distribution context
		ctx := distributions.DistributionContext{
			TeamTotal:      (homeStats.GFPerGame + awayStats.GFPerGame) / 2,
			OppGA60:        awayStats.GAPerGame * 20, // Rough conversion to per-60
			ExpectedTOI:    0, // Set per player
			HomeGame:       true,
			PP1Unit:        false, // Set per player
			IsB2B:          false, // Would need schedule lookup
			Is3In4:         false,
			ArenaSOGBias:   1.0,
			FatigueFactor:  0.0,
		}

		// Evaluate each prop
		for _, line := range propLines {
			player, ok := players[line.PlayerID]
			if !ok {
				continue
			}

			// Update context for this player
			ctx.ExpectedTOI = player.Stats.TOI
			ctx.PP1Unit = player.Stats.PP1Unit

			// Use methodology evaluation
			eval, qualifies, reason, recommendedSide := evaluator.EvaluateWithMethodology(
				line, player, ctx, gameScenario,
			)

			// Apply tier filter
			if *tierFilter != "all" {
				tierMatch := false
				switch *tierFilter {
				case "1":
					tierMatch = eval.Tier == types.PropTier1
				case "1.5":
					tierMatch = eval.Tier == types.PropTier1_5
				case "2":
					tierMatch = eval.Tier == types.PropTier2
				case "3":
					tierMatch = eval.Tier == types.PropTier3
				}
				if !tierMatch {
					continue
				}
			}

			// Skip non-qualified unless showing all
			if !qualifies && !*showAll {
				continue
			}

			// Skip if edge is below minimum
			if eval.Edge < *minEdge && !*showAll {
				continue
			}

			// Build pick
			pick := QualifiedPick{
				GameTime:      gameTime,
				Matchup:       matchup,
				Scenario:      string(scenarioType),
				PlayerName:    player.Player.Name,
				Team:          player.Player.TeamAbbr,
				Market:        string(line.Market),
				Line:          line.Line,
				Side:          string(recommendedSide),
				OurProjection: eval.OurLine,
				Edge:          eval.Edge,
				Stars:         eval.Stars,
				Tier:          string(eval.Tier),
				Determinants:  strings.Join(eval.Determinants, ", "),
				RiskFlags:     strings.Join(eval.RiskFlags, ", "),
				Qualification: reason,
			}

			if !qualifies {
				pick.Qualification = fmt.Sprintf("NOT QUALIFIED: %s", reason)
			}

			allPicks = append(allPicks, pick)
		}
	}

	// Sort by edge descending
	sort.Slice(allPicks, func(i, j int) bool {
		return allPicks[i].Edge > allPicks[j].Edge
	})

	// Limit picks
	if len(allPicks) > *maxPicks {
		allPicks = allPicks[:*maxPicks]
	}

	// Output
	fmt.Println()
	switch *outputFormat {
	case "json":
		outputJSON(allPicks)
	case "csv":
		outputCSV(allPicks)
	default:
		outputTable(allPicks)
	}
}

func getCurrentSeason(date time.Time) string {
	year := date.Year()
	month := date.Month()

	// NHL season spans two years, starting in October
	if month >= time.October {
		return fmt.Sprintf("%d%d", year, year+1)
	}
	return fmt.Sprintf("%d%d", year-1, year)
}

func determineScenario(homeStats, awayStats types.TeamStats) types.ScenarioType {
	// Simple heuristic based on goals per game
	combinedGF := homeStats.GFPerGame + awayStats.GFPerGame

	if combinedGF >= 6.5 {
		return types.ScenarioRunAndGun
	} else if combinedGF <= 5.0 {
		return types.ScenarioGoalieDuel
	}
	// Default to run and gun (favor overs) if neutral
	return types.ScenarioRunAndGun
}

func generateRealisticPropLines(players map[int]*types.PlayerWithStats) []types.PropLine {
	var lines []types.PropLine

	for _, p := range players {
		// Only generate lines for skaters with enough TOI
		if p.Stats.TOI < 10 {
			continue
		}

		// Points O0.5 (bread and butter per methodology)
		lines = append(lines, types.PropLine{
			PlayerID:   p.Player.ID,
			PlayerName: p.Player.Name,
			TeamAbbr:   p.Player.TeamAbbr,
			Market:     types.PropMarketPointsOver,
			Line:       0.5,
			Price:      -115,
			ImpliedProb: 0.535,
			Timeframe:  types.TimeframeFullGame,
			Side:       types.PropSideOver,
		})

		// SOG - set realistic line based on average
		sogLine := getRealisticSOGLine(p.Stats.SOG)
		lines = append(lines, types.PropLine{
			PlayerID:    p.Player.ID,
			PlayerName:  p.Player.Name,
			TeamAbbr:    p.Player.TeamAbbr,
			Market:      types.PropMarketSOGOver,
			Line:        sogLine,
			Price:       -110,
			ImpliedProb: 0.524,
			Timeframe:   types.TimeframeFullGame,
			Side:        types.PropSideOver,
		})

		// Assists for high-assist players (calculate assists per game)
		if p.Stats.GamesPlayed > 0 {
			assistsPerGame := float64(p.Stats.Assists) / float64(p.Stats.GamesPlayed)
			if assistsPerGame >= 0.3 {
				lines = append(lines, types.PropLine{
					PlayerID:    p.Player.ID,
					PlayerName:  p.Player.Name,
					TeamAbbr:    p.Player.TeamAbbr,
					Market:      types.PropMarketAssistsOver,
					Line:        0.5,
					Price:       -120,
					ImpliedProb: 0.545,
					Timeframe:   types.TimeframeFullGame,
					Side:        types.PropSideOver,
				})
			}
		}
	}

	return lines
}

func getRealisticSOGLine(avgSOG float64) float64 {
	// Sportsbooks set lines at common values near player averages
	// Common lines: 1.5, 2.5, 3.5, 4.5
	if avgSOG < 1.5 {
		return 1.5
	} else if avgSOG < 2.5 {
		return 1.5
	} else if avgSOG < 3.0 {
		return 2.5
	} else if avgSOG < 4.0 {
		return 3.5
	}
	return 4.5
}

func outputTable(picks []QualifiedPick) {
	if len(picks) == 0 {
		fmt.Println("No qualified picks found.")
		return
	}

	fmt.Println("METHODOLOGY-QUALIFIED PICKS")
	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("%-6s %-20s %-15s %-12s %-6s %-5s %-6s %-6s %-4s %s\n",
		"Time", "Player", "Matchup", "Market", "Line", "Side", "Proj", "Edge", "★", "Scenario")
	fmt.Println(strings.Repeat("-", 120))

	for _, pick := range picks {
		stars := strings.Repeat("★", pick.Stars)
		fmt.Printf("%-6s %-20s %-15s %-12s %-6.1f %-5s %-6.2f %+5.1f%% %-4s %s\n",
			pick.GameTime,
			truncate(pick.PlayerName, 20),
			pick.Matchup,
			pick.Market,
			pick.Line,
			pick.Side,
			pick.OurProjection,
			pick.Edge,
			stars,
			pick.Scenario,
		)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total: %d qualified picks\n", len(picks))

	// Summary by tier
	tierCounts := make(map[string]int)
	for _, p := range picks {
		tierCounts[p.Tier]++
	}
	fmt.Printf("\nBy Tier: ")
	for tier, count := range tierCounts {
		fmt.Printf("%s: %d  ", tier, count)
	}
	fmt.Println()

	// Summary by scenario
	scenarioCounts := make(map[string]int)
	for _, p := range picks {
		scenarioCounts[p.Scenario]++
	}
	fmt.Printf("By Scenario: ")
	for scenarioName, count := range scenarioCounts {
		fmt.Printf("%s: %d  ", scenarioName, count)
	}
	fmt.Println()
}

func outputJSON(picks []QualifiedPick) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(picks)
}

func outputCSV(picks []QualifiedPick) {
	fmt.Println("GameTime,Player,Team,Matchup,Scenario,Market,Line,Side,Projection,Edge,Stars,Tier,Qualification")
	for _, p := range picks {
		fmt.Printf("%s,%s,%s,%s,%s,%s,%.1f,%s,%.2f,%.1f,%d,%s,\"%s\"\n",
			p.GameTime,
			p.PlayerName,
			p.Team,
			p.Matchup,
			p.Scenario,
			p.Market,
			p.Line,
			p.Side,
			p.OurProjection,
			p.Edge,
			p.Stars,
			p.Tier,
			p.Qualification,
		)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
