package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/scenario"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// DetailedAnalysis holds comprehensive backtest analysis
type DetailedAnalysis struct {
	// Core metrics
	TotalGames       int
	TotalPlayers     int
	TotalPicks       int

	// Calibration metrics
	CalibrationBins  map[string]*CalibrationBin

	// Pick quality breakdown
	PicksByEdge      map[string]*PickBucket
	PicksByMarket    map[string]*MarketAnalysis
	PicksByScenario  map[types.ScenarioType]*ScenarioAnalysis

	// Player role analysis
	PicksByTOI       map[string]*TOIBucket
	PicksByPosition  map[string]*PositionAnalysis

	// ROI calculations
	ROIByEdge        map[string]float64

	// Edge cases
	BigMisses        []PickDetail
	BigHits          []PickDetail

	// Distribution accuracy
	DistributionStats map[string]*DistributionAccuracy
}

type CalibrationBin struct {
	PredictedProb float64
	TotalPicks    int
	ActualHits    int
	ActualRate    float64
}

type PickBucket struct {
	EdgeRange  string
	TotalPicks int
	Wins       int
	Losses     int
	HitRate    float64
	AvgEdge    float64
	ROI        float64
}

type MarketAnalysis struct {
	Market        string
	TotalPicks    int
	Wins          int
	Losses        int
	HitRate       float64
	AvgPredicted  float64
	AvgActual     float64
	MAE           float64
	Bias          float64 // Positive = over-predicting
	ByLine        map[float64]*LineAnalysis
}

type LineAnalysis struct {
	Line       float64
	TotalPicks int
	Wins       int
	HitRate    float64
	AvgEdge    float64
}

type ScenarioAnalysis struct {
	Scenario      types.ScenarioType
	TotalGames    int
	CorrectPredictions int
	TotalPicks    int
	Wins          int
	HitRate       float64
	AvgTotalGoals float64
}

type TOIBucket struct {
	Range      string
	TotalPicks int
	Wins       int
	HitRate    float64
}

type PositionAnalysis struct {
	Position   string
	TotalPicks int
	Wins       int
	HitRate    float64
	AvgTOI     float64
}

type PickDetail struct {
	GameID     int
	PlayerID   int
	Market     string
	Line       float64
	Predicted  float64
	Actual     float64
	Edge       float64
	Result     string
	TOI        float64
}

type DistributionAccuracy struct {
	Market     string
	TotalPicks int
	MeanError  float64
	StdError   float64
	P10Actual  float64 // Actual rate in predicted 10% bucket
	P50Actual  float64 // Actual rate in predicted 50% bucket
	P90Actual  float64 // Actual rate in predicted 90% bucket
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                NHL PROP MODEL - RIGOROUS ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Analyze last week
	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -6)

	fmt.Printf("Analysis Period: %s to %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	analysis := runDetailedAnalysis(startDate, endDate)
	printAnalysis(analysis)
}

