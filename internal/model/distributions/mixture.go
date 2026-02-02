package distributions

import (
	"math"

	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonBinomialMixture implements a Poisson-Binomial mixture for points/goals/assists
// This captures the multi-stage nature: opportunities (Poisson) × conversion (Binomial)
type PoissonBinomialMixture struct {
	// Opportunity rate (how many scoring chances)
	OpportunityRate float64

	// Conversion probability (how likely to convert)
	ConversionProb float64

	// Weight parameters
	SeasonOppRate  float64
	RecentOppRate  float64
	SeasonConv     float64
	RecentConv     float64
	RecentWeight   float64
	SeasonWeight   float64

	// Covariate adjustments
	Covariates PointsCovariates
}

// PointsCovariates contains adjustment factors for scoring projections
type PointsCovariates struct {
	XGF60Ratio       float64 // Expected goals for ratio vs league avg
	FinishingSkill   float64 // Career SH% with shrinkage (1.0 = average)
	PP1Share         float64 // Share of PP1 time (0-1)
	RushVsCycle      float64 // Play style factor
	NetworkWeight    float64 // Linemate quality for primary assists
	OppDefense       float64 // Opponent defensive quality
	TeamTotal        float64 // Team total projection (goals)
	FatiguePenalty   float64 // Fatigue adjustment
}

// NewPoissonBinomialMixture creates a new mixture model for scoring stats
func NewPoissonBinomialMixture(seasonRate, recentRate, seasonConv, recentConv float64) *PoissonBinomialMixture {
	m := &PoissonBinomialMixture{
		SeasonOppRate: seasonRate,
		RecentOppRate: recentRate,
		SeasonConv:    seasonConv,
		RecentConv:    recentConv,
		RecentWeight:  0.6,
		SeasonWeight:  0.4,
		Covariates:    PointsCovariates{XGF60Ratio: 1.0, FinishingSkill: 1.0, OppDefense: 1.0, TeamTotal: 3.0},
	}
	m.recalculateParams()
	return m
}

// WithCovariates sets the covariate adjustments
func (m *PoissonBinomialMixture) WithCovariates(cov PointsCovariates) *PoissonBinomialMixture {
	m.Covariates = cov
	m.recalculateParams()
	return m
}

func (m *PoissonBinomialMixture) recalculateParams() {
	// Blend rates
	m.OpportunityRate = m.SeasonWeight*m.SeasonOppRate + m.RecentWeight*m.RecentOppRate
	m.ConversionProb = m.SeasonWeight*m.SeasonConv + m.RecentWeight*m.RecentConv

	// Apply adjustments to opportunity rate (treat 0 as "not set" = neutral 1.0)
	if m.Covariates.XGF60Ratio > 0 {
		m.OpportunityRate *= m.Covariates.XGF60Ratio
	}
	if m.Covariates.OppDefense > 0 {
		m.OpportunityRate *= m.Covariates.OppDefense
	}

	// PP1 share boosts opportunities
	if m.Covariates.PP1Share > 0 {
		ppBoost := 1.0 + m.Covariates.PP1Share*0.3
		m.OpportunityRate *= ppBoost
	}

	// Team total sanity check - only apply penalty if explicitly set to low value
	// If TeamTotal is 0 (not set), treat as neutral (no penalty)
	if m.Covariates.TeamTotal > 0 && m.Covariates.TeamTotal < 2.5 {
		penalty := m.Covariates.TeamTotal / 2.5
		m.OpportunityRate *= penalty
	}

	// Fatigue penalty
	m.OpportunityRate *= (1.0 + m.Covariates.FatiguePenalty)

	// Apply adjustments to conversion probability
	if m.Covariates.FinishingSkill > 0 {
		m.ConversionProb *= m.Covariates.FinishingSkill
	}

	// Network effect for assists
	if m.Covariates.NetworkWeight > 0 {
		m.ConversionProb *= (0.8 + 0.2*m.Covariates.NetworkWeight)
	}

	// Clamp conversion probability
	m.ConversionProb = math.Max(0.01, math.Min(0.99, m.ConversionProb))
	m.OpportunityRate = math.Max(0.1, m.OpportunityRate)
}

// Mean returns the expected value
func (m *PoissonBinomialMixture) Mean() float64 {
	return m.OpportunityRate * m.ConversionProb
}

// Variance returns the variance
func (m *PoissonBinomialMixture) Variance() float64 {
	// For Poisson-Binomial mixture: Var = λp(1-p) + λp²
	// = λp(1-p+p) = λp
	return m.OpportunityRate * m.ConversionProb
}

// PMF returns P(X = k) using the compound Poisson-Binomial distribution
func (m *PoissonBinomialMixture) PMF(k int) float64 {
	if k < 0 {
		return 0
	}

	// Use Poisson approximation for the compound distribution
	// When conversion probability is small, this is a good approximation
	lambda := m.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	return poisson.Prob(float64(k))
}

// CDF returns P(X <= k)
func (m *PoissonBinomialMixture) CDF(k int) float64 {
	lambda := m.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	return poisson.CDF(float64(k))
}

// ProbOver returns P(X > line)
func (m *PoissonBinomialMixture) ProbOver(line float64) float64 {
	lambda := m.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	return 1.0 - poisson.CDF(math.Floor(line))
}

// ProbUnder returns P(X < line)
func (m *PoissonBinomialMixture) ProbUnder(line float64) float64 {
	lambda := m.Mean()
	poisson := distuv.Poisson{Lambda: lambda}

	if line == math.Floor(line) {
		return poisson.CDF(line - 1)
	}
	return poisson.CDF(math.Floor(line))
}

// ProbExact returns P(X = k) directly
func (m *PoissonBinomialMixture) ProbExact(k int) float64 {
	if k < 0 {
		return 0
	}

	// For more accuracy, sum over possible opportunity counts
	// P(Score = k) = sum_n P(Opps = n) * P(Convert exactly k | n opps)
	lambda := m.OpportunityRate
	p := m.ConversionProb

	maxN := int(lambda*5) + 20 // Reasonable upper bound
	prob := 0.0

	poisson := distuv.Poisson{Lambda: lambda}

	for n := k; n <= maxN; n++ {
		// P(Opps = n)
		pOpps := poisson.Prob(float64(n))

		// P(Convert exactly k out of n)
		binom := distuv.Binomial{N: float64(n), P: p}
		pConvert := binom.Prob(float64(k))

		prob += pOpps * pConvert
	}

	return prob
}

// AdjustForOutlierSpree shrinks recent weight if player is on an unsustainable hot streak
func (m *PoissonBinomialMixture) AdjustForOutlierSpree(recentPDO, seasonPDO float64) {
	// PDO = on-ice SV% + on-ice SH%
	// League average PDO is 100 (or 1.0)
	// Sustained PDO > 102 or < 98 tends to regress

	pdoDiff := recentPDO - seasonPDO

	if pdoDiff > 3.0 {
		// Hot streak likely unsustainable
		shrink := 0.7
		m.RecentWeight *= shrink
		m.SeasonWeight = 1.0 - m.RecentWeight
	} else if pdoDiff < -3.0 {
		// Cold streak likely to bounce back
		shrink := 0.7
		m.RecentWeight *= shrink
		m.SeasonWeight = 1.0 - m.RecentWeight
	}
}

// Simulate generates n random samples
func (m *PoissonBinomialMixture) Simulate(n int) []int {
	samples := make([]int, n)
	poisson := distuv.Poisson{Lambda: m.OpportunityRate}

	for i := 0; i < n; i++ {
		opps := int(poisson.Rand())
		if opps > 0 {
			binom := distuv.Binomial{N: float64(opps), P: m.ConversionProb}
			samples[i] = int(binom.Rand())
		} else {
			samples[i] = 0
		}
	}

	return samples
}

// GoalieSavesModel implements a model for goalie saves
type GoalieSavesModel struct {
	// Expected shots against
	ExpectedShots float64

	// Quality mix (fraction of high-danger chances)
	HDFraction float64

	// Save percentages by quality
	HDSavePct  float64 // High-danger save %
	MDSavePct  float64 // Medium-danger save %
	LDSavePct  float64 // Low-danger save %

	// Adjustments
	LeashRisk float64 // Probability of early pull
	B2BPenalty float64 // Back-to-back fatigue
}

// NewGoalieSavesModel creates a new goalie saves model
func NewGoalieSavesModel(expectedShots float64, gsax float64) *GoalieSavesModel {
	// Base save percentages (league average ~ 0.905)
	// Adjust based on GSAx (goals saved above expected)
	baseHD := 0.80
	baseMD := 0.92
	baseLD := 0.97

	// GSAx adjustment (positive = better than expected)
	gsaxFactor := 1.0 + gsax*0.02

	return &GoalieSavesModel{
		ExpectedShots: expectedShots,
		HDFraction:    0.20, // ~20% of shots are high-danger
		HDSavePct:     math.Min(0.95, baseHD*gsaxFactor),
		MDSavePct:     math.Min(0.98, baseMD*gsaxFactor),
		LDSavePct:     math.Min(0.995, baseLD*gsaxFactor),
		LeashRisk:     0.05,
		B2BPenalty:    0.0,
	}
}

// WithAdjustments sets the adjustments
func (g *GoalieSavesModel) WithAdjustments(leashRisk, b2bPenalty float64) *GoalieSavesModel {
	g.LeashRisk = leashRisk
	g.B2BPenalty = b2bPenalty
	return g
}

// Mean returns expected saves
func (g *GoalieSavesModel) Mean() float64 {
	// Adjust for potential early pull
	effectiveShots := g.ExpectedShots * (1.0 - g.LeashRisk*0.3)

	// Adjust for fatigue
	effectiveShots *= (1.0 + g.B2BPenalty)

	// Calculate saves by quality tier
	hdShots := effectiveShots * g.HDFraction
	mdShots := effectiveShots * 0.30
	ldShots := effectiveShots * 0.50

	expectedSaves := hdShots*g.HDSavePct + mdShots*g.MDSavePct + ldShots*g.LDSavePct
	return expectedSaves
}

// ProbOver returns P(Saves > line)
func (g *GoalieSavesModel) ProbOver(line float64) float64 {
	// Model saves as Poisson with mean = expected saves
	lambda := g.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	return 1.0 - poisson.CDF(math.Floor(line))
}

// ProbUnder returns P(Saves < line)
func (g *GoalieSavesModel) ProbUnder(line float64) float64 {
	lambda := g.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	if line == math.Floor(line) {
		return poisson.CDF(line - 1)
	}
	return poisson.CDF(math.Floor(line))
}

// Variance returns the variance (same as mean for Poisson)
func (g *GoalieSavesModel) Variance() float64 {
	return g.Mean()
}

// PMF returns P(X = k)
func (g *GoalieSavesModel) PMF(k int) float64 {
	lambda := g.Mean()
	poisson := distuv.Poisson{Lambda: lambda}
	return poisson.Prob(float64(k))
}
