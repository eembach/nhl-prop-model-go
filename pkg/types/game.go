package types

import "time"

// GameState represents the current state of a game
type GameState string

const (
	GameStateScheduled  GameState = "scheduled"
	GameStateLive       GameState = "live"
	GameStateFinal      GameState = "final"
	GameStatePostponed  GameState = "postponed"
)

// Game represents an NHL game
type Game struct {
	ID            int       `json:"id"`
	Season        string    `json:"season"`        // e.g., "20252026"
	GameDate      time.Time `json:"gameDate"`
	StartTime     time.Time `json:"startTime"`
	HomeTeamID    int       `json:"homeTeamId"`
	HomeTeamAbbr  string    `json:"homeTeamAbbr"`
	AwayTeamID    int       `json:"awayTeamId"`
	AwayTeamAbbr  string    `json:"awayTeamAbbr"`
	Venue         string    `json:"venue"`
	State         GameState `json:"state"`

	// Final score (if completed)
	HomeScore     int  `json:"homeScore"`
	AwayScore     int  `json:"awayScore"`
	Overtime      bool `json:"overtime"`
	Shootout      bool `json:"shootout"`

	UpdatedAt     time.Time `json:"updatedAt"`
}

// Team represents an NHL team
type Team struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Division     string `json:"division"`
	Conference   string `json:"conference"`
}

// TeamStats holds team-level statistics
type TeamStats struct {
	TeamID   int    `json:"teamId"`
	TeamAbbr string `json:"teamAbbr"`

	// Goals
	GF          int     `json:"gf"`           // Goals for
	GA          int     `json:"ga"`           // Goals against
	GFPerGame   float64 `json:"gfPerGame"`
	GAPerGame   float64 `json:"gaPerGame"`
	RecentGFPG  float64 `json:"recentGfPg"`
	RecentGAPG  float64 `json:"recentGaPg"`

	// Shots
	SF          int     `json:"sf"`           // Shots for
	SA          int     `json:"sa"`           // Shots against
	SFPerGame   float64 `json:"sfPerGame"`
	SAPerGame   float64 `json:"saPerGame"`
	RecentSFPG  float64 `json:"recentSfPg"`
	RecentSAPG  float64 `json:"recentSaPg"`

	// Power play
	PPPct       float64 `json:"ppPct"`        // Power play %
	PKPct       float64 `json:"pkPct"`        // Penalty kill %
	PPOppsGame  float64 `json:"ppOppsGame"`   // PP opportunities per game
	TimesShort  float64 `json:"timesShort"`   // Times shorthanded per game

	// Pace and style
	Pace        float64 `json:"pace"`         // CF/60 or similar pace metric
	XGF60       float64 `json:"xGf60"`
	XGA60       float64 `json:"xGa60"`
	CA60        float64 `json:"ca60"`         // Corsi against per 60 (suppression)
	CF60        float64 `json:"cf60"`         // Corsi for per 60

	// Physical
	HitsPerGame   float64 `json:"hitsPerGame"`
	BlocksPerGame float64 `json:"blocksPerGame"`
	PIMPerGame    float64 `json:"pimPerGame"`

	// Record
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
	OTLosses   int     `json:"otLosses"`
	Points     int     `json:"points"`
	PointsPct  float64 `json:"pointsPct"`

	UpdatedAt  time.Time `json:"updatedAt"`
}

// GameContext contains contextual factors for a game
type GameContext struct {
	GameID int `json:"gameId"`

	// Team info
	HomeTeamAbbr   string `json:"homeTeamAbbr"`
	AwayTeamAbbr   string `json:"awayTeamAbbr"`

	// Schedule factors
	HomeB2B        bool    `json:"homeB2b"`       // Home team on back-to-back
	AwayB2B        bool    `json:"awayB2b"`       // Away team on back-to-back
	Home3In4       bool    `json:"home3In4"`      // Home team 3 games in 4 days
	Away3In4       bool    `json:"away3In4"`      // Away team 3 games in 4 days
	HomeFatigue    float64 `json:"homeFatigue"`   // 0-1 fatigue penalty
	AwayFatigue    float64 `json:"awayFatigue"`

	// Travel
	AwayTravelMiles  float64 `json:"awayTravelMiles"`
	AwayTimeZoneShift int    `json:"awayTimeZoneShift"` // Hours difference

	// Arena factors
	ArenaSOGBias     float64 `json:"arenaSogBias"`    // Rink scorer bias for SOG
	ArenaHitsBias    float64 `json:"arenaHitsBias"`   // Rink scorer bias for Hits
	ArenaBlocksBias  float64 `json:"arenaBlocksBias"`
	AltitudeEffect   float64 `json:"altitudeEffect"`  // e.g., Denver

	// Officials (if available)
	RefCrew          string  `json:"refCrew"`
	RefPPRateBias    float64 `json:"refPpRateBias"` // Tendency to call penalties

	// Goalie confirmations
	HomeGoalieID     int     `json:"homeGoalieId"`
	AwayGoalieID     int     `json:"awayGoalieId"`
	HomGoalieConfirmed bool  `json:"homeGoalieConfirmed"`
	AwayGoalieConfirmed bool `json:"awayGoalieConfirmed"`

	// Vegas/implied
	HomeMoneyline    int     `json:"homeMoneyline"`
	AwayMoneyline    int     `json:"awayMoneyline"`
	ImpliedHomeWin   float64 `json:"impliedHomeWin"`   // Win probability
	ImpliedAwayWin   float64 `json:"impliedAwayWin"`
	TotalLine        float64 `json:"totalLine"`        // Over/under
	HomeSpread       float64 `json:"homeSpread"`       // Puck line
}

// Boxscore contains final game statistics for grading
type Boxscore struct {
	GameID    int       `json:"gameId"`
	FetchedAt time.Time `json:"fetchedAt"`

	HomeScore int `json:"homeScore"`
	AwayScore int `json:"awayScore"`

	// Player stats keyed by player ID
	PlayerStats map[int]BoxscorePlayerStats `json:"playerStats"`
	GoalieStats map[int]BoxscoreGoalieStats `json:"goalieStats"`
}

// BoxscorePlayerStats contains a player's stats from a single game
type BoxscorePlayerStats struct {
	PlayerID   int     `json:"playerId"`
	TeamID     int     `json:"teamId"`
	TOI        float64 `json:"toi"`        // Minutes
	Goals      int     `json:"goals"`
	Assists    int     `json:"assists"`
	Points     int     `json:"points"`
	SOG        int     `json:"sog"`
	Hits       int     `json:"hits"`
	Blocks     int     `json:"blocks"`
	PIM        int     `json:"pim"`
	PPGoals    int     `json:"ppGoals"`
	PPAssists  int     `json:"ppAssists"`
	PPPoints   int     `json:"ppPoints"`
	PPTOI      float64 `json:"ppToi"`
	PlusMinus  int     `json:"plusMinus"`
}

// BoxscoreGoalieStats contains a goalie's stats from a single game
type BoxscoreGoalieStats struct {
	PlayerID     int     `json:"playerId"`
	TeamID       int     `json:"teamId"`
	TOI          float64 `json:"toi"`
	Saves        int     `json:"saves"`
	ShotsAgainst int     `json:"shotsAgainst"`
	GoalsAgainst int     `json:"goalsAgainst"`
	SavePct      float64 `json:"savePct"`
	Decision     string  `json:"decision"` // W, L, OTL, or ""
}
