package storage

import (
	"database/sql"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// CalibrationStore handles calibration data persistence
type CalibrationStore struct {
	db *DB
}

// NewCalibrationStore creates a new CalibrationStore
func NewCalibrationStore(db *DB) *CalibrationStore {
	return &CalibrationStore{db: db}
}

// UpsertArenaBias inserts or updates arena bias
func (s *CalibrationStore) UpsertArenaBias(bias *types.ArenaBias) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO arena_bias (arena_name, team_abbr, sog_bias, hits_bias, blocks_bias, sample_games, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(arena_name) DO UPDATE SET
			team_abbr = excluded.team_abbr,
			sog_bias = excluded.sog_bias,
			hits_bias = excluded.hits_bias,
			blocks_bias = excluded.blocks_bias,
			sample_games = excluded.sample_games,
			updated_at = excluded.updated_at
	`, bias.ArenaName, bias.TeamAbbr, bias.SOGBias, bias.HitsBias, bias.BlocksBias, bias.SampleGames, nowUTC())
	return err
}

// GetArenaBias retrieves arena bias by name
func (s *CalibrationStore) GetArenaBias(arenaName string) (*types.ArenaBias, error) {
	var bias types.ArenaBias

	err := s.db.conn.QueryRow(`
		SELECT arena_name, team_abbr, sog_bias, hits_bias, blocks_bias, sample_games
		FROM arena_bias WHERE arena_name = ?
	`, arenaName).Scan(
		&bias.ArenaName, &bias.TeamAbbr, &bias.SOGBias, &bias.HitsBias,
		&bias.BlocksBias, &bias.SampleGames,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &bias, nil
}

// GetAllArenaBiases retrieves all arena biases
func (s *CalibrationStore) GetAllArenaBiases() ([]types.ArenaBias, error) {
	rows, err := s.db.conn.Query(`
		SELECT arena_name, team_abbr, sog_bias, hits_bias, blocks_bias, sample_games
		FROM arena_bias ORDER BY arena_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var biases []types.ArenaBias
	for rows.Next() {
		var bias types.ArenaBias
		err := rows.Scan(
			&bias.ArenaName, &bias.TeamAbbr, &bias.SOGBias, &bias.HitsBias,
			&bias.BlocksBias, &bias.SampleGames,
		)
		if err != nil {
			return nil, err
		}
		biases = append(biases, bias)
	}

	return biases, rows.Err()
}

// UpsertRefBias inserts or updates ref bias
func (s *CalibrationStore) UpsertRefBias(bias *types.RefBias) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO ref_bias (ref_name, pp_rate_bias, pim_rate_bias, games_worked, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(ref_name) DO UPDATE SET
			pp_rate_bias = excluded.pp_rate_bias,
			pim_rate_bias = excluded.pim_rate_bias,
			games_worked = excluded.games_worked,
			updated_at = excluded.updated_at
	`, bias.RefName, bias.PPRateBias, bias.PIMRateBias, bias.GamesWorked, nowUTC())
	return err
}

// GetRefBias retrieves ref bias by name
func (s *CalibrationStore) GetRefBias(refName string) (*types.RefBias, error) {
	var bias types.RefBias

	err := s.db.conn.QueryRow(`
		SELECT ref_name, pp_rate_bias, pim_rate_bias, games_worked
		FROM ref_bias WHERE ref_name = ?
	`, refName).Scan(
		&bias.RefName, &bias.PPRateBias, &bias.PIMRateBias, &bias.GamesWorked,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &bias, nil
}

// UpsertFatiguePenalty inserts or updates fatigue penalty
func (s *CalibrationStore) UpsertFatiguePenalty(penalty *types.FatiguePenalty) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO fatigue_penalties (situation, sog_penalty, points_penalty, goals_penalty, saves_penalty, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation) DO UPDATE SET
			sog_penalty = excluded.sog_penalty,
			points_penalty = excluded.points_penalty,
			goals_penalty = excluded.goals_penalty,
			saves_penalty = excluded.saves_penalty,
			updated_at = excluded.updated_at
	`, penalty.Situation, penalty.SOGPenalty, penalty.PointsPenalty, penalty.GoalsPenalty, penalty.SavesPenalty, nowUTC())
	return err
}

