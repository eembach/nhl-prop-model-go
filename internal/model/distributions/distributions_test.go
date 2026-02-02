package distributions

import (
	"math"
	"testing"
)

func TestHierarchicalPoissonGamma(t *testing.T) {
	tests := []struct {
		name         string
		seasonRate   float64
		recentRate   float64
		line         float64
		expectedMean float64
		expectedOver float64 // approximate
	}{
		{
			name:         "Average SOG player",
			seasonRate:   2.5,
			recentRate:   2.8,
			line:         2.5,
			expectedMean: 2.68, // 0.4*2.5 + 0.6*2.8
			expectedOver: 0.45, // roughly
		},
		{
			name:         "High volume shooter",
			seasonRate:   4.0,
			recentRate:   4.5,
			line:         3.5,
			expectedMean: 4.3,
			expectedOver: 0.65,
		},
		{
			name:         "Low volume player",
			seasonRate:   1.5,
			recentRate:   1.2,
			line:         1.5,
			expectedMean: 1.32,
			expectedOver: 0.35,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := NewHierarchicalPoissonGamma(tt.seasonRate, tt.recentRate)

			mean := dist.Mean()
			if math.Abs(mean-tt.expectedMean) > 0.1 {
				t.Errorf("Mean = %.2f, expected %.2f", mean, tt.expectedMean)
			}

			probOver := dist.ProbOver(tt.line)
			if math.Abs(probOver-tt.expectedOver) > 0.15 {
				t.Errorf("P(Over %.1f) = %.2f, expected ~%.2f", tt.line, probOver, tt.expectedOver)
			}

			// Test probabilities sum correctly
			probUnder := dist.ProbUnder(tt.line)
			if probOver+probUnder > 1.01 || probOver+probUnder < 0.99 {
				t.Errorf("P(Over) + P(Under) = %.3f, should be ~1.0", probOver+probUnder)
			}
		})
	}
}

func TestHierarchicalPoissonGammaWithCovariates(t *testing.T) {
	dist := NewHierarchicalPoissonGamma(3.0, 3.0)

	// Test TOI boost
	cov := SOGCovariates{
		TOIRatio:       1.2, // 20% more TOI
		OZStartPct:     0.5,
		PPTOIRatio:     1.0,
		TeamPace:       1.0,
		OppSuppression: 1.0,
		RinkBias:       1.0,
		FatiguePenalty: 0.0,
	}
	dist.WithCovariates(cov)

	mean := dist.Mean()
	expectedMean := 3.0 * 1.2 // Base rate * TOI ratio
	if math.Abs(mean-expectedMean) > 0.1 {
		t.Errorf("Mean with TOI boost = %.2f, expected ~%.2f", mean, expectedMean)
	}

	// Test fatigue penalty
	cov.FatiguePenalty = -0.10 // 10% reduction
	dist.WithCovariates(cov)

	meanFatigued := dist.Mean()
	if meanFatigued >= mean {
		t.Errorf("Fatigued mean (%.2f) should be less than normal (%.2f)", meanFatigued, mean)
	}
}

func TestZeroInflatedNegBin(t *testing.T) {
	tests := []struct {
		name       string
		seasonRate float64
		recentRate float64
		pi         float64 // zero inflation
		line       float64
	}{
		{
			name:       "Hits - normal player",
			seasonRate: 2.0,
			recentRate: 2.2,
			pi:         0.0,
			line:       1.5,
		},
		{
			name:       "PIM - high zero inflation",
			seasonRate: 0.5,
			recentRate: 0.3,
			pi:         0.4,
			line:       0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := NewZeroInflatedNegBin(tt.seasonRate, tt.recentRate)
			dist.WithZeroInflation(tt.pi)

			mean := dist.Mean()
			if mean <= 0 {
				t.Errorf("Mean should be positive, got %.2f", mean)
			}

			// Zero probability should be elevated with zero inflation
			p0 := dist.PMF(0)
			if tt.pi > 0 && p0 < tt.pi {
				t.Errorf("P(X=0) = %.3f should be at least %.3f with zero inflation", p0, tt.pi)
			}

			// CDF should be monotonic
			var lastCDF float64
			for k := 0; k <= 10; k++ {
				cdf := dist.CDF(k)
				if cdf < lastCDF {
					t.Errorf("CDF not monotonic: CDF(%d) = %.3f < CDF(%d) = %.3f", k, cdf, k-1, lastCDF)
				}
				lastCDF = cdf
			}
		})
	}
}

