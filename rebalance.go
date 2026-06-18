package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	lowAllocationRatio  = 0.75
	highAllocationRatio = 1.25
	moneyEpsilon        = 0.000001
)

type AssetDefinition struct {
	Name      string
	TargetPct float64
}

type AssetResult struct {
	Name             string
	TargetPct        float64
	CurrentPct       float64
	CurrentAmount    float64
	TargetAmount     float64
	Gap              float64
	BuyAmount        float64
	AfterPct         float64
	LowLine          float64
	HighLine         float64
	IsClearlyLow     bool
	IsOverTarget     bool
	CanBuy           bool
	ShouldSellReview bool
	Status           string
}

type PortfolioResult struct {
	CurrentTotal  float64
	InvestAmount  float64
	AfterTotal    float64
	AllocatedCash float64
	RemainingCash float64
	Assets        []*AssetResult
}

// CalculatePortfolio calculates a buy-only rebalance. New cash first raises the
// assets with the lowest current-to-target fulfillment ratio, then lets the
// next-lowest assets join as their ratios become equal.
func CalculatePortfolio(
	currentTotal float64,
	investAmount float64,
	definitions []AssetDefinition,
	currentPcts []float64,
) (*PortfolioResult, error) {
	if currentTotal < 0 || investAmount < 0 {
		return nil, fmt.Errorf("金额不能为负数")
	}
	if len(definitions) == 0 || len(definitions) != len(currentPcts) {
		return nil, fmt.Errorf("资产定义与当前仓位数量不一致")
	}

	var targetSum float64
	var currentPctSum float64
	for i, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" {
			return nil, fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		if definition.TargetPct <= 0 || definition.TargetPct > 100 {
			return nil, fmt.Errorf("%s 的目标仓位必须在 0%%—100%% 之间", definition.Name)
		}
		if currentPcts[i] < 0 || currentPcts[i] > 100 {
			return nil, fmt.Errorf("%s 的当前仓位必须在 0%%—100%% 之间", definition.Name)
		}
		targetSum += definition.TargetPct
		currentPctSum += currentPcts[i]
	}

	if math.Abs(targetSum-100) > 0.01 {
		return nil, fmt.Errorf("目标仓位合计必须为 100%%，当前为 %.2f%%", targetSum)
	}
	if currentTotal > 0 && math.Abs(currentPctSum-100) > 0.1 {
		return nil, fmt.Errorf("当前仓位合计必须为 100%%，当前为 %.2f%%", currentPctSum)
	}
	if currentTotal == 0 && currentPctSum > 0.01 {
		return nil, fmt.Errorf("当前总资产为 0 时，各项当前仓位也应为 0")
	}

	afterTotal := currentTotal + investAmount
	if afterTotal <= 0 {
		return nil, fmt.Errorf("当前总资产和本次投入金额不能同时为 0")
	}

	result := &PortfolioResult{
		CurrentTotal: currentTotal,
		InvestAmount: investAmount,
		AfterTotal:   afterTotal,
		Assets:       make([]*AssetResult, 0, len(definitions)),
	}

	for i, definition := range definitions {
		currentPct := currentPcts[i]
		currentAmount := currentTotal * currentPct / 100
		targetAmount := afterTotal * definition.TargetPct / 100
		gap := targetAmount - currentAmount
		lowLine := definition.TargetPct * lowAllocationRatio
		highLine := definition.TargetPct * highAllocationRatio

		asset := &AssetResult{
			Name:             definition.Name,
			TargetPct:        definition.TargetPct,
			CurrentPct:       currentPct,
			CurrentAmount:    currentAmount,
			TargetAmount:     targetAmount,
			Gap:              gap,
			LowLine:          lowLine,
			HighLine:         highLine,
			IsClearlyLow:     currentPct < lowLine,
			IsOverTarget:     currentPct > definition.TargetPct,
			ShouldSellReview: currentPct > highLine,
		}
		asset.CanBuy = !asset.IsOverTarget && gap > moneyEpsilon

		switch {
		case asset.ShouldSellReview:
			asset.Status = "超过半年卖出提醒线"
		case asset.IsOverTarget:
			asset.Status = "当前高于目标，本次不买"
		case asset.IsClearlyLow:
			asset.Status = "明显低配，优先补齐"
		case gap > moneyEpsilon:
			asset.Status = "买入后目标金额仍有缺口"
		default:
			asset.Status = "无需买入"
		}

		result.Assets = append(result.Assets, asset)
	}

	allocateCash(result.Assets, investAmount)

	for _, asset := range result.Assets {
		asset.AfterPct = (asset.CurrentAmount + asset.BuyAmount) / afterTotal * 100
		result.AllocatedCash += asset.BuyAmount
	}
	result.AllocatedCash = roundMoney(result.AllocatedCash)
	result.RemainingCash = roundMoney(math.Max(0, investAmount-result.AllocatedCash))

	return result, nil
}

func allocateCash(assets []*AssetResult, availableCash float64) {
	if availableCash <= moneyEpsilon {
		return
	}

	candidates := make([]*AssetResult, 0, len(assets))
	var totalGap float64
	for _, asset := range assets {
		if asset.CanBuy {
			candidates = append(candidates, asset)
			totalGap += asset.Gap
		}
	}
	if len(candidates) == 0 {
		return
	}

	budget := math.Min(availableCash, totalGap)
	low, high := 0.0, 1.0
	for range 100 {
		level := (low + high) / 2
		required := cashRequiredAtLevel(candidates, level)
		if required <= budget {
			low = level
		} else {
			high = level
		}
	}

	rawBuys := make(map[*AssetResult]float64, len(candidates))
	for _, asset := range candidates {
		buy := low*asset.TargetAmount - asset.CurrentAmount
		rawBuys[asset] = math.Max(0, math.Min(asset.Gap, buy))
	}
	applyCentRounding(candidates, rawBuys, budget)
}

