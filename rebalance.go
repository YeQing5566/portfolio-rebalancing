package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultRelativeDeviationThresholdPct = 20.0
	moneyEpsilon                         = 0.000001
)

type AssetInput struct {
	Name          string  `json:"name"`
	TargetPct     float64 `json:"target_pct"`
	CurrentAmount float64 `json:"current_amount"`
}

type AssetResult struct {
	Name               string
	TargetPct          float64
	CurrentPct         float64
	CurrentAmount      float64
	TargetAmount       float64
	Gap                float64
	BuyAmount          float64
	AfterPct           float64
	LowLine            float64
	HighLine           float64
	CanBuy             bool
	PreBuySeverelyHigh bool
	IsSeverelyLow      bool
	IsSeverelyHigh     bool
	Status             string
}

type PortfolioResult struct {
	CurrentTotal                  float64
	InvestAmount                  float64
	AfterTotal                    float64
	AllocatedCash                 float64
	RemainingCash                 float64
	EstimatedSellTotal            float64
	RelativeDeviationThresholdPct float64
	Assets                        []*AssetResult
}

// CalculatePortfolio calculates buy-only changes unless a current holding is
// already above the configured severe-overweight line. In that case it returns
// signed changes (positive buys and negative estimated sells) that rebalance
// every holding to its final target.
func CalculatePortfolio(investAmount float64, inputs []AssetInput) (*PortfolioResult, error) {
	return CalculatePortfolioWithDeviationThreshold(investAmount, inputs, defaultRelativeDeviationThresholdPct)
}

func CalculatePortfolioWithDeviationThreshold(investAmount float64, inputs []AssetInput, relativeDeviationThresholdPct float64) (*PortfolioResult, error) {
	if investAmount < 0 {
		return nil, fmt.Errorf("本次可投入金额不能为负数")
	}
	if !validRelativeDeviationThresholdPct(relativeDeviationThresholdPct) {
		return nil, fmt.Errorf("触发再平衡的相对偏离阈值必须大于 0%% 且不超过 100%%")
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
		return nil, fmt.Errorf("目标仓位合计必须为 100%%，当前为 %s", formatPercent(targetSum))
	}

	afterTotal := currentTotal + investAmount
	if afterTotal <= 0 {
		return nil, fmt.Errorf("当前资产总额和本次可投入金额不能同时为 0")
	}

	result := &PortfolioResult{
		CurrentTotal:                  currentTotal,
		InvestAmount:                  investAmount,
		AfterTotal:                    afterTotal,
		RelativeDeviationThresholdPct: relativeDeviationThresholdPct,
		Assets:                        make([]*AssetResult, 0, len(inputs)),
	}
	deviationRatio := relativeDeviationThresholdPct / 100

	for _, input := range inputs {
		currentPct := 0.0
		if currentTotal > moneyEpsilon {
			currentPct = input.CurrentAmount / currentTotal * 100
		}

		targetAmount := afterTotal * input.TargetPct / 100
		gap := targetAmount - input.CurrentAmount

		result.Assets = append(result.Assets, &AssetResult{
			Name:               strings.TrimSpace(input.Name),
			TargetPct:          input.TargetPct,
			CurrentPct:         currentPct,
			CurrentAmount:      input.CurrentAmount,
			TargetAmount:       targetAmount,
			Gap:                gap,
			LowLine:            input.TargetPct * (1 - deviationRatio),
			HighLine:           input.TargetPct * (1 + deviationRatio),
			CanBuy:             gap > moneyEpsilon,
			PreBuySeverelyHigh: currentPct > input.TargetPct*(1+deviationRatio),
		})
	}

	needsSellRebalance := false
	for _, asset := range result.Assets {
		if asset.PreBuySeverelyHigh {
			needsSellRebalance = true
			break
		}
	}
	if needsSellRebalance {
		allocateTargetChanges(result.Assets, investAmount)
	} else {
		allocateCash(result.Assets, investAmount)
	}

	for _, asset := range result.Assets {
		asset.AfterPct = (asset.CurrentAmount + asset.BuyAmount) / afterTotal * 100
		asset.IsSeverelyLow = severelyLow(asset)
		asset.IsSeverelyHigh = severelyHigh(asset)
		asset.Status = allocationStatus(asset)
		if asset.BuyAmount >= 0 {
			result.AllocatedCash += asset.BuyAmount
		} else {
			result.EstimatedSellTotal += -asset.BuyAmount
		}
	}

	result.AllocatedCash = roundMoney(result.AllocatedCash)
	result.EstimatedSellTotal = roundMoney(result.EstimatedSellTotal)
	result.RemainingCash = roundMoney(math.Max(0, investAmount+result.EstimatedSellTotal-result.AllocatedCash))
	return result, nil
}

