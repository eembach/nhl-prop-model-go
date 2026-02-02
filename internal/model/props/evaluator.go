package props

import (
	"fmt"
	"math"

	"github.com/paul/nhl-prop-model/internal/model/correlation"
	"github.com/paul/nhl-prop-model/internal/model/distributions"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Evaluator evaluates props and applies guardrails
type Evaluator struct {
	factory     *distributions.Factory
	corrMatrix  *correlation.Matrix
	starCutoffs map[int]float64

	// Guardrail parameters
	MinTOIForward  float64 // Minimum TOI for forwards (default 14)
	MinTOIDefense  float64 // Minimum TOI for defensemen (default 18)
	MinTeamGF      float64 // Minimum team GF/game for point overs (default 2.5)
	MaxDepthLine   int     // Maximum line number to avoid depth (default 3)

	// Methodology parameters (from spec)
	MinTOIUsageAnchor   float64 // Minimum TOI for "usage anchor" status (default 16)
	MinPPTOIForPP1      float64 // Minimum PPTOI to be considered PP1 caliber (default 1.5)
	MinSOGForVolume     float64 // Minimum SOG/G for volume shooter status (default 2.5)
	MinEdgeForPlay      float64 // Minimum edge to recommend a play (default 0)
	HighEdgeThreshold   float64 // Edge threshold for high-confidence plays (default 10)
}

// NewEvaluator creates a new prop evaluator
func NewEvaluator() *Evaluator {
	return &Evaluator{
		factory:        distributions.NewFactory(),
		corrMatrix:     correlation.NewMatrix(),
		starCutoffs:    types.DefaultStarCutoffs(),
		MinTOIForward:  14.0,
		MinTOIDefense:  18.0,
		MinTeamGF:      2.5,
		MaxDepthLine:   3,
		// Methodology parameters from spec
		MinTOIUsageAnchor: 16.0,  // High TOI threshold for usage anchor
		MinPPTOIForPP1:    1.5,   // PP1 caliber requires 1.5+ PPTOI
		MinSOGForVolume:   2.5,   // Volume shooter threshold
		MinEdgeForPlay:    0.0,   // Minimum edge to recommend
		HighEdgeThreshold: 10.0,  // High-confidence edge threshold
	}
}

// WithCorrMatrix sets a custom correlation matrix
func (e *Evaluator) WithCorrMatrix(m *correlation.Matrix) *Evaluator {
	e.corrMatrix = m
	return e
}

// WithStarCutoffs sets custom star rating cutoffs
func (e *Evaluator) WithStarCutoffs(cutoffs map[int]float64) *Evaluator {
	e.starCutoffs = cutoffs
	return e
}

// IsUsageAnchor determines if a player qualifies as a "usage anchor" per methodology
// Usage anchor = High TOI (≥16min) OR PP1 caliber (≥1.5 PPTOI)
func (e *Evaluator) IsUsageAnchor(stats *types.PlayerStats) bool {
	// High TOI threshold
	if stats.TOI >= e.MinTOIUsageAnchor {
		return true
	}
	// PP1 caliber
	if stats.PPTOI >= e.MinPPTOIForPP1 {
		return true
	}
	// Top line status also qualifies
	if stats.IsTopSix || stats.IsTopFour {
		return true
	}
	return false
}

// IsVolumeShooter determines if a player qualifies as a "volume shooter" per methodology
// Volume shooter = ≥2.5-3.0 SOG/G historical average
func (e *Evaluator) IsVolumeShooter(stats *types.PlayerStats) bool {
	return stats.SOG >= e.MinSOGForVolume
}

// IsPP1Caliber determines if a player has PP1-level power play usage
func (e *Evaluator) IsPP1Caliber(stats *types.PlayerStats) bool {
	return stats.PPTOI >= e.MinPPTOIForPP1 || stats.PP1Unit
}

// GetRecommendedSide returns the recommended side (over/under) based on scenario
// Per methodology: Unders in Goalie Duel/Bench Slog, Overs elsewhere
func (e *Evaluator) GetRecommendedSide(market types.PropMarket, scenario types.ScenarioType) types.PropSide {
	// Scenario-based side selection per methodology
	switch scenario {
	case types.ScenarioGoalieDuel, types.ScenarioBenchSlog:
		// Prefer unders in low-event games
		if market.IsOver() {
			return types.PropSideUnder
		}
		return types.PropSideOver // If market is under, we want over for negative correlation
	case types.ScenarioRunAndGun, types.ScenarioScoreEffects, types.ScenarioSpecialTeams:
		// Prefer overs in high-event games
		if market.IsOver() {
			return types.PropSideOver
		}
		return types.PropSideUnder
	default:
		// Neutral - go with market direction
		if market.IsOver() {
			return types.PropSideOver
		}
		return types.PropSideUnder
	}
}

// EvaluateProp evaluates a single prop line
func (e *Evaluator) EvaluateProp(
	line types.PropLine,
	player *types.PlayerWithStats,
	ctx distributions.DistributionContext,
	scenario *types.GameScenario,
) *types.PropEvaluation {

	eval := &types.PropEvaluation{
		Line:       line,
		ScenarioTag: scenario.Primary.Scenario,
	}

	// Create distribution for this market
	dist := e.factory.CreateDistribution(line.Market, &player.Stats, ctx)

	// Calculate our projected line (mean of distribution)
	eval.OurLine = dist.Mean()

	// Calculate probabilities
	if line.Market.IsOver() || line.Side == types.PropSideOver {
		eval.ProbOver = dist.ProbOver(line.Line)
		eval.ProbUnder = 1.0 - eval.ProbOver
	} else {
		eval.ProbUnder = dist.ProbUnder(line.Line)
		eval.ProbOver = 1.0 - eval.ProbUnder
	}

	// Calculate edge
	probHit := eval.ProbOver
	if line.Side == types.PropSideUnder {
		probHit = eval.ProbUnder
	}

	// Edge = our probability - implied probability (in percentage points)
	eval.Edge = (probHit - line.ImpliedProb) * 100

	// Calculate EV
	// EV = (prob_win * payout) - (prob_lose * stake)
	// For American odds: payout = odds/100 if +, or 100/abs(odds) if -
	var payout float64
	if line.Price > 0 {
		payout = float64(line.Price) / 100.0
	} else {
		payout = 100.0 / math.Abs(float64(line.Price))
	}
	eval.EV = probHit*payout - (1-probHit)

	// EV multiplied for UD/Betr (p × multiplier)
	// Assume standard 2-leg multiplier of 3x
	eval.EVMultiplied = probHit * 3.0

	// Assign tier
	isTopLine := player.Stats.IsTopSix || player.Stats.IsTopFour
	isPP1 := player.Stats.PP1Unit
	eval.Tier = types.GetMarketTier(line.Market, isTopLine, isPP1)

	// Calculate scenario fit
	eval.ScenarioFit = e.calculateScenarioFit(line.Market, scenario)

	// Assign stars
	eval.Stars = types.GetStars(eval.Edge, e.starCutoffs)

	// Apply guardrails
	eval.PassesGuardrails, eval.GuardrailFlags = e.applyGuardrails(line, player, ctx)

	// Calculate stop line
	eval.StopLine, eval.StopPrice = e.calculateStopLine(line, eval.Edge)

	// Generate determinants
	eval.Determinants = e.generateDeterminants(player, ctx, scenario)

	// Generate risk flags
	eval.RiskFlags = e.generateRiskFlags(player, ctx)

	// Determine site fit
	eval.FitsUD = eval.EVMultiplied >= 1.00 && eval.PassesGuardrails
	eval.FitsPP = eval.PassesGuardrails
	eval.FitsDK = eval.PassesGuardrails
	eval.FitsBetr = eval.EVMultiplied >= 1.00 && eval.PassesGuardrails

	return eval
}

func (e *Evaluator) calculateScenarioFit(market types.PropMarket, scenario *types.GameScenario) float64 {
	preferred := scenario.Primary.Scenario.GetPreferredProps()

	// Check if preferred
	for _, p := range preferred.Preferred {
		if market == p {
			return 1.0
		}
	}

	// Check if avoided
	for _, a := range preferred.Avoid {
		if market == a {
			return 0.0
		}
	}

	// Neutral
	return 0.5
}

func (e *Evaluator) applyGuardrails(line types.PropLine, player *types.PlayerWithStats, ctx distributions.DistributionContext) (bool, []string) {
	var flags []string
	passes := true

	stats := player.Stats
	pos := player.Player.Position

	// Guardrail 1: No depth-role Overs
	if line.Market.IsOver() && stats.IsDepthRole {
		flags = append(flags, "Depth role (4th line/3rd pair)")
		passes = false
	}

	// Guardrail 2: TOI minimums
	minTOI := e.MinTOIForward
	if pos == types.PositionDefense {
		minTOI = e.MinTOIDefense
	}
	if stats.TOI < minTOI && line.Market.IsOver() {
		flags = append(flags, fmt.Sprintf("TOI below minimum (%.1f < %.1f)", stats.TOI, minTOI))
		passes = false
	}

	// Guardrail 3: Top-6 F / Top-4 D required for point props
	if (line.Market == types.PropMarketPointsOver || line.Market == types.PropMarketPointsUnder) {
		if pos == types.PositionDefense && !stats.IsTopFour {
			flags = append(flags, "Not top-4 D for points prop")
			passes = false
		} else if pos != types.PositionDefense && !stats.IsTopSix && pos != types.PositionGoalie {
			flags = append(flags, "Not top-6 F for points prop")
			passes = false
		}
	}

	// Guardrail 4: PP1 required for assist props
	if (line.Market == types.PropMarketAssistsOver) && !stats.PP1Unit {
		flags = append(flags, "Not PP1 for assists over")
		passes = false
	}

	// Guardrail 5: Team total sanity check
	if line.Market.IsOver() && ctx.TeamTotal < e.MinTeamGF {
		flags = append(flags, fmt.Sprintf("Weak team offense (%.2f GF/G)", ctx.TeamTotal))
		// Warning but don't fail unless severe
		if ctx.TeamTotal < 2.0 {
			passes = false
		}
	}

	// Guardrail 6: Line number check for depth
	if stats.LineNumber >= 4 && line.Market.IsOver() {
		flags = append(flags, "4th line or lower")
		passes = false
	}

	// Guardrail 7: Volatile PP2 check
	if stats.PPTOI > 0 && stats.PPTOI < 1.5 && !stats.PP1Unit {
		if line.Market == types.PropMarketPPPointsOver {
			flags = append(flags, "Volatile PP2 time")
			passes = false
		}
	}

	// Guardrail 8: SOG line sanity
	if line.Market == types.PropMarketSOGOver && line.Line >= 4.5 {
		// O4.5 SOG is risky - require stronger edge
		flags = append(flags, "High SOG line (≥4.5)")
	}

	return passes, flags
}

func (e *Evaluator) calculateStopLine(line types.PropLine, edge float64) (float64, int) {
	// Stop line: where our edge would drop to near zero
	// For overs, stop if line goes up
	// For unders, stop if line goes down

	var stopLine float64
	var stopPrice int

	if line.Market.IsOver() || line.Side == types.PropSideOver {
		// For overs, stop at higher line
		stopLine = line.Line + 0.5
		// Stop price: don't play if price goes worse than -150
		stopPrice = -150
	} else {
		// For unders, stop at lower line
		stopLine = line.Line - 0.5
		stopPrice = -150
	}

	// Adjust based on edge magnitude
	// If we have a big edge, we can tolerate more line movement
	if edge > 8 {
		if line.Market.IsOver() {
			stopLine = line.Line + 1.0
		} else {
			stopLine = line.Line - 1.0
		}
		stopPrice = -175
	}

	return stopLine, stopPrice
}

func (e *Evaluator) generateDeterminants(player *types.PlayerWithStats, ctx distributions.DistributionContext, scenario *types.GameScenario) []string {
	var determinants []string

	stats := player.Stats

	// TOI
	if ctx.ExpectedTOI > stats.TOI*1.1 {
		determinants = append(determinants, fmt.Sprintf("TOI↑ (%.1f exp)", ctx.ExpectedTOI))
	}

	// PP role
	if ctx.PP1Unit {
		determinants = append(determinants, "PP1")
	}

	// Recent form
	if stats.RecentPPG > stats.PointsPerGame*1.2 {
		determinants = append(determinants, "Hot streak")
	} else if stats.RecentPPG < stats.PointsPerGame*0.8 {
		determinants = append(determinants, "Cold streak")
	}

	// Scenario
	determinants = append(determinants, string(scenario.Primary.Scenario))

	// Team strength
	if ctx.TeamTotal > 3.5 {
		determinants = append(determinants, fmt.Sprintf("Strong offense (%.1f TT)", ctx.TeamTotal))
	}

	// Opponent
	if ctx.OppGA60 > 3.2 {
		determinants = append(determinants, "Weak opp D")
	}

	// Limit to 120 chars total
	result := ""
	for i, d := range determinants {
		if i > 0 {
			result += ", "
		}
		if len(result)+len(d) > 115 {
			break
		}
		result += d
	}

	return determinants
}

func (e *Evaluator) generateRiskFlags(player *types.PlayerWithStats, ctx distributions.DistributionContext) []string {
	var flags []string

	// Fatigue
	if ctx.IsB2B {
		flags = append(flags, "B2B")
	}
	if ctx.Is3In4 {
		flags = append(flags, "3in4")
	}

	// Arena bias
	if ctx.ArenaSOGBias > 1.05 {
		flags = append(flags, "Rink↑")
	} else if ctx.ArenaSOGBias < 0.95 {
		flags = append(flags, "Rink↓")
	}

	// Travel
	if !ctx.HomeGame && ctx.FatigueFactor > 0.1 {
		flags = append(flags, "Travel")
	}

	// Role instability
	if player.Stats.RoleStability < 0.5 {
		flags = append(flags, "Unstable role")
	}

	// Outlier spree
	if player.Stats.OutlierSpreeFlag {
		flags = append(flags, "Outlier spree")
	}

	return flags
}

// QualifyPick applies full methodology filters to determine if a pick qualifies
// Returns (qualifies bool, reason string, recommendedSide PropSide)
func (e *Evaluator) QualifyPick(
	eval *types.PropEvaluation,
	player *types.PlayerWithStats,
	scenario types.ScenarioType,
) (bool, string, types.PropSide) {
	market := eval.Line.Market
	stats := player.Stats

	// Get scenario-based recommended side
	recommendedSide := e.GetRecommendedSide(market, scenario)

	// Tier 1: Points O0.5 - requires usage anchor status
	if market == types.PropMarketPointsOver && eval.Line.Line == 0.5 {
		if !e.IsUsageAnchor(&stats) {
			return false, "Not a usage anchor (TOI<16, not PP1, not top-6/4)", recommendedSide
		}
		// In Goalie Duel/Bench Slog scenarios, recommend under instead
		if scenario == types.ScenarioGoalieDuel || scenario == types.ScenarioBenchSlog {
			recommendedSide = types.PropSideUnder
		}
	}

	// Tier 1.5: SOG Overs - requires volume shooter status
	if market == types.PropMarketSOGOver {
		if !e.IsVolumeShooter(&stats) {
			return false, fmt.Sprintf("Not a volume shooter (SOG/G=%.1f < %.1f)", stats.SOG, e.MinSOGForVolume), recommendedSide
		}
		// In Goalie Duel/Bench Slog, recommend SOG under
		if scenario == types.ScenarioGoalieDuel || scenario == types.ScenarioBenchSlog {
			recommendedSide = types.PropSideUnder
		}
	}

	// Tier 2: Assists Overs - requires PP1 caliber
	if market == types.PropMarketAssistsOver {
		if !e.IsPP1Caliber(&stats) {
			return false, "Not PP1 caliber for assists prop", recommendedSide
		}
	}

	// Check edge threshold
	if eval.Edge < e.MinEdgeForPlay {
		return false, fmt.Sprintf("Edge below minimum (%.1f%% < %.1f%%)", eval.Edge, e.MinEdgeForPlay), recommendedSide
	}

	// Check guardrails
	if !eval.PassesGuardrails {
		return false, fmt.Sprintf("Failed guardrails: %v", eval.GuardrailFlags), recommendedSide
	}

	return true, "Qualifies under methodology", recommendedSide
}

// EvaluateWithMethodology performs a full methodology-aligned evaluation
func (e *Evaluator) EvaluateWithMethodology(
	line types.PropLine,
	player *types.PlayerWithStats,
	ctx distributions.DistributionContext,
	scenario *types.GameScenario,
) (*types.PropEvaluation, bool, string, types.PropSide) {
	// First do standard evaluation
	eval := e.EvaluateProp(line, player, ctx, scenario)

	// Then apply methodology qualification
	qualifies, reason, recommendedSide := e.QualifyPick(eval, player, scenario.Primary.Scenario)

	// Add methodology flags to evaluation
	eval.MethodologyQualified = qualifies
	eval.MethodologyReason = reason
	eval.RecommendedSide = recommendedSide

	return eval, qualifies, reason, recommendedSide
}

// EvaluateBatch evaluates a batch of props for a game
func (e *Evaluator) EvaluateBatch(
	lines []types.PropLine,
	players map[int]*types.PlayerWithStats,
	ctx distributions.DistributionContext,
	scenario *types.GameScenario,
) []*types.PropEvaluation {

	var evals []*types.PropEvaluation

	for _, line := range lines {
		player, ok := players[line.PlayerID]
		if !ok {
			continue
		}

		eval := e.EvaluateProp(line, player, ctx, scenario)
		evals = append(evals, eval)
	}

	// Sort by edge descending
	for i := 0; i < len(evals); i++ {
		for j := i + 1; j < len(evals); j++ {
			if evals[j].Edge > evals[i].Edge {
				evals[i], evals[j] = evals[j], evals[i]
			}
		}
	}

	return evals
}

// FilterByTier filters evaluations by tier
func (e *Evaluator) FilterByTier(evals []*types.PropEvaluation, maxTier types.PropTier) []*types.PropEvaluation {
	var filtered []*types.PropEvaluation
	for _, eval := range evals {
		tierValue := int(eval.Tier)
		if eval.Tier == types.PropTier1_5 {
			tierValue = 1 // Treat 1.5 as between 1 and 2
		}
		if tierValue <= int(maxTier) || eval.Tier == types.PropTier1_5 && maxTier >= types.PropTier1 {
			filtered = append(filtered, eval)
		}
	}
	return filtered
}

// FilterByEdge filters evaluations by minimum edge
func (e *Evaluator) FilterByEdge(evals []*types.PropEvaluation, minEdge float64) []*types.PropEvaluation {
	var filtered []*types.PropEvaluation
	for _, eval := range evals {
		if eval.Edge >= minEdge {
			filtered = append(filtered, eval)
		}
	}
	return filtered
}

// FilterByGuardrails returns only props that pass all guardrails
func (e *Evaluator) FilterByGuardrails(evals []*types.PropEvaluation) []*types.PropEvaluation {
	var filtered []*types.PropEvaluation
	for _, eval := range evals {
		if eval.PassesGuardrails {
			filtered = append(filtered, eval)
		}
	}
	return filtered
}

// GetTopPicks returns the top N picks for a game
func (e *Evaluator) GetTopPicks(evals []*types.PropEvaluation, n int, mustPassGuardrails bool) []*types.PropEvaluation {
	var candidates []*types.PropEvaluation

	if mustPassGuardrails {
		candidates = e.FilterByGuardrails(evals)
	} else {
		candidates = evals
	}

	// Already sorted by edge
	if len(candidates) <= n {
		return candidates
	}

	return candidates[:n]
}

// RecommendedPicksPerTeam returns recommended picks per team (max 4 as per spec)
func (e *Evaluator) RecommendedPicksPerTeam(evals []*types.PropEvaluation) []*types.PropEvaluation {
	// Group by team
	byTeam := make(map[string][]*types.PropEvaluation)
	for _, eval := range evals {
		if eval.PassesGuardrails {
			byTeam[eval.Line.TeamAbbr] = append(byTeam[eval.Line.TeamAbbr], eval)
		}
	}

	var recommended []*types.PropEvaluation
	for _, teamEvals := range byTeam {
		// Sort by tier then edge
		for i := 0; i < len(teamEvals); i++ {
			for j := i + 1; j < len(teamEvals); j++ {
				if teamEvals[j].Tier < teamEvals[i].Tier ||
					(teamEvals[j].Tier == teamEvals[i].Tier && teamEvals[j].Edge > teamEvals[i].Edge) {
					teamEvals[i], teamEvals[j] = teamEvals[j], teamEvals[i]
				}
			}
		}

		// Take up to 4 per team, prioritizing Tier 1 then 1.5
		count := 0
		for _, eval := range teamEvals {
			if count >= 4 {
				break
			}
			recommended = append(recommended, eval)
			count++
		}
	}

	return recommended
}
