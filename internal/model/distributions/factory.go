package distributions

import (
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Distribution defines the interface for all statistical distributions
type Distribution interface {
	Mean() float64
	Variance() float64
	ProbOver(line float64) float64
	ProbUnder(line float64) float64
	PMF(k int) float64
}

// Factory creates the appropriate distribution based on market type
type Factory struct {
	// Default covariate values
	DefaultSOGCov      SOGCovariates
	DefaultPhysicalCov PhysicalCovariates
	DefaultPointsCov   PointsCovariates
}

// NewFactory creates a new distribution factory
func NewFactory() *Factory {
	return &Factory{
		DefaultSOGCov: SOGCovariates{
			TOIRatio:       1.0,
			OZStartPct:     0.5,
			PPTOIRatio:     1.0,
			TeamPace:       1.0,
			OppSuppression: 1.0,
			RinkBias:       1.0,
			FatiguePenalty: 0.0,
			ScoreEffects:   0.0,
		},
		DefaultPhysicalCov: PhysicalCovariates{
			TOIRatio:       1.0,
			OppPhysical:    1.0,
			RinkBias:       1.0,
			RefTendency:    1.0,
			FatiguePenalty: 0.0,
			ScenarioAdjust: 0.0,
		},
		DefaultPointsCov: PointsCovariates{
			XGF60Ratio:     1.0,
			FinishingSkill: 1.0,
			PP1Share:       0.0,
			RushVsCycle:    0.5,
			NetworkWeight:  0.5,
			OppDefense:     1.0,
			TeamTotal:      3.0,
			FatiguePenalty: 0.0,
		},
	}
}

// CreateDistribution creates the appropriate distribution for a market
func (f *Factory) CreateDistribution(market types.PropMarket, stats *types.PlayerStats, context DistributionContext) Distribution {
	switch market.BaseMarket() {
	case "sog":
		return f.createSOGDistribution(stats, context)
	case "points":
		return f.createPointsDistribution(stats, context)
	case "goals":
		return f.createGoalsDistribution(stats, context)
	case "assists":
		return f.createAssistsDistribution(stats, context)
	case "hits":
		return f.createHitsDistribution(stats, context)
	case "blocks":
		return f.createBlocksDistribution(stats, context)
	case "pim":
		return f.createPIMDistribution(stats, context)
	case "saves":
		return f.createSavesDistribution(context)
	default:
		// Default to Poisson for unknown markets
		return NewHierarchicalPoissonGamma(1.0, 1.0)
	}
}

// DistributionContext provides game context for distribution creation
type DistributionContext struct {
	// Game context
	IsB2B          bool
	Is3In4         bool
	HomeGame       bool
	FatigueFactor  float64

	// Opponent context
	OppCA60        float64 // Opponent corsi against per 60
	OppGA60        float64 // Opponent goals against per 60
	OppPhysical    float64 // Opponent physical play factor

	// Arena/ref context
	ArenaSOGBias   float64
	ArenaHitsBias  float64
	ArenaBlocksBias float64
	RefPIMBias     float64

	// Team context
	TeamPace       float64
	TeamTotal      float64 // Projected team goals

	// Player role context
	ExpectedTOI    float64
	ExpectedPPTOI  float64
	PP1Unit        bool
	OZStartPct     float64

	// Scenario context
	ScenarioType   types.ScenarioType
	ScenarioAdjust float64

	// Goalie-specific
	ExpectedShots  float64
	GSAx           float64
	LeashRisk      float64
}

func (f *Factory) createSOGDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	dist := NewHierarchicalPoissonGamma(stats.SOG, stats.RecentSOG)

	cov := f.DefaultSOGCov
	cov.TOIRatio = ctx.ExpectedTOI / maxFloat(stats.TOI, 10.0)
	cov.OZStartPct = ctx.OZStartPct
	cov.PPTOIRatio = ctx.ExpectedPPTOI / maxFloat(stats.PPTOI, 1.0)
	cov.TeamPace = ctx.TeamPace
	cov.OppSuppression = normalizeOppMetric(ctx.OppCA60, 60.0) // League avg CA/60 ~ 60
	cov.RinkBias = ctx.ArenaSOGBias
	cov.FatiguePenalty = ctx.FatigueFactor

	// Scenario adjustments
	if ctx.ScenarioType == types.ScenarioRunAndGun {
		cov.ScoreEffects = 0.10 // Boost for high-event game
	} else if ctx.ScenarioType == types.ScenarioGoalieDuel {
		cov.ScoreEffects = -0.10 // Reduction for low-event game
	} else if ctx.ScenarioType == types.ScenarioBenchSlog {
		cov.ScoreEffects = -0.15 // Bigger reduction for fatigue/trap
	}

	dist.WithCovariates(cov)

	// Adjust weights for role stability
	dist.AdjustWeightsForStability(stats.TOIVariance, stats.RoleStability)

	return dist
}

