package main

import (
	"math"
	"testing"
)

func TestCalculatePortfolioSupportsArbitraryAssetCount(t *testing.T) {
	inputs := []AssetInput{
		{Name: "资产A", TargetPct: 10, CurrentAmount: 10000},
		{Name: "资产B", TargetPct: 15, CurrentAmount: 15000},
		{Name: "资产C", TargetPct: 20, CurrentAmount: 20000},
		{Name: "资产D", TargetPct: 25, CurrentAmount: 25000},
		{Name: "资产E", TargetPct: 30, CurrentAmount: 30000},
	}

	result, err := CalculatePortfolio(10000, inputs)
	if err != nil {
		t.Fatal(err)
	}

	want := []float64{1000, 1500, 2000, 2500, 3000}
	for i, asset := range result.Assets {
		assertClose(t, asset.BuyAmount, want[i], 0.01)
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
	assertClose(t, result.CurrentTotal, 100000, 0.01)
	assertClose(t, result.AllocatedCash, 10000, 0.01)
}

func TestCalculatePortfolioRequiresAtLeastTwoAssets(t *testing.T) {
	_, err := CalculatePortfolio(1000, []AssetInput{
		{Name: "唯一资产", TargetPct: 100, CurrentAmount: 10000},
	})
	if err == nil {
		t.Fatal("expected at least two assets validation error")
	}
}

func TestCurrentPercentagesComeFromCurrentAmounts(t *testing.T) {
	result, err := CalculatePortfolio(0, []AssetInput{
		{Name: "资产A", TargetPct: 60, CurrentAmount: 30000},
		{Name: "资产B", TargetPct: 40, CurrentAmount: 20000},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertClose(t, result.CurrentTotal, 50000, 0.01)
	assertClose(t, result.Assets[0].CurrentPct, 60, 0.0001)
	assertClose(t, result.Assets[1].CurrentPct, 40, 0.0001)
}

func TestPreBuyOverTargetAssetCanStillReceiveCashForFinalTarget(t *testing.T) {
	inputs := []AssetInput{
		{Name: "资产A", TargetPct: 40, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 30, CurrentAmount: 30000},
		{Name: "资产C", TargetPct: 30, CurrentAmount: 20000},
	}

	result, err := CalculatePortfolio(50000, inputs)
	if err != nil {
		t.Fatal(err)
	}

	// Asset A starts at 50%, above its 40% target. After adding 50,000,
	// however, its final target amount is 60,000, so it must buy 10,000.
	want := []float64{10000, 15000, 25000}
	for i, asset := range result.Assets {
		assertClose(t, asset.BuyAmount, want[i], 0.01)
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
}

func TestAssetAtTargetBeforeBuyStillReceivesCash(t *testing.T) {
	result, err := CalculatePortfolio(20000, []AssetInput{
		{Name: "资产A", TargetPct: 50, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 50, CurrentAmount: 50000},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertClose(t, result.Assets[0].BuyAmount, 10000, 0.01)
	assertClose(t, result.Assets[1].BuyAmount, 10000, 0.01)
	assertClose(t, result.Assets[0].AfterPct, 50, 0.0001)
	assertClose(t, result.Assets[1].AfterPct, 50, 0.0001)
}

func TestAllocationUsesFinalTargetWhenSomeAssetRemainsOverweight(t *testing.T) {
	result, err := CalculatePortfolio(20000, []AssetInput{
		{Name: "资产A", TargetPct: 40, CurrentAmount: 80000},
		{Name: "资产B", TargetPct: 30, CurrentAmount: 10000},
		{Name: "资产C", TargetPct: 30, CurrentAmount: 10000},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Assets[0].BuyAmount != 0 {
		t.Fatalf("asset already above its final target amount should not be bought, got %.2f", result.Assets[0].BuyAmount)
	}
	assertClose(t, result.Assets[1].BuyAmount, 10000, 0.01)
	assertClose(t, result.Assets[2].BuyAmount, 10000, 0.01)
	if !result.Assets[0].IsSeverelyHigh {
		t.Fatal("asset A should be severely overweight after the proposed buys")
	}
	if !result.Assets[1].IsSeverelyLow || !result.Assets[2].IsSeverelyLow {
		t.Fatal("assets B and C should remain severely underweight")
	}
}

func TestSevereThresholdsUseAfterBuyPercentagesAndAreStrict(t *testing.T) {
	exact, err := CalculatePortfolio(0, []AssetInput{
		{Name: "资产A", TargetPct: 33, CurrentAmount: 41250},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 21250},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 24750},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 12750},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, asset := range exact.Assets {
		if asset.IsSeverelyHigh || asset.IsSeverelyLow {
			t.Fatalf("%s should not trigger at an exact threshold", asset.Name)
		}
	}

	beyond, err := CalculatePortfolio(0, []AssetInput{
		{Name: "资产A", TargetPct: 33, CurrentAmount: 41260},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 21260},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 24740},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 12740},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !beyond.Assets[0].IsSeverelyHigh || !beyond.Assets[1].IsSeverelyHigh {
		t.Fatal("values above high thresholds should trigger reminders")
	}
	if !beyond.Assets[2].IsSeverelyLow || !beyond.Assets[3].IsSeverelyLow {
		t.Fatal("values below low thresholds should trigger reminders")
	}
}

func TestCalculatePortfolioRejectsInvalidTargetSum(t *testing.T) {
	_, err := CalculatePortfolio(5000, []AssetInput{
		{Name: "资产A", TargetPct: 60, CurrentAmount: 60000},
		{Name: "资产B", TargetPct: 30, CurrentAmount: 40000},
	})
	if err == nil {
		t.Fatal("expected target allocation sum validation error")
	}
}

func TestInvestmentRecordStoresBeforeAndAfterDetails(t *testing.T) {
	result, err := CalculatePortfolio(10000, []AssetInput{
		{Name: "资产A", TargetPct: 40, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 60, CurrentAmount: 50000},
	})
	if err != nil {
		t.Fatal(err)
	}

	record := recordFromResult(result)
	if len(record.Assets) != 2 {
		t.Fatalf("expected two archived assets, got %d", len(record.Assets))
	}
	assertClose(t, record.CurrentTotal, 100000, 0.01)
	assertClose(t, record.AfterTotal, 110000, 0.01)
	assertClose(t, record.AllocatedCash, 10000, 0.01)
	for i, asset := range record.Assets {
		assertClose(t, asset.AfterAmount, asset.BeforeAmount+asset.BuyAmount, 0.01)
		assertClose(t, asset.AfterPct, result.Assets[i].AfterPct, 0.0001)
	}
}

func TestRecalculateInvestmentRecordAfterEditing(t *testing.T) {
	record := InvestmentRecord{
		InvestAmount: 10000,
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", TargetPct: 40, BeforeAmount: 50000, BuyAmount: 0},
			{Name: "资产B", TargetPct: 60, BeforeAmount: 50000, BuyAmount: 10000},
		},
	}

	recalculateInvestmentRecord(&record)
	assertClose(t, record.CurrentTotal, 100000, 0.01)
	assertClose(t, record.AllocatedCash, 10000, 0.01)
	assertClose(t, record.AfterTotal, 110000, 0.01)
	assertClose(t, record.Assets[0].BeforePct, 50, 0.0001)
	assertClose(t, record.Assets[1].AfterAmount, 60000, 0.01)
	assertClose(t, record.Assets[1].AfterPct, 54.5454545, 0.0001)
}

func assertClose(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("got %.8f, want %.8f (tolerance %.8f)", got, want, tolerance)
	}
}
