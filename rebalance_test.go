package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/lxn/walk"
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

	result, err := CalculatePortfolioWithDeviationThreshold(50000, inputs, 30)
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

	assertClose(t, result.Assets[0].BuyAmount, -32000, 0.01)
	assertClose(t, result.Assets[1].BuyAmount, 26000, 0.01)
	assertClose(t, result.Assets[2].BuyAmount, 26000, 0.01)
	assertClose(t, result.EstimatedSellTotal, 32000, 0.01)
	assertClose(t, result.AllocatedCash, 52000, 0.01)
	for _, asset := range result.Assets {
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
}

func TestSevereThresholdsUseAfterBuyPercentagesAndAreStrict(t *testing.T) {
	exact, err := CalculatePortfolio(0, []AssetInput{
		{Name: "资产A", TargetPct: 33, CurrentAmount: 39600},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 20400},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 26400},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 13600},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, asset := range exact.Assets {
		if asset.PreBuySeverelyHigh {
			t.Fatalf("%s should not trigger sell rebalancing at an exact threshold", asset.Name)
		}
	}

	beyond, err := CalculatePortfolio(0, []AssetInput{
		{Name: "资产A", TargetPct: 33, CurrentAmount: 39610},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 20410},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 26390},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 13590},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !beyond.Assets[0].PreBuySeverelyHigh || !beyond.Assets[1].PreBuySeverelyHigh {
		t.Fatal("values above high thresholds should trigger sell rebalancing")
	}
	if beyond.EstimatedSellTotal <= 0 {
		t.Fatal("sell rebalancing should calculate estimated sells")
	}
	for _, asset := range beyond.Assets {
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
}

func TestCustomRelativeDeviationThresholdControlsSevereStatus(t *testing.T) {
	inputs := []AssetInput{
		{Name: "资产A", TargetPct: 50, CurrentAmount: 62500},
		{Name: "资产B", TargetPct: 50, CurrentAmount: 37500},
	}

	result, err := CalculatePortfolioWithDeviationThreshold(0, inputs, 30)
	if err != nil {
		t.Fatal(err)
	}
	if result.Assets[0].IsSeverelyHigh || result.Assets[1].IsSeverelyLow {
		t.Fatal("25% relative deviations should not trigger a 30% threshold")
	}
	assertClose(t, result.Assets[0].HighLine, 65, 0.0001)
	assertClose(t, result.Assets[1].LowLine, 35, 0.0001)

	result, err = CalculatePortfolioWithDeviationThreshold(0, inputs, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Assets[0].PreBuySeverelyHigh || result.EstimatedSellTotal <= 0 {
		t.Fatal("25% relative overweight should trigger a 20% sell threshold")
	}
	for _, asset := range result.Assets {
		assertClose(t, asset.AfterPct, asset.TargetPct, 0.0001)
	}
}

func TestCalculatePortfolioRejectsInvalidRelativeDeviationThreshold(t *testing.T) {
	inputs := []AssetInput{
		{Name: "资产A", TargetPct: 50, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 50, CurrentAmount: 50000},
	}
	for _, threshold := range []float64{0, -1, 100.01} {
		if _, err := CalculatePortfolioWithDeviationThreshold(0, inputs, threshold); err == nil {
			t.Fatalf("threshold %v should be rejected", threshold)
		}
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

func TestValuationRecordStoresCurrentPortfolioSnapshot(t *testing.T) {
	record, err := valuationRecordFromAssets([]AssetInput{
		{Name: "资产A", TargetPct: 40, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 60, CurrentAmount: 50000},
	}, mustArchiveTime(t, "2026-07-21 12:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Assets) != 2 {
		t.Fatalf("expected two snapshot assets, got %d", len(record.Assets))
	}
	assertClose(t, record.CurrentTotal, 100000, 0.01)
	assertClose(t, record.Assets[0].CurrentPct, 50, 0.0001)
	assertClose(t, record.Assets[1].CurrentAmount, 50000, 0.01)
}

func TestFinalizedBuyRecordStoresOnlyNonZeroCashFlows(t *testing.T) {
	record := buyRecordFromAssets([]AssetInput{{Name: "资产A"}, {Name: "资产B"}, {Name: "资产C"}}, mustArchiveTime(t, "2026-07-21 12:00:00"))
	record.Assets[0].BuyAmount = 1200
	record.Assets[1].BuyAmount = 0
	record.Assets[2].BuyAmount = 7600
	finalized, err := finalizedBuyRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalized.Assets) != 2 {
		t.Fatalf("expected two non-zero buy assets, got %d", len(finalized.Assets))
	}
	assertClose(t, finalized.InvestAmount, 8800, 0.01)
	assertClose(t, finalized.Assets[0].BuyAmount, 1200, 0.01)
	assertClose(t, finalized.Assets[1].BuyAmount, 7600, 0.01)
}

func TestRecalculateValuationRecordAfterEditing(t *testing.T) {
	record := InvestmentRecord{
		RecordType: recordTypeValuation,
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", TargetPct: 40, CurrentAmount: 40000},
			{Name: "资产B", TargetPct: 60, CurrentAmount: 60000},
		},
	}

	recalculateInvestmentRecord(&record)
	assertClose(t, record.CurrentTotal, 100000, 0.01)
	assertClose(t, record.Assets[0].CurrentPct, 40, 0.0001)
	assertClose(t, record.Assets[1].CurrentPct, 60, 0.0001)
}

func TestRecalculateInvestmentRecordUsesActualBuyTotalAsInvestment(t *testing.T) {
	record := InvestmentRecord{
		RecordType: recordTypeBuy,
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", BuyAmount: 1200},
			{Name: "资产B", BuyAmount: 6800},
		},
	}

	recalculateInvestmentRecord(&record)
	assertClose(t, record.AllocatedCash, 8000, 0.01)
	assertClose(t, record.InvestAmount, 8000, 0.01)
	assertClose(t, record.RemainingCash, 0, 0.01)
}

func TestFinalizedSellRecordFiltersZeroAmounts(t *testing.T) {
	record := InvestmentRecord{
		RecordType: recordTypeSell,
		ArchivedAt: "2026-05-29 11:41:16",
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", SellAmount: 1200},
			{Name: "资产B", SellAmount: 0},
			{Name: "资产C", SellAmount: 800},
		},
	}

	finalized, err := finalizedSellRecord(record)
	if err != nil {
		t.Fatalf("finalizedSellRecord returned error: %v", err)
	}
	if !isSellRecord(finalized) {
		t.Fatal("finalized record should be a sell record")
	}
	if len(finalized.Assets) != 2 {
		t.Fatalf("expected two non-zero sell assets, got %d", len(finalized.Assets))
	}
	assertClose(t, finalized.SellAmount, 2000, 0.01)
	assertClose(t, finalized.InvestAmount, 0, 0.01)
	if finalized.Assets[0].Name != "资产A" || finalized.Assets[1].Name != "资产C" {
		t.Fatalf("unexpected finalized assets: %+v", finalized.Assets)
	}
}

