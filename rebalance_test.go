package main

import (
	"math"
	"testing"
)

func TestCalculatePortfolioAtTargetBuysByTargetWeights(t *testing.T) {
	result, err := CalculatePortfolio(
		100000,
		10000,
		assetDefinitions,
		[]float64{33, 17, 33, 17},
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []float64{3300, 1700, 3300, 1700}
	for i, asset := range result.Assets {
		assertClose(t, asset.BuyAmount, want[i], 0.01)
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
	assertClose(t, result.AllocatedCash, 10000, 0.01)
	assertClose(t, result.RemainingCash, 0, 0.01)
}

func TestCalculatePortfolioPrioritizesLargestRelativeUnderweight(t *testing.T) {
	result, err := CalculatePortfolio(
		100000,
		10000,
		assetDefinitions,
		[]float64{40, 10, 33, 17},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Assets[0].BuyAmount != 0 {
		t.Fatalf("overweight asset should not be bought, got %.2f", result.Assets[0].BuyAmount)
	}
	if result.Assets[1].BuyAmount <= result.Assets[2].BuyAmount ||
		result.Assets[1].BuyAmount <= result.Assets[3].BuyAmount {
		t.Fatalf("largest relative underweight should receive the most cash: %#v", result.Assets)
	}

	ratios := []float64{
		fulfillmentRatio(result.Assets[1]),
		fulfillmentRatio(result.Assets[2]),
		fulfillmentRatio(result.Assets[3]),
	}
	for i := 1; i < len(ratios); i++ {
		assertClose(t, ratios[i], ratios[0], 0.00001)
	}
	assertClose(t, result.AllocatedCash, 10000, 0.01)
}

func TestCalculatePortfolioNeverBuysCurrentlyOverTargetAsset(t *testing.T) {
	result, err := CalculatePortfolio(
		100000,
		200000,
		assetDefinitions,
		[]float64{40, 10, 33, 17},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Assets[0].BuyAmount != 0 {
		t.Fatalf("currently over-target asset must not be bought, got %.2f", result.Assets[0].BuyAmount)
	}
	if result.RemainingCash <= 0 {
		t.Fatal("cash should remain unallocated rather than buying an over-target asset")
	}
}

func TestCalculatePortfolioFromZeroUsesTargets(t *testing.T) {
	result, err := CalculatePortfolio(
		0,
		5000,
		assetDefinitions,
		[]float64{0, 0, 0, 0},
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []float64{1650, 850, 1650, 850}
	for i, asset := range result.Assets {
		assertClose(t, asset.BuyAmount, want[i], 0.01)
	}
}

func TestHalfYearThresholdsAreStrict(t *testing.T) {
	result, err := CalculatePortfolio(
		100000,
		0,
		assetDefinitions,
		[]float64{41.25, 21.25, 24.75, 12.75},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, asset := range result.Assets {
		if asset.ShouldSellReview {
			t.Fatalf("%s should not trigger at the exact high line", asset.Name)
		}
		if asset.IsClearlyLow {
			t.Fatalf("%s should not be clearly low at the exact low line", asset.Name)
		}
	}

	result, err = CalculatePortfolio(
		100000,
		0,
		assetDefinitions,
		[]float64{41.26, 21.26, 24.74, 12.74},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Assets[0].ShouldSellReview || !result.Assets[1].ShouldSellReview {
		t.Fatal("values above the high lines should trigger a review")
	}
	if !result.Assets[2].IsClearlyLow || !result.Assets[3].IsClearlyLow {
		t.Fatal("values below the low lines should be clearly underweight")
	}
}

func TestCalculatePortfolioRejectsInvalidCurrentSum(t *testing.T) {
	_, err := CalculatePortfolio(
		100000,
		5000,
		assetDefinitions,
		[]float64{30, 17, 33, 17},
	)
	if err == nil {
		t.Fatal("expected current allocation sum validation error")
	}
}

func TestParseLegacyCurrentPctsReadsFirstThreeColumns(t *testing.T) {
	values, err := parseLegacyCurrentPcts(
		"标普500ETF,33,31,99,88\n" +
			"纳指100ETF,17,19,77,66\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, values["标普500ETF"], 31, 0)
	assertClose(t, values["纳指100ETF"], 19, 0)
}

func assertClose(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("got %.8f, want %.8f (tolerance %.8f)", got, want, tolerance)
	}
}
