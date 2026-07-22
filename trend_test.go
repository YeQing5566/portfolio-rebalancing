package main

import (
	"testing"
	"time"
)

func TestRecentYearMonthRangeUsesCurrentMonthAsEnd(t *testing.T) {
	start, end := recentYearMonthRange(time.Date(2026, 7, 6, 12, 0, 0, 0, time.Local))

	if got := start.Format(trendMonthFmt); got != "2025-07" {
		t.Fatalf("start = %s, want 2025-07", got)
	}
	if got := end.Format(trendMonthFmt); got != "2026-07" {
		t.Fatalf("end = %s, want 2026-07", got)
	}
}

func TestBuildMonthlyTrendRecordsUsesLatestRecordInMonth(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType:   recordTypeValuation,
			ArchivedAt:   "2026-01-20 09:00:00",
			CurrentTotal: 180000,
			AfterTotal:   200000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 110000},
			},
		},
		{
			RecordType:   recordTypeValuation,
			ArchivedAt:   "2026-01-05 09:00:00",
			CurrentTotal: 90000,
			AfterTotal:   100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 55000},
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
	if got.Record.ArchivedAt != "2026-01-20 09:00:00" {
		t.Fatalf("got %s, want latest record in month", got.Record.ArchivedAt)
	}
	if got.Record.CurrentTotal != 180000 {
		t.Fatalf("got total %.2f, want 180000", got.Record.CurrentTotal)
	}
}

func TestBuildTrendChartDataUsesCurrentHoldingsAndKeepsMissingMonthsOnAxis(t *testing.T) {
	records := []InvestmentRecord{
		{
			RecordType:   recordTypeValuation,
			ArchivedAt:   "2026-03-08 09:00:00",
			CurrentTotal: 125000,
			AfterTotal:   130000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 65000},
			},
		},
		{
			RecordType:   recordTypeValuation,
			ArchivedAt:   "2026-01-08 09:00:00",
			CurrentTotal: 90000,
			AfterTotal:   100000,
			Assets: []InvestmentAssetRecord{
				{Name: "资产A", CurrentAmount: 54000},
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
	assertClose(t, asset.Points[0].Pct, 60, 0.0001)
	assertClose(t, asset.Points[2].Pct, 52, 0.0001)
}