// allocateTargetChanges is used only when a pre-buy holding breaches the
// configured severe-overweight line. It calculates signed changes that bring
// every holding to its target in the final portfolio. Positive values are buys
// and negative values are estimated sells; their net equals investAmount.
func allocateTargetChanges(assets []*AssetResult, investAmount float64) {
	if len(assets) == 0 {
		return
	}
	targetNetCents := int64(math.Round(investAmount * 100))
	assignedCents := int64(0)
	for _, asset := range assets {
		changeCents := int64(math.Round((asset.TargetAmount - asset.CurrentAmount) * 100))
		asset.BuyAmount = float64(changeCents) / 100
		assignedCents += changeCents
	}
	// Independent cent rounding can leave a small net discrepancy. Apply it to
	// the largest target holding so cash remains exactly balanced.
	delta := targetNetCents - assignedCents
	if delta != 0 {
		largest := 0
		for i := 1; i < len(assets); i++ {
			if assets[i].TargetAmount > assets[largest].TargetAmount {
				largest = i
			}
		}
		assets[largest].BuyAmount = roundMoney(assets[largest].BuyAmount + float64(delta)/100)
	}
}

func validRelativeDeviationThresholdPct(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= 100
}

func normalizedRelativeDeviationThresholdPct(value float64) float64 {
	if !validRelativeDeviationThresholdPct(value) {
		return defaultRelativeDeviationThresholdPct
	}
	return value
}

func allocationStatus(asset *AssetResult) string {
	switch {
	case severelyHigh(asset):
		return "严重超配提醒"
	case severelyLow(asset):
		return "严重低配提醒"
	case asset.AfterPct > asset.TargetPct+0.005:
		return "略高于目标"
	case asset.AfterPct < asset.TargetPct-0.005:
		return "略低于目标"
	default:
		return "接近目标"
	}
}

func severelyLow(asset *AssetResult) bool {
	return asset.AfterPct < asset.LowLine
}

func severelyHigh(asset *AssetResult) bool {
	return asset.AfterPct > asset.HighLine
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
	widths := resultTableWidths(result.Assets)
	b.WriteString("建议买入\r\n")
	if len(result.Assets) > 0 {
		b.WriteString(resultTableHeader([]resultColumn{
			{Text: "资产", Width: widths.name},
			{Text: "金额变动", Width: widths.buyAmount},
			{Text: "当前仓位", Width: widths.currentPct},
			{Text: "买入后仓位", Width: widths.afterPct},
			{Text: "目标仓位", Width: widths.targetPct},
		}))
		for _, asset := range result.Assets {
			b.WriteString(resultTableRow([]resultColumn{
				{Text: asset.Name, Width: widths.name},
				{Text: formatSignedMoneyChange(asset.BuyAmount), Width: widths.buyAmount},
				{Text: formatPercent(asset.CurrentPct), Width: widths.currentPct},
				{Text: formatPercent(asset.AfterPct), Width: widths.afterPct},
				{Text: formatPercent(asset.TargetPct), Width: widths.targetPct},
			}))
		}
	}

	hasEstimatedSell := false
	for _, asset := range result.Assets {
		if asset.BuyAmount < -moneyEpsilon {
			if !hasEstimatedSell {
				b.WriteString("\r\n严重偏离提醒\r\n")
				hasEstimatedSell = true
			}
			b.WriteString(fmt.Sprintf(
				"• %s 存在严重超配，预估需卖出 %s 元，请执行卖出操作后，使用记录卖出功能留档。\r\n",
				asset.Name,
				formatMoney(-asset.BuyAmount),
			))
		}
	}
	if !hasEstimatedSell {
		b.WriteString("\r\n")
	}
	b.WriteString("请执行完买入操作后，使用记录买入留档\r\n")

	return b.String()
}

