package distributions

import (
	"math"

	"gonum.org/v1/gonum/mathext"
	"gonum.org/v1/gonum/stat/distuv"
)

// HierarchicalPoissonGamma implements a hierarchical Poisson-Gamma model
// for shots on goal with covariates
type HierarchicalPoissonGamma struct {
	// Base rate parameters
	SeasonRate    float64 // Season average rate (lambda)
	RecentRate    float64 // Recent games rate
	PriorAlpha    float64 // Prior shape parameter
	PriorBeta     float64 // Prior rate parameter

	// Weights for blending
	RecentWeight  float64 // Weight for recent form (default 0.6)
	SeasonWeight  float64 // Weight for season average (default 0.4)

	// Covariate adjustments
	Covariates    SOGCovariates
}

// SOGCovariates contains adjustment factors for SOG projections
type SOGCovariates struct {
	TOIRatio        float64 // Ratio of expected TOI to average (e.g., 1.1 = 10% more)
	OZStartPct      float64 // Offensive zone start percentage (0-1)
	PPTOIRatio      float64 // Power play TOI ratio
	TeamPace        float64 // Team pace factor (1.0 = average)
	OppSuppression  float64 // Opponent CA/60 suppression (1.0 = average, <1 = suppresses)
	RinkBias        float64 // Arena scorer bias (1.0 = neutral)
	FatiguePenalty  float64 // B2B/travel penalty (0 = none, negative = penalty)
	ScoreEffects    float64 // Expected score state adjustment
}

// NewHierarchicalPoissonGamma creates a new hierarchical Poisson-Gamma model
func NewHierarchicalPoissonGamma(seasonRate, recentRate float64) *HierarchicalPoissonGamma {
	return &HierarchicalPoissonGamma{
		SeasonRate:   seasonRate,
		RecentRate:   recentRate,
		PriorAlpha:   2.0, // Weakly informative prior
		PriorBeta:    1.0,
		RecentWeight: 0.6,
		SeasonWeight: 0.4,
		Covariates:   SOGCovariates{TOIRatio: 1.0, OZStartPct: 0.5, PPTOIRatio: 1.0, TeamPace: 1.0, OppSuppression: 1.0, RinkBias: 1.0},
	}
}

// WithCovariates sets the covariate adjustments
func (h *HierarchicalPoissonGamma) WithCovariates(cov SOGCovariates) *HierarchicalPoissonGamma {
	h.Covariates = cov
	return h
}

// AdjustWeightsForStability adjusts recent/season weights based on role stability
// If the player has high variance in TOI or role, we shrink the recent weight
func (h *HierarchicalPoissonGamma) AdjustWeightsForStability(variance, stabilityScore float64) {
	// If recent rate is > 2 std devs from season, apply shrinkage
	if h.SeasonRate > 0 {
		diff := math.Abs(h.RecentRate - h.SeasonRate)
		stdDev := math.Sqrt(h.SeasonRate) // Poisson std dev approximation

		if diff > 2*stdDev {
			// Outlier spree detected - shrink recent weight
			shrinkFactor := 0.5
			h.RecentWeight = h.RecentWeight * shrinkFactor
			h.SeasonWeight = 1.0 - h.RecentWeight
		}
	}

	// Further adjust based on stability score (0-1, where 1 = very stable)
	if stabilityScore < 0.5 {
		// Unstable role - rely more on season average
		adjustFactor := 0.5 + stabilityScore
		h.RecentWeight = h.RecentWeight * adjustFactor
		h.SeasonWeight = 1.0 - h.RecentWeight
	}
}

