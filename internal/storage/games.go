package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// GameStore handles game persistence
type GameStore struct {
	db *DB
}

// NewGameStore creates a new GameStore
func NewGameStore(db *DB) *GameStore {
	return &GameStore{db: db}
}

// Upsert inserts or updates a game
func (s *GameStore) Upsert(game *types.Game) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO games (
			id, season, game_date, start_time, home_team_id, home_team_abbr,
			away_team_id, away_team_abbr, venue, state,
			home_score, away_score, overtime, shootout, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			state = excluded.state,
			home_score = excluded.home_score,
			away_score = excluded.away_score,
			overtime = excluded.overtime,
			shootout = excluded.shootout,
			updated_at = excluded.updated_at
	`,
		game.ID, game.Season, game.GameDate, game.StartTime,
		game.HomeTeamID, game.HomeTeamAbbr, game.AwayTeamID, game.AwayTeamAbbr,
		game.Venue, game.State, game.HomeScore, game.AwayScore,
		game.Overtime, game.Shootout, nowUTC(),
	)
	return err
}

// GetByID retrieves a game by ID
func (s *GameStore) GetByID(id int) (*types.Game, error) {
	var game types.Game
	var overtime, shootout int

	err := s.db.conn.QueryRow(`
		SELECT id, season, game_date, start_time, home_team_id, home_team_abbr,
			away_team_id, away_team_abbr, venue, state,
			home_score, away_score, overtime, shootout, updated_at
		FROM games WHERE id = ?
	`, id).Scan(
		&game.ID, &game.Season, &game.GameDate, &game.StartTime,
		&game.HomeTeamID, &game.HomeTeamAbbr, &game.AwayTeamID, &game.AwayTeamAbbr,
		&game.Venue, &game.State, &game.HomeScore, &game.AwayScore,
		&overtime, &shootout, &game.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	game.Overtime = overtime == 1
	game.Shootout = shootout == 1
	return &game, nil
}

// GetByDate retrieves all games on a specific date
func (s *GameStore) GetByDate(date time.Time) ([]types.Game, error) {
	dateStr := date.Format("2006-01-02")
	rows, err := s.db.conn.Query(`
		SELECT id, season, game_date, start_time, home_team_id, home_team_abbr,
			away_team_id, away_team_abbr, venue, state,
			home_score, away_score, overtime, shootout, updated_at
		FROM games WHERE DATE(game_date) = ?
		ORDER BY start_time
	`, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanGames(rows)
}

// GetScheduled retrieves all scheduled (not yet played) games
func (s *GameStore) GetScheduled() ([]types.Game, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, season, game_date, start_time, home_team_id, home_team_abbr,
			away_team_id, away_team_abbr, venue, state,
			home_score, away_score, overtime, shootout, updated_at
		FROM games WHERE state = 'scheduled'
		ORDER BY start_time
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanGames(rows)
}

// GetCompleted retrieves completed games that may need grading
func (s *GameStore) GetCompleted(since time.Time) ([]types.Game, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, season, game_date, start_time, home_team_id, home_team_abbr,
			away_team_id, away_team_abbr, venue, state,
			home_score, away_score, overtime, shootout, updated_at
		FROM games WHERE state = 'final' AND game_date >= ?
		ORDER BY game_date DESC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanGames(rows)
}

// UpsertContext inserts or updates game context
func (s *GameStore) UpsertContext(ctx *types.GameContext) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO game_context (
			game_id, home_b2b, away_b2b, home_3in4, away_3in4,
			home_fatigue, away_fatigue, arena_sog_bias, arena_hits_bias,
			ref_crew, ref_pp_rate_bias, home_goalie_id, away_goalie_id,
			home_goalie_confirmed, away_goalie_confirmed,
			home_moneyline, away_moneyline, total_line, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(game_id) DO UPDATE SET
			home_b2b = excluded.home_b2b,
			away_b2b = excluded.away_b2b,
			home_3in4 = excluded.home_3in4,
			away_3in4 = excluded.away_3in4,
			home_fatigue = excluded.home_fatigue,
			away_fatigue = excluded.away_fatigue,
			arena_sog_bias = excluded.arena_sog_bias,
			arena_hits_bias = excluded.arena_hits_bias,
			ref_crew = excluded.ref_crew,
			ref_pp_rate_bias = excluded.ref_pp_rate_bias,
			home_goalie_id = excluded.home_goalie_id,
			away_goalie_id = excluded.away_goalie_id,
			home_goalie_confirmed = excluded.home_goalie_confirmed,
			away_goalie_confirmed = excluded.away_goalie_confirmed,
			home_moneyline = excluded.home_moneyline,
			away_moneyline = excluded.away_moneyline,
			total_line = excluded.total_line,
			updated_at = excluded.updated_at
	`,
		ctx.GameID, ctx.HomeB2B, ctx.AwayB2B, ctx.Home3In4, ctx.Away3In4,
		ctx.HomeFatigue, ctx.AwayFatigue, ctx.ArenaSOGBias, ctx.ArenaHitsBias,
		ctx.RefCrew, ctx.RefPPRateBias, ctx.HomeGoalieID, ctx.AwayGoalieID,
		ctx.HomGoalieConfirmed, ctx.AwayGoalieConfirmed,
		ctx.HomeMoneyline, ctx.AwayMoneyline, ctx.TotalLine, nowUTC(),
	)
	return err
}