func TestFinalizedSellRecordRejectsZeroTotal(t *testing.T) {
	record := InvestmentRecord{
		RecordType: recordTypeSell,
		ArchivedAt: "2026-05-29 11:41:16",
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", SellAmount: 0},
		},
	}

	_, err := finalizedSellRecord(record)
	if err == nil || err.Error() != "没有填写卖出金额，不生成卖出记录" {
		t.Fatalf("expected zero sell warning, got %v", err)
	}
}

func TestFormatResultAlignsSuggestedBuyColumns(t *testing.T) {
	result := &PortfolioResult{
		Assets: []*AssetResult{
			{
				Name:          "短",
				TargetPct:     3,
				CurrentPct:    1,
				AfterPct:      2,
				CurrentAmount: 100,
				BuyAmount:     10,
				Status:        "略低于目标",
			},
			{
				Name:          "很长的资产名称B",
				TargetPct:     34,
				CurrentPct:    12,
				AfterPct:      23,
				CurrentAmount: 2000,
				BuyAmount:     1000,
				Status:        "接近目标",
			},
			{
				Name:          "零买入",
				TargetPct:     63,
				CurrentPct:    87,
				AfterPct:      75,
				CurrentAmount: 3000,
				BuyAmount:     0,
				Status:        "略高于目标",
			},
		},
	}

	output := FormatResult(result)
	rows := resultSectionRows(t, output, "建议买入")
	if len(rows) != 3 {
		t.Fatalf("expected three suggested buy rows, got %d: %#v", len(rows), rows)
	}

	if strings.Contains(output, "【建议买入】") || strings.Contains(output, "【全部资产】") {
		t.Fatalf("section headings should not use square brackets:\n%s", output)
	}
	if strings.Contains(output, "全部资产") {
		t.Fatalf("all-assets table should not be printed in the compact suggestion output:\n%s", output)
	}
	if strings.Contains(output, "说明：") {
		t.Fatalf("final explanatory note should not be printed:\n%s", output)
	}
	if header := resultSectionHeader(t, output, "建议买入"); !strings.HasPrefix(header, "资产") {
		t.Fatalf("suggested buy table should start at the left edge, got %q", header)
	}
	header := resultSectionHeader(t, output, "建议买入")
	assertSameDisplayColumn(t, header, "金额变动", rows[0], "+10 元")
	assertSameDisplayColumn(t, header, "当前仓位", rows[0], "1%")
	assertSameDisplayColumn(t, header, "买入后仓位", rows[0], "2%")
	assertSameDisplayColumn(t, header, "目标仓位", rows[0], "3%")

	assertSameDisplayColumn(t, rows[0], "+10 元", rows[1], "+1,000 元")
	assertSameDisplayColumn(t, rows[0], "1%", rows[1], "12%")
	assertSameDisplayColumn(t, rows[0], "2%", rows[1], "23%")
	assertSameDisplayColumn(t, rows[0], "3%", rows[1], "34%")
	assertSameDisplayColumn(t, rows[0], "+10 元", rows[2], "0 元")
	assertSameDisplayColumn(t, rows[0], "1%", rows[2], "87%")
	if !strings.Contains(rows[2], "0 元") {
		t.Fatalf("non-buying assets should still be shown with zero buy amount, got %q", rows[2])
	}
}

