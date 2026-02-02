package types

import "time"

// Site represents a betting platform
type Site string

const (
	SiteUnderdog   Site = "underdog"
	SitePrizePicks Site = "prizepicks"
	SiteDraftKings Site = "draftkings"
	SiteBetr       Site = "betr"
)

// Parlay represents a multi-leg bet
type Parlay struct {
	ID           int64        `json:"id"`
	Site         Site         `json:"site"`
	Legs         []ParlayLeg  `json:"legs"`
	ScenarioTag  ScenarioType `json:"scenarioTag"`

	// Correlation info
	CorrelationValid bool     `json:"correlationValid"` // All legs correlate properly
	CorrelationNotes []string `json:"correlationNotes"`

	// Probabilities and EV
	JointProb     float64 `json:"jointProb"`     // Product of leg probs with correlation
	ImpliedProb   float64 `json:"impliedProb"`   // Implied from payout
	Payout        float64 `json:"payout"`        // Payout multiplier
	EV            float64 `json:"ev"`            // Expected value
	EVGate        float64 `json:"evGate"`        // p(hit) * multiplier for UD/Betr

	// Rating
	Stars         int      `json:"stars"`
	StopLines     []string `json:"stopLines"` // Stop conditions for each leg

	// Risk flags
	RiskFlags     []string `json:"riskFlags"`

	// Result (if graded)
	Result        ParlayResult `json:"result"`
	LegsWon       int          `json:"legsWon"`
	LegsPushed    int          `json:"legsPushed"`
	LegsLost      int          `json:"legsLost"`

	// Timestamps
	CreatedAt     time.Time  `json:"createdAt"`
	GradedAt      *time.Time `json:"gradedAt,omitempty"`
}

// ParlayLeg represents a single leg in a parlay
type ParlayLeg struct {
	ID          int64      `json:"id"`
	ParlayID    int64      `json:"parlayId"`
	PickID      int64      `json:"pickId"`      // Reference to PropPick
	PlayerID    int        `json:"playerId"`
	PlayerName  string     `json:"playerName"`
	TeamAbbr    string     `json:"teamAbbr"`
	GameID      int        `json:"gameId"`
	Market      PropMarket `json:"market"`
	Timeframe   Timeframe  `json:"timeframe"`
	Side        PropSide   `json:"side"`
	Line        float64    `json:"line"`
	Price       int        `json:"price"`

	// Model data
	ProbHit     float64    `json:"probHit"`
	Edge        float64    `json:"edge"`
	StopLine    float64    `json:"stopLine"`

	// Result
	Result      PropResult `json:"result"`
	ActualValue float64    `json:"actualValue"`
}

// ParlayResult represents the result of a parlay
type ParlayResult string

const (
	ParlayResultPending ParlayResult = "pending"
	ParlayResultWin     ParlayResult = "win"
	ParlayResultLoss    ParlayResult = "loss"
	ParlayResultPush    ParlayResult = "push"
)

// SiteRules contains the rules for each betting site
type SiteRules struct {
	Site            Site    `json:"site"`
	MinLegs         int     `json:"minLegs"`
	MaxLegs         int     `json:"maxLegs"`
	MinTeams        int     `json:"minTeams"`        // UD/Betr require >= 2 teams
	SameGameAllowed bool    `json:"sameGameAllowed"` // PP allows same game
	DuplicatePlayer bool    `json:"duplicatePlayer"` // Can same player appear twice
	EVGateMin       float64 `json:"evGateMin"`       // p(hit) * mult minimum
}

// GetSiteRules returns the rules for a specific site
func GetSiteRules(site Site) SiteRules {
	switch site {
	case SiteUnderdog:
		return SiteRules{
			Site:            SiteUnderdog,
			MinLegs:         2,
			MaxLegs:         6,
			MinTeams:        2,
			SameGameAllowed: true,
			DuplicatePlayer: false,
			EVGateMin:       1.00, // Prefer >= 1.02 if uncertain
		}
	case SitePrizePicks:
		return SiteRules{
			Site:            SitePrizePicks,
			MinLegs:         2,
			MaxLegs:         6,
			MinTeams:        1,
			SameGameAllowed: true,
			DuplicatePlayer: false,
			EVGateMin:       0, // Fixed payouts
		}
	case SiteDraftKings:
		return SiteRules{
			Site:            SiteDraftKings,
			MinLegs:         2,
			MaxLegs:         6,
			MinTeams:        1,
			SameGameAllowed: true,
			DuplicatePlayer: false,
			EVGateMin:       0,
		}
	case SiteBetr:
		return SiteRules{
			Site:            SiteBetr,
			MinLegs:         2,
			MaxLegs:         6,
			MinTeams:        2,
			SameGameAllowed: true,
			DuplicatePlayer: false,
			EVGateMin:       1.00,
		}
	default:
		return SiteRules{}
	}
}

// ValidateParlay checks if a parlay meets site rules
func ValidateParlay(parlay Parlay) []string {
	var violations []string
	rules := GetSiteRules(parlay.Site)

	if len(parlay.Legs) < rules.MinLegs {
		violations = append(violations, "too few legs")
	}
	if len(parlay.Legs) > rules.MaxLegs {
		violations = append(violations, "too many legs")
	}

	// Check team count
	teams := make(map[string]bool)
	for _, leg := range parlay.Legs {
		teams[leg.TeamAbbr] = true
	}
	if len(teams) < rules.MinTeams {
		violations = append(violations, "requires more teams")
	}

	// Check duplicate players
	if !rules.DuplicatePlayer {
		players := make(map[int]bool)
		for _, leg := range parlay.Legs {
			if players[leg.PlayerID] {
				violations = append(violations, "duplicate player not allowed")
				break
			}
			players[leg.PlayerID] = true
		}
	}

	// Check EV gate
	if rules.EVGateMin > 0 && parlay.EVGate < rules.EVGateMin {
		violations = append(violations, "below EV gate minimum")
	}

	return violations
}

// CorrelationPair represents a pair of correlated markets
type CorrelationPair struct {
	Market1       PropMarket   `json:"market1"`
	Market2       PropMarket   `json:"market2"`
	BaseCorr      float64      `json:"baseCorr"`      // Base correlation coefficient
	ScenarioCorr  map[ScenarioType]float64 `json:"scenarioCorr"` // Scenario-adjusted correlations
}

// ParlayStats tracks performance by parlay configuration
type ParlayStats struct {
	Site           Site    `json:"site"`
	LegCount       int     `json:"legCount"`
	ScenarioTag    ScenarioType `json:"scenarioTag,omitempty"`
	TotalParlays   int     `json:"totalParlays"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	Pushes         int     `json:"pushes"`
	HitRate        float64 `json:"hitRate"`
	AvgEV          float64 `json:"avgEv"`
	ActualROI      float64 `json:"actualRoi"`
}
