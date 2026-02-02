package types

import "time"

// PropMarket represents a type of player prop market
type PropMarket string

const (
	// Skater markets
	PropMarketPointsOver   PropMarket = "points_over"
	PropMarketPointsUnder  PropMarket = "points_under"
	PropMarketGoalsOver    PropMarket = "goals_over"
	PropMarketGoalsUnder   PropMarket = "goals_under"
	PropMarketAssistsOver  PropMarket = "assists_over"
	PropMarketAssistsUnder PropMarket = "assists_under"
	PropMarketSOGOver      PropMarket = "sog_over"
	PropMarketSOGUnder     PropMarket = "sog_under"
	PropMarketHitsOver     PropMarket = "hits_over"
	PropMarketHitsUnder    PropMarket = "hits_under"
	PropMarketBlocksOver   PropMarket = "blocks_over"
	PropMarketBlocksUnder  PropMarket = "blocks_under"
	PropMarketPIMOver      PropMarket = "pim_over"
	PropMarketPIMUnder     PropMarket = "pim_under"
	PropMarketPPPointsOver PropMarket = "pp_points_over"
	PropMarketAGS          PropMarket = "ags" // Anytime goal scorer

	// Goalie markets
	PropMarketSavesOver  PropMarket = "saves_over"
	PropMarketSavesUnder PropMarket = "saves_under"
)

// PropTier represents the tier hierarchy of a prop market
type PropTier int

const (
	PropTier1   PropTier = 1  // Core: Points O0.5 on anchors
	PropTier1_5 PropTier = 15 // Volume: SOG Overs
	PropTier2   PropTier = 2  // Secondary: Assists, secondary SOG
	PropTier3   PropTier = 3  // Thin: AGS (add-ons only)
)

// Timeframe represents the period for a prop
type Timeframe string

const (
	TimeframeFullGame Timeframe = "full_game"
	TimeframePeriod1  Timeframe = "1P"
	TimeframePeriod2  Timeframe = "2P"
	TimeframePeriod3  Timeframe = "3P"
)

// PropSide represents over or under
type PropSide string

const (
	PropSideOver  PropSide = "over"
	PropSideUnder PropSide = "under"
)

