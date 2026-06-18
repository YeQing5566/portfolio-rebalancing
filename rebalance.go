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

type AssetInput struct {
	Name          string  `json:"name"`
	TargetPct     float64 `json:"target_pct"`
	CurrentAmount float64 `json:"current_amount"`
}

type AssetResult struct {
	Name           string
	TargetPct      float64
	CurrentPct     float64
	CurrentAmount  float64
	TargetAmount   float64
	Gap            float64
	BuyAmount      float64
	AfterPct       float64
	LowLine        float64
	HighLine       float64
	CanBuy         bool
	IsSeverelyLow  bool
	IsSeverelyHigh bool
	Status         string
}

type PortfolioResult struct {
	CurrentTotal  float64
	InvestAmount  float64
	AfterTotal    float64
	AllocatedCash float64
	RemainingCash float64
	Assets        []*AssetResult
}

// CalculatePortfolio performs a buy-only rebalance against the portfolio total
// after the new cash is added. An asset's pre-buy percentage never excludes it
// from buying: it can receive cash whenever its current amount is below its
// target amount in the final portfolio.
func CalculatePortfolio(investAmount float64, inputs []AssetInput) (*PortfolioResult, error) {
	if investAmount < 0 {
		return nil, fmt.Errorf("本次可投入金额不能为负数")
	}
	if len(inputs) < 2 {
		return nil, fmt.Errorf("至少需要添加两项资产")
	}

	var currentTotal float64
	var targetSum float64
	seenNames := make(map[string]struct{}, len(inputs))

	for i, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		key := strings.ToLower(name)
		if _, exists := seenNames[key]; exists {
			return nil, fmt.Errorf("资产名称不能重复：%s", name)
		}
		seenNames[key] = struct{}{}

		if input.TargetPct <= 0 || input.TargetPct > 100 {
			return nil, fmt.Errorf("%s 的目标仓位必须大于 0%% 且不超过 100%%", name)
		}
		if input.CurrentAmount < 0 {
			return nil, fmt.Errorf("%s 的当前持有金额不能为负数", name)
		}

		targetSum += input.TargetPct
		currentTotal += input.CurrentAmount
	}

	if math.Abs(targetSum-100) > 0.01 {
		return nil, fmt.Errorf("目标仓位合计必须为 100%%，当前为 %.2f%%", targetSum)
	}

	afterTotal := currentTotal + investAmount
	if afterTotal <= 0 {
		return nil, fmt.Errorf("当前资产总额和本次可投入金额不能同时为 0")
	}

	result := &PortfolioResult{
		CurrentTotal: currentTotal,
		InvestAmount: investAmount,
		AfterTotal:   afterTotal,
		Assets:       make([]*AssetResult, 0, len(inputs)),
	}

	for _, input := range inputs {
		currentPct := 0.0
		if currentTotal > moneyEpsilon {
			currentPct = input.CurrentAmount / currentTotal * 100
		}

		targetAmount := afterTotal * input.TargetPct / 100
		gap := targetAmount - input.CurrentAmount

		result.Assets = append(result.Assets, &AssetResult{
			Name:          strings.TrimSpace(input.Name),
			TargetPct:     input.TargetPct,
			CurrentPct:    currentPct,
			CurrentAmount: input.CurrentAmount,
			TargetAmount:  targetAmount,
			Gap:           gap,
			LowLine:       input.TargetPct * lowAllocationRatio,
			HighLine:      input.TargetPct * highAllocationRatio,
			CanBuy:        gap > moneyEpsilon,
		})
	}

	allocateCash(result.Assets, investAmount)

	for _, asset := range result.Assets {
		asset.AfterPct = (asset.CurrentAmount + asset.BuyAmount) / afterTotal * 100
		asset.IsSeverelyLow = asset.AfterPct < asset.LowLine
		asset.IsSeverelyHigh = asset.AfterPct > asset.HighLine
		asset.Status = allocationStatus(asset)
		result.AllocatedCash += asset.BuyAmount
	}

	result.AllocatedCash = roundMoney(result.AllocatedCash)
	result.RemainingCash = roundMoney(math.Max(0, investAmount-result.AllocatedCash))
	return result, nil
}