func (f *Factory) createPointsDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	// Estimate opportunity rate from xGF/60 or use points per game
	oppRate := stats.PointsPerGame * 2.0 // Rough conversion to opportunities
	if stats.XGF60 > 0 {
		oppRate = stats.XGF60 / 60.0 * stats.TOI * 2.0
	}

	recentOppRate := stats.RecentPPG * 2.0

	// Estimate conversion probability
	convProb := 0.4 // Base conversion
	if stats.ShootingPct > 0 {
		convProb = stats.ShootingPct / 100.0 * 3.0 // Scale shooting % to conversion
	}

	dist := NewPoissonBinomialMixture(oppRate, recentOppRate, convProb, convProb)

	cov := f.DefaultPointsCov
	cov.XGF60Ratio = stats.XGF60 / maxFloat(2.5, 1.0) // Normalize to ~league avg
	cov.FinishingSkill = stats.ShootingPct / maxFloat(10.0, 1.0)
	if ctx.PP1Unit {
		cov.PP1Share = ctx.ExpectedPPTOI / maxFloat(stats.TOI, 10.0)
	}
	cov.OppDefense = normalizeOppMetric(ctx.OppGA60, 3.0)
	cov.TeamTotal = ctx.TeamTotal
	cov.FatiguePenalty = ctx.FatigueFactor

	dist.WithCovariates(cov)

	// Check for outlier spree
	if stats.OutlierSpreeFlag && stats.PDO > 103 {
		dist.AdjustForOutlierSpree(stats.PDO, 100.0)
	}

	return dist
}

func (f *Factory) createGoalsDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	goalsPerGame := float64(stats.Goals) / maxFloat(float64(stats.GamesPlayed), 1.0)
	recentGPG := float64(stats.RecentGoals) / maxFloat(float64(stats.RecentGamesPlayed), 1.0)

	dist := NewPoissonBinomialMixture(goalsPerGame*3, recentGPG*3, 0.33, 0.33)

	cov := f.DefaultPointsCov
	cov.FinishingSkill = stats.ShootingPct / maxFloat(10.0, 1.0)
	cov.TeamTotal = ctx.TeamTotal
	cov.FatiguePenalty = ctx.FatigueFactor

	dist.WithCovariates(cov)

	return dist
}

func (f *Factory) createAssistsDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	assistsPerGame := float64(stats.Assists) / maxFloat(float64(stats.GamesPlayed), 1.0)
	recentAPG := float64(stats.RecentAssists) / maxFloat(float64(stats.RecentGamesPlayed), 1.0)

	dist := NewPoissonBinomialMixture(assistsPerGame*2, recentAPG*2, 0.5, 0.5)

	cov := f.DefaultPointsCov
	cov.NetworkWeight = 0.7 // Assists depend more on linemates
	if ctx.PP1Unit {
		cov.PP1Share = 0.8 // PP QBs get more assists
	}
	cov.TeamTotal = ctx.TeamTotal
	cov.FatiguePenalty = ctx.FatigueFactor

	dist.WithCovariates(cov)

	return dist
}

func (f *Factory) createHitsDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	dist := NewZeroInflatedNegBin(stats.HitsPerGame, stats.RecentHPG)

	cov := f.DefaultPhysicalCov
	cov.TOIRatio = ctx.ExpectedTOI / maxFloat(stats.TOI, 10.0)
	cov.OppPhysical = ctx.OppPhysical
	cov.RinkBias = ctx.ArenaHitsBias
	cov.FatiguePenalty = ctx.FatigueFactor

	// Scenario adjustments for hits
	if ctx.ScenarioType == types.ScenarioBenchSlog {
		cov.ScenarioAdjust = 0.15 // More hits in grinding games
	} else if ctx.ScenarioType == types.ScenarioRunAndGun {
		cov.ScenarioAdjust = -0.10 // Fewer hits in high-pace games
	}

	dist.WithCovariates(cov)

	return dist
}

func (f *Factory) createBlocksDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	dist := NewZeroInflatedNegBin(stats.BlocksPerGame, stats.RecentBPG)

	cov := f.DefaultPhysicalCov
	cov.TOIRatio = ctx.ExpectedTOI / maxFloat(stats.TOI, 10.0)
	cov.RinkBias = ctx.ArenaBlocksBias
	cov.FatiguePenalty = ctx.FatigueFactor

	// More blocks when team is playing defense
	if ctx.ScenarioType == types.ScenarioBenchSlog {
		cov.ScenarioAdjust = 0.20
	} else if ctx.ScenarioType == types.ScenarioScoreEffects {
		cov.ScenarioAdjust = 0.10 // Protecting lead
	}

	dist.WithCovariates(cov)

	return dist
}

func (f *Factory) createPIMDistribution(stats *types.PlayerStats, ctx DistributionContext) Distribution {
	dist := NewZeroInflatedNegBin(stats.PIMPerGame, stats.RecentPIMPG)

	// PIM has high zero-inflation (many games with 0 PIM)
	dist.WithZeroInflation(0.3)

	cov := f.DefaultPhysicalCov
	cov.TOIRatio = ctx.ExpectedTOI / maxFloat(stats.TOI, 10.0)
	cov.RefTendency = ctx.RefPIMBias
	cov.FatiguePenalty = ctx.FatigueFactor

	// More PIM in special teams games
	if ctx.ScenarioType == types.ScenarioSpecialTeams {
		cov.ScenarioAdjust = 0.25
	}

	dist.WithCovariates(cov)

	return dist
}

func (f *Factory) createSavesDistribution(ctx DistributionContext) Distribution {
	dist := NewGoalieSavesModel(ctx.ExpectedShots, ctx.GSAx)
	dist.WithAdjustments(ctx.LeashRisk, ctx.FatigueFactor)
	return dist
}

// Helper functions

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func normalizeOppMetric(value, leagueAvg float64) float64 {
	if value == 0 {
		return 1.0
	}
	return value / leagueAvg
}
