package main

import (
	"math"
	"strings"
	"testing"

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
		{Name: "资产A", TargetPct: 33, CurrentAmount: 39600},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 20400},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 26400},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 13600},
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
		{Name: "资产A", TargetPct: 33, CurrentAmount: 39610},
		{Name: "资产B", TargetPct: 17, CurrentAmount: 20410},
		{Name: "资产C", TargetPct: 33, CurrentAmount: 26390},
		{Name: "资产D", TargetPct: 17, CurrentAmount: 13590},
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

func TestInvestmentRecordWithActualBuysRecalculatesAfterDetails(t *testing.T) {
	result, err := CalculatePortfolio(10000, []AssetInput{
		{Name: "资产A", TargetPct: 40, CurrentAmount: 50000},
		{Name: "资产B", TargetPct: 60, CurrentAmount: 50000},
	})
	if err != nil {
		t.Fatal(err)
	}

	record, err := recordFromResultWithActualBuys(result, []float64{1200, 7600})
	if err != nil {
		t.Fatal(err)
	}

	assertClose(t, record.InvestAmount, 8800, 0.01)
	assertClose(t, record.AllocatedCash, 8800, 0.01)
	assertClose(t, record.RemainingCash, 0, 0.01)
	assertClose(t, record.AfterTotal, 108800, 0.01)
	assertClose(t, record.Assets[0].BuyAmount, 1200, 0.01)
	assertClose(t, record.Assets[0].AfterAmount, 51200, 0.01)
	assertClose(t, record.Assets[0].AfterPct, 47.0588235, 0.0001)
	assertClose(t, record.Assets[1].BuyAmount, 7600, 0.01)
	assertClose(t, record.Assets[1].AfterAmount, 57600, 0.01)
	assertClose(t, record.Assets[1].AfterPct, 52.9411765, 0.0001)
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

func TestRecalculateInvestmentRecordUsesActualBuyTotalAsInvestment(t *testing.T) {
	record := InvestmentRecord{
		InvestAmount:  20000,
		RemainingCash: 12000,
		Assets: []InvestmentAssetRecord{
			{Name: "资产A", TargetPct: 40, BeforeAmount: 50000, BuyAmount: 1200},
			{Name: "资产B", TargetPct: 60, BeforeAmount: 50000, BuyAmount: 6800},
		},
	}

	recalculateInvestmentRecord(&record)
	assertClose(t, record.AllocatedCash, 8000, 0.01)
	assertClose(t, record.InvestAmount, 8000, 0.01)
	assertClose(t, record.RemainingCash, 0, 0.01)
	assertClose(t, record.AfterTotal, 108000, 0.01)
	assertClose(t, record.Assets[0].AfterPct, 47.4074074, 0.0001)
	assertClose(t, record.Assets[1].AfterPct, 52.5925926, 0.0001)
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
	assertSameDisplayColumn(t, header, "买入金额", rows[0], "10 元")
	assertSameDisplayColumn(t, header, "当前仓位", rows[0], "1%")
	assertSameDisplayColumn(t, header, "买入后仓位", rows[0], "2%")
	assertSameDisplayColumn(t, header, "目标仓位", rows[0], "3%")

	assertSameDisplayColumn(t, rows[0], "10 元", rows[1], "1,000 元")
	assertSameDisplayColumn(t, rows[0], "1%", rows[1], "12%")
	assertSameDisplayColumn(t, rows[0], "2%", rows[1], "23%")
	assertSameDisplayColumn(t, rows[0], "3%", rows[1], "34%")
	assertSameDisplayColumn(t, rows[0], "10 元", rows[2], "0 元")
	assertSameDisplayColumn(t, rows[0], "1%", rows[2], "87%")
	if !strings.Contains(rows[2], "0 元") {
		t.Fatalf("non-buying assets should still be shown with zero buy amount, got %q", rows[2])
	}
}

func TestFormatResultShowsSevereLowReminderFromCalculatedPercent(t *testing.T) {
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
	if !strings.Contains(output, "高目标资产 买入后预计 66.67%，低于严重低配线 79.2%。") {
		t.Fatalf("expected severe low reminder for high target asset, got:\n%s", output)
	}
	if !strings.Contains(output, "0 元") {
		t.Fatalf("expected zero buy amounts to be shown, got:\n%s", output)
	}
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
		want      walk.Color
	}{
		{deviation: 9.99, want: dashColors.white},
		{deviation: -9.99, want: dashColors.white},
		{deviation: 10, want: dashColors.warning},
		{deviation: -19.99, want: dashColors.warning},
		{deviation: 20, want: dashColors.danger},
		{deviation: -20, want: dashColors.danger},
	}

	for _, tc := range cases {
		if got := relativeTargetDeviationColor(tc.deviation); got != tc.want {
			t.Fatalf("deviation %.2f got color %v, want %v", tc.deviation, got, tc.want)
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
