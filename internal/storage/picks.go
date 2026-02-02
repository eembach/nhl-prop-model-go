package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// PickStore handles pick persistence
type PickStore struct {
	db *DB
}

// NewPickStore creates a new PickStore
func NewPickStore(db *DB) *PickStore {
	return &PickStore{db: db}
}

// Create inserts a new pick
func (s *PickStore) Create(pick *types.PropPick) error {
	result, err := s.db.conn.Exec(`
		INSERT INTO picks (
			game_id, player_id, player_name, team_abbr, market, timeframe, side,
			line, open_line, close_line, open_price, close_price,
			our_line, prob_hit, edge, tier, stars, scenario_tag,
			result, actual_value, clv, scenario_hit, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		pick.GameID, pick.PlayerID, pick.PlayerName, pick.TeamAbbr,
		pick.Market, pick.Timeframe, pick.Side,
		pick.Line, pick.OpenLine, pick.CloseLine, pick.OpenPrice, pick.ClosePrice,
		pick.OurLine, pick.ProbHit, pick.Edge, pick.Tier, pick.Stars, pick.ScenarioTag,
		pick.Result, pick.ActualValue, pick.CLV, pick.ScenarioHit, nowUTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert pick: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	pick.ID = id
	return nil
}

// GetByID retrieves a pick by ID
func (s *PickStore) GetByID(id int64) (*types.PropPick, error) {
	var pick types.PropPick
	var gradedAt sql.NullTime
	var result sql.NullString

	err := s.db.conn.QueryRow(`
		SELECT id, game_id, player_id, player_name, team_abbr, market, timeframe, side,
			line, open_line, close_line, open_price, close_price,
			our_line, prob_hit, edge, tier, stars, scenario_tag,
			result, actual_value, clv, scenario_hit, created_at, graded_at
		FROM picks WHERE id = ?
	`, id).Scan(
		&pick.ID, &pick.GameID, &pick.PlayerID, &pick.PlayerName, &pick.TeamAbbr,
		&pick.Market, &pick.Timeframe, &pick.Side,
		&pick.Line, &pick.OpenLine, &pick.CloseLine, &pick.OpenPrice, &pick.ClosePrice,
		&pick.OurLine, &pick.ProbHit, &pick.Edge, &pick.Tier, &pick.Stars, &pick.ScenarioTag,
		&result, &pick.ActualValue, &pick.CLV, &pick.ScenarioHit, &pick.CreatedAt, &gradedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if result.Valid {
		pick.Result = types.PropResult(result.String)
	}
	if gradedAt.Valid {
		pick.GradedAt = &gradedAt.Time
	}

	return &pick, nil
}

// GetByGame retrieves all picks for a game
func (s *PickStore) GetByGame(gameID int) ([]types.PropPick, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, game_id, player_id, player_name, team_abbr, market, timeframe, side,
			line, open_line, close_line, open_price, close_price,
			our_line, prob_hit, edge, tier, stars, scenario_tag,
			result, actual_value, clv, scenario_hit, created_at, graded_at
		FROM picks WHERE game_id = ?
		ORDER BY created_at
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPicks(rows)
}

// GetUngraded retrieves all ungraded picks
func (s *PickStore) GetUngraded() ([]types.PropPick, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, game_id, player_id, player_name, team_abbr, market, timeframe, side,
			line, open_line, close_line, open_price, close_price,
			our_line, prob_hit, edge, tier, stars, scenario_tag,
			result, actual_value, clv, scenario_hit, created_at, graded_at
		FROM picks WHERE result IS NULL OR result = ''
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPicks(rows)
}

// GetByDateRange retrieves picks within a date range
func (s *PickStore) GetByDateRange(start, end time.Time) ([]types.PropPick, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, game_id, player_id, player_name, team_abbr, market, timeframe, side,
			line, open_line, close_line, open_price, close_price,
			our_line, prob_hit, edge, tier, stars, scenario_tag,
			result, actual_value, clv, scenario_hit, created_at, graded_at
		FROM picks WHERE created_at BETWEEN ? AND ?
		ORDER BY created_at
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPicks(rows)
}

// Grade updates a pick with its result
func (s *PickStore) Grade(id int64, result types.PropResult, actualValue, closeLine float64, closePrice int, scenarioHit bool) error {
	now := nowUTC()
	clv := closeLine - actualValue // Simplified CLV calculation

	_, err := s.db.conn.Exec(`
		UPDATE picks SET
			result = ?,
			actual_value = ?,
			close_line = ?,
			close_price = ?,
			clv = ?,
			scenario_hit = ?,
			graded_at = ?
		WHERE id = ?
	`, result, actualValue, closeLine, closePrice, clv, scenarioHit, now, id)

	return err
}

// GetStats returns aggregate statistics for picks
func (s *PickStore) GetStats(market string, tier int) (*types.PropFamilyStats, error) {
	var stats types.PropFamilyStats
	stats.Market = types.PropMarket(market)
	stats.Tier = types.PropTier(tier)

	err := s.db.conn.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN result = 'W' THEN 1 ELSE 0 END) as wins,
			SUM(CASE WHEN result = 'L' THEN 1 ELSE 0 END) as losses,
			SUM(CASE WHEN result = 'P' THEN 1 ELSE 0 END) as pushes,
			AVG(edge) as avg_edge,
			AVG(clv) as avg_clv
		FROM picks
		WHERE market = ? AND tier = ? AND result IS NOT NULL
	`, market, tier).Scan(
		&stats.TotalPicks, &stats.Wins, &stats.Losses, &stats.Pushes,
		&stats.AvgEdge, &stats.AvgCLV,
	)
	if err != nil {
		return nil, err
	}

	if stats.TotalPicks > 0 {
		stats.HitRate = float64(stats.Wins) / float64(stats.Wins+stats.Losses)
	}

	return &stats, nil
}

func scanPicks(rows *sql.Rows) ([]types.PropPick, error) {
	var picks []types.PropPick

	for rows.Next() {
		var pick types.PropPick
		var gradedAt sql.NullTime
		var result sql.NullString

		err := rows.Scan(
			&pick.ID, &pick.GameID, &pick.PlayerID, &pick.PlayerName, &pick.TeamAbbr,
			&pick.Market, &pick.Timeframe, &pick.Side,
			&pick.Line, &pick.OpenLine, &pick.CloseLine, &pick.OpenPrice, &pick.ClosePrice,
			&pick.OurLine, &pick.ProbHit, &pick.Edge, &pick.Tier, &pick.Stars, &pick.ScenarioTag,
			&result, &pick.ActualValue, &pick.CLV, &pick.ScenarioHit, &pick.CreatedAt, &gradedAt,
		)
		if err != nil {
			return nil, err
		}

		if result.Valid {
			pick.Result = types.PropResult(result.String)
		}
		if gradedAt.Valid {
			pick.GradedAt = &gradedAt.Time
		}

		picks = append(picks, pick)
	}

	return picks, rows.Err()
}
