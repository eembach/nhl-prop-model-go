// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/api/odds"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// LoggedPick contains everything needed for analysis
type LoggedPick struct {
	// Identification
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	GameDate  string    `json:"game_date"`

	// Game context
	Game       string `json:"game"`
	HomeTeam   string `json:"home_team"`
	AwayTeam   string `json:"away_team"`

	// Player
	PlayerID   int     `json:"player_id"`
	PlayerName string  `json:"player_name"`
	Team       string  `json:"team"`
	Position   string  `json:"position"`

	// Bet
	Market string  `json:"market"`
	Line   float64 `json:"line"`
	Side   string  `json:"side"`
	Odds   int     `json:"odds"`
	Book   string  `json:"bookmaker"`

	// Model Reasoning
	Reasoning struct {
		// Player stats at time of pick
		SeasonAvg   float64 `json:"season_avg"`
		RecentAvg   float64 `json:"recent_avg,omitempty"`
		GamesPlayed int     `json:"games_played"`
		TOI         float64 `json:"toi"`
		PPTOI       float64 `json:"pp_toi"`
		ShootingPct float64 `json:"shooting_pct,omitempty"`

		// Model calculation
		Projection  float64 `json:"projection"`
		ModelProb   float64 `json:"model_prob"`
		ImpliedProb float64 `json:"implied_prob"`
		Edge        float64 `json:"edge"`
		EdgeTier    string  `json:"edge_tier"`

		// Context factors
		TeamTotal     float64 `json:"team_total"`
		OppDefense    float64 `json:"opp_defense"`
		IsHome        bool    `json:"is_home"`
		IsPP1         bool    `json:"is_pp1"`

		// Explanation
		PrimaryReason string   `json:"primary_reason"`
		Factors       []string `json:"factors"`
		Confidence    string   `json:"confidence"`
	} `json:"reasoning"`

	// Result (filled in later)
	Result *struct {
		GradedAt   time.Time `json:"graded_at"`
		Actual     float64   `json:"actual"`
		Hit        bool      `json:"hit"`
		ProfitLoss float64   `json:"profit_loss"`
	} `json:"result,omitempty"`
}

