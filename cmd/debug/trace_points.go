// +build ignore

package main

import (
	"fmt"
	"math"
)

func main() {
	// Simulate MacKinnon's stats
	pointsPerGame := 1.72
	recentPPG := 0.0 // Often not available
	xgf60 := 3.5
	toi := 21.0
	pptoi := 4.0
	shootingPct := 17.0 // Now correctly in percentage form

	fmt.Println("=== Points Distribution Trace ===")
	fmt.Printf("Input: PPG=%.2f, RecentPPG=%.2f, XGF60=%.1f, TOI=%.1f, ShPct=%.1f%%\n\n",
		pointsPerGame, recentPPG, xgf60, toi, shootingPct)

	// Step 1: Calculate opportunity rate (factory.go:197-206)
	oppRate := pointsPerGame * 2.0
	fmt.Printf("Step 1a: oppRate from PPG = %.2f * 2.0 = %.3f\n", pointsPerGame, oppRate)

	if xgf60 > 0 {
		oppRate = xgf60 / 60.0 * toi * 2.0
		fmt.Printf("Step 1b: oppRate from XGF60 = %.1f/60 * %.1f * 2.0 = %.3f\n", xgf60, toi, oppRate)
	}

	// Step 2: Calculate recent rate (factory.go:203-206)
	recentOppRate := recentPPG * 2.0
	fmt.Printf("Step 2a: recentOppRate = %.2f * 2.0 = %.3f\n", recentPPG, recentOppRate)

	if recentOppRate == 0 {
		recentOppRate = oppRate
		fmt.Printf("Step 2b: recentOppRate fallback to oppRate = %.3f\n", recentOppRate)
	}

	// Step 3: Calculate conversion prob (factory.go:209-214)
	convProb := 0.5
	if shootingPct > 0 {
		convProb = 0.3 + (shootingPct/100.0)*0.4
		fmt.Printf("Step 3: convProb = 0.3 + (%.1f/100) * 0.4 = %.3f\n", shootingPct, convProb)
	}

	// Step 4: Initialize mixture (mixture.go:43-55)
	seasonWeight := 0.4
	recentWeight := 0.6

	blendedOpp := seasonWeight*oppRate + recentWeight*recentOppRate
	blendedConv := seasonWeight*convProb + recentWeight*convProb // Same values
	fmt.Printf("\nStep 4: Blended rates\n")
	fmt.Printf("  OpportunityRate = %.2f*%.3f + %.2f*%.3f = %.3f\n",
		seasonWeight, oppRate, recentWeight, recentOppRate, blendedOpp)
	fmt.Printf("  ConversionProb = %.3f\n", blendedConv)

	// Step 5: Calculate covariates (factory.go:218-226)
	fmt.Println("\nStep 5: Covariates")
	xgf60Ratio := xgf60 / 2.5
	fmt.Printf("  XGF60Ratio = %.1f / 2.5 = %.3f\n", xgf60, xgf60Ratio)

	// FIXED: Small adjustment around 1.0, not full multiplier
	finishingSkill := 1.0 + (shootingPct-10.0)/100.0
	fmt.Printf("  FinishingSkill = 1.0 + (%.1f-10.0)/100.0 = %.3f\n", shootingPct, finishingSkill)

	pp1Share := pptoi / toi
	fmt.Printf("  PP1Share = %.1f / %.1f = %.3f\n", pptoi, toi, pp1Share)

	oppDefense := 1.0 // Assuming OppGA60 = 3.0 (league avg)
	fmt.Printf("  OppDefense = 1.0 (league avg opponent)\n")

	teamTotal := 3.5
	fmt.Printf("  TeamTotal = %.1f\n", teamTotal)

	// Step 6: Apply covariates (mixture.go:64-106)
	fmt.Println("\nStep 6: Apply covariates to OpportunityRate")

	fmt.Printf("  Before: OpportunityRate = %.3f\n", blendedOpp)

	if xgf60Ratio > 0 {
		blendedOpp *= xgf60Ratio
		fmt.Printf("  * XGF60Ratio (%.3f) → %.3f\n", xgf60Ratio, blendedOpp)
	}

	if oppDefense > 0 {
		blendedOpp *= oppDefense
		fmt.Printf("  * OppDefense (%.3f) → %.3f\n", oppDefense, blendedOpp)
	}

	if pp1Share > 0 {
		ppBoost := 1.0 + pp1Share*0.3
		blendedOpp *= ppBoost
		fmt.Printf("  * PP1 boost (%.3f) → %.3f\n", ppBoost, blendedOpp)
	}

	// Fatigue penalty = 0
	blendedOpp *= 1.0

	// Floor
	if blendedOpp < 0.1 {
		blendedOpp = 0.1
	}
	fmt.Printf("  Final OpportunityRate = %.3f\n", blendedOpp)

	// Step 7: Apply covariates to ConversionProb
	fmt.Println("\nStep 7: Apply covariates to ConversionProb")
	fmt.Printf("  Before: ConversionProb = %.3f\n", blendedConv)

	if finishingSkill > 0 {
		blendedConv *= finishingSkill
		fmt.Printf("  * FinishingSkill (%.3f) → %.3f  <-- DOUBLE COUNTING!\n", finishingSkill, blendedConv)
	}

	networkWeight := 0.5
	blendedConv *= (0.8 + 0.2*networkWeight)
	fmt.Printf("  * NetworkWeight factor (%.3f) → %.3f\n", 0.8+0.2*networkWeight, blendedConv)

	// Clamp
	blendedConv = math.Max(0.01, math.Min(0.99, blendedConv))
	fmt.Printf("  After clamp: ConversionProb = %.3f\n", blendedConv)

	// Step 8: Calculate mean
	mean := blendedOpp * blendedConv
	fmt.Printf("\nStep 8: Mean = %.3f * %.3f = %.3f\n", blendedOpp, blendedConv, mean)

	// What if we DON'T double-count ShootingPct?
	fmt.Println("\n=== FIX: Remove double-counting ===")
	blendedOpp2 := seasonWeight*oppRate + recentWeight*recentOppRate
	blendedConv2 := convProb // Base value already includes ShootingPct

	// Apply opp-rate covariates
	blendedOpp2 *= xgf60Ratio
	blendedOpp2 *= oppDefense
	blendedOpp2 *= (1.0 + pp1Share*0.3)

	// DON'T multiply by FinishingSkill - it's already in convProb
	// Just apply NetworkWeight
	blendedConv2 *= (0.8 + 0.2*networkWeight)

	mean2 := blendedOpp2 * blendedConv2
	fmt.Printf("OpportunityRate = %.3f\n", blendedOpp2)
	fmt.Printf("ConversionProb = %.3f\n", blendedConv2)
	fmt.Printf("Mean = %.3f (expected ~1.72 PPG)\n", mean2)
}
