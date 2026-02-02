// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
)

// LoggedPick matches the structure in save_picks.go
type LoggedPick struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	GameDate  string    `json:"game_date"`

	Game     string `json:"game"`
	HomeTeam string `json:"home_team"`
	AwayTeam string `json:"away_team"`

	PlayerID   int    `json:"player_id"`
	PlayerName string `json:"player_name"`
	Team       string `json:"team"`
	Position   string `json:"position"`

	Market string  `json:"market"`
	Line   float64 `json:"line"`
	Side   string  `json:"side"`
	Odds   int     `json:"odds"`
	Book   string  `json:"bookmaker"`

	Reasoning struct {
		SeasonAvg   float64 `json:"season_avg"`
		RecentAvg   float64 `json:"recent_avg,omitempty"`
		GamesPlayed int     `json:"games_played"`
		TOI         float64 `json:"toi"`
		PPTOI       float64 `json:"pp_toi"`
		ShootingPct float64 `json:"shooting_pct,omitempty"`

		Projection  float64 `json:"projection"`
		ModelProb   float64 `json:"model_prob"`
		ImpliedProb float64 `json:"implied_prob"`
		Edge        float64 `json:"edge"`
		EdgeTier    string  `json:"edge_tier"`

		TeamTotal  float64 `json:"team_total"`
		OppDefense float64 `json:"opp_defense"`
		IsHome     bool    `json:"is_home"`
		IsPP1      bool    `json:"is_pp1"`

		PrimaryReason string   `json:"primary_reason"`
		Factors       []string `json:"factors"`
		Confidence    string   `json:"confidence"`
	} `json:"reasoning"`

	Result *struct {
		GradedAt   time.Time `json:"graded_at"`
		Actual     float64   `json:"actual"`
		Hit        bool      `json:"hit"`
		ProfitLoss float64   `json:"profit_loss"`
	} `json:"result,omitempty"`
}

type DailyLog struct {
	Date      string       `json:"date"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Picks     []LoggedPick `json:"picks"`
	Summary   *DailySummary `json:"summary,omitempty"`
}

type DailySummary struct {
	TotalPicks  int     `json:"total_picks"`
	GradedPicks int     `json:"graded_picks"`
	Hits        int     `json:"hits"`
	Misses      int     `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	Profit      float64 `json:"profit"`
	ROI         float64 `json:"roi"`
	ByMarket    map[string]MarketStats `json:"by_market"`
	ByTier      map[string]TierStats   `json:"by_tier"`
}

type MarketStats struct {
	Total   int     `json:"total"`
	Hits    int     `json:"hits"`
	HitRate float64 `json:"hit_rate"`
	Profit  float64 `json:"profit"`
}

