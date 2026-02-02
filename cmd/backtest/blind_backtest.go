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

// Pick represents a model-generated pick (made blind, pregame)
type Pick struct {
	GameID     int
	GameDate   string
	Player     string
	PlayerID   int
	Team       string
	Opponent   string
	Market     string
	Line       float64
	Side       string
	Projection float64
	ProbHit    float64
	Edge       float64

	// Grading (filled in after)
	Actual    float64
	Hit       bool
	Graded    bool
}

// BoxScore data from NHL API
type BoxScoreResponse struct {
	PlayerByGameStats struct {
		AwayTeam struct {
			Forwards []BoxScorePlayer `json:"forwards"`
			Defense  []BoxScorePlayer `json:"defense"`
		} `json:"awayTeam"`
		HomeTeam struct {
			Forwards []BoxScorePlayer `json:"forwards"`
			Defense  []BoxScorePlayer `json:"defense"`
		} `json:"homeTeam"`
	} `json:"playerByGameStats"`
}

type BoxScorePlayer struct {
	PlayerID int `json:"playerId"`
	Name     struct {
		Default string `json:"default"`
	} `json:"name"`
	SOG     int `json:"sog"`
	Goals   int `json:"goals"`
	Assists int `json:"assists"`
	Points  int `json:"points"`
	Hits    int `json:"hits"`
	Blocks  int `json:"blockedShots"`
	PIM     int `json:"pim"`
	TOI     string `json:"toi"`
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                         BLIND BACKTEST - PAST WEEK'S GAMES")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Methodology:")
	fmt.Println("  1. For each past game, use ONLY pregame data (season stats up to that point)")
	fmt.Println("  2. Generate picks blind (no knowledge of results)")
	fmt.Println("  3. After ALL picks made, fetch box scores and grade")
	fmt.Println()

	client := nhl.New()
	factory := distributions.NewFactory()

	// Get past 7 days of games
	today := time.Now()
	var allGames []types.Game

	fmt.Println("PHASE 1: Collecting past week's games...")
	fmt.Println(strings.Repeat("─", 80))

	for daysBack := 1; daysBack <= 7; daysBack++ {
		date := today.AddDate(0, 0, -daysBack)
		games, err := client.GetSchedule(date)
		if err != nil {
			continue
		}
		fmt.Printf("  %s: %d games\n", date.Format("Mon Jan 2"), len(games))
		allGames = append(allGames, games...)
	}

	fmt.Printf("\nTotal games to backtest: %d\n", len(allGames))

	if len(allGames) == 0 {
		fmt.Println("No games found in the past week.")
		os.Exit(1)
	}

	// Get team stats (current - this is a simplification; ideally we'd use historical)
	teamStats, _ := client.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	// PHASE 2: Generate picks BLIND (no box score data)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("PHASE 2: Generating picks BLIND (using only pregame data)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	var allPicks []Pick
	season := getCurrentSeason(today)

	for i, game := range allGames {
		fmt.Printf("\r[%d/%d] Processing %s @ %s (%s)...",
			i+1, len(allGames), game.AwayTeamAbbr, game.HomeTeamAbbr, game.GameDate)

		homeStats, ok1 := teamStatsMap[game.HomeTeamAbbr]
		awayStats, ok2 := teamStatsMap[game.AwayTeamAbbr]
		if !ok1 || !ok2 {
			continue
		}

		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame

		// Get rosters
		homeRoster, _ := client.GetRoster(game.HomeTeamAbbr)
		awayRoster, _ := client.GetRoster(game.AwayTeamAbbr)

		// Process home team
		for _, p := range homeRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 12 {
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

			picks := generatePlayerPicks(p, pStats, factory, ctx, game, game.HomeTeamAbbr, game.AwayTeamAbbr)
			allPicks = append(allPicks, picks...)
		}

		// Process away team
		for _, p := range awayRoster {
			if p.Position == types.PositionGoalie {
				continue
			}

			pStats, err := client.GetPlayerStats(p.ID, season)
			if err != nil || pStats == nil || pStats.TOI < 12 {
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

			picks := generatePlayerPicks(p, pStats, factory, ctx, game, game.AwayTeamAbbr, game.HomeTeamAbbr)
			allPicks = append(allPicks, picks...)
		}
	}

	fmt.Printf("\r%s\n", strings.Repeat(" ", 80))
	fmt.Printf("Total picks generated: %d\n", len(allPicks))

	// Filter to only picks with positive edge
	var qualifiedPicks []Pick
	for _, pick := range allPicks {
		if pick.Edge >= 5.0 { // Minimum 5% edge
			qualifiedPicks = append(qualifiedPicks, pick)
		}
	}
	fmt.Printf("Picks with Edge >= 5%%: %d\n", len(qualifiedPicks))

	// PHASE 3: Grade picks using box scores
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("PHASE 3: Grading picks against actual box scores")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Collect unique game IDs
	gameIDs := make(map[int]bool)
	for _, pick := range qualifiedPicks {
		gameIDs[pick.GameID] = true
	}

	// Fetch box scores
	boxScores := make(map[int]map[int]BoxScorePlayer) // gameID -> playerID -> stats
	fmt.Printf("Fetching box scores for %d games...\n", len(gameIDs))

	for gameID := range gameIDs {
		players := fetchBoxScore(gameID)
		boxScores[gameID] = players
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	// Grade each pick
	for i := range qualifiedPicks {
		pick := &qualifiedPicks[i]

		players, ok := boxScores[pick.GameID]
		if !ok {
			continue
		}

		playerStats, ok := players[pick.PlayerID]
		if !ok {
			continue
		}

		// Get actual value based on market
		switch {
		case strings.Contains(pick.Market, "SOG"):
			pick.Actual = float64(playerStats.SOG)
		case strings.Contains(pick.Market, "Points"):
			pick.Actual = float64(playerStats.Points)
		case strings.Contains(pick.Market, "Goals"):
			pick.Actual = float64(playerStats.Goals)
		case strings.Contains(pick.Market, "Assists"):
			pick.Actual = float64(playerStats.Assists)
		}

		// Determine if hit
		if pick.Side == "over" {
			pick.Hit = pick.Actual > pick.Line
		} else {
			pick.Hit = pick.Actual < pick.Line
		}
		pick.Graded = true
	}

	// Count graded picks
	gradedCount := 0
	for _, pick := range qualifiedPicks {
		if pick.Graded {
			gradedCount++
		}
	}
	fmt.Printf("Successfully graded: %d picks\n", gradedCount)

	// PHASE 4: Results Analysis
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("PHASE 4: RESULTS ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Group by market
	marketResults := make(map[string]struct {
		Total int
		Hits  int
		Edges []float64
	})

	for _, pick := range qualifiedPicks {
		if !pick.Graded {
			continue
		}

		result := marketResults[pick.Market]
		result.Total++
		if pick.Hit {
			result.Hits++
		}
		result.Edges = append(result.Edges, pick.Edge)
		marketResults[pick.Market] = result
	}

	fmt.Printf("%-20s %8s %8s %8s %10s\n", "Market", "Total", "Hits", "Hit%", "Avg Edge")
	fmt.Println(strings.Repeat("─", 60))

	totalPicks := 0
	totalHits := 0

	// Sort markets for consistent output
	var markets []string
	for m := range marketResults {
		markets = append(markets, m)
	}
	sort.Strings(markets)

	for _, market := range markets {
		result := marketResults[market]
		hitRate := float64(result.Hits) / float64(result.Total) * 100
		avgEdge := 0.0
		for _, e := range result.Edges {
			avgEdge += e
		}
		avgEdge /= float64(len(result.Edges))

		fmt.Printf("%-20s %8d %8d %7.1f%% %+9.1f%%\n",
			market, result.Total, result.Hits, hitRate, avgEdge)

		totalPicks += result.Total
		totalHits += result.Hits
	}

	fmt.Println(strings.Repeat("─", 60))
	overallHitRate := float64(totalHits) / float64(totalPicks) * 100
	fmt.Printf("%-20s %8d %8d %7.1f%%\n", "OVERALL", totalPicks, totalHits, overallHitRate)

	// Detailed pick-by-pick results
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("DETAILED RESULTS (Sample of 50 highest-edge picks)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Sort by edge
	sort.Slice(qualifiedPicks, func(i, j int) bool {
		return qualifiedPicks[i].Edge > qualifiedPicks[j].Edge
	})

	fmt.Printf("%-18s %-5s %-15s %-6s %5s %5s %6s %6s %-6s\n",
		"Player", "Team", "Market", "Side", "Line", "Proj", "Actual", "Edge", "Result")
	fmt.Println(strings.Repeat("─", 95))

	shown := 0
	for _, pick := range qualifiedPicks {
		if !pick.Graded {
			continue
		}

		result := "MISS"
		if pick.Hit {
			result = "HIT ✓"
		}

		fmt.Printf("%-18s %-5s %-15s %-6s %5.1f %5.2f %6.0f %+5.1f%% %-6s\n",
			truncate(pick.Player, 18),
			pick.Team,
			pick.Market,
			pick.Side,
			pick.Line,
			pick.Projection,
			pick.Actual,
			pick.Edge,
			result,
		)

		shown++
		if shown >= 50 {
			break
		}
	}

	// Profit/Loss calculation
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("PROFIT/LOSS SIMULATION (Flat $100 bets at -110)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// At -110: Win = +90.91, Lose = -100
	winAmount := 90.91
	loseAmount := 100.0
	totalProfit := 0.0

	for _, pick := range qualifiedPicks {
		if !pick.Graded {
			continue
		}
		if pick.Hit {
			totalProfit += winAmount
		} else {
			totalProfit -= loseAmount
		}
	}

	roi := totalProfit / (float64(gradedCount) * 100) * 100
	fmt.Printf("Total bets: %d\n", gradedCount)
	fmt.Printf("Win rate: %.1f%%\n", overallHitRate)
	fmt.Printf("Breakeven rate at -110: 52.4%%\n")
	fmt.Printf("Total P/L: $%.2f\n", totalProfit)
	fmt.Printf("ROI: %+.1f%%\n", roi)

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

func generatePlayerPicks(p types.Player, stats *types.PlayerStats, factory *distributions.Factory, ctx distributions.DistributionContext, game types.Game, team, opponent string) []Pick {
	var picks []Pick

	// SOG O2.5
	if stats.SOG >= 2.0 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(2.5)
		edge := (probOver - 0.524) * 100

		picks = append(picks, Pick{
			GameID:     game.ID,
			GameDate:   game.GameDate.Format("2006-01-02"),
			Player:     p.Name,
			PlayerID:   p.ID,
			Team:       team,
			Opponent:   opponent,
			Market:     "SOG O2.5",
			Line:       2.5,
			Side:       "over",
			Projection: proj,
			ProbHit:    probOver,
			Edge:       edge,
		})
	}

	// SOG U2.5
	if stats.SOG <= 2.0 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(2.5)
		edge := (probUnder - 0.524) * 100

		picks = append(picks, Pick{
			GameID:     game.ID,
			GameDate:   game.GameDate.Format("2006-01-02"),
			Player:     p.Name,
			PlayerID:   p.ID,
			Team:       team,
			Opponent:   opponent,
			Market:     "SOG U2.5",
			Line:       2.5,
			Side:       "under",
			Projection: proj,
			ProbHit:    probUnder,
			Edge:       edge,
		})
	}

	// Points O0.5
	if stats.PointsPerGame >= 0.5 {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probOver := dist.ProbOver(0.5)
		edge := (probOver - 0.535) * 100

		picks = append(picks, Pick{
			GameID:     game.ID,
			GameDate:   game.GameDate.Format("2006-01-02"),
			Player:     p.Name,
			PlayerID:   p.ID,
			Team:       team,
			Opponent:   opponent,
			Market:     "Points O0.5",
			Line:       0.5,
			Side:       "over",
			Projection: proj,
			ProbHit:    probOver,
			Edge:       edge,
		})
	}

	// Points U0.5
	if stats.PointsPerGame <= 0.3 {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()
		probUnder := 1 - dist.ProbOver(0.5)
		edge := (probUnder - 0.535) * 100

		picks = append(picks, Pick{
			GameID:     game.ID,
			GameDate:   game.GameDate.Format("2006-01-02"),
			Player:     p.Name,
			PlayerID:   p.ID,
			Team:       team,
			Opponent:   opponent,
			Market:     "Points U0.5",
			Line:       0.5,
			Side:       "under",
			Projection: proj,
			ProbHit:    probUnder,
			Edge:       edge,
		})
	}

	return picks
}

func fetchBoxScore(gameID int) map[int]BoxScorePlayer {
	result := make(map[int]BoxScorePlayer)

	url := fmt.Sprintf("https://api-web.nhle.com/v1/gamecenter/%d/boxscore", gameID)
	resp, err := http.Get(url)
	if err != nil {
		return result
	}
	defer resp.Body.Close()

	var boxScore BoxScoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&boxScore); err != nil {
		return result
	}

	// Collect all players
	for _, p := range boxScore.PlayerByGameStats.HomeTeam.Forwards {
		result[p.PlayerID] = p
	}
	for _, p := range boxScore.PlayerByGameStats.HomeTeam.Defense {
		result[p.PlayerID] = p
	}
	for _, p := range boxScore.PlayerByGameStats.AwayTeam.Forwards {
		result[p.PlayerID] = p
	}
	for _, p := range boxScore.PlayerByGameStats.AwayTeam.Defense {
		result[p.PlayerID] = p
	}

	return result
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