func cashRequiredAtLevel(candidates []*AssetResult, level float64) float64 {
	var required float64
	for _, asset := range candidates {
		buy := level*asset.TargetAmount - asset.CurrentAmount
		required += math.Max(0, math.Min(asset.Gap, buy))
	}
	return required
}

func applyCentRounding(
	candidates []*AssetResult,
	rawBuys map[*AssetResult]float64,
	budget float64,
) {
	type roundingCandidate struct {
		asset     *AssetResult
		fraction  float64
		remaining float64
	}

	rounding := make([]roundingCandidate, 0, len(candidates))
	targetCents := int64(math.Round(budget * 100))
	var assignedCents int64

	for _, asset := range candidates {
		raw := math.Max(0, math.Min(asset.Gap, rawBuys[asset]))
		cents := int64(math.Floor(raw*100 + 1e-7))
		asset.BuyAmount = float64(cents) / 100
		assignedCents += cents
		rounding = append(rounding, roundingCandidate{
			asset:     asset,
			fraction:  raw*100 - float64(cents),
			remaining: asset.Gap - asset.BuyAmount,
		})
	}

	sort.SliceStable(rounding, func(i, j int) bool {
		if math.Abs(rounding[i].fraction-rounding[j].fraction) > 1e-9 {
			return rounding[i].fraction > rounding[j].fraction
		}
		return fulfillmentRatio(rounding[i].asset) < fulfillmentRatio(rounding[j].asset)
	})

	remainingCents := targetCents - assignedCents
	for remainingCents > 0 {
		added := false
		for i := range rounding {
			if remainingCents == 0 {
				break
			}
			if rounding[i].asset.Gap-rounding[i].asset.BuyAmount >= 0.009999 {
				rounding[i].asset.BuyAmount += 0.01
				remainingCents--
				added = true
			}
		}
		if !added {
			break
		}
	}

	for _, asset := range candidates {
		asset.BuyAmount = roundMoney(asset.BuyAmount)
	}
}

func fulfillmentRatio(asset *AssetResult) float64 {
	if asset.TargetAmount <= 0 {
		return 1
	}
	return (asset.CurrentAmount + asset.BuyAmount) / asset.TargetAmount
}

func FormatResult(result *PortfolioResult) string {
	var b strings.Builder
	b.WriteString("月度买入方案\r\n")
	b.WriteString("────────────────────────────────────────\r\n")
	b.WriteString(fmt.Sprintf("当前总资产：%s 元\r\n", formatMoney(result.CurrentTotal)))
	b.WriteString(fmt.Sprintf("本次投入：  %s 元\r\n", formatMoney(result.InvestAmount)))
	b.WriteString(fmt.Sprintf("买入后合计：%s 元\r\n", formatMoney(result.AfterTotal)))
	if result.RemainingCash > 0.005 {
		b.WriteString(fmt.Sprintf("未分配现金：%s 元\r\n", formatMoney(result.RemainingCash)))
	}

	b.WriteString("\r\n【建议买入】\r\n")
	hasBuy := false
	for _, asset := range result.Assets {
		if asset.BuyAmount <= 0.005 {
			continue
		}
		hasBuy = true
		b.WriteString(fmt.Sprintf(
			"• %-12s  %12s 元    %.2f%% → %.2f%%（目标 %.2f%%）\r\n",
			asset.Name,
			formatMoney(asset.BuyAmount),
			asset.CurrentPct,
			asset.AfterPct,
			asset.TargetPct,
		))
	}
	if !hasBuy {
		b.WriteString("• 本次没有可买入的低配资产；新增资金保持为现金。\r\n")
	}

	b.WriteString("\r\n【买入后仓位】\r\n")
	for _, asset := range result.Assets {
		b.WriteString(fmt.Sprintf(
			"• %-12s  目标 %.2f%%｜当前 %.2f%%｜买后 %.2f%%｜%s\r\n",
			asset.Name,
			asset.TargetPct,
			asset.CurrentPct,
			asset.AfterPct,
			asset.Status,
		))
	}

	b.WriteString("\r\n【半年检查：卖出提醒】\r\n")
	hasSellReview := false
	for _, asset := range result.Assets {
		if !asset.ShouldSellReview {
			continue
		}
		hasSellReview = true
		b.WriteString(fmt.Sprintf(
			"• %s 当前 %.2f%%，已超过 %.2f%% 提醒线；半年检查时可考虑卖出再平衡。\r\n",
			asset.Name,
			asset.CurrentPct,
			asset.HighLine,
		))
	}
	if !hasSellReview {
		b.WriteString("• 当前没有资产超过卖出提醒线，无需卖出。\r\n")
	}

	b.WriteString("\r\n【参考线】\r\n")
	for _, asset := range result.Assets {
		b.WriteString(fmt.Sprintf(
			"• %-12s  明显低配 < %.2f%%｜卖出提醒 > %.2f%%\r\n",
			asset.Name,
			asset.LowLine,
			asset.HighLine,
		))
	}

	b.WriteString("\r\n说明：买入顺序按“当前金额 ÷ 买入后目标金额”的相对低配程度分层补齐；不依据行情涨跌调整目标比例。")
	return b.String()
}

func formatMoney(value float64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	text := fmt.Sprintf("%.2f", value)
	parts := strings.Split(text, ".")
	integer := parts[0]
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "," + integer[i:]
	}
	if negative {
		integer = "-" + integer
	}
	return integer + "." + parts[1]
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
