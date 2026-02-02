package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// RealisticPick represents a pick with market-realistic lines
type RealisticPick struct {
	GameID     int
	PlayerID   int
	Market     string
	Line       float64
	Expected   float64
	Actual     float64
	ProbOver   float64
	ProbUnder  float64
	EdgeOver   float64
	EdgeUnder  float64
	TOI        float64
	Side       string // "over" or "under"
	Edge       float64
	Won        bool
}

// getRealisticLine returns the line a sportsbook would likely offer
// based on the player's expected production
func getRealisticLine(market string, expected float64) float64 {
	switch market {
	case "Points":
		// Points: 0.5 for most players, 1.5 for high scorers (expected > 1.0)
		if expected >= 1.0 {
			return 1.5
		}
		return 0.5

	case "Goals":
		// Goals: Almost always 0.5
		return 0.5

	case "Assists":
		// Assists: 0.5 for most, 1.5 for elite playmakers (expected > 0.8)
		if expected >= 0.8 {
			return 1.5
		}
		return 0.5

	case "SOG":
		// SOG: Round to nearest 0.5, minimum 1.5
		// expected 1.8 -> 1.5, expected 2.3 -> 2.5, expected 3.1 -> 3.5
		rounded := math.Round(expected*2) / 2
		if rounded < 1.5 {
			return 1.5
		}
		// Lines typically offered: 1.5, 2.5, 3.5, 4.5
		if rounded <= 2.0 {
			return 1.5
		} else if rounded <= 3.0 {
			return 2.5
		} else if rounded <= 4.0 {
			return 3.5
		}
		return 4.5

	case "Hits":
		// Hits: Round to nearest 0.5, minimum 0.5
		rounded := math.Round(expected*2) / 2
		if rounded < 0.5 {
			return 0.5
		}
		// Common lines: 0.5, 1.5, 2.5, 3.5
		if rounded <= 1.0 {
			return 0.5
		} else if rounded <= 2.0 {
			return 1.5
		} else if rounded <= 3.0 {
			return 2.5
		}
		return 3.5

	case "Blocks":
		// Blocks: Round to nearest 0.5, minimum 0.5
		rounded := math.Round(expected*2) / 2
		if rounded < 0.5 {
			return 0.5
		}
		// Common lines: 0.5, 1.5, 2.5
		if rounded <= 1.0 {
			return 0.5
		} else if rounded <= 2.0 {
			return 1.5
		}
		return 2.5
	}
	return 0.5
}