func runDetailedAnalysis(startDate, endDate time.Time) *DetailedAnalysis {
	client := nhl.New()
	scenarioEng := scenario.NewEngine()

	analysis := &DetailedAnalysis{
		CalibrationBins:   make(map[string]*CalibrationBin),
		PicksByEdge:       make(map[string]*PickBucket),
		PicksByMarket:     make(map[string]*MarketAnalysis),
		PicksByScenario:   make(map[types.ScenarioType]*ScenarioAnalysis),
		PicksByTOI:        make(map[string]*TOIBucket),
		PicksByPosition:   make(map[string]*PositionAnalysis),
		ROIByEdge:         make(map[string]float64),
		DistributionStats: make(map[string]*DistributionAccuracy),
	}

	// Initialize bins
	for _, bin := range []string{"0-10%", "10-20%", "20-30%", "30-40%", "40-50%", "50-60%", "60-70%", "70-80%", "80-90%", "90-100%"} {
		analysis.CalibrationBins[bin] = &CalibrationBin{}
	}

	for _, bucket := range []string{"<0%", "0-2%", "2-5%", "5-10%", "10-15%", "15-20%", ">20%"} {
		analysis.PicksByEdge[bucket] = &PickBucket{EdgeRange: bucket}
	}

	for _, toi := range []string{"<10min", "10-14min", "14-18min", "18-22min", ">22min"} {
		analysis.PicksByTOI[toi] = &TOIBucket{Range: toi}
	}

	// League averages (calibrated)
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

			analysis.TotalGames++

			// Get scenario
			actualScenario := scenarioEng.DetermineActualScenario(boxscore)
			totalGoals := boxscore.HomeScore + boxscore.AwayScore

			// Track scenario
			if analysis.PicksByScenario[actualScenario] == nil {
				analysis.PicksByScenario[actualScenario] = &ScenarioAnalysis{Scenario: actualScenario}
			}
			scenarioStats := analysis.PicksByScenario[actualScenario]
			scenarioStats.TotalGames++
			scenarioStats.AvgTotalGoals += float64(totalGoals)

			// Analyze each player
			for playerID, actual := range boxscore.PlayerStats {
				if actual.TOI < 5 {
					continue
				}

				analysis.TotalPlayers++

				// Calculate expectations
				toiFactor := actual.TOI / 16.0

				// Analyze each market
				analyzeMarket(analysis, "SOG", leagueAvgSOG*toiFactor, float64(actual.SOG),
					[]float64{1.5, 2.5, 3.5, 4.5}, actual, playerID, game.ID, actualScenario)
				analyzeMarket(analysis, "Points", leagueAvgPoints*toiFactor, float64(actual.Points),
					[]float64{0.5, 1.5}, actual, playerID, game.ID, actualScenario)
				analyzeMarket(analysis, "Goals", leagueAvgGoals*toiFactor, float64(actual.Goals),
					[]float64{0.5}, actual, playerID, game.ID, actualScenario)
				analyzeMarket(analysis, "Assists", leagueAvgAssists*toiFactor, float64(actual.Assists),
					[]float64{0.5, 1.5}, actual, playerID, game.ID, actualScenario)
				analyzeMarket(analysis, "Hits", leagueAvgHits*toiFactor, float64(actual.Hits),
					[]float64{1.5, 2.5, 3.5}, actual, playerID, game.ID, actualScenario)
				analyzeMarket(analysis, "Blocks", leagueAvgBlocks*toiFactor, float64(actual.Blocks),
					[]float64{0.5, 1.5, 2.5}, actual, playerID, game.ID, actualScenario)
			}
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Calculate final metrics
	finalizeAnalysis(analysis)

	return analysis
}

