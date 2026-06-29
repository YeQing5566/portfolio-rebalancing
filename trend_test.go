package main

import "testing"

func TestBuildMonthlyTrendRecordsUsesEarliestRecordInMonth(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt:   "2026-01-20 09:00:00",
			CurrentTotal: 180000,
			AfterTotal:   200000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 110000, AfterAmount: 120000},
			},
		},
		{
			ArchivedAt:   "2026-01-05 09:00:00",
			CurrentTotal: 90000,
			AfterTotal:   100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 55000, AfterAmount: 60000},
			},
		},
	}

	month, err := parseTrendMonth("2026-01")
	if err != nil {
		t.Fatal(err)
	}

	monthly := buildMonthlyTrendRecords(records)
	got, ok := monthly[month]
	if !ok {
		t.Fatal("expected January record")
	}
	if got.Record.ArchivedAt != "2026-01-05 09:00:00" {
		t.Fatalf("got %s, want earliest record in month", got.Record.ArchivedAt)
	}
	if got.Record.CurrentTotal != 90000 {
		t.Fatalf("got total %.2f, want 90000", got.Record.CurrentTotal)
	}
}

func TestBuildTrendChartDataUsesCurrentHoldingsAndKeepsMissingMonthsOnAxis(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt:   "2026-03-08 09:00:00",
			CurrentTotal: 125000,
			AfterTotal:   130000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 65000, BeforePct: 52, AfterAmount: 70000, AfterPct: 53.8461538},
			},
		},
		{
			ArchivedAt:   "2026-01-08 09:00:00",
			CurrentTotal: 90000,
			AfterTotal:   100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", BeforeAmount: 54000, BeforePct: 60, AfterAmount: 60000, AfterPct: 60},
			},
		},
	}
	start, _ := parseTrendMonth("2026-01")
	end, _ := parseTrendMonth("2026-03")

	data := buildTrendChartData(records, map[string]bool{
		trendTotalSeries: true,
		"资产A":            true,
	}, start, end)

	if len(data.Months) != 3 {
		t.Fatalf("got %d months, want 3", len(data.Months))
	}
	if data.Months[1].Format(trendMonthFmt) != "2026-02" {
		t.Fatalf("middle month = %s, want 2026-02", data.Months[1].Format(trendMonthFmt))
	}
	if len(data.Series) != 2 {
		t.Fatalf("got %d series, want 2", len(data.Series))
	}

	total := data.Series[0]
	if total.Name != trendTotalSeries {
		t.Fatalf("first series = %s, want %s", total.Name, trendTotalSeries)
	}
	if !total.Points[0].Present || total.Points[0].Value != 90000 {
		t.Fatalf("January total point not populated correctly: %+v", total.Points[0])
	}
	if total.Points[1].Present {
		t.Fatalf("February should reserve axis space without a point: %+v", total.Points[1])
	}
	if !total.Points[2].Present || total.Points[2].Value != 125000 {
		t.Fatalf("March total point not populated correctly: %+v", total.Points[2])
	}

	asset := data.Series[1]
	if asset.Name != "资产A" {
		t.Fatalf("second series = %s, want 资产A", asset.Name)
	}
	if !asset.Points[0].Present || asset.Points[0].Value != 54000 {
		t.Fatalf("January asset point should use current holding amount: %+v", asset.Points[0])
	}
	if !asset.Points[2].Present || asset.Points[2].Value != 65000 {
		t.Fatalf("March asset point should use current holding amount: %+v", asset.Points[2])
	}
}
