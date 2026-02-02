package types

import "time"

// CalibrationData stores bias and adjustment factors
type CalibrationData struct {
	ID        int64     `json:"id"`
	Category  string    `json:"category"` // "arena", "ref", "fatigue", etc.
	Key       string    `json:"key"`      // Arena name, ref name, etc.
	Market    string    `json:"market"`   // Which market this applies to
	Bias      float64   `json:"bias"`     // Adjustment factor
	SampleSize int      `json:"sampleSize"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ArenaBias stores arena-specific scoring biases
type ArenaBias struct {
	ArenaName    string  `json:"arenaName"`
	TeamAbbr     string  `json:"teamAbbr"`
	SOGBias      float64 `json:"sogBias"`      // Multiplier for SOG
	HitsBias     float64 `json:"hitsBias"`     // Multiplier for Hits
	BlocksBias   float64 `json:"blocksBias"`   // Multiplier for Blocks
	SampleGames  int     `json:"sampleGames"`
}

// RefBias stores referee crew tendencies
type RefBias struct {
	RefName      string  `json:"refName"`
	PPRateBias   float64 `json:"ppRateBias"`   // Relative to league avg
	PIMRateBias  float64 `json:"pimRateBias"`
	GamesWorked  int     `json:"gamesWorked"`
}

// FatiguePenalty stores fatigue adjustment factors
type FatiguePenalty struct {
	Situation    string  `json:"situation"` // "b2b", "3in4", "4in6", etc.
	SOGPenalty   float64 `json:"sogPenalty"`   // Negative adjustment
	PointsPenalty float64 `json:"pointsPenalty"`
	GoalsPenalty float64 `json:"goalsPenalty"`
	SavesPenalty float64 `json:"savesPenalty"`
}

// PropFamilyStats tracks hit rates by prop type
type PropFamilyStats struct {
	Market       PropMarket `json:"market"`
	Tier         PropTier   `json:"tier"`
	TotalPicks   int        `json:"totalPicks"`
	Wins         int        `json:"wins"`
	Losses       int        `json:"losses"`
	Pushes       int        `json:"pushes"`
	HitRate      float64    `json:"hitRate"`
	AvgEdge      float64    `json:"avgEdge"`
	AvgCLV       float64    `json:"avgClv"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// ScenarioAccuracy tracks how well scenarios predict games
type ScenarioAccuracy struct {
	Scenario     ScenarioType `json:"scenario"`
	TotalGames   int          `json:"totalGames"`
	CorrectCalls int          `json:"correctCalls"`
	Accuracy     float64      `json:"accuracy"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// WeeklyStarCutoffs stores the star rating thresholds
type WeeklyStarCutoffs struct {
	Week         string  `json:"week"`      // e.g., "2025-W03"
	Market       string  `json:"market"`
	Star1Edge    float64 `json:"star1Edge"` // Minimum edge for 1 star
	Star2Edge    float64 `json:"star2Edge"`
	Star3Edge    float64 `json:"star3Edge"`
	Star4Edge    float64 `json:"star4Edge"`
	Star5Edge    float64 `json:"star5Edge"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DefaultStarCutoffs returns the default edge thresholds for star ratings
func DefaultStarCutoffs() map[int]float64 {
	return map[int]float64{
		1: 2.0,  // 2+ pp edge
		2: 4.0,  // 4+ pp edge
		3: 6.0,  // 6+ pp edge
		4: 8.0,  // 8+ pp edge
		5: 10.0, // 10+ pp edge
	}
}

// GetStars returns the star rating for a given edge
func GetStars(edge float64, cutoffs map[int]float64) int {
	if cutoffs == nil {
		cutoffs = DefaultStarCutoffs()
	}
	stars := 0
	for i := 1; i <= 5; i++ {
		if edge >= cutoffs[i] {
			stars = i
		}
	}
	return stars
}

// DefaultFatiguePenalties returns standard fatigue adjustments
func DefaultFatiguePenalties() map[string]FatiguePenalty {
	return map[string]FatiguePenalty{
		"b2b": {
			Situation:     "b2b",
			SOGPenalty:    -0.08, // 8% reduction
			PointsPenalty: -0.10,
			GoalsPenalty:  -0.12,
			SavesPenalty:  -0.05,
		},
		"3in4": {
			Situation:     "3in4",
			SOGPenalty:    -0.12,
			PointsPenalty: -0.15,
			GoalsPenalty:  -0.18,
			SavesPenalty:  -0.08,
		},
		"4in6": {
			Situation:     "4in6",
			SOGPenalty:    -0.06,
			PointsPenalty: -0.08,
			GoalsPenalty:  -0.10,
			SavesPenalty:  -0.04,
		},
	}
}

// DefaultArenaBiases returns known arena scoring biases
func DefaultArenaBiases() map[string]ArenaBias {
	// These are approximate and should be updated with real data
	return map[string]ArenaBias{
		"Ball Arena": {
			ArenaName:   "Ball Arena",
			TeamAbbr:    "COL",
			SOGBias:     1.05, // Denver tends to have higher shot counts
			HitsBias:    0.95,
			BlocksBias:  1.00,
			SampleGames: 100,
		},
		"TD Garden": {
			ArenaName:   "TD Garden",
			TeamAbbr:    "BOS",
			SOGBias:     1.02,
			HitsBias:    1.08, // Boston tends to record more hits
			BlocksBias:  1.02,
			SampleGames: 100,
		},
		"Madison Square Garden": {
			ArenaName:   "Madison Square Garden",
			TeamAbbr:    "NYR",
			SOGBias:     1.00,
			HitsBias:    1.05,
			BlocksBias:  1.00,
			SampleGames: 100,
		},
		// Add more arenas as data is gathered
	}
}