func analyzeMarket(analysis *DetailedAnalysis, market string, expected, actual float64,
	lines []float64, playerStats types.BoxscorePlayerStats, playerID, gameID int,
	scenario types.ScenarioType) {

	if analysis.PicksByMarket[market] == nil {
		analysis.PicksByMarket[market] = &MarketAnalysis{
			Market: market,
			ByLine: make(map[float64]*LineAnalysis),
		}
	}
	marketStats := analysis.PicksByMarket[market]

	for _, line := range lines {
		// Calculate probability using appropriate distribution
		var probOver float64
		switch market {
		case "SOG":
			dist := distributions.NewHierarchicalPoissonGamma(expected, expected)
			probOver = dist.ProbOver(line)
		case "Points", "Goals", "Assists":
			dist := distributions.NewPoissonBinomialMixture(expected*2, expected*2, 0.5, 0.5)
			probOver = dist.ProbOver(line)
		case "Hits", "Blocks":
			dist := distributions.NewZeroInflatedNegBin(expected, expected)
			probOver = dist.ProbOver(line)
		}

		// Calculate edge
		impliedProb := 0.524 // Standard -110 juice
		edge := (probOver - impliedProb) * 100

		// Determine result
		won := actual > line

		analysis.TotalPicks++

		// Update market stats
		marketStats.TotalPicks++
		marketStats.AvgPredicted += expected
		marketStats.AvgActual += actual
		marketStats.MAE += math.Abs(expected - actual)
		if won {
			marketStats.Wins++
		} else if actual < line {
			marketStats.Losses++
		}

		// Update line stats
		if marketStats.ByLine[line] == nil {
			marketStats.ByLine[line] = &LineAnalysis{Line: line}
		}
		lineStats := marketStats.ByLine[line]
		lineStats.TotalPicks++
		lineStats.AvgEdge += edge
		if won {
			lineStats.Wins++
		}

		// Update edge bucket
		bucket := getDetailedEdgeBucket(edge)
		edgeStats := analysis.PicksByEdge[bucket]
		edgeStats.TotalPicks++
		edgeStats.AvgEdge += edge
		if won {
			edgeStats.Wins++
		} else if actual < line {
			edgeStats.Losses++
		}

		// Update calibration bin
		calBin := getCalibrationBin(probOver)
		if analysis.CalibrationBins[calBin] != nil {
			analysis.CalibrationBins[calBin].TotalPicks++
			if won {
				analysis.CalibrationBins[calBin].ActualHits++
			}
		}

		// Update TOI bucket
		toiBucket := getTOIBucket(playerStats.TOI)
		toiStats := analysis.PicksByTOI[toiBucket]
		toiStats.TotalPicks++
		if won {
			toiStats.Wins++
		}

		// Update scenario stats
		scenarioStats := analysis.PicksByScenario[scenario]
		if scenarioStats != nil {
			scenarioStats.TotalPicks++
			if won {
				scenarioStats.Wins++
			}
		}

		// Track big misses and hits
		if edge > 15 && !won {
			analysis.BigMisses = append(analysis.BigMisses, PickDetail{
				GameID:    gameID,
				PlayerID:  playerID,
				Market:    market,
				Line:      line,
				Predicted: expected,
				Actual:    actual,
				Edge:      edge,
				Result:    "LOSS",
				TOI:       playerStats.TOI,
			})
		}
		if edge > 15 && won {
			analysis.BigHits = append(analysis.BigHits, PickDetail{
				GameID:    gameID,
				PlayerID:  playerID,
				Market:    market,
				Line:      line,
				Predicted: expected,
				Actual:    actual,
				Edge:      edge,
				Result:    "WIN",
				TOI:       playerStats.TOI,
			})
		}
	}
}

func getDetailedEdgeBucket(edge float64) string {
	if edge < 0 {
		return "<0%"
	} else if edge < 2 {
		return "0-2%"
	} else if edge < 5 {
		return "2-5%"
	} else if edge < 10 {
		return "5-10%"
	} else if edge < 15 {
		return "10-15%"
	} else if edge < 20 {
		return "15-20%"
	}
	return ">20%"
}

func getCalibrationBin(prob float64) string {
	if prob < 0.1 {
		return "0-10%"
	} else if prob < 0.2 {
		return "10-20%"
	} else if prob < 0.3 {
		return "20-30%"
	} else if prob < 0.4 {
		return "30-40%"
	} else if prob < 0.5 {
		return "40-50%"
	} else if prob < 0.6 {
		return "50-60%"
	} else if prob < 0.7 {
		return "60-70%"
	} else if prob < 0.8 {
		return "70-80%"
	} else if prob < 0.9 {
		return "80-90%"
	}
	return "90-100%"
}

func getTOIBucket(toi float64) string {
	if toi < 10 {
		return "<10min"
	} else if toi < 14 {
		return "10-14min"
	} else if toi < 18 {
		return "14-18min"
	} else if toi < 22 {
		return "18-22min"
	}
	return ">22min"
}

