package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
	path string
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn, path: dbPath}

	// Run migrations
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying database connection
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// migrate runs all database migrations
func (db *DB) migrate() error {
	migrations := []string{
		migrationPlayers,
		migrationTeams,
		migrationGames,
		migrationPicks,
		migrationParlays,
		migrationCalibration,
		migrationPropFamilyStats,
		migrationScenarioAccuracy,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

const migrationPlayers = `
CREATE TABLE IF NOT EXISTS players (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	first_name TEXT,
	last_name TEXT,
	team_id INTEGER,
	team_abbr TEXT,
	position TEXT,
	number INTEGER,
	active INTEGER DEFAULT 1,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS player_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	player_id INTEGER NOT NULL,
	season TEXT NOT NULL,
	games_played INTEGER DEFAULT 0,
	toi REAL DEFAULT 0,
	recent_toi REAL DEFAULT 0,
	pp_toi REAL DEFAULT 0,
	sog REAL DEFAULT 0,
	recent_sog REAL DEFAULT 0,
	goals INTEGER DEFAULT 0,
	assists INTEGER DEFAULT 0,
	points INTEGER DEFAULT 0,
	ppg REAL DEFAULT 0,
	recent_ppg REAL DEFAULT 0,
	pp_goals INTEGER DEFAULT 0,
	pp_assists INTEGER DEFAULT 0,
	pp_points INTEGER DEFAULT 0,
	pp1_unit INTEGER DEFAULT 0,
	shooting_pct REAL DEFAULT 0,
	hits_per_game REAL DEFAULT 0,
	blocks_per_game REAL DEFAULT 0,
	pim_per_game REAL DEFAULT 0,
	line_number INTEGER,
	is_top_six INTEGER DEFAULT 0,
	is_top_four INTEGER DEFAULT 0,
	xgf60 REAL DEFAULT 0,
	xga60 REAL DEFAULT 0,
	oz_start_pct REAL DEFAULT 0,
	pdo REAL DEFAULT 100,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(player_id, season)
);

CREATE TABLE IF NOT EXISTS goalie_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	player_id INTEGER NOT NULL,
	season TEXT NOT NULL,
	games_played INTEGER DEFAULT 0,
	games_started INTEGER DEFAULT 0,
	saves INTEGER DEFAULT 0,
	saves_per_game REAL DEFAULT 0,
	shots_against INTEGER DEFAULT 0,
	goals_against INTEGER DEFAULT 0,
	gaa REAL DEFAULT 0,
	save_pct REAL DEFAULT 0,
	gsax REAL DEFAULT 0,
	quality_starts INTEGER DEFAULT 0,
	is_starter INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(player_id, season)
);

CREATE INDEX IF NOT EXISTS idx_players_team ON players(team_id);
CREATE INDEX IF NOT EXISTS idx_player_stats_player ON player_stats(player_id);
CREATE INDEX IF NOT EXISTS idx_goalie_stats_player ON goalie_stats(player_id);
`

const migrationTeams = `
CREATE TABLE IF NOT EXISTS teams (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	abbreviation TEXT NOT NULL UNIQUE,
	division TEXT,
	conference TEXT
);

CREATE TABLE IF NOT EXISTS team_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	season TEXT NOT NULL,
	gf INTEGER DEFAULT 0,
	ga INTEGER DEFAULT 0,
	gf_per_game REAL DEFAULT 0,
	ga_per_game REAL DEFAULT 0,
	sf_per_game REAL DEFAULT 0,
	sa_per_game REAL DEFAULT 0,
	pp_pct REAL DEFAULT 0,
	pk_pct REAL DEFAULT 0,
	pace REAL DEFAULT 0,
	xgf60 REAL DEFAULT 0,
	xga60 REAL DEFAULT 0,
	cf60 REAL DEFAULT 0,
	ca60 REAL DEFAULT 0,
	hits_per_game REAL DEFAULT 0,
	blocks_per_game REAL DEFAULT 0,
	pim_per_game REAL DEFAULT 0,
	wins INTEGER DEFAULT 0,
	losses INTEGER DEFAULT 0,
	ot_losses INTEGER DEFAULT 0,
	points INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(team_id, season)
);

CREATE INDEX IF NOT EXISTS idx_team_stats_team ON team_stats(team_id);
`

const migrationGames = `
CREATE TABLE IF NOT EXISTS games (
	id INTEGER PRIMARY KEY,
	season TEXT NOT NULL,
	game_date DATE NOT NULL,
	start_time DATETIME,
	home_team_id INTEGER NOT NULL,
	home_team_abbr TEXT NOT NULL,
	away_team_id INTEGER NOT NULL,
	away_team_abbr TEXT NOT NULL,
	venue TEXT,
	state TEXT DEFAULT 'scheduled',
	home_score INTEGER,
	away_score INTEGER,
	overtime INTEGER DEFAULT 0,
	shootout INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS game_context (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL UNIQUE,
	home_b2b INTEGER DEFAULT 0,
	away_b2b INTEGER DEFAULT 0,
	home_3in4 INTEGER DEFAULT 0,
	away_3in4 INTEGER DEFAULT 0,
	home_fatigue REAL DEFAULT 0,
	away_fatigue REAL DEFAULT 0,
	arena_sog_bias REAL DEFAULT 1.0,
	arena_hits_bias REAL DEFAULT 1.0,
	ref_crew TEXT,
	ref_pp_rate_bias REAL DEFAULT 1.0,
	home_goalie_id INTEGER,
	away_goalie_id INTEGER,
	home_goalie_confirmed INTEGER DEFAULT 0,
	away_goalie_confirmed INTEGER DEFAULT 0,
	home_moneyline INTEGER,
	away_moneyline INTEGER,
	total_line REAL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS game_scenarios (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	primary_scenario TEXT NOT NULL,
	primary_score REAL NOT NULL,
	secondary_scenario TEXT,
	secondary_score REAL,
	combined_pace REAL,
	combined_gsax REAL,
	combined_fatigue REAL,
	scenario_correct INTEGER,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(game_id)
);

CREATE TABLE IF NOT EXISTS boxscores (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL UNIQUE,
	home_score INTEGER NOT NULL,
	away_score INTEGER NOT NULL,
	fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS boxscore_player_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	player_id INTEGER NOT NULL,
	team_id INTEGER NOT NULL,
	toi REAL DEFAULT 0,
	goals INTEGER DEFAULT 0,
	assists INTEGER DEFAULT 0,
	points INTEGER DEFAULT 0,
	sog INTEGER DEFAULT 0,
	hits INTEGER DEFAULT 0,
	blocks INTEGER DEFAULT 0,
	pim INTEGER DEFAULT 0,
	pp_goals INTEGER DEFAULT 0,
	pp_assists INTEGER DEFAULT 0,
	pp_toi REAL DEFAULT 0,
	plus_minus INTEGER DEFAULT 0,
	UNIQUE(game_id, player_id)
);

CREATE TABLE IF NOT EXISTS boxscore_goalie_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	player_id INTEGER NOT NULL,
	team_id INTEGER NOT NULL,
	toi REAL DEFAULT 0,
	saves INTEGER DEFAULT 0,
	shots_against INTEGER DEFAULT 0,
	goals_against INTEGER DEFAULT 0,
	save_pct REAL DEFAULT 0,
	decision TEXT,
	UNIQUE(game_id, player_id)
);

CREATE INDEX IF NOT EXISTS idx_games_date ON games(game_date);
CREATE INDEX IF NOT EXISTS idx_games_state ON games(state);
CREATE INDEX IF NOT EXISTS idx_boxscore_player_game ON boxscore_player_stats(game_id);
CREATE INDEX IF NOT EXISTS idx_boxscore_goalie_game ON boxscore_goalie_stats(game_id);
`

const migrationPicks = `
CREATE TABLE IF NOT EXISTS picks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	player_id INTEGER NOT NULL,
	player_name TEXT NOT NULL,
	team_abbr TEXT NOT NULL,
	market TEXT NOT NULL,
	timeframe TEXT DEFAULT 'full_game',
	side TEXT NOT NULL,
	line REAL NOT NULL,
	open_line REAL,
	close_line REAL,
	open_price INTEGER,
	close_price INTEGER,
	our_line REAL,
	prob_hit REAL,
	edge REAL,
	tier INTEGER,
	stars INTEGER,
	scenario_tag TEXT,
	result TEXT,
	actual_value REAL,
	clv REAL,
	scenario_hit INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	graded_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_picks_game ON picks(game_id);
CREATE INDEX IF NOT EXISTS idx_picks_player ON picks(player_id);
CREATE INDEX IF NOT EXISTS idx_picks_market ON picks(market);
CREATE INDEX IF NOT EXISTS idx_picks_result ON picks(result);
CREATE INDEX IF NOT EXISTS idx_picks_created ON picks(created_at);
`

const migrationParlays = `
CREATE TABLE IF NOT EXISTS parlays (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	site TEXT NOT NULL,
	scenario_tag TEXT,
	joint_prob REAL,
	implied_prob REAL,
	payout REAL,
	ev REAL,
	ev_gate REAL,
	stars INTEGER,
	correlation_valid INTEGER DEFAULT 1,
	result TEXT DEFAULT 'pending',
	legs_won INTEGER DEFAULT 0,
	legs_pushed INTEGER DEFAULT 0,
	legs_lost INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	graded_at DATETIME
);

CREATE TABLE IF NOT EXISTS parlay_legs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	parlay_id INTEGER NOT NULL,
	pick_id INTEGER NOT NULL,
	leg_order INTEGER NOT NULL,
	result TEXT,
	actual_value REAL,
	FOREIGN KEY (parlay_id) REFERENCES parlays(id) ON DELETE CASCADE,
	FOREIGN KEY (pick_id) REFERENCES picks(id)
);

CREATE INDEX IF NOT EXISTS idx_parlays_site ON parlays(site);
CREATE INDEX IF NOT EXISTS idx_parlays_result ON parlays(result);
CREATE INDEX IF NOT EXISTS idx_parlay_legs_parlay ON parlay_legs(parlay_id);
`

const migrationCalibration = `
CREATE TABLE IF NOT EXISTS calibration (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	category TEXT NOT NULL,
	key TEXT NOT NULL,
	market TEXT,
	bias REAL DEFAULT 1.0,
	sample_size INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(category, key, market)
);

CREATE TABLE IF NOT EXISTS arena_bias (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	arena_name TEXT NOT NULL UNIQUE,
	team_abbr TEXT,
	sog_bias REAL DEFAULT 1.0,
	hits_bias REAL DEFAULT 1.0,
	blocks_bias REAL DEFAULT 1.0,
	sample_games INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ref_bias (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ref_name TEXT NOT NULL UNIQUE,
	pp_rate_bias REAL DEFAULT 1.0,
	pim_rate_bias REAL DEFAULT 1.0,
	games_worked INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fatigue_penalties (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	situation TEXT NOT NULL UNIQUE,
	sog_penalty REAL DEFAULT 0,
	points_penalty REAL DEFAULT 0,
	goals_penalty REAL DEFAULT 0,
	saves_penalty REAL DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationPropFamilyStats = `
CREATE TABLE IF NOT EXISTS prop_family_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	market TEXT NOT NULL,
	tier INTEGER,
	total_picks INTEGER DEFAULT 0,
	wins INTEGER DEFAULT 0,
	losses INTEGER DEFAULT 0,
	pushes INTEGER DEFAULT 0,
	hit_rate REAL DEFAULT 0,
	avg_edge REAL DEFAULT 0,
	avg_clv REAL DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(market, tier)
);

CREATE TABLE IF NOT EXISTS weekly_star_cutoffs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	week TEXT NOT NULL,
	market TEXT NOT NULL,
	star_1_edge REAL DEFAULT 2.0,
	star_2_edge REAL DEFAULT 4.0,
	star_3_edge REAL DEFAULT 6.0,
	star_4_edge REAL DEFAULT 8.0,
	star_5_edge REAL DEFAULT 10.0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(week, market)
);
`

const migrationScenarioAccuracy = `
CREATE TABLE IF NOT EXISTS scenario_accuracy (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	scenario TEXT NOT NULL UNIQUE,
	total_games INTEGER DEFAULT 0,
	correct_calls INTEGER DEFAULT 0,
	accuracy REAL DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// Transaction helpers

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// WithTx executes a function within a transaction
func (db *DB) WithTx(fn func(*sql.Tx) error) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Helper to get current time in UTC
func nowUTC() time.Time {
	return time.Now().UTC()
}