// DailyLog is a day's worth of picks
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
	fmt.Println("              SAVE TODAY'S PICKS WITH FULL REASONING")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")

	today := time.Now().Format("2006-01-02")
	fmt.Printf("Date: %s\n\n", today)

	apiKey := os.Getenv("ODDS_API_KEY")
	if apiKey == "" {
		fmt.Println("⚠️  ODDS_API_KEY not set - will save picks without live odds")
	}

	// Setup
	nhlClient := nhl.New()
	factory := distributions.NewFactory()
	season := getCurrentSeason(time.Now())

	var oddsClient *odds.Client
	var allProps map[string][]odds.PlayerProp

	if apiKey != "" {
		oddsClient = odds.NewWithKey(apiKey)
		fmt.Println("✓ Fetching live odds...")
		var err error
		allProps, err = oddsClient.GetAllPlayerProps()
		if err != nil {
			fmt.Printf("⚠️  Odds fetch error: %v\n", err)
		} else {
			fmt.Printf("✓ Got props for %d games\n", len(allProps))
		}
	}

	// Get today's games
	games, err := nhlClient.GetSchedule(time.Now())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Found %d games today\n\n", len(games))

	// Get team stats
	teamStats, _ := nhlClient.GetStandings()
	teamStatsMap := make(map[string]types.TeamStats)
	for _, ts := range teamStats {
		teamStatsMap[ts.TeamAbbr] = ts
	}

	var picks []LoggedPick
	pickCount := 0

	for _, game := range games {
		homeTeam := game.HomeTeamAbbr
		awayTeam := game.AwayTeamAbbr

		homeStats := teamStatsMap[homeTeam]
		awayStats := teamStatsMap[awayTeam]
		expectedTotal := homeStats.GFPerGame + awayStats.GFPerGame

		gameStr := fmt.Sprintf("%s@%s", awayTeam, homeTeam)
		fmt.Printf("Processing %s...\n", gameStr)

		// Find props for this game
		var gameProps []odds.PlayerProp
		if allProps != nil {
			for gameKey, props := range allProps {
				if strings.Contains(gameKey, homeTeam) || strings.Contains(gameKey, awayTeam) ||
					strings.Contains(gameKey, teamAbbrevToFull(homeTeam)) || strings.Contains(gameKey, teamAbbrevToFull(awayTeam)) {
					gameProps = append(gameProps, props...)
				}
			}
		}

		processTeam := func(roster []types.Player, team string, isHome bool, oppStats types.TeamStats) {
			for _, p := range roster {
				if p.Position == types.PositionGoalie {
					continue
				}

				pStats, err := nhlClient.GetPlayerStats(p.ID, season)
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

				pos := "F"
				if p.Position == types.PositionDefense {
					pos = "D"
				}

				// Generate picks for this player
				playerPicks := generatePicksForPlayer(p, pStats, factory, ctx, team, homeTeam, awayTeam, gameStr, pos, gameProps, today)
				picks = append(picks, playerPicks...)
				pickCount += len(playerPicks)
			}
		}

		homeRoster, _ := nhlClient.GetRoster(homeTeam)
		awayRoster, _ := nhlClient.GetRoster(awayTeam)

		processTeam(homeRoster, homeTeam, true, awayStats)
		processTeam(awayRoster, awayTeam, false, homeStats)
	}

	fmt.Printf("\n✓ Generated %d picks with edge >= 3%%\n", pickCount)

	// Sort by edge
	sort.Slice(picks, func(i, j int) bool {
		return picks[i].Reasoning.Edge > picks[j].Reasoning.Edge
	})

	// Create log
	log := DailyLog{
		Date:      today,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Picks:     picks,
	}

	// Save to file
	dataDir := "data/picks"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	filename := filepath.Join(dataDir, fmt.Sprintf("picks_%s.json", today))
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Saved to %s\n\n", filename)

	// Show summary
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              PICKS SAVED FOR GRADING")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Count by tier
	tier1, tier2, tier3 := 0, 0, 0
	for _, p := range picks {
		switch p.Reasoning.EdgeTier {
		case "Tier 1":
			tier1++
		case "Tier 2":
			tier2++
		case "Tier 3":
			tier3++
		}
	}

	fmt.Printf("  Tier 1 (Edge >= 10%%):  %d picks\n", tier1)
	fmt.Printf("  Tier 2 (Edge 5-10%%):   %d picks\n", tier2)
	fmt.Printf("  Tier 3 (Edge 3-5%%):    %d picks\n", tier3)
	fmt.Printf("  Total:                 %d picks\n", len(picks))
	fmt.Println()

	// Show top 10
	fmt.Println("  TOP 10 PICKS:")
	fmt.Println("  " + strings.Repeat("─", 85))
	fmt.Printf("  %-20s %-5s %-12s %6s %6s %7s %s\n",
		"Player", "Team", "Market", "Model%", "Impl%", "Edge", "Reason")
	fmt.Println("  " + strings.Repeat("─", 85))

	for i := 0; i < 10 && i < len(picks); i++ {
		p := picks[i]
		reason := p.Reasoning.PrimaryReason
		if len(reason) > 25 {
			reason = reason[:22] + "..."
		}
		fmt.Printf("  %-20s %-5s %-12s %5.1f%% %5.1f%% %+6.1f%% %s\n",
			truncate(p.PlayerName, 20),
			p.Team,
			p.Market,
			p.Reasoning.ModelProb*100,
			p.Reasoning.ImpliedProb*100,
			p.Reasoning.Edge,
			reason,
		)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("  To grade tomorrow: go run cmd/picks/grade_picks.go %s\n", today)
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════════")
}

