package props

import (
	"fmt"

	"github.com/paul/nhl-prop-model/internal/model/correlation"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// ParlayBuilder builds and validates parlays
type ParlayBuilder struct {
	corrMatrix *correlation.Matrix
	evaluator  *Evaluator
}

// NewParlayBuilder creates a new parlay builder
func NewParlayBuilder(corrMatrix *correlation.Matrix, evaluator *Evaluator) *ParlayBuilder {
	return &ParlayBuilder{
		corrMatrix: corrMatrix,
		evaluator:  evaluator,
	}
}

// BuildParlay creates a parlay from evaluations
func (b *ParlayBuilder) BuildParlay(
	evals []*types.PropEvaluation,
	site types.Site,
	scenario types.ScenarioType,
) (*types.Parlay, error) {

	rules := types.GetSiteRules(site)

	if len(evals) < rules.MinLegs {
		return nil, fmt.Errorf("need at least %d legs for %s", rules.MinLegs, site)
	}

	// Convert evaluations to legs
	legs := make([]types.ParlayLeg, len(evals))
	for i, eval := range evals {
		legs[i] = types.ParlayLeg{
			PlayerID:   eval.Line.PlayerID,
			PlayerName: eval.Line.PlayerName,
			TeamAbbr:   eval.Line.TeamAbbr,
			GameID:     eval.Line.GameID,
			Market:     eval.Line.Market,
			Timeframe:  eval.Line.Timeframe,
			Side:       eval.Line.Side,
			Line:       eval.Line.Line,
			Price:      eval.Line.Price,
			ProbHit:    eval.ProbOver,
			Edge:       eval.Edge,
			StopLine:   eval.StopLine,
		}
		if eval.Line.Side == types.PropSideUnder {
			legs[i].ProbHit = eval.ProbUnder
		}
	}

	parlay := &types.Parlay{
		Site:        site,
		Legs:        legs,
		ScenarioTag: scenario,
		Result:      types.ParlayResultPending,
	}

	// Validate correlation
	parlay.CorrelationValid, parlay.CorrelationNotes = b.corrMatrix.CheckParlayCorrelation(legs, scenario)

	// Calculate joint probability
	parlay.JointProb = b.corrMatrix.CalculateJointProbability(legs, scenario)

	// Calculate payout and implied probability
	parlay.Payout, parlay.ImpliedProb = b.calculatePayout(legs, site)

	// Calculate EV
	parlay.EV = parlay.JointProb*parlay.Payout - 1.0

	// EV gate for UD/Betr
	parlay.EVGate = parlay.JointProb * parlay.Payout

	// Calculate stars (average of leg stars, weighted by edge)
	parlay.Stars = b.calculateParlayStars(evals)

	// Generate stop lines
	parlay.StopLines = b.generateStopLines(evals)

	// Validate against site rules
	violations := types.ValidateParlay(*parlay)
	parlay.RiskFlags = violations

	return parlay, nil
}

func (b *ParlayBuilder) calculatePayout(legs []types.ParlayLeg, site types.Site) (payout, impliedProb float64) {
	switch site {
	case types.SiteUnderdog, types.SiteBetr:
		// Power-based multipliers
		switch len(legs) {
		case 2:
			payout = 3.0
		case 3:
			payout = 6.0
		case 4:
			payout = 10.0
		case 5:
			payout = 20.0
		case 6:
			payout = 40.0
		default:
			payout = float64(len(legs)) * 2.0
		}
	case types.SitePrizePicks:
		// Fixed payouts (flex play)
		switch len(legs) {
		case 2:
			payout = 3.0
		case 3:
			payout = 5.0
		case 4:
			payout = 10.0
		case 5:
			payout = 20.0
		case 6:
			payout = 25.0
		default:
			payout = float64(len(legs)) * 3.0
		}
	case types.SiteDraftKings:
		// Parlay odds calculation
		payout = 1.0
		for _, leg := range legs {
			var legPayout float64
			if leg.Price > 0 {
				legPayout = 1.0 + float64(leg.Price)/100.0
			} else {
				legPayout = 1.0 + 100.0/float64(-leg.Price)
			}
			payout *= legPayout
		}
	}

	impliedProb = 1.0 / payout
	return
}

func (b *ParlayBuilder) calculateParlayStars(evals []*types.PropEvaluation) int {
	if len(evals) == 0 {
		return 0
	}

	totalWeightedStars := 0.0
	totalWeight := 0.0

	for _, eval := range evals {
		weight := eval.Edge
		if weight < 1 {
			weight = 1
		}
		totalWeightedStars += float64(eval.Stars) * weight
		totalWeight += weight
	}

	avgStars := totalWeightedStars / totalWeight
	return int(avgStars + 0.5) // Round to nearest
}

func (b *ParlayBuilder) generateStopLines(evals []*types.PropEvaluation) []string {
	stopLines := make([]string, len(evals))
	for i, eval := range evals {
		stopLines[i] = fmt.Sprintf("%s: %.1f @ %d",
			eval.Line.PlayerName, eval.StopLine, eval.StopPrice)
	}
	return stopLines
}

// BuildScenarioPureCards builds 2-3 scenario-pure cards per site
func (b *ParlayBuilder) BuildScenarioPureCards(
	allEvals []*types.PropEvaluation,
	scenario *types.GameScenario,
	site types.Site,
) ([]*types.Parlay, error) {

	var cards []*types.Parlay
	rules := types.GetSiteRules(site)

	// Group evals by scenario fit
	preferred := scenario.Primary.Scenario.GetPreferredProps()
	preferredMap := make(map[types.PropMarket]bool)
	for _, p := range preferred.Preferred {
		preferredMap[p] = true
	}

	// Filter to only passing guardrails and preferred markets
	var primaryEvals []*types.PropEvaluation
	var secondaryEvals []*types.PropEvaluation

	for _, eval := range allEvals {
		if !eval.PassesGuardrails {
			continue
		}

		if preferredMap[eval.Line.Market] {
			primaryEvals = append(primaryEvals, eval)
		} else {
			// Check not in avoid list
			isAvoid := false
			for _, a := range preferred.Avoid {
				if eval.Line.Market == a {
					isAvoid = true
					break
				}
			}
			if !isAvoid {
				secondaryEvals = append(secondaryEvals, eval)
			}
		}
	}

	// Card A: Primary scenario card (all preferred markets)
	if len(primaryEvals) >= rules.MinLegs {
		cardA, err := b.buildOptimalCard(primaryEvals, site, scenario.Primary.Scenario, rules, "A")
		if err == nil && cardA != nil {
			cards = append(cards, cardA)
		}
	}

	// Card B: Mixed card (primary + secondary)
	if len(primaryEvals)+len(secondaryEvals) >= rules.MinLegs {
		mixed := append(primaryEvals, secondaryEvals...)
		cardB, err := b.buildOptimalCard(mixed, site, scenario.Primary.Scenario, rules, "B")
		if err == nil && cardB != nil {
			cards = append(cards, cardB)
		}
	}

	// Card C: Secondary scenario if available
	if scenario.Secondary != nil {
		secondaryPreferred := scenario.Secondary.Scenario.GetPreferredProps()
		var secEvals []*types.PropEvaluation
		for _, eval := range allEvals {
			if !eval.PassesGuardrails {
				continue
			}
			for _, p := range secondaryPreferred.Preferred {
				if eval.Line.Market == p {
					secEvals = append(secEvals, eval)
					break
				}
			}
		}

		if len(secEvals) >= rules.MinLegs {
			cardC, err := b.buildOptimalCard(secEvals, site, scenario.Secondary.Scenario, rules, "C")
			if err == nil && cardC != nil {
				cards = append(cards, cardC)
			}
		}
	}

	return cards, nil
}

func (b *ParlayBuilder) buildOptimalCard(
	evals []*types.PropEvaluation,
	site types.Site,
	scenario types.ScenarioType,
	rules types.SiteRules,
	cardID string,
) (*types.Parlay, error) {

	// Ensure we have enough legs
	if len(evals) < rules.MinLegs {
		return nil, fmt.Errorf("not enough legs")
	}

	// Find optimal combination
	optimal := b.corrMatrix.FindOptimalCorrelatedLegs(
		convertToEvalSlice(evals),
		scenario,
		rules.MaxLegs,
	)

	// Ensure team diversity for UD/Betr
	if rules.MinTeams >= 2 {
		optimal = b.ensureTeamDiversity(optimal, rules.MinTeams)
	}

	// Ensure at least one under (unless Run-and-Gun)
	if scenario != types.ScenarioRunAndGun {
		optimal = b.ensureUnder(optimal, evals)
	}

	if len(optimal) < rules.MinLegs {
		return nil, fmt.Errorf("not enough legs after optimization")
	}

	// Convert to pointer slice
	optimalPtrs := make([]*types.PropEvaluation, len(optimal))
	for i := range optimal {
		optimalPtrs[i] = &optimal[i]
	}

	return b.BuildParlay(optimalPtrs, site, scenario)
}

func (b *ParlayBuilder) ensureTeamDiversity(evals []types.PropEvaluation, minTeams int) []types.PropEvaluation {
	teams := make(map[string][]types.PropEvaluation)
	for _, eval := range evals {
		teams[eval.Line.TeamAbbr] = append(teams[eval.Line.TeamAbbr], eval)
	}

	if len(teams) >= minTeams {
		return evals
	}

	// Need to diversify - take at most 2 from each team
	var result []types.PropEvaluation
	for _, teamEvals := range teams {
		count := 0
		for _, eval := range teamEvals {
			if count >= 2 {
				break
			}
			result = append(result, eval)
			count++
		}
	}

	return result
}

func (b *ParlayBuilder) ensureUnder(evals []types.PropEvaluation, allEvals []*types.PropEvaluation) []types.PropEvaluation {
	hasUnder := false
	for _, eval := range evals {
		if !eval.Line.Market.IsOver() || eval.Line.Side == types.PropSideUnder {
			hasUnder = true
			break
		}
	}

	if hasUnder {
		return evals
	}

	// Find best under from all evals
	var bestUnder *types.PropEvaluation
	for _, eval := range allEvals {
		if eval.PassesGuardrails && !eval.Line.Market.IsOver() {
			if bestUnder == nil || eval.Edge > bestUnder.Edge {
				bestUnder = eval
			}
		}
	}

	if bestUnder != nil {
		// Replace the weakest over with the under
		if len(evals) > 0 {
			weakest := 0
			for i, eval := range evals {
				if eval.Edge < evals[weakest].Edge {
					weakest = i
				}
			}
			evals[weakest] = *bestUnder
		}
	}

	return evals
}

func convertToEvalSlice(evals []*types.PropEvaluation) []types.PropEvaluation {
	result := make([]types.PropEvaluation, len(evals))
	for i, e := range evals {
		result[i] = *e
	}
	return result
}

// ValidateGoalieLeg checks goalie leg constraints
func (b *ParlayBuilder) ValidateGoalieLeg(parlay *types.Parlay) []string {
	var issues []string

	goalieLegs := 0
	goalieIDs := make(map[int]int)

	for _, leg := range parlay.Legs {
		if leg.Market == types.PropMarketSavesOver || leg.Market == types.PropMarketSavesUnder {
			goalieLegs++
			goalieIDs[leg.PlayerID]++
		}
	}

	// Max one goalie leg per card
	if goalieLegs > 1 {
		issues = append(issues, "Multiple goalie legs (max 1)")
	}

	// Never stack legs on same goalie
	for _, count := range goalieIDs {
		if count > 1 {
			issues = append(issues, "Same goalie stacked")
		}
	}

	return issues
}
