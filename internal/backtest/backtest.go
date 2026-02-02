package backtest

import (
	"fmt"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/correlation"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/props"
	"github.com/paul/nhl-prop-model/internal/model/scenario"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// BacktestResult holds the results of a backtest run
type BacktestResult struct {
	StartDate    time.Time
	EndDate      time.Time
	GamesAnalyzed int

	// Scenario accuracy
	ScenarioResults map[types.ScenarioType]*ScenarioResult

	// Prop results by market
	PropResults     map[types.PropMarket]*PropMarketResult

	// Overall metrics
	TotalPicks      int
	TotalWins       int
	TotalLosses     int
	TotalPushes     int
	OverallHitRate  float64
	AvgEdge         float64
	AvgCLV          float64

	// By tier
	TierResults     map[types.PropTier]*TierResult

	// Errors encountered
	Errors          []string
}

// ScenarioResult tracks accuracy for a scenario type
type ScenarioResult struct {
	Scenario       types.ScenarioType
	Predicted      int
	Correct        int
	Accuracy       float64
	AvgScore       float64
}

// PropMarketResult tracks results for a prop market
type PropMarketResult struct {
	Market         types.PropMarket
	TotalPicks     int
	Wins           int
	Losses         int
	Pushes         int
	HitRate        float64
	AvgEdge        float64
	AvgProjectedLine float64
	AvgActualValue float64
}

// TierResult tracks results by tier
type TierResult struct {
	Tier           types.PropTier
	TotalPicks     int
	Wins           int
	Losses         int
	HitRate        float64
	AvgEdge        float64
}

// GameBacktest holds backtest data for a single game
type GameBacktest struct {
	Game            types.Game
	Boxscore        *types.Boxscore
	PredictedScenario types.ScenarioType
	ActualScenario  types.ScenarioType
	ScenarioCorrect bool
	PropPredictions []PropPrediction
}

// PropPrediction holds a single prop prediction and result
type PropPrediction struct {
	PlayerID       int
	PlayerName     string
	Market         types.PropMarket
	Line           float64
	Side           types.PropSide
	OurLine        float64
	ProbHit        float64
	Edge           float64
	Tier           types.PropTier
	Stars          int
	ActualValue    float64
	Result         types.PropResult
	PassedGuardrails bool
}

// Backtester runs backtests on historical data
type Backtester struct {
	nhlClient    *nhl.Client
	scenarioEng  *scenario.Engine
	evaluator    *props.Evaluator
	corrMatrix   *correlation.Matrix
}

// NewBacktester creates a new backtester
func NewBacktester() *Backtester {
	corrMatrix := correlation.NewMatrix()
	return &Backtester{
		nhlClient:   nhl.New(),
		scenarioEng: scenario.NewEngine(),
		evaluator:   props.NewEvaluator().WithCorrMatrix(corrMatrix),
		corrMatrix:  corrMatrix,
	}
}

// RunBacktest runs a backtest for a date range
func (b *Backtester) RunBacktest(startDate, endDate time.Time) (*BacktestResult, error) {
	result := &BacktestResult{
		StartDate:       startDate,
		EndDate:         endDate,
		ScenarioResults: make(map[types.ScenarioType]*ScenarioResult),
		PropResults:     make(map[types.PropMarket]*PropMarketResult),
		TierResults:     make(map[types.PropTier]*TierResult),
	}

	// Initialize scenario results
	for _, s := range []types.ScenarioType{
		types.ScenarioRunAndGun,
		types.ScenarioGoalieDuel,
		types.ScenarioSpecialTeams,
		types.ScenarioScoreEffects,
		types.ScenarioBenchSlog,
	} {
		result.ScenarioResults[s] = &ScenarioResult{Scenario: s}
	}

	// Initialize tier results
	for _, t := range []types.PropTier{types.PropTier1, types.PropTier1_5, types.PropTier2, types.PropTier3} {
		result.TierResults[t] = &TierResult{Tier: t}
	}

	// Iterate through each day
	currentDate := startDate
	for !currentDate.After(endDate) {
		games, err := b.nhlClient.GetSchedule(currentDate)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Error fetching %s: %v", currentDate.Format("2006-01-02"), err))
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		for _, game := range games {
			if game.State != types.GameStateFinal {
				continue
			}

			gameResult, err := b.backtestGame(game)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Game %d error: %v", game.ID, err))
				continue
			}

			// Aggregate results
			b.aggregateGameResults(result, gameResult)
			result.GamesAnalyzed++
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Calculate final metrics
	b.calculateFinalMetrics(result)

	return result, nil
}

func (b *Backtester) backtestGame(game types.Game) (*GameBacktest, error) {
	// Fetch boxscore
	boxscore, err := b.nhlClient.GetBoxscore(game.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch boxscore: %w", err)
	}

	gameBacktest := &GameBacktest{
		Game:     game,
		Boxscore: boxscore,
	}

	// Create mock team stats based on what we know
	// In a real backtest, we'd use historical stats from before the game
	homeStats := b.createMockTeamStats(game.HomeTeamAbbr, boxscore, true)
	awayStats := b.createMockTeamStats(game.AwayTeamAbbr, boxscore, false)

	// Create game context
	ctx := &types.GameContext{
		GameID:       game.ID,
		HomeTeamAbbr: game.HomeTeamAbbr,
		AwayTeamAbbr: game.AwayTeamAbbr,
		TotalLine:    6.0, // Default
	}

	// Run scenario engine
	gameScenario := b.scenarioEng.ClassifyGame(homeStats, awayStats, nil, nil, ctx)
	gameBacktest.PredictedScenario = gameScenario.Primary.Scenario

	// Determine actual scenario from boxscore
	gameBacktest.ActualScenario = b.scenarioEng.DetermineActualScenario(boxscore)
	gameBacktest.ScenarioCorrect = gameBacktest.PredictedScenario == gameBacktest.ActualScenario

	// Generate and evaluate props for each player in boxscore
	for playerID, playerStats := range boxscore.PlayerStats {
		predictions := b.evaluatePlayerProps(playerID, playerStats, gameScenario, ctx, boxscore)
		gameBacktest.PropPredictions = append(gameBacktest.PropPredictions, predictions...)
	}

	return gameBacktest, nil
}

func (b *Backtester) createMockTeamStats(teamAbbr string, box *types.Boxscore, isHome bool) *types.TeamStats {
	// Calculate team stats from boxscore for this game
	totalGoals := 0
	totalShots := 0
	totalHits := 0
	totalPIM := 0
	playerCount := 0

	for _, ps := range box.PlayerStats {
		// Determine which team this player is on based on goals differential
		// This is a simplification - in reality we'd have team IDs
		totalGoals += ps.Goals
		totalShots += ps.SOG
		totalHits += ps.Hits
		totalPIM += ps.PIM
		playerCount++
	}

	return &types.TeamStats{
		TeamAbbr:      teamAbbr,
		GFPerGame:     float64(totalGoals) / 2, // Rough estimate
		GAPerGame:     float64(totalGoals) / 2,
		SFPerGame:     float64(totalShots) / 2,
		SAPerGame:     float64(totalShots) / 2,
		CF60:          55.0, // Default
		CA60:          55.0,
		PPPct:         20.0,
		PKPct:         80.0,
		HitsPerGame:   float64(totalHits) / 2,
		PIMPerGame:    float64(totalPIM) / 2,
	}
}

func (b *Backtester) evaluatePlayerProps(playerID int, actual types.BoxscorePlayerStats, gameScenario *types.GameScenario, ctx *types.GameContext, box *types.Boxscore) []PropPrediction {
	var predictions []PropPrediction

	// Create mock player stats (would come from pre-game data in reality)
	player := &types.PlayerWithStats{
		Player: types.Player{
			ID:       playerID,
			Position: types.PositionCenter, // Default
		},
		Stats: types.PlayerStats{
			PlayerID:      playerID,
			GamesPlayed:   40,
			TOI:           actual.TOI,
			SOG:           float64(actual.SOG),
			RecentSOG:     float64(actual.SOG),
			PointsPerGame: float64(actual.Points),
			RecentPPG:     float64(actual.Points),
			IsTopSix:      actual.TOI >= 14,
			IsTopFour:     actual.TOI >= 18,
			PP1Unit:       actual.PPTOI >= 2,
			PPTOI:         actual.PPTOI,
		},
	}

	// Set position based on TOI patterns
	if actual.TOI >= 18 {
		player.Player.Position = types.PositionDefense
		player.Stats.IsTopFour = true
	}

	distCtx := distributions.DistributionContext{
		TeamPace:      1.0,
		TeamTotal:     3.0,
		ExpectedTOI:   actual.TOI,
		ExpectedPPTOI: actual.PPTOI,
		PP1Unit:       actual.PPTOI >= 2,
		ArenaSOGBias:  1.0,
		ScenarioType:  gameScenario.Primary.Scenario,
	}

	// Evaluate SOG props
	sogLines := []float64{1.5, 2.5, 3.5, 4.5}
	for _, line := range sogLines {
		pred := b.evaluateSingleProp(player, types.PropMarketSOGOver, line, distCtx, gameScenario, float64(actual.SOG))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	// Evaluate Points props
	if actual.Points > 0 || player.Stats.PointsPerGame > 0.3 {
		pred := b.evaluateSingleProp(player, types.PropMarketPointsOver, 0.5, distCtx, gameScenario, float64(actual.Points))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	// Evaluate Goals props (AGS)
	if actual.Goals > 0 || player.Stats.PointsPerGame > 0.5 {
		pred := b.evaluateSingleProp(player, types.PropMarketGoalsOver, 0.5, distCtx, gameScenario, float64(actual.Goals))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	// Evaluate Assists props
	if actual.Assists > 0 || player.Stats.PointsPerGame > 0.4 {
		pred := b.evaluateSingleProp(player, types.PropMarketAssistsOver, 0.5, distCtx, gameScenario, float64(actual.Assists))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	// Evaluate Hits props
	hitsLines := []float64{1.5, 2.5, 3.5}
	for _, line := range hitsLines {
		pred := b.evaluateSingleProp(player, types.PropMarketHitsOver, line, distCtx, gameScenario, float64(actual.Hits))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	// Evaluate Blocks props
	blocksLines := []float64{0.5, 1.5, 2.5}
	for _, line := range blocksLines {
		pred := b.evaluateSingleProp(player, types.PropMarketBlocksOver, line, distCtx, gameScenario, float64(actual.Blocks))
		if pred != nil {
			predictions = append(predictions, *pred)
		}
	}

	return predictions
}

func (b *Backtester) evaluateSingleProp(player *types.PlayerWithStats, market types.PropMarket, line float64, ctx distributions.DistributionContext, gameScenario *types.GameScenario, actualValue float64) *PropPrediction {
	propLine := types.PropLine{
		PlayerID:    player.Player.ID,
		PlayerName:  player.Player.Name,
		Market:      market,
		Line:        line,
		Side:        types.PropSideOver,
		Price:       -110, // Standard juice
		ImpliedProb: 0.524,
	}

	eval := b.evaluator.EvaluateProp(propLine, player, ctx, gameScenario)

	// Determine result
	var result types.PropResult
	if actualValue > line {
		result = types.PropResultWin
	} else if actualValue < line {
		result = types.PropResultLoss
	} else {
		result = types.PropResultPush
	}

	return &PropPrediction{
		PlayerID:         player.Player.ID,
		PlayerName:       player.Player.Name,
		Market:           market,
		Line:             line,
		Side:             types.PropSideOver,
		OurLine:          eval.OurLine,
		ProbHit:          eval.ProbOver,
		Edge:             eval.Edge,
		Tier:             eval.Tier,
		Stars:            eval.Stars,
		ActualValue:      actualValue,
		Result:           result,
		PassedGuardrails: eval.PassesGuardrails,
	}
}

func (b *Backtester) aggregateGameResults(result *BacktestResult, gameResult *GameBacktest) {
	// Aggregate scenario results
	scenResult := result.ScenarioResults[gameResult.PredictedScenario]
	scenResult.Predicted++
	if gameResult.ScenarioCorrect {
		scenResult.Correct++
	}

	// Aggregate prop results
	for _, pred := range gameResult.PropPredictions {
		// By market
		if result.PropResults[pred.Market] == nil {
			result.PropResults[pred.Market] = &PropMarketResult{Market: pred.Market}
		}
		marketResult := result.PropResults[pred.Market]
		marketResult.TotalPicks++
		marketResult.AvgProjectedLine += pred.OurLine
		marketResult.AvgActualValue += pred.ActualValue
		marketResult.AvgEdge += pred.Edge

		switch pred.Result {
		case types.PropResultWin:
			marketResult.Wins++
			result.TotalWins++
		case types.PropResultLoss:
			marketResult.Losses++
			result.TotalLosses++
		case types.PropResultPush:
			marketResult.Pushes++
			result.TotalPushes++
		}

		// By tier
		tierResult := result.TierResults[pred.Tier]
		if tierResult != nil {
			tierResult.TotalPicks++
			tierResult.AvgEdge += pred.Edge
			switch pred.Result {
			case types.PropResultWin:
				tierResult.Wins++
			case types.PropResultLoss:
				tierResult.Losses++
			}
		}

		result.TotalPicks++
		result.AvgEdge += pred.Edge
	}
}

func (b *Backtester) calculateFinalMetrics(result *BacktestResult) {
	// Scenario accuracy
	for _, sr := range result.ScenarioResults {
		if sr.Predicted > 0 {
			sr.Accuracy = float64(sr.Correct) / float64(sr.Predicted) * 100
		}
	}

	// Market hit rates
	for _, mr := range result.PropResults {
		if mr.TotalPicks > 0 {
			mr.HitRate = float64(mr.Wins) / float64(mr.Wins+mr.Losses) * 100
			mr.AvgProjectedLine /= float64(mr.TotalPicks)
			mr.AvgActualValue /= float64(mr.TotalPicks)
			mr.AvgEdge /= float64(mr.TotalPicks)
		}
	}

	// Tier hit rates
	for _, tr := range result.TierResults {
		if tr.TotalPicks > 0 {
			tr.HitRate = float64(tr.Wins) / float64(tr.Wins+tr.Losses) * 100
			tr.AvgEdge /= float64(tr.TotalPicks)
		}
	}

	// Overall
	if result.TotalPicks > 0 {
		result.OverallHitRate = float64(result.TotalWins) / float64(result.TotalWins+result.TotalLosses) * 100
		result.AvgEdge /= float64(result.TotalPicks)
	}
}

// FormatResults returns a formatted string of backtest results
func (result *BacktestResult) FormatResults() string {
	var output string

	output += "═══════════════════════════════════════════════════════════════\n"
	output += "                    NHL PROP MODEL BACKTEST RESULTS\n"
	output += "═══════════════════════════════════════════════════════════════\n\n"

	output += fmt.Sprintf("Period: %s to %s\n", result.StartDate.Format("2006-01-02"), result.EndDate.Format("2006-01-02"))
	output += fmt.Sprintf("Games Analyzed: %d\n", result.GamesAnalyzed)
	output += fmt.Sprintf("Total Props Evaluated: %d\n\n", result.TotalPicks)

	output += "───────────────────────────────────────────────────────────────\n"
	output += "                      OVERALL PERFORMANCE\n"
	output += "───────────────────────────────────────────────────────────────\n"
	output += fmt.Sprintf("Win/Loss/Push: %d / %d / %d\n", result.TotalWins, result.TotalLosses, result.TotalPushes)
	output += fmt.Sprintf("Overall Hit Rate: %.1f%%\n", result.OverallHitRate)
	output += fmt.Sprintf("Average Edge: %.2f pp\n\n", result.AvgEdge)

	output += "───────────────────────────────────────────────────────────────\n"
	output += "                     SCENARIO ACCURACY\n"
	output += "───────────────────────────────────────────────────────────────\n"
	output += fmt.Sprintf("%-20s %10s %10s %10s\n", "Scenario", "Predicted", "Correct", "Accuracy")
	output += "───────────────────────────────────────────────────────────────\n"

	for _, scenario := range []types.ScenarioType{
		types.ScenarioRunAndGun,
		types.ScenarioGoalieDuel,
		types.ScenarioSpecialTeams,
		types.ScenarioScoreEffects,
		types.ScenarioBenchSlog,
	} {
		sr := result.ScenarioResults[scenario]
		if sr != nil && sr.Predicted > 0 {
			output += fmt.Sprintf("%-20s %10d %10d %9.1f%%\n", scenario, sr.Predicted, sr.Correct, sr.Accuracy)
		}
	}

	output += "\n───────────────────────────────────────────────────────────────\n"
	output += "                   RESULTS BY PROP MARKET\n"
	output += "───────────────────────────────────────────────────────────────\n"
	output += fmt.Sprintf("%-15s %8s %8s %8s %8s %8s\n", "Market", "Total", "Wins", "Losses", "HitRate", "AvgEdge")
	output += "───────────────────────────────────────────────────────────────\n"

	markets := []types.PropMarket{
		types.PropMarketPointsOver,
		types.PropMarketSOGOver,
		types.PropMarketGoalsOver,
		types.PropMarketAssistsOver,
		types.PropMarketHitsOver,
		types.PropMarketBlocksOver,
	}

	for _, market := range markets {
		mr := result.PropResults[market]
		if mr != nil && mr.TotalPicks > 0 {
			output += fmt.Sprintf("%-15s %8d %8d %8d %7.1f%% %8.2f\n",
				market, mr.TotalPicks, mr.Wins, mr.Losses, mr.HitRate, mr.AvgEdge)
		}
	}

	output += "\n───────────────────────────────────────────────────────────────\n"
	output += "                      RESULTS BY TIER\n"
	output += "───────────────────────────────────────────────────────────────\n"
	output += fmt.Sprintf("%-10s %10s %10s %10s %10s %10s\n", "Tier", "Total", "Wins", "Losses", "HitRate", "AvgEdge")
	output += "───────────────────────────────────────────────────────────────\n"

	for _, tier := range []types.PropTier{types.PropTier1, types.PropTier1_5, types.PropTier2, types.PropTier3} {
		tr := result.TierResults[tier]
		if tr != nil && tr.TotalPicks > 0 {
			tierName := fmt.Sprintf("Tier %d", tier)
			if tier == types.PropTier1_5 {
				tierName = "Tier 1.5"
			}
			output += fmt.Sprintf("%-10s %10d %10d %10d %9.1f%% %10.2f\n",
				tierName, tr.TotalPicks, tr.Wins, tr.Losses, tr.HitRate, tr.AvgEdge)
		}
	}

	if len(result.Errors) > 0 {
		output += "\n───────────────────────────────────────────────────────────────\n"
		output += "                         ERRORS\n"
		output += "───────────────────────────────────────────────────────────────\n"
		for i, err := range result.Errors {
			if i >= 10 {
				output += fmt.Sprintf("... and %d more errors\n", len(result.Errors)-10)
				break
			}
			output += fmt.Sprintf("• %s\n", err)
		}
	}

	output += "\n═══════════════════════════════════════════════════════════════\n"

	return output
}
