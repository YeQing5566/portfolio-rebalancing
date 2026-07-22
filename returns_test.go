package main

import "testing"

func TestBuildYieldChartDataUsesStartBaselineAndBuysBeforePoint(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-03-20 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 150},
			},
		},
		{
			RecordType: recordTypeBuy,
			ArchivedAt: "2026-03-20 09:00:00",
			Assets:     []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 5}},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-02-10 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 130},
			},
		},
		{
			RecordType: recordTypeBuy,
			ArchivedAt: "2026-02-10 09:00:00",
			Assets:     []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 10}},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-20 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
			},
		},
		{
			RecordType: recordTypeBuy,
			ArchivedAt: "2026-01-20 09:00:00",
			Assets:     []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 20}},
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
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-02-15 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 120},
				{Name: "资产B", CurrentAmount: 230},
				{Name: "资产C", CurrentAmount: 350},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-15 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
				{Name: "资产B", CurrentAmount: 200},
				{Name: "资产C", CurrentAmount: 300},
			},
		},
		{
			RecordType: recordTypeBuy,
			ArchivedAt: "2026-01-15 09:00:00",
			Assets:     []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 10}, {Name: "资产B", BuyAmount: 20}},
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
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-03-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 150},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
			},
		},
		{RecordType: recordTypeBuy, ArchivedAt: "2026-01-08 09:00:00", Assets: []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 20}}},
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

func TestBuildYieldChartDataUsesFirstAvailableMonthAsBaseline(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-04-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 125},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-03-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
			},
		},
		{RecordType: recordTypeBuy, ArchivedAt: "2026-03-08 09:00:00", Assets: []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 10}}},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-04")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)

	if data.Message != "" {
		t.Fatalf("unexpected message: %s", data.Message)
	}
	if len(data.Months) != 4 || len(data.Points) != 4 {
		t.Fatalf("got %d months and %d points, want 4 each", len(data.Months), len(data.Points))
	}
	if got := data.StartAt.Format(archiveTimeFmt); got != "2026-03-08 09:00:00" {
		t.Fatalf("start at = %s, want first available data month archive time", got)
	}
	for i, wantMonth := range []string{"2026-01", "2026-02", "2026-03"} {
		if got := data.Points[i].Month.Format(trendMonthFmt); got != wantMonth {
			t.Fatalf("point %d month = %s, want %s", i, got, wantMonth)
		}
		if !data.Points[i].Present {
			t.Fatalf("%s should be shown as a zero return point", wantMonth)
		}
		assertClose(t, data.Points[i].Profit, 0, 0.01)
		assertClose(t, data.Points[i].Rate, 0, 0.0001)
		assertClose(t, data.Points[i].AnnualizedRate, 0, 0.0001)
	}
	if !data.Points[3].Present {
		t.Fatal("April point should be present")
	}
	assertClose(t, data.Points[3].Profit, 15, 0.01)
	if data.Points[3].Rate <= 0 {
		t.Fatalf("April return rate should be positive, got %.8f", data.Points[3].Rate)
	}
}

func TestBuildYieldChartDataTrimsTrailingMonthsWithoutData(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-03-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 150},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
			},
		},
		{RecordType: recordTypeBuy, ArchivedAt: "2026-01-08 09:00:00", Assets: []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 20}}},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-06")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)

	if data.Message != "" {
		t.Fatalf("unexpected message: %s", data.Message)
	}
	if len(data.Months) != 3 || len(data.Points) != 3 {
		t.Fatalf("got %d months and %d points, want 3 each", len(data.Months), len(data.Points))
	}
	if got := data.Months[len(data.Months)-1].Format(trendMonthFmt); got != "2026-03" {
		t.Fatalf("last visible month = %s, want 2026-03", got)
	}
	if data.Points[1].Present {
		t.Fatalf("February point should stay empty inside the effective range: %+v", data.Points[1])
	}
	if !data.Points[2].Present {
		t.Fatal("March point should be present")
	}
}

func TestBuildYieldChartDataReportsWhenRangeHasNoData(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-03-08 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 150},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-02")

	data := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)

	if data.Message != "所选时间周期内无法取到可测算数据" {
		t.Fatalf("message = %q", data.Message)
	}
	if len(data.Points) != 0 {
		t.Fatalf("got %d points, want none", len(data.Points))
	}
}

func TestBuildYieldChartDataPeriodRateWithoutBuys(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 110},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2025-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
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

func TestBuildYieldChartDataIncludesSellCashFlowsForSelectedAssets(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2027-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 80},
				{Name: "资产B", CurrentAmount: 100},
			},
		},
		{
			RecordType: recordTypeSell,
			ArchivedAt: "2026-07-01 09:00:00",
			SellAmount: 30,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", SellAmount: 30},
			},
		},
		{
			RecordType: recordTypeValuation,
			ArchivedAt: "2026-01-01 09:00:00",
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 100},
				{Name: "资产B", CurrentAmount: 100},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2027-01")

	assetA := buildYieldChartData(records, map[string]bool{"资产A": true}, start, end)
	latestA, ok := latestYieldPoint(assetA.Points)
	if !ok {
		t.Fatal("expected latest yield point for asset A")
	}
	assertClose(t, latestA.Profit, 10, 0.01)
	if latestA.Rate <= 0 || latestA.AnnualizedRate <= 0 {
		t.Fatalf("asset A sell cash flow should produce positive return, got rate %.8f annualized %.8f", latestA.Rate, latestA.AnnualizedRate)
	}

	assetB := buildYieldChartData(records, map[string]bool{"资产B": true}, start, end)
	latestB, ok := latestYieldPoint(assetB.Points)
	if !ok {
		t.Fatal("expected latest yield point for asset B")
	}
	assertClose(t, latestB.Profit, 0, 0.01)
	assertClose(t, latestB.Rate, 0, 0.0001)

	total := buildYieldChartData(records, map[string]bool{trendTotalSeries: true}, start, end)
	latestTotal, ok := latestYieldPoint(total.Points)
	if !ok {
		t.Fatal("expected latest yield point for total")
	}
	assertClose(t, latestTotal.Profit, 10, 0.01)
}

func TestModifiedDietzConsidersSellCashFlows(t *testing.T) {
	start, _ := parseArchiveTime("2026-01-01 09:00:00")
	soldAt, _ := parseArchiveTime("2026-07-01 09:00:00")
	end, _ := parseArchiveTime("2027-01-01 09:00:00")
	cashFlows := []yieldCashFlow{
		{At: start, Amount: -100},
		{At: soldAt, Amount: 30, External: true},
		{At: end, Amount: 80},
	}

	rate, ok := modifiedDietzRate(start, end, 100, 0, 30, 80, cashFlows)
	if !ok {
		t.Fatal("expected Modified Dietz rate")
	}
	periodDays := end.Sub(start).Hours() / 24
	weight := end.Sub(soldAt).Hours() / 24 / periodDays
	want := 10 / (100 - 30*weight)
	assertClose(t, rate, want, 0.0001)
}
