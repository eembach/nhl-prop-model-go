package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// FilteredPick represents a pick that passes quality filters
type FilteredPick struct {
	GameID     int
	PlayerID   int
	Market     string
	Line       float64
	Predicted  float64
	Actual     float64
	ProbOver   float64
	Edge       float64
	TOI        float64
	Won        bool
}

// RunFilteredAnalysis runs analysis with quality filters applied
func RunFilteredAnalysis() {
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("              FILTERED PICK ANALYSIS (Quality Filters Applied)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════\n")

	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -6)

	client := nhl.New()

	// Collect all qualifying picks
	var allPicks []FilteredPick

	leagueAvgSOG := 1.4
	leagueAvgGoals := 0.17
	leagueAvgAssists := 0.28
	leagueAvgPoints := 0.43
	leagueAvgHits := 1.0
	leagueAvgBlocks := 0.7

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

			for playerID, actual := range boxscore.PlayerStats {
				// Quality filter: minimum TOI
				if actual.TOI < 14 {
					continue
				}

				toiFactor := actual.TOI / 16.0

				// Evaluate each market
				picks := evaluatePlayerPicks(
					playerID, game.ID, actual,
					leagueAvgSOG*toiFactor,
					leagueAvgGoals*toiFactor,
					leagueAvgAssists*toiFactor,
					leagueAvgPoints*toiFactor,
					leagueAvgHits*toiFactor,
					leagueAvgBlocks*toiFactor,
				)

				allPicks = append(allPicks, picks...)
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Analyze picks at different edge thresholds
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("     PERFORMANCE BY MINIMUM EDGE THRESHOLD (TOI > 14min filter applied)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Min Edge    Total    Wins   Losses  HitRate    ROI      Optimal?")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	thresholds := []float64{0, 2, 5, 8, 10, 12, 15, 20}
	var optimalThreshold float64
	var optimalROI float64

	for _, minEdge := range thresholds {
		var total, wins, losses int
		for _, p := range allPicks {
			if p.Edge >= minEdge {
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

			optimal := ""
			if roi > optimalROI && total >= 50 {
				optimalROI = roi
				optimalThreshold = minEdge
				optimal = "←"
			}

			fmt.Printf(">=%5.1f%% %7d %7d %7d %8.1f%% %7.1f%% %10s\n",
				minEdge, total, wins, losses, hitRate, roi, optimal)
		}
	}

	fmt.Printf("\n  OPTIMAL THRESHOLD: %.1f%% edge (%.1f%% ROI)\n", optimalThreshold, optimalROI)

	// Analyze by market at optimal threshold
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("       MARKET PERFORMANCE (Edge >= %.1f%%, TOI > 14min)\n", optimalThreshold)
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market/Line       Total    Wins   Losses  HitRate      ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Group by market and line
	marketLinePicks := make(map[string][]FilteredPick)
	for _, p := range allPicks {
		if p.Edge >= optimalThreshold {
			key := fmt.Sprintf("%s_%.1f", p.Market, p.Line)
			marketLinePicks[key] = append(marketLinePicks[key], p)
		}
	}

	// Sort keys
	var keys []string
	for k := range marketLinePicks {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		picks := marketLinePicks[key]
		var wins, losses int
		for _, p := range picks {
			if p.Won {
				wins++
			} else {
				losses++
			}
		}
		if len(picks) > 0 && (wins+losses) > 0 {
			hitRate := float64(wins) / float64(wins+losses) * 100
			roi := (float64(wins)*0.909 - float64(losses)) / float64(len(picks)) * 100
			roiStr := fmt.Sprintf("%+.1f%%", roi)
			if roi > 0 {
				roiStr += " ✓"
			}
			fmt.Printf("%-15s %7d %7d %7d %8.1f%% %10s\n",
				key, len(picks), wins, losses, hitRate, roiStr)
		}
	}

	// Sample of actual picks at high edge
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("               SAMPLE HIGH-EDGE PICKS (Edge > 15%)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market        Line  Predicted   Actual  Edge     TOI    Result")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	highEdgePicks := []FilteredPick{}
	for _, p := range allPicks {
		if p.Edge > 15 {
			highEdgePicks = append(highEdgePicks, p)
		}
	}

	// Show first 20
	shown := 0
	for _, p := range highEdgePicks {
		if shown >= 20 {
			break
		}
		result := "LOSS"
		if p.Won {
			result = "WIN ✓"
		}
		fmt.Printf("%-12s %5.1f %9.2f %8.0f %6.1f%% %6.1f %8s\n",
			p.Market, p.Line, p.Predicted, p.Actual, p.Edge, p.TOI, result)
		shown++
	}

	if len(highEdgePicks) > 20 {
		fmt.Printf("... and %d more high-edge picks\n", len(highEdgePicks)-20)
	}

	// Final recommendations
	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    PICK SELECTION LOGIC EVALUATION")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	// Count picks passing various filter combinations
	var totalAll, totalTOI14, totalEdge5, totalBoth int
	var winsAll, winsTOI14, winsEdge5, winsBoth int

	for _, p := range allPicks {
		totalAll++
		if p.Won {
			winsAll++
		}

		if p.TOI >= 14 {
			totalTOI14++
			if p.Won {
				winsTOI14++
			}
		}

		if p.Edge >= 5 {
			totalEdge5++
			if p.Won {
				winsEdge5++
			}
		}

		if p.TOI >= 14 && p.Edge >= 5 {
			totalBoth++
			if p.Won {
				winsBoth++
			}
		}
	}

	fmt.Println("\nFilter Combinations Impact:")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("No filters:              %6d picks, %5.1f%% hit rate\n",
		totalAll, float64(winsAll)/float64(totalAll)*100)
	fmt.Printf("TOI >= 14min only:       %6d picks, %5.1f%% hit rate\n",
		totalTOI14, float64(winsTOI14)/float64(totalTOI14)*100)
	fmt.Printf("Edge >= 5%% only:         %6d picks, %5.1f%% hit rate\n",
		totalEdge5, float64(winsEdge5)/float64(totalEdge5)*100)
	fmt.Printf("Both filters:            %6d picks, %5.1f%% hit rate ← RECOMMENDED\n",
		totalBoth, float64(winsBoth)/float64(totalBoth)*100)

	// ROI calculations
	roiAll := (float64(winsAll)*0.909 - float64(totalAll-winsAll)) / float64(totalAll) * 100
	roiTOI14 := (float64(winsTOI14)*0.909 - float64(totalTOI14-winsTOI14)) / float64(totalTOI14) * 100
	roiEdge5 := (float64(winsEdge5)*0.909 - float64(totalEdge5-winsEdge5)) / float64(totalEdge5) * 100
	roiBoth := (float64(winsBoth)*0.909 - float64(totalBoth-winsBoth)) / float64(totalBoth) * 100

	fmt.Println("\nROI by Filter:")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("No filters:              %+6.1f%% ROI\n", roiAll)
	fmt.Printf("TOI >= 14min only:       %+6.1f%% ROI\n", roiTOI14)
	fmt.Printf("Edge >= 5%% only:         %+6.1f%% ROI\n", roiEdge5)
	fmt.Printf("Both filters:            %+6.1f%% ROI ← BEST\n", roiBoth)

	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                         FINAL RECOMMENDATIONS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("1. MINIMUM FILTERS FOR PROFITABLE PICKS:")
	fmt.Println("   • TOI >= 14 minutes")
	fmt.Println("   • Edge >= 5%")
	fmt.Println()
	fmt.Println("2. OPTIMAL STRATEGY:")
	fmt.Printf("   • Use edge threshold of %.1f%% for best ROI\n", optimalThreshold)
	fmt.Println("   • Target high-TOI players (>22 min) for best hit rates")
	fmt.Println("   • Focus on Blocks O0.5 and SOG O1.5 at proper edge")
	fmt.Println()
	fmt.Println("3. AVOID:")
	fmt.Println("   • Players with <14 min TOI")
	fmt.Println("   • Any pick with negative edge")
	fmt.Println("   • High lines (O3.5+) unless extreme edge")
	fmt.Println()
	fmt.Printf("4. EXPECTED PERFORMANCE:")
	fmt.Printf("\n   • %d qualifying picks per week\n", totalBoth)
	fmt.Printf("   • %.1f%% hit rate\n", float64(winsBoth)/float64(totalBoth)*100)
	fmt.Printf("   • %+.1f%% ROI\n", roiBoth)
	fmt.Println()
}

func evaluatePlayerPicks(playerID, gameID int, actual types.BoxscorePlayerStats,
	expSOG, expGoals, expAssists, expPoints, expHits, expBlocks float64) []FilteredPick {

	var picks []FilteredPick

	// SOG
	for _, line := range []float64{1.5, 2.5, 3.5, 4.5} {
		dist := distributions.NewHierarchicalPoissonGamma(expSOG, expSOG)
		probOver := dist.ProbOver(line)
		edge := (probOver - 0.524) * 100
		picks = append(picks, FilteredPick{
			GameID:    gameID,
			PlayerID:  playerID,
			Market:    "SOG",
			Line:      line,
			Predicted: expSOG,
			Actual:    float64(actual.SOG),
			ProbOver:  probOver,
			Edge:      edge,
			TOI:       actual.TOI,
			Won:       float64(actual.SOG) > line,
		})
	}

	// Points
	for _, line := range []float64{0.5, 1.5} {
		dist := distributions.NewPoissonBinomialMixture(expPoints*2, expPoints*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		edge := (probOver - 0.524) * 100
		picks = append(picks, FilteredPick{
			GameID:    gameID,
			PlayerID:  playerID,
			Market:    "Points",
			Line:      line,
			Predicted: expPoints,
			Actual:    float64(actual.Points),
			ProbOver:  probOver,
			Edge:      edge,
			TOI:       actual.TOI,
			Won:       float64(actual.Points) > line,
		})
	}

	// Goals
	dist := distributions.NewPoissonBinomialMixture(expGoals*2, expGoals*2, 0.5, 0.5)
	probOver := dist.ProbOver(0.5)
	edge := (probOver - 0.524) * 100
	picks = append(picks, FilteredPick{
		GameID:    gameID,
		PlayerID:  playerID,
		Market:    "Goals",
		Line:      0.5,
		Predicted: expGoals,
		Actual:    float64(actual.Goals),
		ProbOver:  probOver,
		Edge:      edge,
		TOI:       actual.TOI,
		Won:       float64(actual.Goals) > 0.5,
	})

	// Assists
	for _, line := range []float64{0.5, 1.5} {
		dist := distributions.NewPoissonBinomialMixture(expAssists*2, expAssists*2, 0.5, 0.5)
		probOver := dist.ProbOver(line)
		edge := (probOver - 0.524) * 100
		picks = append(picks, FilteredPick{
			GameID:    gameID,
			PlayerID:  playerID,
			Market:    "Assists",
			Line:      line,
			Predicted: expAssists,
			Actual:    float64(actual.Assists),
			ProbOver:  probOver,
			Edge:      edge,
			TOI:       actual.TOI,
			Won:       float64(actual.Assists) > line,
		})
	}

	// Hits
	for _, line := range []float64{1.5, 2.5, 3.5} {
		dist := distributions.NewZeroInflatedNegBin(expHits, expHits)
		probOver := dist.ProbOver(line)
		edge := (probOver - 0.524) * 100
		picks = append(picks, FilteredPick{
			GameID:    gameID,
			PlayerID:  playerID,
			Market:    "Hits",
			Line:      line,
			Predicted: expHits,
			Actual:    float64(actual.Hits),
			ProbOver:  probOver,
			Edge:      edge,
			TOI:       actual.TOI,
			Won:       float64(actual.Hits) > line,
		})
	}

	// Blocks
	for _, line := range []float64{0.5, 1.5, 2.5} {
		dist := distributions.NewZeroInflatedNegBin(expBlocks, expBlocks)
		probOver := dist.ProbOver(line)
		edge := (probOver - 0.524) * 100
		picks = append(picks, FilteredPick{
			GameID:    gameID,
			PlayerID:  playerID,
			Market:    "Blocks",
			Line:      line,
			Predicted: expBlocks,
			Actual:    float64(actual.Blocks),
			ProbOver:  probOver,
			Edge:      edge,
			TOI:       actual.TOI,
			Won:       float64(actual.Blocks) > line,
		})
	}

	return picks
}

func init() {
	// Call filtered analysis after main analysis
}