// ProjectedRate returns the adjusted projected rate (lambda)
func (h *HierarchicalPoissonGamma) ProjectedRate() float64 {
	// Blend season and recent rates
	baseRate := h.SeasonWeight*h.SeasonRate + h.RecentWeight*h.RecentRate

	// Apply covariate adjustments
	adjustedRate := baseRate

	// TOI adjustment (more ice time = more shots)
	adjustedRate *= h.Covariates.TOIRatio

	// OZ starts adjustment (more offensive zone starts = more shots)
	// OZ% of 60% is considered neutral
	ozAdjust := 1.0 + (h.Covariates.OZStartPct-0.5)*0.3
	adjustedRate *= ozAdjust

	// Power play time adjustment
	if h.Covariates.PPTOIRatio > 1.0 {
		ppBoost := 1.0 + (h.Covariates.PPTOIRatio-1.0)*0.1
		adjustedRate *= ppBoost
	}

	// Team pace adjustment
	adjustedRate *= h.Covariates.TeamPace

	// Opponent suppression (lower CA/60 = harder to get shots)
	adjustedRate *= h.Covariates.OppSuppression

	// Rink bias (some arenas credit more/fewer shots)
	adjustedRate *= h.Covariates.RinkBias

	// Fatigue penalty
	adjustedRate *= (1.0 + h.Covariates.FatiguePenalty)

	// Score effects adjustment
	if h.Covariates.ScoreEffects != 0 {
		adjustedRate *= (1.0 + h.Covariates.ScoreEffects)
	}

	return math.Max(adjustedRate, 0.1) // Floor at 0.1
}

// ProbOver returns P(X > line)
func (h *HierarchicalPoissonGamma) ProbOver(line float64) float64 {
	lambda := h.ProjectedRate()
	poisson := distuv.Poisson{Lambda: lambda}

	// P(X > line) = 1 - P(X <= floor(line))
	// For half-point lines like 2.5, we want P(X >= 3)
	return 1.0 - poisson.CDF(math.Floor(line))
}

// ProbUnder returns P(X < line)
func (h *HierarchicalPoissonGamma) ProbUnder(line float64) float64 {
	lambda := h.ProjectedRate()
	poisson := distuv.Poisson{Lambda: lambda}

	// P(X < line) = P(X <= floor(line - 0.5)) for half-point lines
	// For line 2.5, we want P(X <= 2)
	if line == math.Floor(line) {
		// Whole number line - under means strictly less
		return poisson.CDF(line - 1)
	}
	return poisson.CDF(math.Floor(line))
}

// PMF returns P(X = k)
func (h *HierarchicalPoissonGamma) PMF(k int) float64 {
	lambda := h.ProjectedRate()
	poisson := distuv.Poisson{Lambda: lambda}
	return poisson.Prob(float64(k))
}

// Mean returns the expected value
func (h *HierarchicalPoissonGamma) Mean() float64 {
	return h.ProjectedRate()
}

// Variance returns the variance (with overdispersion from Gamma mixing)
func (h *HierarchicalPoissonGamma) Variance() float64 {
	lambda := h.ProjectedRate()
	// Poisson-Gamma mixture has variance = lambda + lambda^2/alpha
	return lambda + lambda*lambda/h.PriorAlpha
}

// PosteriorParameters returns the posterior alpha and beta after observing n games
// with total shots sumShots
func (h *HierarchicalPoissonGamma) PosteriorParameters(n int, sumShots int) (alpha, beta float64) {
	alpha = h.PriorAlpha + float64(sumShots)
	beta = h.PriorBeta + float64(n)
	return
}

// CredibleInterval returns the (1-alpha) credible interval for the rate parameter
func (h *HierarchicalPoissonGamma) CredibleInterval(credLevel float64) (lower, upper float64) {
	lambda := h.ProjectedRate()

	// Use Poisson confidence interval based on chi-squared
	alpha := 1.0 - credLevel
	lower = mathext.GammaIncRegInv(lambda, alpha/2)
	upper = mathext.GammaIncRegInv(lambda+1, 1-alpha/2)

	return lower, upper
}

// SimulatePosterior simulates draws from the posterior predictive distribution
func (h *HierarchicalPoissonGamma) SimulatePosterior(n int, observedGames, observedShots int) []int {
	// Update posterior
	postAlpha, postBeta := h.PosteriorParameters(observedGames, observedShots)

	samples := make([]int, n)
	gamma := distuv.Gamma{Alpha: postAlpha, Beta: postBeta}

	for i := 0; i < n; i++ {
		lambda := gamma.Rand()
		poisson := distuv.Poisson{Lambda: lambda}
		samples[i] = int(poisson.Rand())
	}

	return samples
}