func formatSignedMoneyChange(value float64) string {
	value = roundMoney(value)
	switch {
	case value > moneyEpsilon:
		return "+" + formatMoney(value) + " 元"
	case value < -moneyEpsilon:
		return "-" + formatMoney(-value) + " 元"
	default:
		return "0 元"
	}
}

type resultFormatWidths struct {
	name       int
	buyAmount  int
	targetPct  int
	currentPct int
	afterPct   int
}

type resultColumn struct {
	Text  string
	Width int
}

func resultTableWidths(assets []*AssetResult) resultFormatWidths {
	widths := resultFormatWidths{
		name:       16,
		buyAmount:  displayWidth("金额变动"),
		targetPct:  displayWidth("目标仓位"),
		currentPct: displayWidth("当前仓位"),
		afterPct:   displayWidth("买入后仓位"),
	}
	for _, asset := range assets {
		widths.name = maxDisplayWidth(widths.name, asset.Name)
		widths.buyAmount = maxDisplayWidth(widths.buyAmount, formatSignedMoneyChange(asset.BuyAmount))
		widths.targetPct = maxDisplayWidth(widths.targetPct, formatPercent(asset.TargetPct))
		widths.currentPct = maxDisplayWidth(widths.currentPct, formatPercent(asset.CurrentPct))
		widths.afterPct = maxDisplayWidth(widths.afterPct, formatPercent(asset.AfterPct))
	}
	return widths
}

func resultTableHeader(columns []resultColumn) string {
	var b strings.Builder
	b.WriteString(resultTableRow(columns))
	for i, column := range columns {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", column.Width))
	}
	b.WriteString("\r\n")
	return b.String()
}

func resultTableRow(columns []resultColumn) string {
	var b strings.Builder
	for i, column := range columns {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(padRightDisplay(column.Text, column.Width))
	}
	b.WriteString("\r\n")
	return b.String()
}

func padRightDisplay(text string, width int) string {
	padding := width - displayWidth(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}

func maxDisplayWidth(current int, text string) int {
	width := displayWidth(text)
	if width > current {
		return width
	}
	return current
}

func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r == '\t':
			width += 4
		case r < 0x20:
			continue
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6)
}

func formatMoney(value float64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	text := formatFlexibleNumber(value, 2)
	parts := strings.Split(text, ".")
	integer := parts[0]
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "," + integer[i:]
	}
	if negative {
		integer = "-" + integer
	}
	if len(parts) == 1 {
		return integer
	}
	return integer + "." + parts[1]
}

func formatPercent(value float64) string {
	return formatFlexibleNumber(value, 2) + "%"
}

func formatSignedPercent(value float64) string {
	if math.Abs(value) < moneyEpsilon {
		return "0%"
	}
	if value > 0 {
		return "+" + formatPercent(value)
	}
	return "-" + formatPercent(-value)
}

func formatFlexibleNumber(value float64, decimals int) string {
	if math.Abs(value) < moneyEpsilon {
		value = 0
	}
	text := strconv.FormatFloat(value, 'f', decimals, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(text, "0")
		text = strings.TrimRight(text, ".")
	}
	if text == "-0" {
		return "0"
	}
	return text
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
