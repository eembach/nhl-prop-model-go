package types

import "time"

// Position represents a player's position
type Position string

const (
	PositionCenter Position = "C"
	PositionLeft   Position = "LW"
	PositionRight  Position = "RW"
	PositionDefense Position = "D"
	PositionGoalie Position = "G"
)

// Player represents an NHL player with all relevant stats for the model
type Player struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	TeamID    int       `json:"teamId"`
	TeamAbbr  string    `json:"teamAbbr"`
	Position  Position  `json:"position"`
	Number    int       `json:"number"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PlayerStats holds season and recent stats for a player
type PlayerStats struct {
	PlayerID int `json:"playerId"`

	// Games played
	GamesPlayed       int `json:"gamesPlayed"`
	RecentGamesPlayed int `json:"recentGamesPlayed"` // Last N games

	// Time on ice (minutes per game)
	TOI         float64 `json:"toi"`
	RecentTOI   float64 `json:"recentToi"`
	PPTOI       float64 `json:"ppToi"`       // Power play TOI
	RecentPPTOI float64 `json:"recentPpToi"`

	// Shots on goal
	SOG           float64 `json:"sog"`           // Per game
	RecentSOG     float64 `json:"recentSog"`
	SOGTotal      int     `json:"sogTotal"`
	RecentSOGTotal int    `json:"recentSogTotal"`

	// Points
	Goals         int     `json:"goals"`
	Assists       int     `json:"assists"`
	Points        int     `json:"points"`
	PointsPerGame float64 `json:"pointsPerGame"`
	RecentGoals   int     `json:"recentGoals"`
	RecentAssists int     `json:"recentAssists"`
	RecentPoints  int     `json:"recentPoints"`
	RecentPPG     float64 `json:"recentPpg"` // Recent points per game

	// Power play
	PPGoals     int     `json:"ppGoals"`
	PPAssists   int     `json:"ppAssists"`
	PPPoints    int     `json:"ppPoints"`
	PP1Unit     bool    `json:"pp1Unit"`    // Is on PP1
	PPShare     float64 `json:"ppShare"`    // Share of team PP points

	// Shooting
	ShootingPct       float64 `json:"shootingPct"`
	CareerShootingPct float64 `json:"careerShootingPct"`
	RecentShootingPct float64 `json:"recentShootingPct"`

	// Physical
	Hits           int     `json:"hits"`
	HitsPerGame    float64 `json:"hitsPerGame"`
	RecentHPG      float64 `json:"recentHpg"`
	Blocks         int     `json:"blocks"`
	BlocksPerGame  float64 `json:"blocksPerGame"`
	RecentBPG      float64 `json:"recentBpg"`
	PIM            int     `json:"pim"`
	PIMPerGame     float64 `json:"pimPerGame"`
	RecentPIMPG    float64 `json:"recentPimPg"`

	// Line/role info
	LineNumber   int  `json:"lineNumber"`   // 1-4 for forwards, 1-3 for D pairs
	IsTopSix     bool `json:"isTopSix"`     // Top-6 forward
	IsTopFour    bool `json:"isTopFour"`    // Top-4 defenseman
	IsDepthRole  bool `json:"isDepthRole"`  // 4th line / 3rd pair

	// Advanced metrics (from MoneyPuck/xG sources)
	XGF60         float64 `json:"xGf60"`         // Expected goals for per 60
	XGA60         float64 `json:"xGa60"`         // Expected goals against per 60
	OnIceXGF60    float64 `json:"onIceXGf60"`    // On-ice xGF/60
	OnIceXGA60    float64 `json:"onIceXGa60"`    // On-ice xGA/60
	OZStartPct    float64 `json:"ozStartPct"`    // Offensive zone start %
	HDCF          float64 `json:"hdcf"`          // High-danger chances for
	HDCFPct       float64 `json:"hdcfPct"`       // High-danger chance for %
	ReboundChances float64 `json:"reboundChances"`
	PDO           float64 `json:"pdo"`           // On-ice save% + shooting%

	// Stability metrics for shrinkage
	TOIVariance     float64 `json:"toiVariance"`
	RoleStability   float64 `json:"roleStability"` // 0-1 score
	OutlierSpreeFlag bool   `json:"outlierSpreeFlag"` // Recent unsustainable shooting

	UpdatedAt time.Time `json:"updatedAt"`
}

// GoalieStats holds season and recent stats for a goalie
type GoalieStats struct {
	PlayerID int `json:"playerId"`

	// Games
	GamesPlayed   int `json:"gamesPlayed"`
	GamesStarted  int `json:"gamesStarted"`
	RecentStarts  int `json:"recentStarts"` // Last N games

	// Saves
	Saves           int     `json:"saves"`
	SavesPerGame    float64 `json:"savesPerGame"`
	RecentSPG       float64 `json:"recentSpg"`
	ShotsAgainst    int     `json:"shotsAgainst"`
	ShotsAgainstPG  float64 `json:"shotsAgainstPg"`
	RecentSAPG      float64 `json:"recentSaPg"`

	// Goals against
	GoalsAgainst    int     `json:"goalsAgainst"`
	GAA             float64 `json:"gaa"`
	RecentGAA       float64 `json:"recentGaa"`

	// Save percentage
	SavePct        float64 `json:"savePct"`
	RecentSavePct  float64 `json:"recentSavePct"`

	// Advanced
	GSAx           float64 `json:"gsax"`    // Goals saved above expected
	XGA            float64 `json:"xGa"`     // Expected goals against
	HDSA           float64 `json:"hdsa"`    // High-danger save %
	QualityStarts  int     `json:"qualityStarts"`
	QSPct          float64 `json:"qsPct"`

	// Workload
	IsStarter      bool    `json:"isStarter"`
	StartPct       float64 `json:"startPct"`   // % of team starts
	B2BPenalty     float64 `json:"b2bPenalty"` // Fatigue adjustment
	LeashRisk      float64 `json:"leashRisk"`  // Pull probability

	UpdatedAt time.Time `json:"updatedAt"`
}

// PlayerWithStats combines player info with their stats
type PlayerWithStats struct {
	Player Player
	Stats  PlayerStats
}

// GoalieWithStats combines goalie info with their stats
type GoalieWithStats struct {
	Player Player
	Stats  GoalieStats
}
