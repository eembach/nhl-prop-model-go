package storage

import (
	"database/sql"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// PlayerStore handles player persistence
type PlayerStore struct {
	db *DB
}

// NewPlayerStore creates a new PlayerStore
func NewPlayerStore(db *DB) *PlayerStore {
	return &PlayerStore{db: db}
}

// Upsert inserts or updates a player
func (s *PlayerStore) Upsert(player *types.Player) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO players (id, name, first_name, last_name, team_id, team_abbr, position, number, active, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			team_id = excluded.team_id,
			team_abbr = excluded.team_abbr,
			position = excluded.position,
			number = excluded.number,
			active = excluded.active,
			updated_at = excluded.updated_at
	`, player.ID, player.Name, player.FirstName, player.LastName,
		player.TeamID, player.TeamAbbr, player.Position, player.Number,
		player.Active, nowUTC())
	return err
}

// GetByID retrieves a player by ID
func (s *PlayerStore) GetByID(id int) (*types.Player, error) {
	var player types.Player

	err := s.db.conn.QueryRow(`
		SELECT id, name, first_name, last_name, team_id, team_abbr, position, number, active, updated_at
		FROM players WHERE id = ?
	`, id).Scan(
		&player.ID, &player.Name, &player.FirstName, &player.LastName,
		&player.TeamID, &player.TeamAbbr, &player.Position, &player.Number,
		&player.Active, &player.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &player, nil
}

// GetByTeam retrieves all players for a team
func (s *PlayerStore) GetByTeam(teamID int) ([]types.Player, error) {
	rows, err := s.db.conn.Query(`
		SELECT id, name, first_name, last_name, team_id, team_abbr, position, number, active, updated_at
		FROM players WHERE team_id = ? AND active = 1
		ORDER BY position, name
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []types.Player
	for rows.Next() {
		var player types.Player
		err := rows.Scan(
			&player.ID, &player.Name, &player.FirstName, &player.LastName,
			&player.TeamID, &player.TeamAbbr, &player.Position, &player.Number,
			&player.Active, &player.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}

	return players, rows.Err()
}

// UpsertStats inserts or updates player stats
func (s *PlayerStore) UpsertStats(playerID int, season string, stats *types.PlayerStats) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO player_stats (
			player_id, season, games_played, toi, recent_toi, pp_toi,
			sog, recent_sog, goals, assists, points, ppg, recent_ppg,
			pp_goals, pp_assists, pp_points, pp1_unit, shooting_pct,
			hits_per_game, blocks_per_game, pim_per_game,
			line_number, is_top_six, is_top_four,
			xgf60, xga60, oz_start_pct, pdo, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id, season) DO UPDATE SET
			games_played = excluded.games_played,
			toi = excluded.toi,
			recent_toi = excluded.recent_toi,
			pp_toi = excluded.pp_toi,
			sog = excluded.sog,
			recent_sog = excluded.recent_sog,
			goals = excluded.goals,
			assists = excluded.assists,
			points = excluded.points,
			ppg = excluded.ppg,
			recent_ppg = excluded.recent_ppg,
			pp_goals = excluded.pp_goals,
			pp_assists = excluded.pp_assists,
			pp_points = excluded.pp_points,
			pp1_unit = excluded.pp1_unit,
			shooting_pct = excluded.shooting_pct,
			hits_per_game = excluded.hits_per_game,
			blocks_per_game = excluded.blocks_per_game,
			pim_per_game = excluded.pim_per_game,
			line_number = excluded.line_number,
			is_top_six = excluded.is_top_six,
			is_top_four = excluded.is_top_four,
			xgf60 = excluded.xgf60,
			xga60 = excluded.xga60,
			oz_start_pct = excluded.oz_start_pct,
			pdo = excluded.pdo,
			updated_at = excluded.updated_at
	`,
		playerID, season, stats.GamesPlayed, stats.TOI, stats.RecentTOI, stats.PPTOI,
		stats.SOG, stats.RecentSOG, stats.Goals, stats.Assists, stats.Points,
		stats.PointsPerGame, stats.RecentPPG, stats.PPGoals, stats.PPAssists,
		stats.PPPoints, stats.PP1Unit, stats.ShootingPct,
		stats.HitsPerGame, stats.BlocksPerGame, stats.PIMPerGame,
		stats.LineNumber, stats.IsTopSix, stats.IsTopFour,
		stats.XGF60, stats.XGA60, stats.OZStartPct, stats.PDO, nowUTC())
	return err
}

// GetStats retrieves player stats for a season
func (s *PlayerStore) GetStats(playerID int, season string) (*types.PlayerStats, error) {
	var stats types.PlayerStats
	var pp1Unit, isTopSix, isTopFour int

	err := s.db.conn.QueryRow(`
		SELECT player_id, games_played, toi, recent_toi, pp_toi,
			sog, recent_sog, goals, assists, points, ppg, recent_ppg,
			pp_goals, pp_assists, pp_points, pp1_unit, shooting_pct,
			hits_per_game, blocks_per_game, pim_per_game,
			line_number, is_top_six, is_top_four,
			xgf60, xga60, oz_start_pct, pdo, updated_at
		FROM player_stats WHERE player_id = ? AND season = ?
	`, playerID, season).Scan(
		&stats.PlayerID, &stats.GamesPlayed, &stats.TOI, &stats.RecentTOI, &stats.PPTOI,
		&stats.SOG, &stats.RecentSOG, &stats.Goals, &stats.Assists, &stats.Points,
		&stats.PointsPerGame, &stats.RecentPPG, &stats.PPGoals, &stats.PPAssists,
		&stats.PPPoints, &pp1Unit, &stats.ShootingPct,
		&stats.HitsPerGame, &stats.BlocksPerGame, &stats.PIMPerGame,
		&stats.LineNumber, &isTopSix, &isTopFour,
		&stats.XGF60, &stats.XGA60, &stats.OZStartPct, &stats.PDO, &stats.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	stats.PP1Unit = pp1Unit == 1
	stats.IsTopSix = isTopSix == 1
	stats.IsTopFour = isTopFour == 1

	return &stats, nil
}

// UpsertGoalieStats inserts or updates goalie stats
func (s *PlayerStore) UpsertGoalieStats(playerID int, season string, stats *types.GoalieStats) error {
	_, err := s.db.conn.Exec(`
		INSERT INTO goalie_stats (
			player_id, season, games_played, games_started,
			saves, saves_per_game, shots_against, goals_against,
			gaa, save_pct, gsax, quality_starts, is_starter, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id, season) DO UPDATE SET
			games_played = excluded.games_played,
			games_started = excluded.games_started,
			saves = excluded.saves,
			saves_per_game = excluded.saves_per_game,
			shots_against = excluded.shots_against,
			goals_against = excluded.goals_against,
			gaa = excluded.gaa,
			save_pct = excluded.save_pct,
			gsax = excluded.gsax,
			quality_starts = excluded.quality_starts,
			is_starter = excluded.is_starter,
			updated_at = excluded.updated_at
	`,
		playerID, season, stats.GamesPlayed, stats.GamesStarted,
		stats.Saves, stats.SavesPerGame, stats.ShotsAgainst, stats.GoalsAgainst,
		stats.GAA, stats.SavePct, stats.GSAx, stats.QualityStarts,
		stats.IsStarter, nowUTC())
	return err
}

// GetGoalieStats retrieves goalie stats for a season
func (s *PlayerStore) GetGoalieStats(playerID int, season string) (*types.GoalieStats, error) {
	var stats types.GoalieStats
	var isStarter int

	err := s.db.conn.QueryRow(`
		SELECT player_id, games_played, games_started,
			saves, saves_per_game, shots_against, goals_against,
			gaa, save_pct, gsax, quality_starts, is_starter, updated_at
		FROM goalie_stats WHERE player_id = ? AND season = ?
	`, playerID, season).Scan(
		&stats.PlayerID, &stats.GamesPlayed, &stats.GamesStarted,
		&stats.Saves, &stats.SavesPerGame, &stats.ShotsAgainst, &stats.GoalsAgainst,
		&stats.GAA, &stats.SavePct, &stats.GSAx, &stats.QualityStarts,
		&isStarter, &stats.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	stats.IsStarter = isStarter == 1

	return &stats, nil
}

// GetPlayersWithStats retrieves all players with their stats for a team
func (s *PlayerStore) GetPlayersWithStats(teamID int, season string) ([]types.PlayerWithStats, error) {
	rows, err := s.db.conn.Query(`
		SELECT p.id, p.name, p.first_name, p.last_name, p.team_id, p.team_abbr,
			p.position, p.number, p.active, p.updated_at,
			COALESCE(ps.games_played, 0), COALESCE(ps.toi, 0), COALESCE(ps.recent_toi, 0),
			COALESCE(ps.pp_toi, 0), COALESCE(ps.sog, 0), COALESCE(ps.recent_sog, 0),
			COALESCE(ps.goals, 0), COALESCE(ps.assists, 0), COALESCE(ps.points, 0),
			COALESCE(ps.ppg, 0), COALESCE(ps.recent_ppg, 0),
			COALESCE(ps.pp_goals, 0), COALESCE(ps.pp_assists, 0), COALESCE(ps.pp_points, 0),
			COALESCE(ps.pp1_unit, 0), COALESCE(ps.shooting_pct, 0),
			COALESCE(ps.hits_per_game, 0), COALESCE(ps.blocks_per_game, 0), COALESCE(ps.pim_per_game, 0),
			COALESCE(ps.line_number, 0), COALESCE(ps.is_top_six, 0), COALESCE(ps.is_top_four, 0),
			COALESCE(ps.xgf60, 0), COALESCE(ps.xga60, 0), COALESCE(ps.oz_start_pct, 0), COALESCE(ps.pdo, 100)
		FROM players p
		LEFT JOIN player_stats ps ON p.id = ps.player_id AND ps.season = ?
		WHERE p.team_id = ? AND p.active = 1 AND p.position != 'G'
		ORDER BY COALESCE(ps.toi, 0) DESC
	`, season, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []types.PlayerWithStats
	for rows.Next() {
		var pw types.PlayerWithStats
		var pp1Unit, isTopSix, isTopFour int

		err := rows.Scan(
			&pw.Player.ID, &pw.Player.Name, &pw.Player.FirstName, &pw.Player.LastName,
			&pw.Player.TeamID, &pw.Player.TeamAbbr, &pw.Player.Position, &pw.Player.Number,
			&pw.Player.Active, &pw.Player.UpdatedAt,
			&pw.Stats.GamesPlayed, &pw.Stats.TOI, &pw.Stats.RecentTOI, &pw.Stats.PPTOI,
			&pw.Stats.SOG, &pw.Stats.RecentSOG, &pw.Stats.Goals, &pw.Stats.Assists, &pw.Stats.Points,
			&pw.Stats.PointsPerGame, &pw.Stats.RecentPPG,
			&pw.Stats.PPGoals, &pw.Stats.PPAssists, &pw.Stats.PPPoints,
			&pp1Unit, &pw.Stats.ShootingPct,
			&pw.Stats.HitsPerGame, &pw.Stats.BlocksPerGame, &pw.Stats.PIMPerGame,
			&pw.Stats.LineNumber, &isTopSix, &isTopFour,
			&pw.Stats.XGF60, &pw.Stats.XGA60, &pw.Stats.OZStartPct, &pw.Stats.PDO,
		)
		if err != nil {
			return nil, err
		}

		pw.Stats.PlayerID = pw.Player.ID
		pw.Stats.PP1Unit = pp1Unit == 1
		pw.Stats.IsTopSix = isTopSix == 1
		pw.Stats.IsTopFour = isTopFour == 1

		players = append(players, pw)
	}

	return players, rows.Err()
}
