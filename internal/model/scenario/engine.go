package scenario

import (
	"fmt"
	"math"
	"sort"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// Engine handles scenario classification for games
type Engine struct {
	// Threshold parameters for each scenario
	Thresholds ScenarioThresholds
}

// ScenarioThresholds contains the thresholds for scenario classification
type ScenarioThresholds struct {
	// Run-and-Gun thresholds
	HighPace        float64 // Combined CF/60 threshold
	WeakGSAx        float64 // Threshold for weak goaltending (negative GSAx)
	StrongOffense   float64 // xGF/60 threshold for strong offense

	// Goalie Duel thresholds
	EliteGSAx       float64 // GSAx threshold for elite goalie
	LowPace         float64 // Low pace threshold
	LowTotal        float64 // Expected total under this is low-scoring

	// Special Teams thresholds
	HighPIM         float64 // Combined PIM/game threshold
	WeakPK          float64 // PK% below this is weak
	StrongPP        float64 // PP% above this is strong

	// Score Effects thresholds
	BigFavorite     float64 // Win probability for big favorite
	BigSpread       float64 // Puck line spread magnitude

	// Bench Slog thresholds
	FatigueLevel    float64 // Fatigue score threshold
	TrapSystem      float64 // Low pace for trap systems
}

// DefaultThresholds returns sensible default thresholds
func DefaultThresholds() ScenarioThresholds {
	return ScenarioThresholds{
		// Run-and-Gun
		HighPace:      120.0, // Combined CF/60
		WeakGSAx:      -3.0,  // Negative GSAx = below average
		StrongOffense: 3.0,   // xGF/60 above this

		// Goalie Duel
		EliteGSAx:     5.0,  // Top-tier GSAx
		LowPace:       100.0, // Lower combined pace
		LowTotal:      5.0,   // Under 5.0 expected total

		// Special Teams
		HighPIM:       18.0, // Combined 18+ PIM/game (stricter)
		WeakPK:        77.0, // PK% below 77%
		StrongPP:      25.0, // PP% above 25%

		// Score Effects
		BigFavorite:   0.62, // 62%+ implied win
		BigSpread:     1.5,  // -1.5 or wider

		// Bench Slog
		FatigueLevel:  0.15, // 15% fatigue penalty
		TrapSystem:    95.0, // Very low pace
	}
}

// NewEngine creates a new scenario engine
func NewEngine() *Engine {
	return &Engine{
		Thresholds: DefaultThresholds(),
	}
}

// WithThresholds sets custom thresholds
func (e *Engine) WithThresholds(t ScenarioThresholds) *Engine {
	e.Thresholds = t
	return e
}

// ClassifyGame returns the scenario classification for a game
func (e *Engine) ClassifyGame(homeStats, awayStats *types.TeamStats, homeGoalie, awayGoalie *types.GoalieStats, ctx *types.GameContext) *types.GameScenario {
	// Calculate metrics
	metrics := e.calculateMetrics(homeStats, awayStats, homeGoalie, awayGoalie, ctx)

	// Score each scenario
	scores := e.scoreAllScenarios(metrics)

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Assign ranks
	for i := range scores {
		scores[i].Rank = i + 1
	}

	scenario := &types.GameScenario{
		GameID:    ctx.GameID,
		Primary:   scores[0],
		AllScores: scores,
		Metrics:   metrics,
	}

	// Add secondary if score is close enough (within 20 points)
	if len(scores) > 1 && scores[1].Score >= scores[0].Score*0.8 {
		scenario.Secondary = &scores[1]
	}

	return scenario
}

func (e *Engine) calculateMetrics(homeStats, awayStats *types.TeamStats, homeGoalie, awayGoalie *types.GoalieStats, ctx *types.GameContext) types.ScenarioMetrics {
	metrics := types.ScenarioMetrics{}

	// Pace metrics
	if homeStats != nil && awayStats != nil {
		metrics.CombinedPace = homeStats.CF60 + awayStats.CF60
		metrics.HomeSuppression = homeStats.CA60
		metrics.AwaySuppression = awayStats.CA60

		// Special teams
		metrics.HomePPPct = homeStats.PPPct
		metrics.AwayPPPct = awayStats.PPPct
		metrics.HomePKPct = homeStats.PKPct
		metrics.AwayPKPct = awayStats.PKPct
		metrics.CombinedPIMPG = homeStats.PIMPerGame + awayStats.PIMPerGame

		// Expected total from team stats
		metrics.ExpectedTotal = homeStats.GFPerGame + awayStats.GFPerGame
	}

	// Goalie metrics
	if homeGoalie != nil {
		metrics.HomeGSAx = homeGoalie.GSAx
		metrics.CombinedGSAx += homeGoalie.GSAx
	}
	if awayGoalie != nil {
		metrics.AwayGSAx = awayGoalie.GSAx
		metrics.CombinedGSAx += awayGoalie.GSAx
	}

	// Fatigue metrics
	if ctx != nil {
		metrics.HomeFatigue = ctx.HomeFatigue
		metrics.AwayFatigue = ctx.AwayFatigue
		metrics.CombinedFatigue = ctx.HomeFatigue + ctx.AwayFatigue

		// Score effects from Vegas lines
		if ctx.ImpliedHomeWin > ctx.ImpliedAwayWin {
			metrics.ImpliedFavorite = ctx.HomeTeamAbbr
			metrics.FavoriteWinProb = ctx.ImpliedHomeWin
		} else {
			metrics.ImpliedFavorite = ctx.AwayTeamAbbr
			metrics.FavoriteWinProb = ctx.ImpliedAwayWin
		}
		metrics.SpreadMagnitude = math.Abs(ctx.HomeSpread)

		// Override expected total if we have Vegas line
		if ctx.TotalLine > 0 {
			metrics.ExpectedTotal = ctx.TotalLine
		}
	}

	return metrics
}

func (e *Engine) scoreAllScenarios(m types.ScenarioMetrics) []types.ScenarioScore {
	return []types.ScenarioScore{
		e.scoreRunAndGun(m),
		e.scoreGoalieDuel(m),
		e.scoreSpecialTeams(m),
		e.scoreScoreEffects(m),
		e.scoreBenchSlog(m),
	}
}

func (e *Engine) scoreRunAndGun(m types.ScenarioMetrics) types.ScenarioScore {
	score := types.ScenarioScore{
		Scenario: types.ScenarioRunAndGun,
		Score:    0,
		Factors:  []string{},
	}

	// High pace
	if m.CombinedPace > e.Thresholds.HighPace {
		paceScore := (m.CombinedPace - e.Thresholds.HighPace) / 10 * 20
		score.Score += math.Min(paceScore, 30)
		score.Factors = append(score.Factors, fmt.Sprintf("High pace: %.1f CF/60", m.CombinedPace))
	}

	// Weak goaltending
	if m.CombinedGSAx < e.Thresholds.WeakGSAx {
		gsaxScore := math.Abs(m.CombinedGSAx-e.Thresholds.WeakGSAx) * 5
		score.Score += math.Min(gsaxScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("Weak GSAx: %.1f", m.CombinedGSAx))
	}

	// High expected total
	if m.ExpectedTotal > 6.0 {
		totalScore := (m.ExpectedTotal - 6.0) * 15
		score.Score += math.Min(totalScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("High total: %.1f", m.ExpectedTotal))
	}

	// Weak suppression
	avgSuppression := (m.HomeSuppression + m.AwaySuppression) / 2
	if avgSuppression > 62 { // Above average CA/60 = weak defense
		suppScore := (avgSuppression - 60) * 3
		score.Score += math.Min(suppScore, 20)
		score.Factors = append(score.Factors, fmt.Sprintf("Weak D: %.1f CA/60", avgSuppression))
	}

	// Penalty for fatigue (less likely in tired games)
	score.Score -= m.CombinedFatigue * 50

	return score
}

func (e *Engine) scoreGoalieDuel(m types.ScenarioMetrics) types.ScenarioScore {
	score := types.ScenarioScore{
		Scenario: types.ScenarioGoalieDuel,
		Score:    0,
		Factors:  []string{},
	}

	// Elite goaltending
	if m.CombinedGSAx > e.Thresholds.EliteGSAx {
		gsaxScore := (m.CombinedGSAx - e.Thresholds.EliteGSAx) * 8
		score.Score += math.Min(gsaxScore, 35)
		score.Factors = append(score.Factors, fmt.Sprintf("Elite GSAx: %.1f", m.CombinedGSAx))
	}

	// Low pace
	if m.CombinedPace < e.Thresholds.LowPace {
		paceScore := (e.Thresholds.LowPace - m.CombinedPace) / 5 * 15
		score.Score += math.Min(paceScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("Low pace: %.1f", m.CombinedPace))
	}

	// Low expected total
	if m.ExpectedTotal < e.Thresholds.LowTotal {
		totalScore := (e.Thresholds.LowTotal - m.ExpectedTotal) * 15
		score.Score += math.Min(totalScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("Low total: %.1f", m.ExpectedTotal))
	}

	// Strong suppression
	avgSuppression := (m.HomeSuppression + m.AwaySuppression) / 2
	if avgSuppression < 58 { // Below average CA/60 = strong defense
		suppScore := (58 - avgSuppression) * 4
		score.Score += math.Min(suppScore, 15)
		score.Factors = append(score.Factors, fmt.Sprintf("Strong D: %.1f CA/60", avgSuppression))
	}

	return score
}

func (e *Engine) scoreSpecialTeams(m types.ScenarioMetrics) types.ScenarioScore {
	score := types.ScenarioScore{
		Scenario: types.ScenarioSpecialTeams,
		Score:    0,
		Factors:  []string{},
	}

	// High combined PIM
	if m.CombinedPIMPG > e.Thresholds.HighPIM {
		pimScore := (m.CombinedPIMPG - e.Thresholds.HighPIM) * 4
		score.Score += math.Min(pimScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("High PIM: %.1f/game", m.CombinedPIMPG))
	}

	// Weak PK vs Strong PP matchup
	if m.HomePKPct < e.Thresholds.WeakPK && m.AwayPPPct > e.Thresholds.StrongPP {
		mismatchScore := 20.0
		score.Score += mismatchScore
		score.Factors = append(score.Factors, fmt.Sprintf("Away PP (%.1f%%) vs Home PK (%.1f%%)", m.AwayPPPct, m.HomePKPct))
	}
	if m.AwayPKPct < e.Thresholds.WeakPK && m.HomePPPct > e.Thresholds.StrongPP {
		mismatchScore := 20.0
		score.Score += mismatchScore
		score.Factors = append(score.Factors, fmt.Sprintf("Home PP (%.1f%%) vs Away PK (%.1f%%)", m.HomePPPct, m.AwayPKPct))
	}

	// Strong PP on either side
	maxPP := math.Max(m.HomePPPct, m.AwayPPPct)
	if maxPP > e.Thresholds.StrongPP {
		ppScore := (maxPP - e.Thresholds.StrongPP) * 3
		score.Score += math.Min(ppScore, 20)
		score.Factors = append(score.Factors, fmt.Sprintf("Strong PP: %.1f%%", maxPP))
	}

	// Weak PK on either side
	minPK := math.Min(m.HomePKPct, m.AwayPKPct)
	if minPK < e.Thresholds.WeakPK {
		pkScore := (e.Thresholds.WeakPK - minPK) * 3
		score.Score += math.Min(pkScore, 20)
		score.Factors = append(score.Factors, fmt.Sprintf("Weak PK: %.1f%%", minPK))
	}

	return score
}

func (e *Engine) scoreScoreEffects(m types.ScenarioMetrics) types.ScenarioScore {
	score := types.ScenarioScore{
		Scenario: types.ScenarioScoreEffects,
		Score:    0,
		Factors:  []string{},
	}

	// Big favorite
	if m.FavoriteWinProb > e.Thresholds.BigFavorite {
		favScore := (m.FavoriteWinProb - e.Thresholds.BigFavorite) * 150
		score.Score += math.Min(favScore, 35)
		score.Factors = append(score.Factors, fmt.Sprintf("Big favorite: %.0f%% implied", m.FavoriteWinProb*100))
	}

	// Large spread
	if m.SpreadMagnitude >= e.Thresholds.BigSpread {
		spreadScore := (m.SpreadMagnitude - e.Thresholds.BigSpread + 1) * 15
		score.Score += math.Min(spreadScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("Large spread: %.1f", m.SpreadMagnitude))
	}

	// Combine with high total for chase potential
	if m.ExpectedTotal > 6.0 && m.FavoriteWinProb > 0.60 {
		score.Score += 15
		score.Factors = append(score.Factors, "High total + favorite = chase potential")
	}

	// Penalty if game is likely to be close
	if m.FavoriteWinProb < 0.55 {
		score.Score -= 20
	}

	return score
}

func (e *Engine) scoreBenchSlog(m types.ScenarioMetrics) types.ScenarioScore {
	score := types.ScenarioScore{
		Scenario: types.ScenarioBenchSlog,
		Score:    0,
		Factors:  []string{},
	}

	// High fatigue
	if m.CombinedFatigue > e.Thresholds.FatigueLevel {
		fatigueScore := m.CombinedFatigue * 150
		score.Score += math.Min(fatigueScore, 35)
		if m.HomeFatigue > 0 {
			score.Factors = append(score.Factors, "Home team fatigued")
		}
		if m.AwayFatigue > 0 {
			score.Factors = append(score.Factors, "Away team fatigued")
		}
	}

	// Very low pace (trap system)
	if m.CombinedPace < e.Thresholds.TrapSystem {
		paceScore := (e.Thresholds.TrapSystem - m.CombinedPace) / 3 * 10
		score.Score += math.Min(paceScore, 25)
		score.Factors = append(score.Factors, fmt.Sprintf("Low pace: %.1f CF/60", m.CombinedPace))
	}

	// Low expected total
	if m.ExpectedTotal < 5.5 {
		totalScore := (5.5 - m.ExpectedTotal) * 12
		score.Score += math.Min(totalScore, 20)
		score.Factors = append(score.Factors, fmt.Sprintf("Low total: %.1f", m.ExpectedTotal))
	}

	// Strong goaltending contributes to slog
	if m.CombinedGSAx > 3.0 {
		score.Score += 10
		score.Factors = append(score.Factors, "Strong goaltending")
	}

	// Penalty if high-scoring potential
	if m.ExpectedTotal > 6.5 {
		score.Score -= 25
	}

	return score
}

// GetScenarioDescription returns a human-readable description
func (e *Engine) GetScenarioDescription(scenario *types.GameScenario) string {
	primary := scenario.Primary

	desc := fmt.Sprintf("Primary: %s (%.0f pts)\n", primary.Scenario, primary.Score)
	desc += fmt.Sprintf("  %s\n", primary.Scenario.Description())
	if len(primary.Factors) > 0 {
		desc += "  Factors: "
		for i, f := range primary.Factors {
			if i > 0 {
				desc += ", "
			}
			desc += f
		}
		desc += "\n"
	}

	if scenario.Secondary != nil {
		desc += fmt.Sprintf("Secondary: %s (%.0f pts)\n", scenario.Secondary.Scenario, scenario.Secondary.Score)
	}

	return desc
}

// DetermineActualScenario determines what scenario actually occurred based on boxscore
func (e *Engine) DetermineActualScenario(box *types.Boxscore) types.ScenarioType {
	totalGoals := box.HomeScore + box.AwayScore
	totalShots := 0
	totalHits := 0
	totalPIM := 0

	for _, ps := range box.PlayerStats {
		totalShots += ps.SOG
		totalHits += ps.Hits
		totalPIM += ps.PIM
	}

	// Large margin = Score Effects (check first - blowouts are score effects even if high-scoring)
	margin := abs(box.HomeScore - box.AwayScore)
	if margin >= 4 {
		return types.ScenarioScoreEffects
	}

	// High-scoring = Run-and-Gun
	if totalGoals >= 7 {
		return types.ScenarioRunAndGun
	}

	// Low-scoring = Goalie Duel
	if totalGoals <= 3 {
		return types.ScenarioGoalieDuel
	}

	// High PIM = Special Teams
	if totalPIM >= 20 {
		return types.ScenarioSpecialTeams
	}

	// High hits, low goals = Bench Slog
	if totalHits >= 50 && totalGoals <= 4 {
		return types.ScenarioBenchSlog
	}

	// Medium margin = Score Effects
	if margin >= 3 {
		return types.ScenarioScoreEffects
	}

	// Default to Run-and-Gun for moderate games
	return types.ScenarioRunAndGun
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