func allocationStatus(asset *AssetResult) string {
	switch {
	case asset.IsSeverelyHigh:
		return "严重超配提醒"
	case asset.IsSeverelyLow:
		return "严重低配提醒"
	case asset.AfterPct > asset.TargetPct+0.005:
		return "略高于目标"
	case asset.AfterPct < asset.TargetPct-0.005:
		return "略低于目标"
	default:
		return "接近目标"
	}
}

// allocateCash equalizes the final target-fulfillment ratio of every asset that
// is below its target amount in the after-investment portfolio. Assets already
// above their final target amount receive no cash.
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
		asset    *AssetResult
		fraction float64
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
			asset:    asset,
			fraction: raw*100 - float64(cents),
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
	b.WriteString("再平衡买入方案\r\n")
	b.WriteString("────────────────────────────────────────\r\n")
	b.WriteString(fmt.Sprintf("当前资产总额：%s 元\r\n", formatMoney(result.CurrentTotal)))
	b.WriteString(fmt.Sprintf("本次可投入：  %s 元\r\n", formatMoney(result.InvestAmount)))
	b.WriteString(fmt.Sprintf("买入后总额：  %s 元\r\n", formatMoney(result.AfterTotal)))
	if result.RemainingCash > 0.005 {
		b.WriteString(fmt.Sprintf("未分配现金：  %s 元\r\n", formatMoney(result.RemainingCash)))
	}

	b.WriteString("\r\n【建议买入】\r\n")
	hasBuy := false
	for _, asset := range result.Assets {
		if asset.BuyAmount <= 0.005 {
			continue
		}
		hasBuy = true
		b.WriteString(fmt.Sprintf(
			"• %-16s %12s 元｜当前 %.2f%% → 买入后 %.2f%%｜目标 %.2f%%\r\n",
			asset.Name,
			formatMoney(asset.BuyAmount),
			asset.CurrentPct,
			asset.AfterPct,
			asset.TargetPct,
		))
	}
	if !hasBuy {
		b.WriteString("• 没有需要买入的资产，本次资金保持为现金。\r\n")
	}

	b.WriteString("\r\n【全部资产】\r\n")
	for _, asset := range result.Assets {
		b.WriteString(fmt.Sprintf(
			"• %-16s 当前金额 %12s 元｜目标 %.2f%%｜当前 %.2f%%｜买入后 %.2f%%｜%s\r\n",
			asset.Name,
			formatMoney(asset.CurrentAmount),
			asset.TargetPct,
			asset.CurrentPct,
			asset.AfterPct,
			asset.Status,
		))
	}

	b.WriteString("\r\n【严重偏离提醒（按预计买入后仓位判断）】\r\n")
	hasHigh, hasLow := false, false
	for _, asset := range result.Assets {
		switch {
		case asset.IsSeverelyHigh:
			hasHigh = true
			b.WriteString(fmt.Sprintf(
				"• %s 买入后预计 %.2f%%，高于严重超配线 %.2f%%。\r\n",
				asset.Name,
				asset.AfterPct,
				asset.HighLine,
			))
		case asset.IsSeverelyLow:
			hasLow = true
			b.WriteString(fmt.Sprintf(
				"• %s 买入后预计 %.2f%%，低于严重低配线 %.2f%%。\r\n",
				asset.Name,
				asset.AfterPct,
				asset.LowLine,
			))
		}
	}

	if !hasHigh && !hasLow {
		b.WriteString("• 买入后没有资产达到严重超配或严重低配提醒线。\r\n")
	} else {
		switch {
		case hasHigh && hasLow:
			b.WriteString("• 可考虑卖出部分严重超配资产，并将资金调入严重低配资产；程序仅作提醒，不自动计算卖出。\r\n")
		case hasHigh:
			b.WriteString("• 可考虑卖出部分严重超配资产；程序仅作提醒，不自动计算卖出。\r\n")
		case hasLow:
			b.WriteString("• 可考虑继续向严重低配资产投入资金；程序仅作提醒。\r\n")
		}
	}

	b.WriteString("\r\n【提醒线】\r\n")
	for _, asset := range result.Assets {
		b.WriteString(fmt.Sprintf(
			"• %-16s 严重低配 < %.2f%%｜严重超配 > %.2f%%\r\n",
			asset.Name,
			asset.LowLine,
			asset.HighLine,
		))
	}

	b.WriteString("\r\n说明：所有目标金额都按“当前资产总额 + 本次投入”计算。即使某项资产买入前高于目标，只要它在买入后总额下仍低于目标金额，就会参与买入分配。")
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
