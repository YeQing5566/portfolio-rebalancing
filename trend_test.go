package main

import "testing"

func TestBuildMonthlyTrendRecordsUsesEarliestRecordInMonth(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-01-20 09:00:00",
			AfterTotal: 200000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", AfterAmount: 120000},
			},
		},
		{
			ArchivedAt: "2026-01-05 09:00:00",
			AfterTotal: 100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", AfterAmount: 60000},
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
	if got.Record.AfterTotal != 100000 {
		t.Fatalf("got total %.2f, want 100000", got.Record.AfterTotal)
	}
}

func TestBuildTrendChartDataKeepsMissingMonthsOnAxis(t *testing.T) {
	records := []InvestmentRecord{
		{
			ArchivedAt: "2026-03-08 09:00:00",
			AfterTotal: 130000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", AfterAmount: 70000},
			},
		},
		{
			ArchivedAt: "2026-01-08 09:00:00",
			AfterTotal: 100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", AfterAmount: 60000},
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
	if !total.Points[0].Present || total.Points[0].Value != 100000 {
		t.Fatalf("January total point not populated correctly: %+v", total.Points[0])
	}
	if total.Points[1].Present {
		t.Fatalf("February should reserve axis space without a point: %+v", total.Points[1])
	}
	if !total.Points[2].Present || total.Points[2].Value != 130000 {
		t.Fatalf("March total point not populated correctly: %+v", total.Points[2])
	}
}
