package correlation

import (
	"math"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// Matrix holds scenario-conditioned correlations between prop markets
type Matrix struct {
	// Base correlations (scenario-agnostic)
	BaseCorrelations map[MarketPair]float64

	// Scenario-specific adjustments
	ScenarioAdjustments map[types.ScenarioType]map[MarketPair]float64
}

// MarketPair represents a pair of prop markets
type MarketPair struct {
	Market1 types.PropMarket
	Market2 types.PropMarket
}

// NewMatrix creates a new correlation matrix with default values
func NewMatrix() *Matrix {
	m := &Matrix{
		BaseCorrelations:    make(map[MarketPair]float64),
		ScenarioAdjustments: make(map[types.ScenarioType]map[MarketPair]float64),
	}
	m.initializeDefaults()
	return m
}

func (m *Matrix) initializeDefaults() {
	// Base correlations (same player, same team)

	// SOG correlations
	m.setBase(types.PropMarketSOGOver, types.PropMarketPointsOver, 0.45)
	m.setBase(types.PropMarketSOGOver, types.PropMarketGoalsOver, 0.50)
	m.setBase(types.PropMarketSOGOver, types.PropMarketAssistsOver, 0.25)

	// Points/Goals/Assists correlations
	m.setBase(types.PropMarketPointsOver, types.PropMarketGoalsOver, 0.55)
	m.setBase(types.PropMarketPointsOver, types.PropMarketAssistsOver, 0.60)
	m.setBase(types.PropMarketGoalsOver, types.PropMarketAssistsOver, 0.10)

	// Physical correlations
	m.setBase(types.PropMarketHitsOver, types.PropMarketBlocksOver, 0.30)
	m.setBase(types.PropMarketHitsOver, types.PropMarketPIMOver, 0.25)
	m.setBase(types.PropMarketBlocksOver, types.PropMarketPIMOver, 0.15)

	// Cross-category (usually weak)
	m.setBase(types.PropMarketSOGOver, types.PropMarketHitsOver, -0.10)
	m.setBase(types.PropMarketSOGOver, types.PropMarketBlocksOver, -0.05)
	m.setBase(types.PropMarketPointsOver, types.PropMarketHitsOver, -0.15)

	// Goalie saves correlations with opponent shots
	m.setBase(types.PropMarketSavesOver, types.PropMarketSOGOver, 0.55)

	// PP Points correlations
	m.setBase(types.PropMarketPPPointsOver, types.PropMarketPointsOver, 0.50)
	m.setBase(types.PropMarketPPPointsOver, types.PropMarketPIMOver, 0.40)

	// Initialize scenario adjustments
	m.initializeScenarioAdjustments()
}

func (m *Matrix) setBase(market1, market2 types.PropMarket, corr float64) {
	m.BaseCorrelations[MarketPair{market1, market2}] = corr
	m.BaseCorrelations[MarketPair{market2, market1}] = corr
}

func (m *Matrix) initializeScenarioAdjustments() {
	// Run-and-Gun scenario
	runAndGun := make(map[MarketPair]float64)
	runAndGun[MarketPair{types.PropMarketSOGOver, types.PropMarketPointsOver}] = 0.65
	runAndGun[MarketPair{types.PropMarketSOGOver, types.PropMarketGoalsOver}] = 0.60
	runAndGun[MarketPair{types.PropMarketSOGOver, types.PropMarketSavesOver}] = 0.70
	runAndGun[MarketPair{types.PropMarketPointsOver, types.PropMarketGoalsOver}] = 0.65
	m.ScenarioAdjustments[types.ScenarioRunAndGun] = runAndGun

	// Goalie Duel scenario
	goalieDuel := make(map[MarketPair]float64)
	goalieDuel[MarketPair{types.PropMarketSOGUnder, types.PropMarketPointsUnder}] = 0.60
	goalieDuel[MarketPair{types.PropMarketSOGUnder, types.PropMarketGoalsUnder}] = 0.65
	goalieDuel[MarketPair{types.PropMarketSavesUnder, types.PropMarketSOGUnder}] = 0.55
	m.ScenarioAdjustments[types.ScenarioGoalieDuel] = goalieDuel

	// Special Teams scenario
	specialTeams := make(map[MarketPair]float64)
	specialTeams[MarketPair{types.PropMarketPIMOver, types.PropMarketPPPointsOver}] = 0.60
	specialTeams[MarketPair{types.PropMarketPPPointsOver, types.PropMarketPointsOver}] = 0.65
	specialTeams[MarketPair{types.PropMarketPIMOver, types.PropMarketSavesOver}] = 0.45
	m.ScenarioAdjustments[types.ScenarioSpecialTeams] = specialTeams

	// Score Effects scenario
	scoreEffects := make(map[MarketPair]float64)
	scoreEffects[MarketPair{types.PropMarketSOGOver, types.PropMarketPointsOver}] = 0.55
	scoreEffects[MarketPair{types.PropMarketSOGOver, types.PropMarketSavesOver}] = 0.65
	m.ScenarioAdjustments[types.ScenarioScoreEffects] = scoreEffects

	// Bench Slog scenario
	benchSlog := make(map[MarketPair]float64)
	benchSlog[MarketPair{types.PropMarketSOGUnder, types.PropMarketPointsUnder}] = 0.55
	benchSlog[MarketPair{types.PropMarketHitsOver, types.PropMarketBlocksOver}] = 0.45
	benchSlog[MarketPair{types.PropMarketHitsOver, types.PropMarketSOGUnder}] = 0.40
	m.ScenarioAdjustments[types.ScenarioBenchSlog] = benchSlog
}

// GetCorrelation returns the correlation between two markets under a given scenario
func (m *Matrix) GetCorrelation(market1, market2 types.PropMarket, scenario types.ScenarioType) float64 {
	pair := MarketPair{market1, market2}

	// Check scenario-specific adjustment first
	if scenarioAdj, ok := m.ScenarioAdjustments[scenario]; ok {
		if corr, ok := scenarioAdj[pair]; ok {
			return corr
		}
		// Check reverse pair
		if corr, ok := scenarioAdj[MarketPair{market2, market1}]; ok {
			return corr
		}
	}

	// Fall back to base correlation
	if corr, ok := m.BaseCorrelations[pair]; ok {
		return corr
	}

	return 0.0 // No known correlation
}

// ArePropsPurelyCorrelated checks if two props support each other under the scenario
func (m *Matrix) ArePropsPurelyCorrelated(prop1, prop2 *types.PropEvaluation, scenario types.ScenarioType) bool {
	// Same side (both overs or both unders) + positive correlation = purely correlated
	// Different sides + negative correlation = purely correlated

	corr := m.GetCorrelation(prop1.Line.Market, prop2.Line.Market, scenario)

	sameSide := prop1.Line.Side == prop2.Line.Side
	sameDirection := (prop1.Line.Market.IsOver() == prop2.Line.Market.IsOver())

	if sameSide && sameDirection && corr > 0 {
		return true
	}

	if !sameSide && corr < 0 {
		return true
	}

	return false
}

// CheckParlayCorrelation validates that all legs in a parlay are scenario-pure
func (m *Matrix) CheckParlayCorrelation(legs []types.ParlayLeg, scenario types.ScenarioType) (bool, []string) {
	var notes []string
	allPure := true

	// Check each pair of legs
	for i := 0; i < len(legs); i++ {
		for j := i + 1; j < len(legs); j++ {
			leg1 := legs[i]
			leg2 := legs[j]

			// Same player checks
			if leg1.PlayerID == leg2.PlayerID {
				// Same player, check correlation
				corr := m.GetCorrelation(leg1.Market, leg2.Market, scenario)
				if corr < 0.2 {
					notes = append(notes, "Weak same-player correlation")
					allPure = false
				}
				continue
			}

			// Same team checks (linemates/line pairs)
			if leg1.TeamAbbr == leg2.TeamAbbr {
				// Teammates on same unit should have positive correlation
				corr := m.GetCorrelation(leg1.Market, leg2.Market, scenario)
				if corr < 0 {
					notes = append(notes, "Conflicting same-team props")
					allPure = false
				}
			}

			// Check if markets fight the scenario
			preferred := scenario.GetPreferredProps()
			for _, avoid := range preferred.Avoid {
				if leg1.Market == avoid || leg2.Market == avoid {
					notes = append(notes, "Prop conflicts with scenario")
					allPure = false
				}
			}
		}
	}

	return allPure, notes
}

// CalculateJointProbability calculates joint probability with correlation adjustment
func (m *Matrix) CalculateJointProbability(legs []types.ParlayLeg, scenario types.ScenarioType) float64 {
	if len(legs) == 0 {
		return 0
	}
	if len(legs) == 1 {
		return legs[0].ProbHit
	}

	// Start with independent probability
	independentProb := 1.0
	for _, leg := range legs {
		independentProb *= leg.ProbHit
	}

	// Calculate correlation adjustment
	// For positive correlations, joint probability is higher than independent
	// For negative correlations, joint probability is lower
	correlationAdjustment := 0.0

	for i := 0; i < len(legs); i++ {
		for j := i + 1; j < len(legs); j++ {
			corr := m.GetCorrelation(legs[i].Market, legs[j].Market, scenario)

			// Adjust based on whether we want both to hit or not
			// If same direction (both overs or both unders), positive corr helps
			sameDirection := (legs[i].Market.IsOver() == legs[j].Market.IsOver())
			if sameDirection {
				correlationAdjustment += corr * 0.05 // Small boost for positive correlation
			} else {
				correlationAdjustment -= corr * 0.05 // Penalty for negative correlation
			}
		}
	}

	// Apply adjustment (clamped to reasonable range)
	adjustment := math.Max(-0.15, math.Min(0.15, correlationAdjustment))
	jointProb := independentProb * (1 + adjustment)

	// Clamp to valid probability range
	return math.Max(0.01, math.Min(0.99, jointProb))
}

// FindOptimalCorrelatedLegs finds legs that correlate well together under a scenario
func (m *Matrix) FindOptimalCorrelatedLegs(candidates []types.PropEvaluation, scenario types.ScenarioType, maxLegs int) []types.PropEvaluation {
	if len(candidates) <= maxLegs {
		return candidates
	}

	// Score each candidate based on edge and correlation with others
	type scoredCandidate struct {
		prop           types.PropEvaluation
		correlationSum float64
	}

	preferred := scenario.GetPreferredProps()
	preferredMap := make(map[types.PropMarket]bool)
	for _, p := range preferred.Preferred {
		preferredMap[p] = true
	}

	var scored []scoredCandidate
	for _, cand := range candidates {
		// Skip if market is avoided for this scenario
		skip := false
		for _, avoid := range preferred.Avoid {
			if cand.Line.Market == avoid {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Calculate correlation sum with all other candidates
		corrSum := 0.0
		for _, other := range candidates {
			if cand.Line.ID != other.Line.ID {
				corr := m.GetCorrelation(cand.Line.Market, other.Line.Market, scenario)
				if cand.Line.Market.IsOver() == other.Line.Market.IsOver() {
					corrSum += corr
				} else {
					corrSum -= corr
				}
			}
		}

		// Boost for preferred markets
		if preferredMap[cand.Line.Market] {
			corrSum += 0.5
		}

		scored = append(scored, scoredCandidate{cand, corrSum})
	}

	// Sort by edge * correlation factor
	// Higher edge and better correlation = better pick
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			scoreI := scored[i].prop.Edge * (1 + scored[i].correlationSum*0.1)
			scoreJ := scored[j].prop.Edge * (1 + scored[j].correlationSum*0.1)
			if scoreJ > scoreI {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Return top N
	result := make([]types.PropEvaluation, 0, maxLegs)
	for i := 0; i < len(scored) && i < maxLegs; i++ {
		result = append(result, scored[i].prop)
	}

	return result
}

// GetConflictingMarkets returns markets that conflict under the given scenario
func (m *Matrix) GetConflictingMarkets(scenario types.ScenarioType) []types.PropMarket {
	preferred := scenario.GetPreferredProps()
	return preferred.Avoid
}

// IsMarketPreferred checks if a market is preferred for the scenario
func (m *Matrix) IsMarketPreferred(market types.PropMarket, scenario types.ScenarioType) bool {
	preferred := scenario.GetPreferredProps()
	for _, p := range preferred.Preferred {
		if market == p {
			return true
		}
	}
	return false
}
