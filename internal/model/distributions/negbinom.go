package distributions

import (
	"math"
)

// ZeroInflatedNegBin implements a zero-inflated negative binomial distribution
// for hits, blocks, and PIM
type ZeroInflatedNegBin struct {
	// Negative binomial parameters
	R    float64 // Number of failures (dispersion parameter)
	P    float64 // Probability of success

	// Zero-inflation parameter
	Pi   float64 // Probability of extra zeros (0-1)

	// Raw rates for weighting
	SeasonRate float64
	RecentRate float64
	RecentWeight float64
	SeasonWeight float64

	// Covariate adjustments
	Covariates PhysicalCovariates
}

// PhysicalCovariates contains adjustment factors for physical stat projections
type PhysicalCovariates struct {
	TOIRatio       float64 // Ratio of expected TOI to average
	OppPhysical    float64 // Opponent physical playstyle (1.0 = avg)
	RinkBias       float64 // Arena scoring bias for hits/blocks
	RefTendency    float64 // Ref crew PIM tendency (1.0 = avg)
	FatiguePenalty float64 // Fatigue adjustment
	ScenarioAdjust float64 // Scenario-based adjustment
}

// NewZeroInflatedNegBin creates a new zero-inflated negative binomial model
func NewZeroInflatedNegBin(seasonRate, recentRate float64) *ZeroInflatedNegBin {
	// Estimate parameters from rates
	// For hits/blocks, variance typically exceeds mean (overdispersion)
	mean := 0.4*seasonRate + 0.6*recentRate

	// Assume overdispersion factor of 1.5 for physical stats
	variance := mean * 1.5

	// NegBin parameterization: mean = r(1-p)/p, var = r(1-p)/p^2
	// Solving: p = mean/variance, r = mean^2 / (variance - mean)
	if variance <= mean {
		variance = mean * 1.01 // Ensure overdispersion
	}

	p := mean / variance
	r := mean * mean / (variance - mean)

	return &ZeroInflatedNegBin{
		R:            r,
		P:            p,
		Pi:           0.0, // Default no zero-inflation
		SeasonRate:   seasonRate,
		RecentRate:   recentRate,
		RecentWeight: 0.6,
		SeasonWeight: 0.4,
		Covariates:   PhysicalCovariates{TOIRatio: 1.0, OppPhysical: 1.0, RinkBias: 1.0, RefTendency: 1.0},
	}
}

// WithZeroInflation sets the zero-inflation probability
func (z *ZeroInflatedNegBin) WithZeroInflation(pi float64) *ZeroInflatedNegBin {
	z.Pi = math.Max(0, math.Min(1, pi))
	return z
}

// WithCovariates sets the covariate adjustments
func (z *ZeroInflatedNegBin) WithCovariates(cov PhysicalCovariates) *ZeroInflatedNegBin {
	z.Covariates = cov
	// Recalculate parameters based on adjusted mean
	z.recalculateParams()
	return z
}

func (z *ZeroInflatedNegBin) recalculateParams() {
	mean := z.adjustedMean()

	// Maintain same overdispersion ratio
	variance := mean * 1.5

	if variance <= mean {
		variance = mean * 1.01
	}

	z.P = mean / variance
	z.R = mean * mean / (variance - mean)
}

func (z *ZeroInflatedNegBin) adjustedMean() float64 {
	baseMean := z.SeasonWeight*z.SeasonRate + z.RecentWeight*z.RecentRate

	// Apply adjustments
	adjustedMean := baseMean * z.Covariates.TOIRatio
	adjustedMean *= z.Covariates.OppPhysical
	adjustedMean *= z.Covariates.RinkBias
	adjustedMean *= z.Covariates.RefTendency
	adjustedMean *= (1.0 + z.Covariates.FatiguePenalty)
	adjustedMean *= (1.0 + z.Covariates.ScenarioAdjust)

	return math.Max(adjustedMean, 0.01)
}

// Mean returns the expected value accounting for zero-inflation
func (z *ZeroInflatedNegBin) Mean() float64 {
	nbMean := z.R * (1 - z.P) / z.P
	return (1 - z.Pi) * nbMean
}

