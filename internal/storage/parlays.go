package storage

import (
	"database/sql"
	"fmt"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// ParlayStore handles parlay persistence
type ParlayStore struct {
	db *DB
}

// NewParlayStore creates a new ParlayStore
func NewParlayStore(db *DB) *ParlayStore {
	return &ParlayStore{db: db}
}

// Create inserts a new parlay with its legs
func (s *ParlayStore) Create(parlay *types.Parlay) error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			INSERT INTO parlays (
				site, scenario_tag, joint_prob, implied_prob, payout, ev, ev_gate,
				stars, correlation_valid, result, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			parlay.Site, parlay.ScenarioTag, parlay.JointProb, parlay.ImpliedProb,
			parlay.Payout, parlay.EV, parlay.EVGate, parlay.Stars,
			parlay.CorrelationValid, parlay.Result, nowUTC(),
		)
		if err != nil {
			return fmt.Errorf("failed to insert parlay: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		parlay.ID = id

		// Insert legs
		for i, leg := range parlay.Legs {
			_, err := tx.Exec(`
				INSERT INTO parlay_legs (parlay_id, pick_id, leg_order)
				VALUES (?, ?, ?)
			`, id, leg.PickID, i+1)
			if err != nil {
				return fmt.Errorf("failed to insert parlay leg: %w", err)
			}
		}

		return nil
	})
}

// GetByID retrieves a parlay with its legs
func (s *ParlayStore) GetByID(id int64) (*types.Parlay, error) {
	var parlay types.Parlay
	var gradedAt sql.NullTime

	err := s.db.conn.QueryRow(`
		SELECT id, site, scenario_tag, joint_prob, implied_prob, payout, ev, ev_gate,
			stars, correlation_valid, result, legs_won, legs_pushed, legs_lost,
			created_at, graded_at
		FROM parlays WHERE id = ?
	`, id).Scan(
		&parlay.ID, &parlay.Site, &parlay.ScenarioTag, &parlay.JointProb,
		&parlay.ImpliedProb, &parlay.Payout, &parlay.EV, &parlay.EVGate,
		&parlay.Stars, &parlay.CorrelationValid, &parlay.Result,
		&parlay.LegsWon, &parlay.LegsPushed, &parlay.LegsLost,
		&parlay.CreatedAt, &gradedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if gradedAt.Valid {
		parlay.GradedAt = &gradedAt.Time
	}

	// Load legs
	legs, err := s.getLegs(id)
	if err != nil {
		return nil, err
	}
	parlay.Legs = legs

	return &parlay, nil
}

// GetUngraded retrieves all ungraded parlays
func (s *ParlayStore) GetUngraded() ([]types.Parlay, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, site, scenario_tag, joint_prob, implied_prob, payout, ev, ev_gate,
			stars, correlation_valid, result, legs_won, legs_pushed, legs_lost,
			created_at, graded_at
		FROM parlays WHERE result = 'pending'
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parlays []types.Parlay
	for rows.Next() {
		var parlay types.Parlay
		var gradedAt sql.NullTime

		err := rows.Scan(
			&parlay.ID, &parlay.Site, &parlay.ScenarioTag, &parlay.JointProb,
			&parlay.ImpliedProb, &parlay.Payout, &parlay.EV, &parlay.EVGate,
			&parlay.Stars, &parlay.CorrelationValid, &parlay.Result,
			&parlay.LegsWon, &parlay.LegsPushed, &parlay.LegsLost,
			&parlay.CreatedAt, &gradedAt,
		)
		if err != nil {
			return nil, err
		}

		if gradedAt.Valid {
			parlay.GradedAt = &gradedAt.Time
		}

		legs, err := s.getLegs(parlay.ID)
		if err != nil {
			return nil, err
		}
		parlay.Legs = legs

		parlays = append(parlays, parlay)
	}

	return parlays, rows.Err()
}

// Grade updates a parlay with its result
func (s *ParlayStore) Grade(id int64, result types.ParlayResult, won, pushed, lost int) error {
	now := nowUTC()

	_, err := s.db.conn.Exec(`
		UPDATE parlays SET
			result = ?,
			legs_won = ?,
			legs_pushed = ?,
			legs_lost = ?,
			graded_at = ?
		WHERE id = ?
	`, result, won, pushed, lost, now, id)

	return err
}

// GetStats returns aggregate statistics for parlays
func (s *ParlayStore) GetStats(site types.Site, legCount int) (*types.ParlayStats, error) {
	var stats types.ParlayStats
	stats.Site = site
	stats.LegCount = legCount

	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN result = 'win' THEN 1 ELSE 0 END) as wins,
			SUM(CASE WHEN result = 'loss' THEN 1 ELSE 0 END) as losses,
			SUM(CASE WHEN result = 'push' THEN 1 ELSE 0 END) as pushes,
			AVG(ev) as avg_ev
		FROM parlays
		WHERE site = ? AND result != 'pending'
	`

	args := []interface{}{site}

	if legCount > 0 {
		query += ` AND (SELECT COUNT(*) FROM parlay_legs WHERE parlay_id = parlays.id) = ?`
		args = append(args, legCount)
	}

	err := s.db.conn.QueryRow(query, args...).Scan(
		&stats.TotalParlays, &stats.Wins, &stats.Losses, &stats.Pushes, &stats.AvgEV,
	)
	if err != nil {
		return nil, err
	}

	if stats.TotalParlays > 0 {
		stats.HitRate = float64(stats.Wins) / float64(stats.Wins+stats.Losses)
	}

	return &stats, nil
}

func (s *ParlayStore) getLegs(parlayID int64) ([]types.ParlayLeg, error) {
	rows, err := s.db.conn.Query(`
		SELECT pl.id, pl.parlay_id, pl.pick_id, pl.leg_order, pl.result, pl.actual_value,
			p.player_id, p.player_name, p.team_abbr, p.game_id, p.market, p.timeframe,
			p.side, p.line, p.open_price, p.prob_hit, p.edge
		FROM parlay_legs pl
		JOIN picks p ON pl.pick_id = p.id
		WHERE pl.parlay_id = ?
		ORDER BY pl.leg_order
	`, parlayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var legs []types.ParlayLeg
	for rows.Next() {
		var leg types.ParlayLeg
		var result sql.NullString
		var actualValue sql.NullFloat64
		var order int

		err := rows.Scan(
			&leg.ID, &leg.ParlayID, &leg.PickID, &order, &result, &actualValue,
			&leg.PlayerID, &leg.PlayerName, &leg.TeamAbbr, &leg.GameID,
			&leg.Market, &leg.Timeframe, &leg.Side, &leg.Line, &leg.Price,
			&leg.ProbHit, &leg.Edge,
		)
		if err != nil {
			return nil, err
		}

		if result.Valid {
			leg.Result = types.PropResult(result.String)
		}
		if actualValue.Valid {
			leg.ActualValue = actualValue.Float64
		}

		legs = append(legs, leg)
	}

	return legs, rows.Err()
}