func generatePicksForPlayer(p types.Player, stats *types.PlayerStats, factory *distributions.Factory, ctx distributions.DistributionContext, team, homeTeam, awayTeam, game, pos string, gameProps []odds.PlayerProp, date string) []LoggedPick {
	var picks []LoggedPick

	// Find props for this player
	sogProp := findProp(gameProps, p.Name, "shots")
	ptsProp := findProp(gameProps, p.Name, "points")

	// Points Under (best performer in backtest)
	if stats.PointsPerGame <= 0.9 {
		dist := factory.CreateDistribution(types.PropMarketPointsOver, stats, ctx)
		proj := dist.Mean()

		var line float64 = 0.5
		var impliedProb float64 = 0.535 // Default -115 juice
		var liveOdds int = -115
		var book string = "default"

		if ptsProp != nil {
			line = ptsProp.Line
			impliedProb = americanToProb(ptsProp.UnderPrice)
			liveOdds = ptsProp.UnderPrice
			book = ptsProp.Bookmaker
		}

		probUnder := 1 - dist.ProbOver(line)
		edge := (probUnder - impliedProb) * 100

		if edge >= 3 {
			pick := LoggedPick{
				ID:         fmt.Sprintf("%s_%s_pts_u%.1f", date, strings.ReplaceAll(p.Name, " ", "_"), line),
				Timestamp:  time.Now(),
				GameDate:   date,
				Game:       game,
				HomeTeam:   homeTeam,
				AwayTeam:   awayTeam,
				PlayerID:   p.ID,
				PlayerName: p.Name,
				Team:       team,
				Position:   pos,
				Market:     fmt.Sprintf("Pts U%.1f", line),
				Line:       line,
				Side:       "under",
				Odds:       liveOdds,
				Book:       book,
			}

			pick.Reasoning.SeasonAvg = stats.PointsPerGame
			pick.Reasoning.GamesPlayed = stats.GamesPlayed
			pick.Reasoning.TOI = stats.TOI
			pick.Reasoning.PPTOI = stats.PPTOI
			pick.Reasoning.ShootingPct = stats.ShootingPct
			pick.Reasoning.Projection = proj
			pick.Reasoning.ModelProb = probUnder
			pick.Reasoning.ImpliedProb = impliedProb
			pick.Reasoning.Edge = edge
			pick.Reasoning.EdgeTier = getEdgeTier(edge)
			pick.Reasoning.TeamTotal = ctx.TeamTotal
			pick.Reasoning.OppDefense = ctx.OppGA60
			pick.Reasoning.IsHome = ctx.HomeGame
			pick.Reasoning.IsPP1 = ctx.PP1Unit
			pick.Reasoning.PrimaryReason = generateReason("under", "points", stats, proj, edge)
			pick.Reasoning.Factors = generateFactors(stats, ctx, "points", "under")
			pick.Reasoning.Confidence = getConfidence(edge)

			picks = append(picks, pick)
		}
	}

	// SOG Under
	if stats.SOG <= 2.0 && stats.TOI >= 14 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()

		var line float64 = 2.5
		var impliedProb float64 = 0.524
		var liveOdds int = -110
		var book string = "default"

		if sogProp != nil {
			line = sogProp.Line
			impliedProb = americanToProb(sogProp.UnderPrice)
			liveOdds = sogProp.UnderPrice
			book = sogProp.Bookmaker
		}

		probUnder := 1 - dist.ProbOver(line)
		edge := (probUnder - impliedProb) * 100

		if edge >= 3 {
			pick := LoggedPick{
				ID:         fmt.Sprintf("%s_%s_sog_u%.1f", date, strings.ReplaceAll(p.Name, " ", "_"), line),
				Timestamp:  time.Now(),
				GameDate:   date,
				Game:       game,
				HomeTeam:   homeTeam,
				AwayTeam:   awayTeam,
				PlayerID:   p.ID,
				PlayerName: p.Name,
				Team:       team,
				Position:   pos,
				Market:     fmt.Sprintf("SOG U%.1f", line),
				Line:       line,
				Side:       "under",
				Odds:       liveOdds,
				Book:       book,
			}

			pick.Reasoning.SeasonAvg = stats.SOG
			pick.Reasoning.GamesPlayed = stats.GamesPlayed
			pick.Reasoning.TOI = stats.TOI
			pick.Reasoning.PPTOI = stats.PPTOI
			pick.Reasoning.Projection = proj
			pick.Reasoning.ModelProb = probUnder
			pick.Reasoning.ImpliedProb = impliedProb
			pick.Reasoning.Edge = edge
			pick.Reasoning.EdgeTier = getEdgeTier(edge)
			pick.Reasoning.TeamTotal = ctx.TeamTotal
			pick.Reasoning.OppDefense = ctx.OppGA60
			pick.Reasoning.IsHome = ctx.HomeGame
			pick.Reasoning.IsPP1 = ctx.PP1Unit
			pick.Reasoning.PrimaryReason = generateReason("under", "sog", stats, proj, edge)
			pick.Reasoning.Factors = generateFactors(stats, ctx, "sog", "under")
			pick.Reasoning.Confidence = getConfidence(edge)

			picks = append(picks, pick)
		}
	}

	// SOG Over
	if stats.SOG >= 2.5 {
		dist := factory.CreateDistribution(types.PropMarketSOGOver, stats, ctx)
		proj := dist.Mean()

		var line float64 = 2.5
		var impliedProb float64 = 0.524
		var liveOdds int = -110
		var book string = "default"

		if sogProp != nil {
			line = sogProp.Line
			impliedProb = americanToProb(sogProp.OverPrice)
			liveOdds = sogProp.OverPrice
			book = sogProp.Bookmaker
		}

		probOver := dist.ProbOver(line)
		edge := (probOver - impliedProb) * 100

		if edge >= 3 {
			pick := LoggedPick{
				ID:         fmt.Sprintf("%s_%s_sog_o%.1f", date, strings.ReplaceAll(p.Name, " ", "_"), line),
				Timestamp:  time.Now(),
				GameDate:   date,
				Game:       game,
				HomeTeam:   homeTeam,
				AwayTeam:   awayTeam,
				PlayerID:   p.ID,
				PlayerName: p.Name,
				Team:       team,
				Position:   pos,
				Market:     fmt.Sprintf("SOG O%.1f", line),
				Line:       line,
				Side:       "over",
				Odds:       liveOdds,
				Book:       book,
			}

			pick.Reasoning.SeasonAvg = stats.SOG
			pick.Reasoning.GamesPlayed = stats.GamesPlayed
			pick.Reasoning.TOI = stats.TOI
			pick.Reasoning.PPTOI = stats.PPTOI
			pick.Reasoning.Projection = proj
			pick.Reasoning.ModelProb = probOver
			pick.Reasoning.ImpliedProb = impliedProb
			pick.Reasoning.Edge = edge
			pick.Reasoning.EdgeTier = getEdgeTier(edge)
			pick.Reasoning.TeamTotal = ctx.TeamTotal
			pick.Reasoning.OppDefense = ctx.OppGA60
			pick.Reasoning.IsHome = ctx.HomeGame
			pick.Reasoning.IsPP1 = ctx.PP1Unit
			pick.Reasoning.PrimaryReason = generateReason("over", "sog", stats, proj, edge)
			pick.Reasoning.Factors = generateFactors(stats, ctx, "sog", "over")
			pick.Reasoning.Confidence = getConfidence(edge)

			picks = append(picks, pick)
		}
	}

	return picks
}