func TestPoissonBinomialMixture(t *testing.T) {
	// Test points distribution
	dist := NewPoissonBinomialMixture(2.0, 2.5, 0.4, 0.45)

	mean := dist.Mean()
	if mean <= 0 || mean > 2.0 {
		t.Errorf("Points mean = %.2f, seems unreasonable", mean)
	}

	// Test P(Points >= 1) for a good player
	probOver05 := dist.ProbOver(0.5)
	if probOver05 < 0.3 || probOver05 > 0.9 {
		t.Errorf("P(Points > 0.5) = %.2f, seems unreasonable", probOver05)
	}

	// Test covariates
	cov := PointsCovariates{
		XGF60Ratio:     1.2,
		FinishingSkill: 1.0,
		PP1Share:       0.8,
		TeamTotal:      3.5,
		FatiguePenalty: 0.0,
	}
	dist.WithCovariates(cov)

	boostedMean := dist.Mean()
	if boostedMean <= mean*0.9 { // Should be higher with PP1 and good team
		t.Errorf("Boosted mean (%.2f) should be higher than base (%.2f)", boostedMean, mean)
	}
}

func TestGoalieSavesModel(t *testing.T) {
	// Elite goalie facing 30 shots
	model := NewGoalieSavesModel(30.0, 5.0) // 5.0 GSAx = excellent

	mean := model.Mean()
	if mean < 25 || mean > 29 {
		t.Errorf("Elite goalie saves mean = %.1f, expected 25-29", mean)
	}

	// Test with B2B fatigue
	model.WithAdjustments(0.05, -0.08)
	meanFatigued := model.Mean()
	if meanFatigued >= mean {
		t.Errorf("Fatigued saves (%.1f) should be less than normal (%.1f)", meanFatigued, mean)
	}

	// Weak goalie facing same shots
	weakModel := NewGoalieSavesModel(30.0, -5.0) // -5.0 GSAx = poor
	weakMean := weakModel.Mean()
	if weakMean >= mean {
		t.Errorf("Weak goalie mean (%.1f) should be less than elite (%.1f)", weakMean, mean)
	}
}

func TestNegBinomPMF(t *testing.T) {
	// Test that PMF sums to approximately 1
	r := 5.0
	p := 0.6
	sum := 0.0
	for k := 0; k <= 100; k++ {
		sum += negBinomPMF(k, r, p)
	}
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("NegBin PMF sum = %.4f, expected ~1.0", sum)
	}
}

func TestDistributionInterface(t *testing.T) {
	// Test that all distributions implement the interface
	distributions := []Distribution{
		NewHierarchicalPoissonGamma(3.0, 3.0),
		NewZeroInflatedNegBin(2.0, 2.0),
		NewPoissonBinomialMixture(2.0, 2.0, 0.4, 0.4),
		NewGoalieSavesModel(30.0, 0.0),
	}

	for i, d := range distributions {
		mean := d.Mean()
		if mean <= 0 {
			t.Errorf("Distribution %d has non-positive mean: %.2f", i, mean)
		}

		variance := d.Variance()
		if variance <= 0 {
			t.Errorf("Distribution %d has non-positive variance: %.2f", i, variance)
		}

		pmf0 := d.PMF(0)
		if pmf0 < 0 || pmf0 > 1 {
			t.Errorf("Distribution %d PMF(0) = %.3f, should be in [0,1]", i, pmf0)
		}

		probOver := d.ProbOver(2.5)
		probUnder := d.ProbUnder(2.5)
		if probOver < 0 || probOver > 1 {
			t.Errorf("Distribution %d P(Over) = %.3f, should be in [0,1]", i, probOver)
		}
		if probUnder < 0 || probUnder > 1 {
			t.Errorf("Distribution %d P(Under) = %.3f, should be in [0,1]", i, probUnder)
		}
	}
}

func BenchmarkPoissonGammaProbOver(b *testing.B) {
	dist := NewHierarchicalPoissonGamma(3.0, 3.2)
	cov := SOGCovariates{
		TOIRatio:       1.1,
		OZStartPct:     0.55,
		PPTOIRatio:     1.2,
		TeamPace:       1.05,
		OppSuppression: 0.95,
		RinkBias:       1.02,
	}
	dist.WithCovariates(cov)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dist.ProbOver(2.5)
	}
}