// RunRealisticAnalysis runs analysis with market-realistic lines
func RunRealisticAnalysis() {
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("           REALISTIC MARKET ANALYSIS (Actual Betting Lines)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Key change: Lines are now set based on player's expected production,")
	fmt.Println("matching how sportsbooks actually set lines.")
	fmt.Println()

	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -6)

	fmt.Printf("Analysis Period: %s to %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	client := nhl.New()

	// Collect all picks with realistic lines
	var allPicks []RealisticPick

	// League averages (calibrated)
	leagueAvgSOG := 1.4
	leagueAvgGoals := 0.17
	leagueAvgAssists := 0.28
	leagueAvgPoints := 0.43
	leagueAvgHits := 1.0
	leagueAvgBlocks := 0.7

	gamesAnalyzed := 0
	playersAnalyzed := 0

	currentDate := startDate
	for !currentDate.After(endDate) {
		games, err := client.GetSchedule(currentDate)
		if err != nil {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		for _, game := range games {
			if game.State != types.GameStateFinal {
				continue
			}

			boxscore, err := client.GetBoxscore(game.ID)
			if err != nil {
				continue
			}

			gamesAnalyzed++

			for playerID, actual := range boxscore.PlayerStats {
				if actual.TOI < 10 { // Minimum TOI for having props available
					continue
				}

				playersAnalyzed++
				toiFactor := actual.TOI / 16.0

				// Calculate expected values
				expSOG := leagueAvgSOG * toiFactor
				expGoals := leagueAvgGoals * toiFactor
				expAssists := leagueAvgAssists * toiFactor
				expPoints := leagueAvgPoints * toiFactor
				expHits := leagueAvgHits * toiFactor
				expBlocks := leagueAvgBlocks * toiFactor

				// Boost for PP time
				if actual.PPTOI >= 2.0 {
					ppFactor := 1.0 + (actual.PPTOI / 10.0)
					expPoints *= ppFactor
					expGoals *= ppFactor
					expAssists *= ppFactor
				}

				// Evaluate each market at its REALISTIC line
				picks := evaluateRealisticPicks(playerID, game.ID, actual,
					expSOG, expGoals, expAssists, expPoints, expHits, expBlocks)
				allPicks = append(allPicks, picks...)
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	fmt.Printf("Games: %d | Players: %d | Total Picks Evaluated: %d\n\n",
		gamesAnalyzed, playersAnalyzed, len(allPicks))

	// Analyze by market
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    MARKET PERFORMANCE (Realistic Lines)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market/Line/Side    Picks    Wins  Losses  HitRate  AvgEdge      ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Group by market, line, side
	type MarketKey struct {
		Market string
		Line   float64
		Side   string
	}
	marketStats := make(map[MarketKey]*struct {
		Total   int
		Wins    int
		Losses  int
		AvgEdge float64
	})

	for _, p := range allPicks {
		key := MarketKey{p.Market, p.Line, p.Side}
		if marketStats[key] == nil {
			marketStats[key] = &struct {
				Total   int
				Wins    int
				Losses  int
				AvgEdge float64
			}{}
		}
		stats := marketStats[key]
		stats.Total++
		stats.AvgEdge += p.Edge
		if p.Won {
			stats.Wins++
		} else {
			stats.Losses++
		}
	}

	// Sort and print
	var keys []MarketKey
	for k := range marketStats {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Market != keys[j].Market {
			return keys[i].Market < keys[j].Market
		}
		if keys[i].Line != keys[j].Line {
			return keys[i].Line < keys[j].Line
		}
		return keys[i].Side < keys[j].Side
	})

	for _, key := range keys {
		stats := marketStats[key]
		if stats.Total > 0 {
			hitRate := float64(stats.Wins) / float64(stats.Wins+stats.Losses) * 100
			avgEdge := stats.AvgEdge / float64(stats.Total)
			roi := (float64(stats.Wins)*0.909 - float64(stats.Losses)) / float64(stats.Total) * 100

			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}

			fmt.Printf("%-8s O%.1f %-5s %6d %7d %7d %7.1f%% %7.1f%% %10s\n",
				key.Market, key.Line, key.Side, stats.Total, stats.Wins, stats.Losses,
				hitRate, avgEdge, roiStr)
		}
	}

	// Edge-based analysis with realistic lines
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    EDGE-BASED ROI (Realistic Lines)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Edge Range      Picks    Wins  Losses  HitRate      ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	edgeBuckets := []struct {
		name string
		min  float64
		max  float64
	}{
		{"<-10%", -100, -10},
		{"-10% to -5%", -10, -5},
		{"-5% to 0%", -5, 0},
		{"0% to 2%", 0, 2},
		{"2% to 5%", 2, 5},
		{"5% to 10%", 5, 10},
		{">10%", 10, 100},
	}

	for _, bucket := range edgeBuckets {
		var total, wins, losses int
		for _, p := range allPicks {
			if p.Edge >= bucket.min && p.Edge < bucket.max {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-14s %6d %7d %7d %7.1f%% %10s\n",
				bucket.name, total, wins, losses, hitRate, roiStr)
		}
	}

	// Focus on Points O0.5 - the "bread and butter"
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("              POINTS OVER 0.5 DEEP DIVE (Bread & Butter)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	var pointsPicks []RealisticPick
	for _, p := range allPicks {
		if p.Market == "Points" && p.Line == 0.5 && p.Side == "over" {
			pointsPicks = append(pointsPicks, p)
		}
	}

	// By TOI
	fmt.Println("\nPoints O0.5 by TOI:")
	fmt.Println("TOI Range       Picks    Wins  Losses  HitRate  AvgEdge      ROI")
	toiBuckets := []struct {
		name string
		min  float64
		max  float64
	}{
		{"10-14min", 10, 14},
		{"14-18min", 14, 18},
		{"18-22min", 18, 22},
		{">22min", 22, 100},
	}

	for _, bucket := range toiBuckets {
		var total, wins, losses int
		var totalEdge float64
		for _, p := range pointsPicks {
			if p.TOI >= bucket.min && p.TOI < bucket.max {
				total++
				totalEdge += p.Edge
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			avgEdge := totalEdge / float64(total)
			roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-14s %6d %7d %7d %7.1f%% %7.1f%% %10s\n",
				bucket.name, total, wins, losses, hitRate, avgEdge, roiStr)
		}
	}

	// By edge for Points O0.5
	fmt.Println("\nPoints O0.5 by Edge:")
	fmt.Println("Edge Range      Picks    Wins  Losses  HitRate      ROI")
	for _, bucket := range edgeBuckets {
		var total, wins, losses int
		for _, p := range pointsPicks {
			if p.Edge >= bucket.min && p.Edge < bucket.max {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-14s %6d %7d %7d %7.1f%% %10s\n",
				bucket.name, total, wins, losses, hitRate, roiStr)
		}
	}

	// Optimal filter for Points O0.5
	fmt.Println("\nOptimal Filters for Points O0.5:")
	fmt.Println("Filter                  Picks    Wins  Losses  HitRate      ROI")

	filters := []struct {
		name string
		fn   func(p RealisticPick) bool
	}{
		{"All picks", func(p RealisticPick) bool { return true }},
		{"TOI >= 18min", func(p RealisticPick) bool { return p.TOI >= 18 }},
		{"Edge >= 0%", func(p RealisticPick) bool { return p.Edge >= 0 }},
		{"Edge >= 5%", func(p RealisticPick) bool { return p.Edge >= 5 }},
		{"TOI>=18 + Edge>=0%", func(p RealisticPick) bool { return p.TOI >= 18 && p.Edge >= 0 }},
		{"TOI>=18 + Edge>=5%", func(p RealisticPick) bool { return p.TOI >= 18 && p.Edge >= 5 }},
		{"TOI>=20 + Edge>=5%", func(p RealisticPick) bool { return p.TOI >= 20 && p.Edge >= 5 }},
	}

	for _, f := range filters {
		var total, wins, losses int
		for _, p := range pointsPicks {
			if f.fn(p) {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-22s %6d %7d %7d %7.1f%% %10s\n",
				f.name, total, wins, losses, hitRate, roiStr)
		}
	}

	// Also analyze Points Under 0.5 opportunities
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("              POINTS UNDER 0.5 ANALYSIS (Fade Opportunities)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	var pointsUnderPicks []RealisticPick
	for _, p := range allPicks {
		if p.Market == "Points" && p.Line == 0.5 && p.Side == "under" {
			pointsUnderPicks = append(pointsUnderPicks, p)
		}
	}

	fmt.Println("\nPoints U0.5 by Edge:")
	fmt.Println("Edge Range      Picks    Wins  Losses  HitRate      ROI")
	for _, bucket := range edgeBuckets {
		var total, wins, losses int
		for _, p := range pointsUnderPicks {
			if p.Edge >= bucket.min && p.Edge < bucket.max {
				total++
				if p.Won {
					wins++
				} else {
					losses++
				}
			}
		}
		if total > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			roi := (float64(wins)*0.909 - float64(losses)) / float64(total) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-14s %6d %7d %7d %7.1f%% %10s\n",
				bucket.name, total, wins, losses, hitRate, roiStr)
		}
	}

	// Final summary
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                         REALISTIC PERFORMANCE SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	// Best opportunities
	fmt.Println("\nBest Opportunities (ROI > 0):")
	type OpportunityKey struct {
		Market string
		Line   float64
		Side   string
	}
	var bestOpps []struct {
		key     OpportunityKey
		total   int
		hitRate float64
		roi     float64
	}

	for key, stats := range marketStats {
		if stats.Total >= 50 {
			hitRate := float64(stats.Wins) / float64(stats.Wins+stats.Losses) * 100
			roi := (float64(stats.Wins)*0.909 - float64(stats.Losses)) / float64(stats.Total) * 100
			if roi > 0 {
				bestOpps = append(bestOpps, struct {
					key     OpportunityKey
					total   int
					hitRate float64
					roi     float64
				}{OpportunityKey{key.Market, key.Line, key.Side}, stats.Total, hitRate, roi})
			}
		}
	}

	sort.Slice(bestOpps, func(i, j int) bool {
		return bestOpps[i].roi > bestOpps[j].roi
	})

	for _, opp := range bestOpps {
		fmt.Printf("  • %s %s %.1f: %d picks, %.1f%% hit rate, +%.1f%% ROI\n",
			opp.key.Market, opp.key.Side, opp.key.Line, opp.total, opp.hitRate, opp.roi)
	}

	if len(bestOpps) == 0 {
		fmt.Println("  No markets showing positive ROI at current calibration")
	}

	fmt.Println()
}

func evaluateRealisticPicks(playerID, gameID int, actual types.BoxscorePlayerStats,
	expSOG, expGoals, expAssists, expPoints, expHits, expBlocks float64) []RealisticPick {

	var picks []RealisticPick
	impliedProb := 0.524 // -110 juice

	// Points - THE CORE MARKET
	{
		line := getRealisticLine("Points", expPoints)
		dist := distributions.NewPoissonBinomialMixture(expPoints*2, expPoints*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		// Over
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Points", Line: line,
			Expected: expPoints, Actual: float64(actual.Points),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Points) > line,
		})
		// Under
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Points", Line: line,
			Expected: expPoints, Actual: float64(actual.Points),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Points) < line,
		})
	}

	// Goals
	{
		line := getRealisticLine("Goals", expGoals)
		dist := distributions.NewPoissonBinomialMixture(expGoals*2, expGoals*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Goals", Line: line,
			Expected: expGoals, Actual: float64(actual.Goals),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Goals) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Goals", Line: line,
			Expected: expGoals, Actual: float64(actual.Goals),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Goals) < line,
		})
	}

	// Assists
	{
		line := getRealisticLine("Assists", expAssists)
		dist := distributions.NewPoissonBinomialMixture(expAssists*2, expAssists*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Assists", Line: line,
			Expected: expAssists, Actual: float64(actual.Assists),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Assists) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Assists", Line: line,
			Expected: expAssists, Actual: float64(actual.Assists),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Assists) < line,
		})
	}

	// SOG
	{
		line := getRealisticLine("SOG", expSOG)
		dist := distributions.NewHierarchicalPoissonGamma(expSOG, expSOG)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "SOG", Line: line,
			Expected: expSOG, Actual: float64(actual.SOG),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.SOG) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "SOG", Line: line,
			Expected: expSOG, Actual: float64(actual.SOG),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.SOG) < line,
		})
	}

	// Hits
	{
		line := getRealisticLine("Hits", expHits)
		dist := distributions.NewZeroInflatedNegBin(expHits, expHits)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Hits", Line: line,
			Expected: expHits, Actual: float64(actual.Hits),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Hits) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Hits", Line: line,
			Expected: expHits, Actual: float64(actual.Hits),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Hits) < line,
		})
	}

	// Blocks
	{
		line := getRealisticLine("Blocks", expBlocks)
		dist := distributions.NewZeroInflatedNegBin(expBlocks, expBlocks)
		probOver := dist.ProbOver(line)
		probUnder := dist.ProbUnder(line)
		edgeOver := (probOver - impliedProb) * 100
		edgeUnder := (probUnder - impliedProb) * 100

		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Blocks", Line: line,
			Expected: expBlocks, Actual: float64(actual.Blocks),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "over", Edge: edgeOver,
			Won: float64(actual.Blocks) > line,
		})
		picks = append(picks, RealisticPick{
			GameID: gameID, PlayerID: playerID, Market: "Blocks", Line: line,
			Expected: expBlocks, Actual: float64(actual.Blocks),
			ProbOver: probOver, ProbUnder: probUnder,
			EdgeOver: edgeOver, EdgeUnder: edgeUnder,
			TOI: actual.TOI, Side: "under", Edge: edgeUnder,
			Won: float64(actual.Blocks) < line,
		})
	}

	return picks
}