func finalizeAnalysis(analysis *DetailedAnalysis) {
	// Finalize market stats
	for _, m := range analysis.PicksByMarket {
		if m.TotalPicks > 0 {
			m.AvgPredicted /= float64(m.TotalPicks)
			m.AvgActual /= float64(m.TotalPicks)
			m.MAE /= float64(m.TotalPicks)
			m.Bias = m.AvgPredicted - m.AvgActual
			m.HitRate = float64(m.Wins) / float64(m.Wins+m.Losses) * 100

			for _, l := range m.ByLine {
				if l.TotalPicks > 0 {
					l.HitRate = float64(l.Wins) / float64(l.TotalPicks) * 100
					l.AvgEdge /= float64(l.TotalPicks)
				}
			}
		}
	}

	// Finalize edge buckets and calculate ROI
	for bucket, stats := range analysis.PicksByEdge {
		if stats.TotalPicks > 0 {
			stats.HitRate = float64(stats.Wins) / float64(stats.Wins+stats.Losses) * 100
			stats.AvgEdge /= float64(stats.TotalPicks)

			// ROI calculation: (wins * 0.91 - losses) / total * 100
			// At -110 odds, winning pays 0.91 units, losing costs 1 unit
			roi := (float64(stats.Wins)*0.909 - float64(stats.Losses)) / float64(stats.TotalPicks) * 100
			analysis.ROIByEdge[bucket] = roi
			stats.ROI = roi
		}
	}

	// Finalize calibration bins
	for _, bin := range analysis.CalibrationBins {
		if bin.TotalPicks > 0 {
			bin.ActualRate = float64(bin.ActualHits) / float64(bin.TotalPicks) * 100
		}
	}

	// Finalize TOI buckets
	for _, t := range analysis.PicksByTOI {
		if t.TotalPicks > 0 {
			t.HitRate = float64(t.Wins) / float64(t.TotalPicks) * 100
		}
	}

	// Finalize scenario stats
	for _, s := range analysis.PicksByScenario {
		if s.TotalGames > 0 {
			s.AvgTotalGoals /= float64(s.TotalGames)
		}
		if s.TotalPicks > 0 {
			s.HitRate = float64(s.Wins) / float64(s.TotalPicks) * 100
		}
	}
}

