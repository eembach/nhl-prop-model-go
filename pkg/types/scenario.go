package types

// ScenarioType represents a game script classification
type ScenarioType string

const (
	ScenarioRunAndGun        ScenarioType = "run_and_gun"
	ScenarioGoalieDuel       ScenarioType = "goalie_duel"
	ScenarioSpecialTeams     ScenarioType = "special_teams"
	ScenarioScoreEffects     ScenarioType = "score_effects"
	ScenarioBenchSlog        ScenarioType = "bench_slog"
)

// ScenarioScore represents how well a game matches a particular scenario
type ScenarioScore struct {
	Scenario   ScenarioType `json:"scenario"`
	Score      float64      `json:"score"`      // 0-100 confidence score
	Rank       int          `json:"rank"`       // 1 = most likely, 2 = secondary, etc.
	Factors    []string     `json:"factors"`    // Key factors driving this score
}

// GameScenario represents the scenario classification for a game
type GameScenario struct {
	GameID          int             `json:"gameId"`
	Primary         ScenarioScore   `json:"primary"`
	Secondary       *ScenarioScore  `json:"secondary,omitempty"`
	AllScores       []ScenarioScore `json:"allScores"`

	// Underlying metrics that drove classification
	Metrics         ScenarioMetrics `json:"metrics"`
}

// ScenarioMetrics contains the raw data used to classify scenarios
type ScenarioMetrics struct {
	// Pace indicators
	CombinedPace     float64 `json:"combinedPace"`     // Sum of both teams' CF/60
	ExpectedTotal    float64 `json:"expectedTotal"`    // Our projected total goals

	// Goalie quality
	HomeGSAx         float64 `json:"homeGsax"`
	AwayGSAx         float64 `json:"awayGsax"`
	CombinedGSAx     float64 `json:"combinedGsax"`

	// Defense/suppression
	HomeSuppression  float64 `json:"homeSuppression"`  // CA/60
	AwaySuppression  float64 `json:"awaySuppression"`

	// Special teams
	HomePPPct        float64 `json:"homePpPct"`
	AwayPPPct        float64 `json:"awayPpPct"`
	HomePKPct        float64 `json:"homePkPct"`
	AwayPKPct        float64 `json:"awayPkPct"`
	CombinedPIMPG    float64 `json:"combinedPimPg"`

	// Fatigue
	HomeFatigue      float64 `json:"homeFatigue"`
	AwayFatigue      float64 `json:"awayFatigue"`
	CombinedFatigue  float64 `json:"combinedFatigue"`

	// Score effects potential
	ImpliedFavorite  string  `json:"impliedFavorite"`  // Team abbr
	FavoriteWinProb  float64 `json:"favoriteWinProb"`
	SpreadMagnitude  float64 `json:"spreadMagnitude"`  // Absolute puck line
}

// ScenarioPreferredProps returns the preferred prop types for each scenario
type ScenarioPreferredProps struct {
	Preferred []PropMarket `json:"preferred"` // Props that benefit from this scenario
	Avoid     []PropMarket `json:"avoid"`     // Props that conflict with this scenario
}

// GetPreferredProps returns the preferred and avoided props for a scenario
func (s ScenarioType) GetPreferredProps() ScenarioPreferredProps {
	switch s {
	case ScenarioRunAndGun:
		return ScenarioPreferredProps{
			Preferred: []PropMarket{
				PropMarketSOGOver, PropMarketPointsOver, PropMarketGoalsOver,
				PropMarketAssistsOver, PropMarketSavesOver,
			},
			Avoid: []PropMarket{
				PropMarketSOGUnder, PropMarketPointsUnder, PropMarketGoalsUnder,
			},
		}
	case ScenarioGoalieDuel:
		return ScenarioPreferredProps{
			Preferred: []PropMarket{
				PropMarketSOGUnder, PropMarketPointsUnder, PropMarketGoalsUnder,
				PropMarketSavesUnder,
			},
			Avoid: []PropMarket{
				PropMarketSOGOver, PropMarketPointsOver, PropMarketGoalsOver,
			},
		}
	case ScenarioSpecialTeams:
		return ScenarioPreferredProps{
			Preferred: []PropMarket{
				PropMarketPPPointsOver, PropMarketPIMOver, PropMarketSavesOver,
			},
			Avoid: []PropMarket{},
		}
	case ScenarioScoreEffects:
		return ScenarioPreferredProps{
			Preferred: []PropMarket{
				PropMarketSOGOver, PropMarketPointsOver, PropMarketSavesOver,
			},
			Avoid: []PropMarket{},
		}
	case ScenarioBenchSlog:
		return ScenarioPreferredProps{
			Preferred: []PropMarket{
				PropMarketSOGUnder, PropMarketPointsUnder,
				PropMarketHitsOver, PropMarketBlocksOver,
			},
			Avoid: []PropMarket{
				PropMarketSOGOver, PropMarketPointsOver, PropMarketGoalsOver,
			},
		}
	default:
		return ScenarioPreferredProps{}
	}
}

// Description returns a human-readable description of the scenario
func (s ScenarioType) Description() string {
	switch s {
	case ScenarioRunAndGun:
		return "High-pace game with weak goaltending; expect lots of shots and scoring"
	case ScenarioGoalieDuel:
		return "Elite goaltending, low pace; suppressed offense expected"
	case ScenarioSpecialTeams:
		return "High-PIM matchup with special teams opportunities"
	case ScenarioScoreEffects:
		return "Heavy favorite likely to force trailing-team desperation"
	case ScenarioBenchSlog:
		return "Fatigue or trap systems; low-event, grinding game"
	default:
		return "Unknown scenario"
	}
}