type TierStats struct {
	Total   int     `json:"total"`
	Hits    int     `json:"hits"`
	HitRate float64 `json:"hit_rate"`
	Profit  float64 `json:"profit"`
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("              GRADE PICKS AGAINST BOX SCORES")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")

	// Get date from args or use yesterday
	var date string
	if len(os.Args) > 1 {
		date = os.Args[1]
	} else {
		date = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}
	fmt.Printf("Grading picks for: %s\n\n", date)

	// Load picks file
	filename := filepath.Join("data/picks", fmt.Sprintf("picks_%s.json", date))
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading picks file: %v\n", err)
		fmt.Println("Run save_picks.go first to generate picks")
		os.Exit(1)
	}

	var log DailyLog
	if err := json.Unmarshal(data, &log); err != nil {
		fmt.Printf("Error parsing picks file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Loaded %d picks\n", len(log.Picks))

	// Fetch box scores
	nhlClient := nhl.New()
	gameDate, _ := time.Parse("2006-01-02", date)

	fmt.Println("✓ Fetching box scores from NHL API...")

	// Get player stats from box scores
	boxScores, err := fetchBoxScores(nhlClient, gameDate)
	if err != nil {
		fmt.Printf("Error fetching box scores: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Got box scores for %d games\n\n", len(boxScores))

	// Grade each pick
	graded := 0
	hits := 0
	misses := 0
	var profit float64

	byMarket := make(map[string]*MarketStats)
	byTier := make(map[string]*TierStats)

	for i := range log.Picks {
		pick := &log.Picks[i]

		// Find player's actual stats in box scores
		actual, found := findPlayerStats(boxScores, pick.PlayerID, pick.Market)
		if !found {
			continue
		}

		// Grade
		hit := gradeResult(pick.Side, actual, pick.Line)
		pl := calculateProfitLoss(hit, pick.Odds)

		// Save result
		pick.Result = &struct {
			GradedAt   time.Time `json:"graded_at"`
			Actual     float64   `json:"actual"`
			Hit        bool      `json:"hit"`
			ProfitLoss float64   `json:"profit_loss"`
		}{
			GradedAt:   time.Now(),
			Actual:     actual,
			Hit:        hit,
			ProfitLoss: pl,
		}

		graded++
		if hit {
			hits++
		} else {
			misses++
		}
		profit += pl

		// Track by market
		marketKey := getMarketType(pick.Market)
		if byMarket[marketKey] == nil {
			byMarket[marketKey] = &MarketStats{}
		}
		byMarket[marketKey].Total++
		if hit {
			byMarket[marketKey].Hits++
		}
		byMarket[marketKey].Profit += pl

		// Track by tier
		tier := pick.Reasoning.EdgeTier
		if byTier[tier] == nil {
			byTier[tier] = &TierStats{}
		}
		byTier[tier].Total++
		if hit {
			byTier[tier].Hits++
		}
		byTier[tier].Profit += pl
	}

	// Calculate hit rates
	for k := range byMarket {
		if byMarket[k].Total > 0 {
			byMarket[k].HitRate = float64(byMarket[k].Hits) / float64(byMarket[k].Total)
		}
	}
	for k := range byTier {
		if byTier[k].Total > 0 {
			byTier[k].HitRate = float64(byTier[k].Hits) / float64(byTier[k].Total)
		}
	}

	// Convert to non-pointer maps
	marketStats := make(map[string]MarketStats)
	for k, v := range byMarket {
		marketStats[k] = *v
	}
	tierStats := make(map[string]TierStats)
	for k, v := range byTier {
		tierStats[k] = *v
	}

	// Create summary
	var hitRate float64
	if graded > 0 {
		hitRate = float64(hits) / float64(graded)
	}
	log.Summary = &DailySummary{
		TotalPicks:  len(log.Picks),
		GradedPicks: graded,
		Hits:        hits,
		Misses:      misses,
		HitRate:     hitRate,
		Profit:      profit,
		ROI:         profit / float64(graded) * 100,
		ByMarket:    marketStats,
		ByTier:      tierStats,
	}
	log.UpdatedAt = time.Now()

	// Save updated file
	updatedData, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filename, updatedData, 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              GRADING RESULTS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("  Total Picks:    %d\n", len(log.Picks))
	fmt.Printf("  Graded:         %d\n", graded)
	fmt.Printf("  Hits:           %d\n", hits)
	fmt.Printf("  Misses:         %d\n", misses)
	fmt.Printf("  Hit Rate:       %.1f%%\n", hitRate*100)
	fmt.Printf("  Profit/Loss:    %+.2f units\n", profit)
	fmt.Printf("  ROI:            %+.1f%%\n", profit/float64(graded)*100)
	fmt.Println()

	fmt.Println("  BY MARKET:")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Printf("  %-15s %6s %6s %8s %8s\n", "Market", "Total", "Hits", "Rate", "Profit")
	fmt.Println("  " + strings.Repeat("─", 50))
	for market, stats := range marketStats {
		fmt.Printf("  %-15s %6d %6d %7.1f%% %+7.2f\n",
			market, stats.Total, stats.Hits, stats.HitRate*100, stats.Profit)
	}
	fmt.Println()

	fmt.Println("  BY EDGE TIER:")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Printf("  %-15s %6s %6s %8s %8s\n", "Tier", "Total", "Hits", "Rate", "Profit")
	fmt.Println("  " + strings.Repeat("─", 50))
	for _, tier := range []string{"Tier 1", "Tier 2", "Tier 3"} {
		if stats, ok := tierStats[tier]; ok {
			fmt.Printf("  %-15s %6d %6d %7.1f%% %+7.2f\n",
				tier, stats.Total, stats.Hits, stats.HitRate*100, stats.Profit)
		}
	}
	fmt.Println()

	// Show some sample results
	fmt.Println("  SAMPLE RESULTS (first 15):")
	fmt.Println("  " + strings.Repeat("─", 90))
	fmt.Printf("  %-18s %-5s %-12s %5s %6s %6s %s\n",
		"Player", "Team", "Market", "Line", "Actual", "Edge", "Result")
	fmt.Println("  " + strings.Repeat("─", 90))

	count := 0
	for _, pick := range log.Picks {
		if pick.Result != nil && count < 15 {
			result := "MISS"
			if pick.Result.Hit {
				result = "HIT"
			}
			fmt.Printf("  %-18s %-5s %-12s %5.1f %6.1f %+5.1f%% %s\n",
				truncate(pick.PlayerName, 18),
				pick.Team,
				pick.Market,
				pick.Line,
				pick.Result.Actual,
				pick.Reasoning.Edge,
				result,
			)
			count++
		}
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Results saved to: %s\n", filename)
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

type BoxScore struct {
	GameID  int
	Players map[int]PlayerBoxStats
}

type PlayerBoxStats struct {
	PlayerID int
	Name     string
	SOG      int
	Points   int
	Goals    int
	Assists  int
	TOI      float64
}

func fetchBoxScores(client *nhl.Client, date time.Time) ([]BoxScore, error) {
	games, err := client.GetSchedule(date)
	if err != nil {
		return nil, err
	}

	var boxScores []BoxScore

	for _, game := range games {
		bs := BoxScore{
			GameID:  game.ID,
			Players: make(map[int]PlayerBoxStats),
		}

		// Fetch box score for this game
		gameStats, err := client.GetGameBoxScore(game.ID)
		if err != nil {
			continue
		}

		for _, p := range gameStats {
			bs.Players[p.PlayerID] = PlayerBoxStats{
				PlayerID: p.PlayerID,
				Name:     p.Name,
				SOG:      p.SOG,
				Points:   p.Points,
				Goals:    p.Goals,
				Assists:  p.Assists,
				TOI:      p.TOI,
			}
		}

		boxScores = append(boxScores, bs)
	}

	return boxScores, nil
}

func findPlayerStats(boxScores []BoxScore, playerID int, market string) (float64, bool) {
	for _, bs := range boxScores {
		if stats, ok := bs.Players[playerID]; ok {
			marketLower := strings.ToLower(market)
			if strings.Contains(marketLower, "sog") {
				return float64(stats.SOG), true
			}
			if strings.Contains(marketLower, "pts") || strings.Contains(marketLower, "point") {
				return float64(stats.Points), true
			}
			if strings.Contains(marketLower, "goal") {
				return float64(stats.Goals), true
			}
			if strings.Contains(marketLower, "assist") {
				return float64(stats.Assists), true
			}
		}
	}
	return 0, false
}

func gradeResult(side string, actual, line float64) bool {
	if side == "over" {
		return actual > line
	}
	return actual < line
}

func calculateProfitLoss(hit bool, odds int) float64 {
	if !hit {
		return -1.0
	}
	if odds > 0 {
		return float64(odds) / 100.0
	}
	return 100.0 / float64(-odds)
}

func getMarketType(market string) string {
	lower := strings.ToLower(market)
	if strings.Contains(lower, "sog") {
		if strings.Contains(lower, "o") {
			return "SOG Over"
		}
		return "SOG Under"
	}
	if strings.Contains(lower, "pts") || strings.Contains(lower, "point") {
		if strings.Contains(lower, "o") {
			return "Points Over"
		}
		return "Points Under"
	}
	return "Other"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
