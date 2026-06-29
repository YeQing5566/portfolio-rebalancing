package main

import "testing"

func TestBuildYieldChartDataUsesStartBaselineAndBuysBeforePoint(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-03-20 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 150, BuyAmount: 5},
			},
		},
		{
			ArchivedAt: "2026-02-10 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 130, BuyAmount: 10},
			},
		},
		{
			ArchivedAt: "2026-01-20 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 100, BuyAmount: 20},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-03")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)

	if data.Message != "" {
		t.Fatalf("unexpected message: %s", data.Message)
	}
	if len(data.Points) != 3 {
		t.Fatalf("got %d points, want 3", len(data.Points))
	}
	assertClose(t, data.Points[0].Profit, 0, 0.01)
	assertClose(t, data.Points[1].Profit, 10, 0.01)
	assertClose(t, data.Points[2].Profit, 20, 0.01)
	if !data.Points[2].Present {
		t.Fatal("March point should be present")
	}
	if data.Points[2].Rate <= 0 {
		t.Fatalf("March return rate should be positive, got %.8f", data.Points[2].Rate)
	}
}

func TestBuildYieldChartDataAggregatesSelectedAssetsAndTotalOption(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-02-15 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 120},
				{Name: "资产B", BeforeAmount: 230},
				{Name: "资产C", BeforeAmount: 350},
			},
		},
		{
			ArchivedAt: "2026-01-15 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 100, BuyAmount: 10},
				{Name: "资产B", BeforeAmount: 200, BuyAmount: 20},
				{Name: "资产C", BeforeAmount: 300, BuyAmount: 0},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-02")

	combined := buildYieldChartData(records, map[string]bool{"资产A": true, "资产B": true}, start, end)
	assertClose(t, combined.Points[1].Profit, 20, 0.01)
	if combined.SelectionLabel != "资产A 等 2 项" {
		t.Fatalf("combined label = %q", combined.SelectionLabel)
	}

	total := buildYieldChartData(records, map[string]bool{trendTotalSeries: true, "资产A": true}, start, end)
	assertClose(t, total.Points[1].Profit, 70, 0.01)
	if total.SelectionLabel != trendTotalSeries {
		t.Fatalf("total label = %q", total.SelectionLabel)
	}
}

func TestBuildYieldChartDataKeepsMissingMonthsEmpty(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-03-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 150},
			},
		},
		{
			ArchivedAt: "2026-01-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 100, BuyAmount: 20},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-03")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)

	if len(data.Months) != 3 {
		t.Fatalf("got %d months, want 3", len(data.Months))
	}
	if data.Points[1].Present {
		t.Fatalf("February point should be empty: %+v", data.Points[1])
	}
	if !data.Points[2].Present {
		t.Fatal("March point should be present")
	}
}

func TestBuildYieldChartDataPeriodRateWithoutBuys(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 110},
			},
		},
		{
			ArchivedAt: "2025-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 100},
			},
		},
	}
	start, _ := parseTrendMonth("2025-01")
	end, _ := parseTrendMonth("2026-01")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)
	latest, ok := latestYieldPoint(data.Points)
	if !ok {
		t.Fatal("expected a latest yield point")
	}
	assertClose(t, latest.Profit, 10, 0.01)
	assertClose(t, latest.Rate, 0.10, 0.0001)
}
