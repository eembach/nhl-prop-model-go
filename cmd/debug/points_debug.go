// +build ignore

package main

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/stat/distuv"
)

func main() {
	// Simulate a low-production player
	pointsPerGame := 0.10
	recentPPG := 0.0 // Not set

	// Factory calculation
	oppRate := pointsPerGame * 2.0       // = 0.20
	recentOppRate := recentPPG * 2.0     // = 0

	// Blend (0.4 season, 0.6 recent)
	blendedOpp := 0.4*oppRate + 0.6*recentOppRate
	fmt.Printf("Opportunity Rate: season=%.2f, recent=%.2f, blended=%.3f\n", oppRate, recentOppRate, blendedOpp)

	// Conversion prob
	convProb := 0.4 // default
	fmt.Printf("Conversion Prob: %.2f\n", convProb)

	// Mean before floor
	mean := blendedOpp * convProb
	fmt.Printf("Mean (lambda) before floor: %.4f\n", mean)

	// Floor at 0.1 for opportunity rate
	if blendedOpp < 0.1 {
		blendedOpp = 0.1
	}
	mean = blendedOpp * convProb
	fmt.Printf("Mean after opportunity floor: %.4f\n", mean)

	// P(Under 0.5)
	poisson := distuv.Poisson{Lambda: mean}
	probUnder := poisson.CDF(0) // P(X <= 0) = P(X = 0)
	probOver := 1 - probUnder
	fmt.Printf("\nP(X=0) = %.4f (%.1f%%)\n", probUnder, probUnder*100)
	fmt.Printf("P(X>0.5) = %.4f (%.1f%%)\n", probOver, probOver*100)

	// Verify with direct calculation
	p0 := math.Exp(-mean)
	fmt.Printf("\nDirect e^(-lambda) = %.4f\n", p0)

	// What if RecentPPG is also set to same as season?
	fmt.Println("\n--- If RecentPPG = PointsPerGame ---")
	recentOppRate = pointsPerGame * 2.0
	blendedOpp = 0.4*oppRate + 0.6*recentOppRate
	mean = blendedOpp * convProb
	fmt.Printf("Mean: %.4f\n", mean)
	poisson = distuv.Poisson{Lambda: mean}
	fmt.Printf("P(Under 0.5) = %.1f%%\n", poisson.CDF(0)*100)
}
