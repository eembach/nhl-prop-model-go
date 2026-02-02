package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/internal/model/props"
	"github.com/paul/nhl-prop-model/internal/model/scenario"
	"github.com/paul/nhl-prop-model/internal/storage"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Message types

type gamesLoadedMsg struct {
	games []types.Game
	err   error
}

type scenarioLoadedMsg struct {
	scenario *types.GameScenario
	err      error
}

type propsEvaluatedMsg struct {
	evals []*types.PropEvaluation
	err   error
}

type parlaysBuiltMsg struct {
	parlays []*types.Parlay
	err     error
}

type picksLoadedMsg struct {
	picks []types.PropPick
	err   error
}

type errMsg struct {
	err error
}

// Commands

func fetchGames(client *nhl.Client) tea.Cmd {
	return func() tea.Msg {
		games, err := client.GetTodaySchedule()
		return gamesLoadedMsg{games: games, err: err}
	}
}

func analyzeGame(game *types.Game, eng *scenario.Engine, client *nhl.Client) tea.Cmd {
	return func() tea.Msg {
		// In a full implementation, this would:
		// 1. Fetch team stats for both teams
		// 2. Fetch starting goalie stats
		// 3. Build game context
		// 4. Run scenario engine

		// For now, create mock stats for demonstration
		homeStats := &types.TeamStats{
			GFPerGame:    3.2,
			GAPerGame:    2.8,
			SFPerGame:    32.5,
			SAPerGame:    28.3,
			PPPct:        22.5,
			PKPct:        81.2,
			CF60:         55.8,
			CA60:         52.3,
			PIMPerGame:   8.5,
		}

		awayStats := &types.TeamStats{
			GFPerGame:    2.9,
			GAPerGame:    3.1,
			SFPerGame:    30.2,
			SAPerGame:    31.5,
			PPPct:        19.8,
			PKPct:        78.5,
			CF60:         52.1,
			CA60:         55.2,
			PIMPerGame:   9.2,
		}

		homeGoalie := &types.GoalieStats{
			GSAx:       5.2,
			SavePct:    0.915,
			GAA:        2.45,
		}

		awayGoalie := &types.GoalieStats{
			GSAx:       -2.1,
			SavePct:    0.901,
			GAA:        3.12,
		}

		ctx := &types.GameContext{
			GameID:          game.ID,
			TotalLine:       6.0,
			ImpliedHomeWin:  0.55,
			ImpliedAwayWin:  0.45,
			HomeMoneyline:   -130,
			AwayMoneyline:   110,
		}

		scenario := eng.ClassifyGame(homeStats, awayStats, homeGoalie, awayGoalie, ctx)

		return scenarioLoadedMsg{scenario: scenario}
	}
}

func evaluateProps(game *types.Game, gameScenario *types.GameScenario, evaluator *props.Evaluator, client *nhl.Client) tea.Cmd {
	return func() tea.Msg {
		// In a full implementation, this would:
		// 1. Fetch rosters for both teams
		// 2. Fetch player stats for all skaters
		// 3. Create prop lines (from odds API or manual input)
		// 4. Evaluate all props

		// For demonstration, create mock evaluations
		mockPlayers := []struct {
			id       int
			name     string
			team     string
			position types.Position
		}{
			{8478402, "Connor McDavid", "EDM", types.PositionCenter},
			{8477934, "Leon Draisaitl", "EDM", types.PositionCenter},
			{8477492, "Nathan MacKinnon", "COL", types.PositionCenter},
			{8480069, "Cale Makar", "COL", types.PositionDefense},
			{8478550, "Mikko Rantanen", "COL", types.PositionRight},
		}

		var evals []*types.PropEvaluation

		for _, p := range mockPlayers {
			// Create mock player stats
			player := &types.PlayerWithStats{
				Player: types.Player{
					ID:       p.id,
					Name:     p.name,
					TeamAbbr: p.team,
					Position: p.position,
				},
				Stats: types.PlayerStats{
					PlayerID:      p.id,
					GamesPlayed:   45,
					TOI:           21.5,
					SOG:           3.8,
					RecentSOG:     4.2,
					PointsPerGame: 1.35,
					RecentPPG:     1.5,
					IsTopSix:      true,
					IsTopFour:     p.position == types.PositionDefense,
					PP1Unit:       true,
					PPTOI:         4.2,
				},
			}

			// Create mock prop line
			line := types.PropLine{
				GameID:      game.ID,
				PlayerID:    p.id,
				PlayerName:  p.name,
				TeamAbbr:    p.team,
				Market:      types.PropMarketSOGOver,
				Timeframe:   types.TimeframeFullGame,
				Line:        3.5,
				Side:        types.PropSideOver,
				Price:       -115,
				ImpliedProb: 0.535,
			}

			ctx := distributions.DistributionContext{
				TeamPace:      1.05,
				TeamTotal:     3.2,
				ExpectedTOI:   22.0,
				ExpectedPPTOI: 4.5,
				PP1Unit:       true,
				ArenaSOGBias:  1.02,
				ScenarioType:  gameScenario.Primary.Scenario,
			}

			eval := evaluator.EvaluateProp(line, player, ctx, gameScenario)
			evals = append(evals, eval)

			// Also add points prop
			pointsLine := line
			pointsLine.Market = types.PropMarketPointsOver
			pointsLine.Line = 0.5
			pointsLine.Price = -180
			pointsLine.ImpliedProb = 0.643

			pointsEval := evaluator.EvaluateProp(pointsLine, player, ctx, gameScenario)
			evals = append(evals, pointsEval)
		}

		return propsEvaluatedMsg{evals: evals}
	}
}

func buildParlays(evals []*types.PropEvaluation, gameScenario *types.GameScenario, site types.Site, builder *props.ParlayBuilder) tea.Cmd {
	return func() tea.Msg {
		if gameScenario == nil || len(evals) == 0 {
			return parlaysBuiltMsg{parlays: nil}
		}

		parlays, err := builder.BuildScenarioPureCards(evals, gameScenario, site)
		if err != nil {
			return errMsg{err: err}
		}

		return parlaysBuiltMsg{parlays: parlays}
	}
}

func loadPicks(store *storage.PickStore) tea.Cmd {
	return func() tea.Msg {
		// Load recent picks
		picks, err := store.GetUngraded()
		if err != nil {
			return errMsg{err: err}
		}
		return picksLoadedMsg{picks: picks}
	}
}

func loadUngradedPicks(store *storage.PickStore) tea.Cmd {
	return func() tea.Msg {
		picks, err := store.GetUngraded()
		if err != nil {
			return errMsg{err: err}
		}
		return picksLoadedMsg{picks: picks}
	}
}

func gradePick(store *storage.PickStore, pickID int64, result types.PropResult, actualValue, closeLine float64, closePrice int, scenarioHit bool) tea.Cmd {
	return func() tea.Msg {
		err := store.Grade(pickID, result, actualValue, closeLine, closePrice, scenarioHit)
		if err != nil {
			return errMsg{err: err}
		}
		return picksLoadedMsg{} // Trigger reload
	}
}

func savePick(store *storage.PickStore, pick *types.PropPick) tea.Cmd {
	return func() tea.Msg {
		err := store.Create(pick)
		if err != nil {
			return errMsg{err: err}
		}
		return picksLoadedMsg{}
	}
}

func saveParlay(store *storage.ParlayStore, parlay *types.Parlay) tea.Cmd {
	return func() tea.Msg {
		err := store.Create(parlay)
		if err != nil {
			return errMsg{err: err}
		}
		return parlaysBuiltMsg{}
	}
}