// GetFatiguePenalty retrieves fatigue penalty by situation
func (s *CalibrationStore) GetFatiguePenalty(situation string) (*types.FatiguePenalty, error) {
	var penalty types.FatiguePenalty

	err := s.db.conn.QueryRow(`
		SELECT situation, sog_penalty, points_penalty, goals_penalty, saves_penalty
		FROM fatigue_penalties WHERE situation = ?
	`, situation).Scan(
		&penalty.Situation, &penalty.SOGPenalty, &penalty.PointsPenalty,
		&penalty.GoalsPenalty, &penalty.SavesPenalty,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &penalty, nil
}

// UpsertPropFamilyStats inserts or updates prop family statistics
func (s *CalibrationStore) UpsertPropFamilyStats(stats *types.PropFamilyStats) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO prop_family_stats (market, tier, total_picks, wins, losses, pushes, hit_rate, avg_edge, avg_clv, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(market, tier) DO UPDATE SET
			total_picks = excluded.total_picks,
			wins = excluded.wins,
			losses = excluded.losses,
			pushes = excluded.pushes,
			hit_rate = excluded.hit_rate,
			avg_edge = excluded.avg_edge,
			avg_clv = excluded.avg_clv,
			updated_at = excluded.updated_at
	`, stats.Market, stats.Tier, stats.TotalPicks, stats.Wins, stats.Losses, stats.Pushes,
		stats.HitRate, stats.AvgEdge, stats.AvgCLV, nowUTC())
	return err
}

// GetPropFamilyStats retrieves prop family statistics
func (s *CalibrationStore) GetPropFamilyStats(market types.PropMarket, tier types.PropTier) (*types.PropFamilyStats, error) {
	var stats types.PropFamilyStats

	err := s.db.conn.QueryRow(`
		SELECT market, tier, total_picks, wins, losses, pushes, hit_rate, avg_edge, avg_clv
		FROM prop_family_stats WHERE market = ? AND tier = ?
	`, market, tier).Scan(
		&stats.Market, &stats.Tier, &stats.TotalPicks, &stats.Wins, &stats.Losses,
		&stats.Pushes, &stats.HitRate, &stats.AvgEdge, &stats.AvgCLV,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &stats, nil
}

// GetAllPropFamilyStats retrieves all prop family statistics
func (s *CalibrationStore) GetAllPropFamilyStats() ([]types.PropFamilyStats, error) {
	rows, err := s.db.conn.Query(`
		SELECT market, tier, total_picks, wins, losses, pushes, hit_rate, avg_edge, avg_clv
		FROM prop_family_stats ORDER BY market, tier
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []types.PropFamilyStats
	for rows.Next() {
		var s types.PropFamilyStats
		err := rows.Scan(
			&s.Market, &s.Tier, &s.TotalPicks, &s.Wins, &s.Losses,
			&s.Pushes, &s.HitRate, &s.AvgEdge, &s.AvgCLV,
		)
		if err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// UpsertScenarioAccuracy inserts or updates scenario accuracy
func (s *CalibrationStore) UpsertScenarioAccuracy(acc *types.ScenarioAccuracy) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO scenario_accuracy (scenario, total_games, correct_calls, accuracy, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scenario) DO UPDATE SET
			total_games = excluded.total_games,
			correct_calls = excluded.correct_calls,
			accuracy = excluded.accuracy,
			updated_at = excluded.updated_at
	`, acc.Scenario, acc.TotalGames, acc.CorrectCalls, acc.Accuracy, nowUTC())
	return err
}

// GetScenarioAccuracy retrieves scenario accuracy
func (s *CalibrationStore) GetScenarioAccuracy(scenario types.ScenarioType) (*types.ScenarioAccuracy, error) {
	var acc types.ScenarioAccuracy

	err := s.db.conn.QueryRow(`
		SELECT scenario, total_games, correct_calls, accuracy
		FROM scenario_accuracy WHERE scenario = ?
	`, scenario).Scan(
		&acc.Scenario, &acc.TotalGames, &acc.CorrectCalls, &acc.Accuracy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &acc, nil
}

// GetAllScenarioAccuracy retrieves all scenario accuracies
func (s *CalibrationStore) GetAllScenarioAccuracy() ([]types.ScenarioAccuracy, error) {
	rows, err := s.db.conn.Query(`
		SELECT scenario, total_games, correct_calls, accuracy
		FROM scenario_accuracy ORDER BY accuracy DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accs []types.ScenarioAccuracy
	for rows.Next() {
		var acc types.ScenarioAccuracy
		err := rows.Scan(&acc.Scenario, &acc.TotalGames, &acc.CorrectCalls, &acc.Accuracy)
		if err != nil {
			return nil, err
		}
		accs = append(accs, acc)
	}

	return accs, rows.Err()
}

// UpsertStarCutoffs inserts or updates weekly star cutoffs
func (s *CalibrationStore) UpsertStarCutoffs(week string, market string, cutoffs map[int]float64) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO weekly_star_cutoffs (week, market, star_1_edge, star_2_edge, star_3_edge, star_4_edge, star_5_edge, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(week, market) DO UPDATE SET
			star_1_edge = excluded.star_1_edge,
			star_2_edge = excluded.star_2_edge,
			star_3_edge = excluded.star_3_edge,
			star_4_edge = excluded.star_4_edge,
			star_5_edge = excluded.star_5_edge,
			updated_at = excluded.updated_at
	`, week, market, cutoffs[1], cutoffs[2], cutoffs[3], cutoffs[4], cutoffs[5], nowUTC())
	return err
}

// GetStarCutoffs retrieves weekly star cutoffs
func (s *CalibrationStore) GetStarCutoffs(week string, market string) (map[int]float64, error) {
	var s1, s2, s3, s4, s5 float64

	err := s.db.conn.QueryRow(`
		SELECT star_1_edge, star_2_edge, star_3_edge, star_4_edge, star_5_edge
		FROM weekly_star_cutoffs WHERE week = ? AND market = ?
	`, week, market).Scan(&s1, &s2, &s3, &s4, &s5)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.DefaultStarCutoffs(), nil
		}
		return nil, err
	}

	return map[int]float64{1: s1, 2: s2, 3: s3, 4: s4, 5: s5}, nil
}

// InitializeDefaults populates default calibration data
func (s *CalibrationStore) InitializeDefaults() error {
	// Initialize fatigue penalties
	for _, penalty := range types.DefaultFatiguePenalties() {
		if err := s.UpsertFatiguePenalty(&penalty); err != nil {
			return err
		}
	}

	// Initialize arena biases
	for _, bias := range types.DefaultArenaBiases() {
		if err := s.UpsertArenaBias(&bias); err != nil {
			return err
		}
	}

	return nil
}