// GetContext retrieves game context
func (s *GameStore) GetContext(gameID int) (*types.GameContext, error) {
	var ctx types.GameContext

	err := s.db.conn.QueryRow(`
		SELECT game_id, home_b2b, away_b2b, home_3in4, away_3in4,
			home_fatigue, away_fatigue, arena_sog_bias, arena_hits_bias,
			ref_crew, ref_pp_rate_bias, home_goalie_id, away_goalie_id,
			home_goalie_confirmed, away_goalie_confirmed,
			home_moneyline, away_moneyline, total_line
		FROM game_context WHERE game_id = ?
	`, gameID).Scan(
		&ctx.GameID, &ctx.HomeB2B, &ctx.AwayB2B, &ctx.Home3In4, &ctx.Away3In4,
		&ctx.HomeFatigue, &ctx.AwayFatigue, &ctx.ArenaSOGBias, &ctx.ArenaHitsBias,
		&ctx.RefCrew, &ctx.RefPPRateBias, &ctx.HomeGoalieID, &ctx.AwayGoalieID,
		&ctx.HomGoalieConfirmed, &ctx.AwayGoalieConfirmed,
		&ctx.HomeMoneyline, &ctx.AwayMoneyline, &ctx.TotalLine,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &ctx, nil
}

// UpsertScenario inserts or updates game scenario classification
func (s *GameStore) UpsertScenario(scenario *types.GameScenario) error {
	var secondaryScenario, secondaryScore interface{}
	if scenario.Secondary != nil {
		secondaryScenario = scenario.Secondary.Scenario
		secondaryScore = scenario.Secondary.Score
	}

	_, err := s.db.conn.Exec(`
		INSERT INTO game_scenarios (
			game_id, primary_scenario, primary_score,
			secondary_scenario, secondary_score,
			combined_pace, combined_gsax, combined_fatigue, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(game_id) DO UPDATE SET
			primary_scenario = excluded.primary_scenario,
			primary_score = excluded.primary_score,
			secondary_scenario = excluded.secondary_scenario,
			secondary_score = excluded.secondary_score,
			combined_pace = excluded.combined_pace,
			combined_gsax = excluded.combined_gsax,
			combined_fatigue = excluded.combined_fatigue,
			updated_at = excluded.updated_at
	`,
		scenario.GameID, scenario.Primary.Scenario, scenario.Primary.Score,
		secondaryScenario, secondaryScore,
		scenario.Metrics.CombinedPace, scenario.Metrics.CombinedGSAx,
		scenario.Metrics.CombinedFatigue, nowUTC(),
	)
	return err
}

// GetScenario retrieves game scenario
func (s *GameStore) GetScenario(gameID int) (*types.GameScenario, error) {
	var scenario types.GameScenario
	var secondaryScenario sql.NullString
	var secondaryScore sql.NullFloat64

	err := s.db.conn.QueryRow(`
		SELECT game_id, primary_scenario, primary_score,
			secondary_scenario, secondary_score,
			combined_pace, combined_gsax, combined_fatigue
		FROM game_scenarios WHERE game_id = ?
	`, gameID).Scan(
		&scenario.GameID, &scenario.Primary.Scenario, &scenario.Primary.Score,
		&secondaryScenario, &secondaryScore,
		&scenario.Metrics.CombinedPace, &scenario.Metrics.CombinedGSAx,
		&scenario.Metrics.CombinedFatigue,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if secondaryScenario.Valid {
		scenario.Secondary = &types.ScenarioScore{
			Scenario: types.ScenarioType(secondaryScenario.String),
			Score:    secondaryScore.Float64,
		}
	}

	return &scenario, nil
}

// MarkScenarioResult marks whether scenario prediction was correct
func (s *GameStore) MarkScenarioResult(gameID int, correct bool) error {
	_, err := s.db.conn.Exec(`
		UPDATE game_scenarios SET scenario_correct = ? WHERE game_id = ?
	`, correct, gameID)
	return err
}

// UpsertBoxscore inserts or updates a boxscore
func (s *GameStore) UpsertBoxscore(box *types.Boxscore) error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		// Insert main boxscore record
		_, err := tx.Exec(`
			INSERT INTO boxscores (game_id, home_score, away_score, fetched_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(game_id) DO UPDATE SET
				home_score = excluded.home_score,
				away_score = excluded.away_score,
				fetched_at = excluded.fetched_at
		`, box.GameID, box.HomeScore, box.AwayScore, box.FetchedAt)
		if err != nil {
			return fmt.Errorf("failed to insert boxscore: %w", err)
		}

		// Insert player stats
		for _, ps := range box.PlayerStats {
			_, err := tx.Exec(`
				INSERT INTO boxscore_player_stats (
					game_id, player_id, team_id, toi, goals, assists, points,
					sog, hits, blocks, pim, pp_goals, pp_assists, pp_toi, plus_minus
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(game_id, player_id) DO UPDATE SET
					toi = excluded.toi,
					goals = excluded.goals,
					assists = excluded.assists,
					points = excluded.points,
					sog = excluded.sog,
					hits = excluded.hits,
					blocks = excluded.blocks,
					pim = excluded.pim,
					pp_goals = excluded.pp_goals,
					pp_assists = excluded.pp_assists,
					pp_toi = excluded.pp_toi,
					plus_minus = excluded.plus_minus
			`, box.GameID, ps.PlayerID, ps.TeamID, ps.TOI, ps.Goals, ps.Assists, ps.Points,
				ps.SOG, ps.Hits, ps.Blocks, ps.PIM, ps.PPGoals, ps.PPAssists, ps.PPTOI, ps.PlusMinus)
			if err != nil {
				return fmt.Errorf("failed to insert player stats: %w", err)
			}
		}

		// Insert goalie stats
		for _, gs := range box.GoalieStats {
			_, err := tx.Exec(`
				INSERT INTO boxscore_goalie_stats (
					game_id, player_id, team_id, toi, saves, shots_against,
					goals_against, save_pct, decision
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(game_id, player_id) DO UPDATE SET
					toi = excluded.toi,
					saves = excluded.saves,
					shots_against = excluded.shots_against,
					goals_against = excluded.goals_against,
					save_pct = excluded.save_pct,
					decision = excluded.decision
			`, box.GameID, gs.PlayerID, gs.TeamID, gs.TOI, gs.Saves, gs.ShotsAgainst,
				gs.GoalsAgainst, gs.SavePct, gs.Decision)
			if err != nil {
				return fmt.Errorf("failed to insert goalie stats: %w", err)
			}
		}

		return nil
	})
}