func printAnalysis(analysis *DetailedAnalysis) {
	fmt.Printf("Games Analyzed: %d | Players: %d | Total Picks: %d\n\n",
		analysis.TotalGames, analysis.TotalPlayers, analysis.TotalPicks)

	// 1. Calibration Analysis
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                        1. CALIBRATION ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Predicted Prob    Picks      Hits    Actual Rate    Calibration")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	binOrder := []string{"0-10%", "10-20%", "20-30%", "30-40%", "40-50%", "50-60%", "60-70%", "70-80%", "80-90%", "90-100%"}
	for _, bin := range binOrder {
		b := analysis.CalibrationBins[bin]
		if b.TotalPicks > 0 {
			// Expected midpoint for the bin
			var expected float64
			switch bin {
			case "0-10%":
				expected = 5
			case "10-20%":
				expected = 15
			case "20-30%":
				expected = 25
			case "30-40%":
				expected = 35
			case "40-50%":
				expected = 45
			case "50-60%":
				expected = 55
			case "60-70%":
				expected = 65
			case "70-80%":
				expected = 75
			case "80-90%":
				expected = 85
			case "90-100%":
				expected = 95
			}
			diff := b.ActualRate - expected
			calStatus := "✓"
			if math.Abs(diff) > 10 {
				calStatus = "✗ OFF"
			} else if math.Abs(diff) > 5 {
				calStatus = "~"
			}
			fmt.Printf("%-16s %6d %9d %12.1f%% %12s (%+.1f%%)\n",
				bin, b.TotalPicks, b.ActualHits, b.ActualRate, calStatus, diff)
		}
	}

	// 2. Edge-Based ROI Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                      2. EDGE-BASED ROI ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Edge Range      Picks      Wins    Losses   Hit Rate    Avg Edge       ROI")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	edgeOrder := []string{"<0%", "0-2%", "2-5%", "5-10%", "10-15%", "15-20%", ">20%"}
	for _, bucket := range edgeOrder {
		e := analysis.PicksByEdge[bucket]
		if e.TotalPicks > 0 {
			roiStr := fmt.Sprintf("%+.1f%%", e.ROI)
			if e.ROI > 0 {
				roiStr = fmt.Sprintf("%+.1f%% ✓", e.ROI)
			}
			fmt.Printf("%-12s %8d %8d %8d %9.1f%% %10.1f%% %12s\n",
				bucket, e.TotalPicks, e.Wins, e.Losses, e.HitRate, e.AvgEdge, roiStr)
		}
	}

	// Calculate total positive edge ROI
	var posEdgePicks, posEdgeWins, posEdgeLosses int
	for bucket, e := range analysis.PicksByEdge {
		if bucket != "<0%" && e.TotalPicks > 0 {
			posEdgePicks += e.TotalPicks
			posEdgeWins += e.Wins
			posEdgeLosses += e.Losses
		}
	}
	if posEdgePicks > 0 {
		posEdgeROI := (float64(posEdgeWins)*0.909 - float64(posEdgeLosses)) / float64(posEdgePicks) * 100
		posEdgeHitRate := float64(posEdgeWins) / float64(posEdgeWins+posEdgeLosses) * 100
		fmt.Println("───────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("ALL POSITIVE   %8d %8d %8d %9.1f%%            %12s\n",
			posEdgePicks, posEdgeWins, posEdgeLosses, posEdgeHitRate, fmt.Sprintf("%+.1f%%", posEdgeROI))
	}

	// 3. Market-by-Market Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                      3. MARKET-BY-MARKET ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market        Picks     HitRate   AvgPred   AvgAct      MAE      Bias")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	marketOrder := []string{"SOG", "Points", "Goals", "Assists", "Hits", "Blocks"}
	for _, market := range marketOrder {
		m := analysis.PicksByMarket[market]
		if m != nil && m.TotalPicks > 0 {
			biasStr := fmt.Sprintf("%+.2f", m.Bias)
			if math.Abs(m.Bias) > 0.1 {
				if m.Bias > 0 {
					biasStr += " (over)"
				} else {
					biasStr += " (under)"
				}
			}
			fmt.Printf("%-12s %6d %10.1f%% %9.2f %8.2f %8.2f %12s\n",
				market, m.TotalPicks, m.HitRate, m.AvgPredicted, m.AvgActual, m.MAE, biasStr)
		}
	}

	// 4. Line-Specific Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                      4. LINE-SPECIFIC ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Market/Line       Picks    HitRate    AvgEdge    Assessment")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	for _, market := range marketOrder {
		m := analysis.PicksByMarket[market]
		if m == nil {
			continue
		}

		// Sort lines
		var lines []float64
		for line := range m.ByLine {
			lines = append(lines, line)
		}
		sort.Float64s(lines)

		for _, line := range lines {
			l := m.ByLine[line]
			if l.TotalPicks > 0 {
				assessment := "Neutral"
				// Break-even at -110 is ~52.4%
				if l.HitRate > 55 {
					assessment = "PROFITABLE ✓"
				} else if l.HitRate < 45 {
					assessment = "AVOID ✗"
				}
				fmt.Printf("%-8s O%.1f %8d %9.1f%% %10.1f%% %14s\n",
					market, line, l.TotalPicks, l.HitRate, l.AvgEdge, assessment)
			}
		}
	}

	// 5. TOI Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                         5. TOI ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("TOI Range         Picks    HitRate    Assessment")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	toiOrder := []string{"<10min", "10-14min", "14-18min", "18-22min", ">22min"}
	for _, toi := range toiOrder {
		t := analysis.PicksByTOI[toi]
		if t.TotalPicks > 0 {
			assessment := ""
			if t.HitRate > 25 {
				assessment = "Target ✓"
			} else if t.HitRate < 15 {
				assessment = "Avoid ✗"
			}
			fmt.Printf("%-14s %8d %9.1f%% %12s\n", toi, t.TotalPicks, t.HitRate, assessment)
		}
	}

	// 6. Scenario Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                       6. SCENARIO ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Scenario          Games   AvgTotal      Picks    HitRate")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	scenarioOrder := []types.ScenarioType{
		types.ScenarioRunAndGun,
		types.ScenarioGoalieDuel,
		types.ScenarioSpecialTeams,
		types.ScenarioScoreEffects,
		types.ScenarioBenchSlog,
	}
	for _, scenario := range scenarioOrder {
		s := analysis.PicksByScenario[scenario]
		if s != nil && s.TotalGames > 0 {
			fmt.Printf("%-16s %6d %10.1f %10d %9.1f%%\n",
				scenario, s.TotalGames, s.AvgTotalGoals, s.TotalPicks, s.HitRate)
		}
	}

	// 7. Pick Selection Recommendations
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                    7. PICK SELECTION RECOMMENDATIONS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	fmt.Println("\nBased on analysis:")
	fmt.Println()

	// Find best performing combinations
	fmt.Println("  RECOMMENDED PICKS (>55% hit rate or positive ROI):")
	for _, market := range marketOrder {
		m := analysis.PicksByMarket[market]
		if m == nil {
			continue
		}
		for line, l := range m.ByLine {
			if l.TotalPicks >= 100 && (l.HitRate > 55 || l.AvgEdge > 5) {
				fmt.Printf("    • %s Over %.1f: %.1f%% hit rate, %.1f%% avg edge\n",
					market, line, l.HitRate, l.AvgEdge)
			}
		}
	}

	fmt.Println("\n  PICKS TO AVOID (<45% hit rate):")
	for _, market := range marketOrder {
		m := analysis.PicksByMarket[market]
		if m == nil {
			continue
		}
		for line, l := range m.ByLine {
			if l.TotalPicks >= 100 && l.HitRate < 45 {
				fmt.Printf("    • %s Over %.1f: %.1f%% hit rate - AVOID\n",
					market, line, l.HitRate)
			}
		}
	}

	fmt.Println("\n  EDGE THRESHOLD RECOMMENDATION:")
	bestROI := -100.0
	bestBucket := ""
	for bucket, e := range analysis.PicksByEdge {
		if bucket != "<0%" && e.TotalPicks >= 50 && e.ROI > bestROI {
			bestROI = e.ROI
			bestBucket = bucket
		}
	}
	if bestBucket != "" {
		fmt.Printf("    • Best ROI at %s edge: %.1f%% return\n", bestBucket, bestROI)
	}

	// Minimum edge recommendation
	for _, bucket := range []string{"0-2%", "2-5%", "5-10%", "10-15%"} {
		e := analysis.PicksByEdge[bucket]
		if e.TotalPicks >= 50 && e.ROI > 0 {
			fmt.Printf("    • Minimum edge for profitability: %s\n", bucket)
			break
		}
	}

	// 8. Big Misses Analysis
	fmt.Println("\n───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("                      8. BIG MISSES ANALYSIS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if len(analysis.BigMisses) > 0 {
		// Count by market
		missesByMarket := make(map[string]int)
		for _, miss := range analysis.BigMisses {
			missesByMarket[miss.Market]++
		}

		fmt.Println("High-edge picks (>15%) that lost, by market:")
		for market, count := range missesByMarket {
			pct := float64(count) / float64(len(analysis.BigMisses)) * 100
			fmt.Printf("  %s: %d (%.1f%%)\n", market, count, pct)
		}

		fmt.Printf("\nTotal high-edge misses: %d\n", len(analysis.BigMisses))
		fmt.Printf("Total high-edge hits: %d\n", len(analysis.BigHits))
		if len(analysis.BigMisses)+len(analysis.BigHits) > 0 {
			hitRate := float64(len(analysis.BigHits)) / float64(len(analysis.BigMisses)+len(analysis.BigHits)) * 100
			fmt.Printf("High-edge (>15%%) hit rate: %.1f%%\n", hitRate)
		}
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════════════════════")

	// Run realistic market analysis
	RunRealisticAnalysis()

	// Run player-specific analysis
	RunPlayerSpecificAnalysis()

	// Run methodology-aligned analysis
	RunMethodologyAlignedAnalysis()
}