// Variance returns the variance accounting for zero-inflation
func (z *ZeroInflatedNegBin) Variance() float64 {
	nbMean := z.R * (1 - z.P) / z.P
	nbVar := z.R * (1 - z.P) / (z.P * z.P)

	// Zero-inflated variance
	return (1 - z.Pi) * (nbVar + z.Pi*nbMean*nbMean)
}

// PMF returns P(X = k) for the zero-inflated negative binomial
func (z *ZeroInflatedNegBin) PMF(k int) float64 {
	if k == 0 {
		// P(X=0) = pi + (1-pi) * NB(0)
		nbProb := negBinomPMF(0, z.R, z.P)
		return z.Pi + (1-z.Pi)*nbProb
	}

	// P(X=k) = (1-pi) * NB(k) for k > 0
	return (1 - z.Pi) * negBinomPMF(k, z.R, z.P)
}

// negBinomPMF calculates the negative binomial PMF
// P(X = k) = C(k+r-1, k) * p^r * (1-p)^k
func negBinomPMF(k int, r, p float64) float64 {
	if k < 0 {
		return 0
	}
	// Use log-space for numerical stability
	// log(P) = log(Gamma(k+r)) - log(Gamma(k+1)) - log(Gamma(r)) + r*log(p) + k*log(1-p)
	lgKR, _ := math.Lgamma(float64(k) + r)
	lgK1, _ := math.Lgamma(float64(k + 1))
	lgR, _ := math.Lgamma(r)
	logProb := lgKR - lgK1 - lgR + r*math.Log(p) + float64(k)*math.Log(1-p)
	return math.Exp(logProb)
}

// CDF returns P(X <= k)
func (z *ZeroInflatedNegBin) CDF(k int) float64 {
	if k < 0 {
		return 0
	}

	sum := 0.0
	for i := 0; i <= k; i++ {
		sum += z.PMF(i)
	}
	return sum
}

// ProbOver returns P(X > line)
func (z *ZeroInflatedNegBin) ProbOver(line float64) float64 {
	// P(X > line) = 1 - P(X <= floor(line))
	return 1.0 - z.CDF(int(math.Floor(line)))
}

// ProbUnder returns P(X < line)
func (z *ZeroInflatedNegBin) ProbUnder(line float64) float64 {
	// For half-point lines like 1.5, under means X <= 1
	if line == math.Floor(line) {
		return z.CDF(int(line) - 1)
	}
	return z.CDF(int(math.Floor(line)))
}

// EstimateZeroInflation estimates the zero-inflation parameter from observed data
func EstimateZeroInflation(observations []int) float64 {
	if len(observations) == 0 {
		return 0
	}

	zeros := 0
	sum := 0
	for _, obs := range observations {
		if obs == 0 {
			zeros++
		}
		sum += obs
	}

	mean := float64(sum) / float64(len(observations))
	observedZeroRate := float64(zeros) / float64(len(observations))

	// Expected zeros under Poisson with same mean
	expectedZeroRate := math.Exp(-mean)

	// If observed zeros exceed expected, there's zero-inflation
	if observedZeroRate > expectedZeroRate {
		// Estimate pi as the excess zero rate
		return (observedZeroRate - expectedZeroRate) / (1 - expectedZeroRate)
	}

	return 0
}

// Simulate generates n random samples from the distribution
func (z *ZeroInflatedNegBin) Simulate(n int) []int {
	samples := make([]int, n)

	for i := 0; i < n; i++ {
		// Simple inverse CDF sampling
		u := randFloat()
		if u < z.Pi {
			samples[i] = 0 // Extra zero
		} else {
			// Sample from negative binomial using CDF
			samples[i] = z.sampleNegBin()
		}
	}

	return samples
}

// sampleNegBin samples from negative binomial using inverse CDF
func (z *ZeroInflatedNegBin) sampleNegBin() int {
	u := randFloat()
	cumProb := 0.0
	k := 0
	for cumProb < u && k < 1000 {
		cumProb += negBinomPMF(k, z.R, z.P)
		if cumProb >= u {
			return k
		}
		k++
	}
	return k
}

// Simple pseudo-random number (for demonstration - would use proper RNG in production)
var randState uint64 = 12345

func randFloat() float64 {
	randState = randState*6364136223846793005 + 1
	return float64(randState>>33) / float64(1<<31)
}