// GetBoxscore retrieves a game's boxscore
func (s *GameStore) GetBoxscore(gameID int) (*types.Boxscore, error) {
	var box types.Boxscore

	err := s.db.conn.QueryRow(`
		SELECT game_id, home_score, away_score, fetched_at
		FROM boxscores WHERE game_id = ?
	`, gameID).Scan(&box.GameID, &box.HomeScore, &box.AwayScore, &box.FetchedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Load player stats
	box.PlayerStats = make(map[int]types.BoxscorePlayerStats)
	rows, err := s.db.conn.Query(`
		SELECT player_id, team_id, toi, goals, assists, points,
			sog, hits, blocks, pim, pp_goals, pp_assists, pp_toi, plus_minus
		FROM boxscore_player_stats WHERE game_id = ?
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ps types.BoxscorePlayerStats
		err := rows.Scan(
			&ps.PlayerID, &ps.TeamID, &ps.TOI, &ps.Goals, &ps.Assists, &ps.Points,
			&ps.SOG, &ps.Hits, &ps.Blocks, &ps.PIM, &ps.PPGoals, &ps.PPAssists,
			&ps.PPTOI, &ps.PlusMinus,
		)
		if err != nil {
			return nil, err
		}
		box.PlayerStats[ps.PlayerID] = ps
	}

	// Load goalie stats
	box.GoalieStats = make(map[int]types.BoxscoreGoalieStats)
	rows, err = s.db.conn.Query(`
		SELECT player_id, team_id, toi, saves, shots_against,
			goals_against, save_pct, decision
		FROM boxscore_goalie_stats WHERE game_id = ?
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var gs types.BoxscoreGoalieStats
		err := rows.Scan(
			&gs.PlayerID, &gs.TeamID, &gs.TOI, &gs.Saves, &gs.ShotsAgainst,
			&gs.GoalsAgainst, &gs.SavePct, &gs.Decision,
		)
		if err != nil {
			return nil, err
		}
		box.GoalieStats[gs.PlayerID] = gs
	}

	return &box, nil
}

func scanGames(rows *sql.Rows) ([]types.Game, error) {
	var games []types.Game

	for rows.Next() {
		var game types.Game
		var overtime, shootout int

		err := rows.Scan(
			&game.ID, &game.Season, &game.GameDate, &game.StartTime,
			&game.HomeTeamID, &game.HomeTeamAbbr, &game.AwayTeamID, &game.AwayTeamAbbr,
			&game.Venue, &game.State, &game.HomeScore, &game.AwayScore,
			&overtime, &shootout, &game.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		game.Overtime = overtime == 1
		game.Shootout = shootout == 1
		games = append(games, game)
	}

	return games, rows.Err()
}
