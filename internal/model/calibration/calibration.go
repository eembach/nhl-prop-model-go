package calibration

import (
	"math"
	"time"

	"github.com/paul/nhl-prop-model/internal/storage"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Calibrator handles model calibration and bias adjustments
type Calibrator struct {
	store *storage.CalibrationStore
}

// New creates a new calibrator
func New(store *storage.CalibrationStore) *Calibrator {
	return &Calibrator{store: store}
}

// UpdatePropFamilyStats recalculates hit rates by prop family
func (c *Calibrator) UpdatePropFamilyStats(pickStore *storage.PickStore) error {
	markets := []types.PropMarket{
		types.PropMarketPointsOver, types.PropMarketPointsUnder,
		types.PropMarketSOGOver, types.PropMarketSOGUnder,
		types.PropMarketAssistsOver, types.PropMarketGoalsOver,
		types.PropMarketHitsOver, types.PropMarketBlocksOver,
		types.PropMarketSavesOver, types.PropMarketSavesUnder,
	}

	tiers := []int{1, 15, 2, 3}

	for _, market := range markets {
		for _, tier := range tiers {
			stats, err := pickStore.GetStats(string(market), tier)
			if err != nil {
				continue
			}
			if stats.TotalPicks > 0 {
				stats.UpdatedAt = time.Now()
				if err := c.store.UpsertPropFamilyStats(stats); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// UpdateScenarioAccuracy recalculates scenario prediction accuracy
func (c *Calibrator) UpdateScenarioAccuracy(gameStore *storage.GameStore) error {
	scenarios := []types.ScenarioType{
		types.ScenarioRunAndGun,
		types.ScenarioGoalieDuel,
		types.ScenarioSpecialTeams,
		types.ScenarioScoreEffects,
		types.ScenarioBenchSlog,
	}

	// This would query game_scenarios table for accuracy stats
	// For now, just set up the structure
	for _, scenario := range scenarios {
		acc := &types.ScenarioAccuracy{
			Scenario:   scenario,
			UpdatedAt:  time.Now(),
		}
		if err := c.store.UpsertScenarioAccuracy(acc); err != nil {
			return err
		}
	}

	return nil
}

// UpdateStarCutoffs recalculates star rating thresholds
func (c *Calibrator) UpdateStarCutoffs(pickStore *storage.PickStore) error {
	// Calculate cutoffs based on recent performance
	// Target: 5-star picks should hit ~70%, 1-star picks should hit ~55%

	week := time.Now().Format("2006-W01")

	markets := []string{"points", "sog", "assists", "goals", "hits", "blocks", "saves"}

	for _, market := range markets {
		// Default cutoffs - these would be dynamically calculated
		cutoffs := types.DefaultStarCutoffs()

		if err := c.store.UpsertStarCutoffs(week, market, cutoffs); err != nil {
			return err
		}
	}

	return nil
}

// RefreshAllCalibration runs full calibration refresh
func (c *Calibrator) RefreshAllCalibration(pickStore *storage.PickStore, gameStore *storage.GameStore) error {
	if err := c.UpdatePropFamilyStats(pickStore); err != nil {
		return err
	}

	if err := c.UpdateScenarioAccuracy(gameStore); err != nil {
		return err
	}

	if err := c.UpdateStarCutoffs(pickStore); err != nil {
		return err
	}

	return nil
}

// IsotonicRegression performs isotonic regression for probability calibration
type IsotonicRegression struct {
	xPoints []float64
	yPoints []float64
}

// NewIsotonicRegression creates a new isotonic regression model
func NewIsotonicRegression() *IsotonicRegression {
	return &IsotonicRegression{}
}

// Fit fits the isotonic regression model
func (ir *IsotonicRegression) Fit(predicted, actual []float64) {
	if len(predicted) != len(actual) || len(predicted) == 0 {
		return
	}

	// Sort by predicted values
	n := len(predicted)
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}

	// Simple bubble sort for small datasets
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if predicted[indices[j]] > predicted[indices[j+1]] {
				indices[j], indices[j+1] = indices[j+1], indices[j]
			}
		}
	}

	// Pool adjacent violators
	ir.xPoints = make([]float64, n)
	ir.yPoints = make([]float64, n)

	for i, idx := range indices {
		ir.xPoints[i] = predicted[idx]
		ir.yPoints[i] = actual[idx]
	}

	// PAVA algorithm
	for i := 1; i < n; i++ {
		if ir.yPoints[i] < ir.yPoints[i-1] {
			// Pool
			avg := (ir.yPoints[i] + ir.yPoints[i-1]) / 2
			ir.yPoints[i] = avg
			ir.yPoints[i-1] = avg

			// Check previous blocks
			j := i - 1
			for j > 0 && ir.yPoints[j] < ir.yPoints[j-1] {
				sum := ir.yPoints[j] + ir.yPoints[j-1]
				count := 2.0
				k := j - 1
				for k >= 0 && ir.yPoints[k] > ir.yPoints[k+1] {
					sum += ir.yPoints[k]
					count++
					k--
				}
				avg := sum / count
				for m := k + 1; m <= j+1; m++ {
					ir.yPoints[m] = avg
				}
				j = k
			}
		}
	}
}

// Transform applies the calibration to a predicted probability
func (ir *IsotonicRegression) Transform(pred float64) float64 {
	if len(ir.xPoints) == 0 {
		return pred
	}

	// Find the appropriate interval
	for i := 0; i < len(ir.xPoints)-1; i++ {
		if pred >= ir.xPoints[i] && pred < ir.xPoints[i+1] {
			// Linear interpolation
			t := (pred - ir.xPoints[i]) / (ir.xPoints[i+1] - ir.xPoints[i])
			return ir.yPoints[i] + t*(ir.yPoints[i+1]-ir.yPoints[i])
		}
	}

	// Extrapolation
	if pred < ir.xPoints[0] {
		return ir.yPoints[0]
	}
	return ir.yPoints[len(ir.yPoints)-1]
}

// PlattScaling implements Platt scaling for probability calibration
type PlattScaling struct {
	a, b float64
}

// NewPlattScaling creates a new Platt scaling model
func NewPlattScaling() *PlattScaling {
	return &PlattScaling{a: 1.0, b: 0.0}
}

// Fit fits the Platt scaling parameters
func (ps *PlattScaling) Fit(predicted, actual []float64) {
	if len(predicted) != len(actual) || len(predicted) < 2 {
		return
	}

	// Simple logistic regression via gradient descent
	// P(y=1) = 1 / (1 + exp(-(a*pred + b)))

	ps.a = 1.0
	ps.b = 0.0

	lr := 0.01
	iterations := 1000

	for iter := 0; iter < iterations; iter++ {
		gradA := 0.0
		gradB := 0.0

		for i := range predicted {
			z := ps.a*predicted[i] + ps.b
			sigmoid := 1.0 / (1.0 + math.Exp(-z))
			error := sigmoid - actual[i]
			gradA += error * predicted[i]
			gradB += error
		}

		n := float64(len(predicted))
		ps.a -= lr * gradA / n
		ps.b -= lr * gradB / n
	}
}

// Transform applies the calibration
func (ps *PlattScaling) Transform(pred float64) float64 {
	z := ps.a*pred + ps.b
	return 1.0 / (1.0 + math.Exp(-z))
}

// ReliabilityCurve calculates reliability/calibration curve data
type ReliabilityCurve struct {
	Bins       int
	BinCenters []float64
	BinActual  []float64
	BinCounts  []int
}

// NewReliabilityCurve creates a reliability curve
func NewReliabilityCurve(bins int) *ReliabilityCurve {
	return &ReliabilityCurve{
		Bins:       bins,
		BinCenters: make([]float64, bins),
		BinActual:  make([]float64, bins),
		BinCounts:  make([]int, bins),
	}
}

// Calculate computes the reliability curve
func (rc *ReliabilityCurve) Calculate(predicted, actual []float64) {
	binWidth := 1.0 / float64(rc.Bins)

	for i := 0; i < rc.Bins; i++ {
		rc.BinCenters[i] = (float64(i) + 0.5) * binWidth
	}

	for i := range predicted {
		bin := int(predicted[i] * float64(rc.Bins))
		if bin >= rc.Bins {
			bin = rc.Bins - 1
		}
		if bin < 0 {
			bin = 0
		}

		rc.BinActual[bin] += actual[i]
		rc.BinCounts[bin]++
	}

	for i := 0; i < rc.Bins; i++ {
		if rc.BinCounts[i] > 0 {
			rc.BinActual[i] /= float64(rc.BinCounts[i])
		}
	}
}

// CalibrationError returns the expected calibration error
func (rc *ReliabilityCurve) CalibrationError() float64 {
	total := 0
	for _, c := range rc.BinCounts {
		total += c
	}
	if total == 0 {
		return 0
	}

	ece := 0.0
	for i := 0; i < rc.Bins; i++ {
		if rc.BinCounts[i] > 0 {
			weight := float64(rc.BinCounts[i]) / float64(total)
			ece += weight * math.Abs(rc.BinCenters[i]-rc.BinActual[i])
		}
	}

	return ece
}