func TestFormatResultShowsEstimatedSellAndAlwaysShowsBuyReminder(t *testing.T) {
	result, err := CalculatePortfolio(0, []AssetInput{
		{Name: "高目标资产", TargetPct: 99, CurrentAmount: 100},
		{Name: "低目标资产", TargetPct: 1, CurrentAmount: 50},
	})
	if err != nil {
		t.Fatal(err)
	}

	output := FormatResult(result)
	if !strings.Contains(output, "严重偏离提醒") {
		t.Fatalf("expected severe reminder section, got:\n%s", output)
	}
	if !strings.Contains(output, "低目标资产 存在严重超配，预估需卖出 48.5 元") {
		t.Fatalf("expected estimated sell reminder, got:\n%s", output)
	}
	if !strings.Contains(output, "请执行完买入操作后，使用记录买入留档") {
		t.Fatalf("expected the always-visible buy reminder, got:\n%s", output)
	}
}

func TestSignedMoneySpanDoesNotTreatAssetNameHyphenAsSell(t *testing.T) {
	line := "0-3年政金债       -500 元  30%  25%"
	start, end, ok := signedMoneySpan(line)
	if !ok || line[start:end] != "-500 元" {
		t.Fatalf("got span %d:%d (%q), want negative amount cell", start, end, line[start:end])
	}
	if _, _, ok := signedMoneySpan("0-3年政金债       +500 元  30%  35%"); ok {
		t.Fatal("asset-name hyphen must not be treated as a negative amount")
	}
}

func mustArchiveTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(archiveTimeFmt, value, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestRelativeTargetDeviationFormatting(t *testing.T) {
	cases := []struct {
		name  string
		asset InvestmentAssetRecord
		want  string
	}{
		{
			name:  "positive with decimals",
			asset: InvestmentAssetRecord{TargetPct: 40, AfterPct: 44.448},
			want:  "+11.12%",
		},
		{
			name:  "negative without trailing decimals",
			asset: InvestmentAssetRecord{TargetPct: 50, AfterPct: 47},
			want:  "-6%",
		},
		{
			name:  "zero deviation",
			asset: InvestmentAssetRecord{TargetPct: 25, AfterPct: 25},
			want:  "0%",
		},
		{
			name:  "rounded one-third target",
			asset: InvestmentAssetRecord{TargetPct: 33.333333, AfterPct: 36.6666663},
			want:  "+10%",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deviation, ok := relativeTargetDeviationPct(tc.asset)
			if !ok {
				t.Fatal("expected deviation to be calculated")
			}
			if got := formatSignedPercent(deviation); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	if _, ok := relativeTargetDeviationPct(InvestmentAssetRecord{AfterPct: 10}); ok {
		t.Fatal("zero target should not calculate a relative deviation")
	}
}

func TestRelativeTargetDeviationColorThresholds(t *testing.T) {
	cases := []struct {
		deviation float64
		threshold float64
		want      walk.Color
	}{
		{deviation: 9.99, threshold: 20, want: dashColors.white},
		{deviation: -9.99, threshold: 20, want: dashColors.white},
		{deviation: 10, threshold: 20, want: dashColors.warning},
		{deviation: -19.99, threshold: 20, want: dashColors.warning},
		{deviation: 20, threshold: 20, want: dashColors.danger},
		{deviation: -20, threshold: 20, want: dashColors.danger},
		{deviation: 25, threshold: 60, want: dashColors.white},
		{deviation: 25, threshold: 20, want: dashColors.danger},
	}

	for _, tc := range cases {
		if got := relativeTargetDeviationColor(tc.deviation, tc.threshold); got != tc.want {
			t.Fatalf("deviation %.2f at threshold %.2f got color %v, want %v", tc.deviation, tc.threshold, got, tc.want)
		}
	}
}

func TestCalculatorTopCardHeightScalesWithWindowHeight(t *testing.T) {
	if got := scaledCalculatorTopCardHeight(350, 860, 1032, 1032); got != 420 {
		t.Fatalf("scaled card height = %d, want 420", got)
	}
	if got := scaledCalculatorTopCardHeight(350, 860, 860, 860); got != 350 {
		t.Fatalf("unchanged-window card height = %d, want 350", got)
	}
	if got := scaledCalculatorTopCardHeight(350, 860, 720, 720); got != calculatorMinTopCardHeight {
		t.Fatalf("minimum card height = %d, want %d", got, calculatorMinTopCardHeight)
	}
}

func TestTransactionDialogUsesCompactWidthAndMatchesAssetCardRows(t *testing.T) {
	width, height := transactionDialogSize(&dashboardUI{assetTableHeight: 420})
	wantRows := assetTableVisibleRows(420)
	if width != transactionDialogWidth || transactionDialogVisibleRowsForHeight(height) != wantRows {
		t.Fatalf("dialog size = %dx%d and shows %d rows, want width %d and %d rows", width, height, transactionDialogVisibleRowsForHeight(height), transactionDialogWidth, wantRows)
	}
	width, height = transactionDialogSize(&dashboardUI{})
	wantRows = assetTableVisibleRows(defaultCalculatorTopCardHeight)
	if width != transactionDialogWidth || transactionDialogVisibleRowsForHeight(height) != wantRows {
		t.Fatalf("default dialog size = %dx%d and shows %d rows, want width %d and %d rows", width, height, transactionDialogVisibleRowsForHeight(height), transactionDialogWidth, wantRows)
	}
	for cardHeight := calculatorMinTopCardHeight; cardHeight <= 700; cardHeight++ {
		_, dialogHeight := transactionDialogSize(&dashboardUI{assetTableHeight: cardHeight})
		if got, want := transactionDialogVisibleRowsForHeight(dialogHeight), assetTableVisibleRows(cardHeight); got != want {
			t.Fatalf("card height %d shows %d rows but dialog height %d shows %d rows", cardHeight, want, dialogHeight, got)
		}
	}
}

func assertClose(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("got %.8f, want %.8f (tolerance %.8f)", got, want, tolerance)
	}
}

func resultSectionRows(t *testing.T, text, heading string) []string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	inSection := false
	rows := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == heading {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if resultTestHeading(trimmed) || trimmed == "" {
			if len(rows) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "资产") {
			continue
		}
		rows = append(rows, line)
	}
	return rows
}

func resultSectionHeader(t *testing.T, text, heading string) string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			for _, next := range lines[i+1:] {
				if strings.TrimSpace(next) != "" {
					return next
				}
			}
		}
	}
	t.Fatalf("section heading %q not found in output:\n%s", heading, text)
	return ""
}

func resultTestHeading(line string) bool {
	switch line {
	case "建议买入", "严重偏离提醒", "说明：":
		return true
	default:
		return strings.HasPrefix(line, "说明：")
	}
}

func assertSameDisplayColumn(t *testing.T, leftLine, leftValue, rightLine, rightValue string) {
	t.Helper()
	left := displayColumnStart(t, leftLine, leftValue)
	right := displayColumnStart(t, rightLine, rightValue)
	if left != right {
		t.Fatalf("%q starts at display column %d, %q starts at display column %d\nleft:  %q\nright: %q", leftValue, left, rightValue, right, leftLine, rightLine)
	}
}

func displayColumnStart(t *testing.T, line, value string) int {
	t.Helper()
	index := strings.Index(line, value)
	if index < 0 {
		t.Fatalf("value %q not found in line %q", value, line)
	}
	return displayWidth(line[:index])
}