func generateReason(side, market string, stats *types.PlayerStats, proj, edge float64) string {
	if side == "under" {
		if market == "points" {
			if stats.PointsPerGame < 0.3 {
				return fmt.Sprintf("Low producer (%.2f PPG), model proj %.2f", stats.PointsPerGame, proj)
			}
			return fmt.Sprintf("Conservative proj %.2f vs %.2f avg, market overpricing", proj, stats.PointsPerGame)
		}
		return fmt.Sprintf("Low volume (%.1f SOG/G), proj %.2f", stats.SOG, proj)
	}
	if market == "sog" {
		return fmt.Sprintf("High volume shooter (%.1f SOG/G), proj %.2f", stats.SOG, proj)
	}
	return fmt.Sprintf("Model proj %.2f vs %.2f avg", proj, stats.PointsPerGame)
}

func generateFactors(stats *types.PlayerStats, ctx distributions.DistributionContext, market, side string) []string {
	var factors []string

	if stats.TOI >= 18 {
		factors = append(factors, "High TOI (top-6/4)")
	} else if stats.TOI < 14 {
		factors = append(factors, "Limited TOI")
	}

	if ctx.PP1Unit {
		factors = append(factors, "PP1 unit")
	} else if stats.PPTOI < 0.5 {
		factors = append(factors, "No PP time")
	}

	if ctx.OppGA60 > 3.2 {
		factors = append(factors, "Weak opponent defense")
	} else if ctx.OppGA60 < 2.8 {
		factors = append(factors, "Strong opponent defense")
	}

	if ctx.HomeGame {
		factors = append(factors, "Home game")
	}

	return factors
}

func getEdgeTier(edge float64) string {
	if edge >= 10 {
		return "Tier 1"
	}
	if edge >= 5 {
		return "Tier 2"
	}
	return "Tier 3"
}

func getConfidence(edge float64) string {
	if edge >= 15 {
		return "Very High"
	}
	if edge >= 10 {
		return "High"
	}
	if edge >= 5 {
		return "Medium"
	}
	return "Low"
}

func findProp(props []odds.PlayerProp, playerName string, marketType string) *odds.PlayerProp {
	if props == nil {
		return nil
	}
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

func teamAbbrevToFull(abbr string) string {
	names := map[string]string{
		"FLA": "Florida Panthers", "BUF": "Buffalo Sabres",
		"PIT": "Pittsburgh Penguins", "OTT": "Ottawa Senators",
		"WSH": "Washington Capitals", "NYI": "New York Islanders",
		"MIN": "Minnesota Wild", "MTL": "Montreal Canadiens",
		"NSH": "Nashville Predators", "STL": "St Louis Blues",
		"CHI": "Chicago Blackhawks", "SJS": "San Jose Sharks",
		"DAL": "Dallas Stars", "WPG": "Winnipeg Jets",
		"COL": "Colorado Avalanche", "DET": "Detroit Red Wings",
		"UTA": "Utah Hockey Club", "VAN": "Vancouver Canucks",
		"CGY": "Calgary Flames", "TOR": "Toronto Maple Leafs",
	}
	return names[abbr]
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