// PropLine represents a prop bet line from a sportsbook
type PropLine struct {
	ID          int64      `json:"id"`
	GameID      int        `json:"gameId"`
	PlayerID    int        `json:"playerId"`
	PlayerName  string     `json:"playerName"`
	TeamAbbr    string     `json:"teamAbbr"`
	Market      PropMarket `json:"market"`
	Timeframe   Timeframe  `json:"timeframe"`
	Line        float64    `json:"line"`        // The number to beat (e.g., 2.5 for SOG O2.5)
	Side        PropSide   `json:"side"`
	Price       int        `json:"price"`       // American odds (e.g., -115)
	ImpliedProb float64    `json:"impliedProb"` // Calculated from price
	Source      string     `json:"source"`      // e.g., "DraftKings", "FanDuel"
	OpenLine    float64    `json:"openLine"`    // Opening line
	OpenPrice   int        `json:"openPrice"`   // Opening price
	FetchedAt   time.Time  `json:"fetchedAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// PropEvaluation represents our model's evaluation of a prop
type PropEvaluation struct {
	Line PropLine `json:"line"`

	// Our model's projection
	OurLine       float64 `json:"ourLine"`       // Our projected line
	ProbOver      float64 `json:"probOver"`      // P(over)
	ProbUnder     float64 `json:"probUnder"`     // P(under)
	Edge          float64 `json:"edge"`          // Edge in percentage points
	EV            float64 `json:"ev"`            // Expected value
	EVMultiplied  float64 `json:"evMultiplied"`  // EV * multiplier for UD/Betr

	// Classification
	Tier          PropTier     `json:"tier"`
	Stars         int          `json:"stars"`        // 1-5 star rating
	ScenarioTag   ScenarioType `json:"scenarioTag"`  // Which scenario supports this
	ScenarioFit   float64      `json:"scenarioFit"`  // How well it fits (0-1)

	// Guardrail checks
	PassesGuardrails bool     `json:"passesGuardrails"`
	GuardrailFlags   []string `json:"guardrailFlags"` // Which guardrails failed

	// Stop line
	StopLine      float64 `json:"stopLine"`     // Don't play if line moves past this
	StopPrice     int     `json:"stopPrice"`    // Don't play if price worse than this

	// Key factors
	Determinants  []string `json:"determinants"` // Key factors (max 120 chars)
	RiskFlags     []string `json:"riskFlags"`    // Rink/Ref/Travel/Fatigue/Injury/Steam

	// Site fit
	FitsUD        bool    `json:"fitsUd"`
	FitsPP        bool    `json:"fitsPp"`
	FitsDK        bool    `json:"fitsDk"`
	FitsBetr      bool    `json:"fitsBetr"`

	// Methodology qualification (from spec)
	MethodologyQualified bool     `json:"methodologyQualified"` // Passes all methodology filters
	MethodologyReason    string   `json:"methodologyReason"`    // Why qualified or not
	RecommendedSide      PropSide `json:"recommendedSide"`      // Scenario-based recommended side
}

// PropResult represents the graded result of a prop pick
type PropResult string

const (
	PropResultWin  PropResult = "W"
	PropResultLoss PropResult = "L"
	PropResultPush PropResult = "P"
)

// PropPick represents a recorded prop pick for tracking
type PropPick struct {
	ID            int64        `json:"id"`
	GameID        int          `json:"gameId"`
	PlayerID      int          `json:"playerId"`
	PlayerName    string       `json:"playerName"`
	TeamAbbr      string       `json:"teamAbbr"`
	Market        PropMarket   `json:"market"`
	Timeframe     Timeframe    `json:"timeframe"`
	Side          PropSide     `json:"side"`

	// Lines
	Line          float64      `json:"line"`
	OpenLine      float64      `json:"openLine"`
	CloseLine     float64      `json:"closeLine"`
	OpenPrice     int          `json:"openPrice"`
	ClosePrice    int          `json:"closePrice"`

	// Model data
	OurLine       float64      `json:"ourLine"`
	ProbHit       float64      `json:"probHit"`
	Edge          float64      `json:"edge"`
	Tier          PropTier     `json:"tier"`
	Stars         int          `json:"stars"`
	ScenarioTag   ScenarioType `json:"scenarioTag"`

	// Result
	Result        PropResult   `json:"result"`
	ActualValue   float64      `json:"actualValue"`   // What the player actually got
	CLV           float64      `json:"clv"`           // Closing line value in pp
	ScenarioHit   bool         `json:"scenarioHit"`   // Did scenario prediction match?

	// Timestamps
	CreatedAt     time.Time    `json:"createdAt"`
	GradedAt      *time.Time   `json:"gradedAt,omitempty"`
}

// GetTier returns the tier for a given market
func GetMarketTier(market PropMarket, isTopLine bool, isPP1 bool) PropTier {
	switch market {
	case PropMarketPointsOver, PropMarketPointsUnder:
		if isTopLine && isPP1 {
			return PropTier1
		}
		return PropTier2
	case PropMarketSOGOver, PropMarketSOGUnder:
		return PropTier1_5
	case PropMarketAssistsOver, PropMarketAssistsUnder:
		if isPP1 {
			return PropTier2
		}
		return PropTier3
	case PropMarketGoalsOver, PropMarketGoalsUnder:
		return PropTier2
	case PropMarketAGS:
		return PropTier3
	default:
		return PropTier3
	}
}

// IsOver returns true if this is an over prop
func (m PropMarket) IsOver() bool {
	switch m {
	case PropMarketPointsOver, PropMarketGoalsOver, PropMarketAssistsOver,
		PropMarketSOGOver, PropMarketHitsOver, PropMarketBlocksOver,
		PropMarketPIMOver, PropMarketPPPointsOver, PropMarketSavesOver,
		PropMarketAGS:
		return true
	}
	return false
}

// BaseMarket returns the base market type (without over/under)
func (m PropMarket) BaseMarket() string {
	switch m {
	case PropMarketPointsOver, PropMarketPointsUnder:
		return "points"
	case PropMarketGoalsOver, PropMarketGoalsUnder, PropMarketAGS:
		return "goals"
	case PropMarketAssistsOver, PropMarketAssistsUnder:
		return "assists"
	case PropMarketSOGOver, PropMarketSOGUnder:
		return "sog"
	case PropMarketHitsOver, PropMarketHitsUnder:
		return "hits"
	case PropMarketBlocksOver, PropMarketBlocksUnder:
		return "blocks"
	case PropMarketPIMOver, PropMarketPIMUnder:
		return "pim"
	case PropMarketPPPointsOver:
		return "pp_points"
	case PropMarketSavesOver, PropMarketSavesUnder:
		return "saves"
	}
	return ""
}
