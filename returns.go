package main

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	yieldDataHint       = "收益以更新收益记录作为估值点，以记录买入和记录卖出作为现金流；年化收益率优先按 XIRR 计算。"
	yieldInitialMessage = "先保存更新收益记录，再选择资产和月份测算收益"
	yieldSellHint       = "请及时使用更新收益、记录买入和记录卖出保存数据，以确保收益测算准确"
)

type yieldPoint struct {
	Month          time.Time
	Profit         float64
	Rate           float64
	AnnualizedRate float64
	Present        bool
}

type yieldChartData struct {
	Months         []time.Time
	Points         []yieldPoint
	SelectionLabel string
	Message        string
	StartAt        time.Time
	EndAt          time.Time
}

type yieldSelection struct {
	Total bool
	Names []string
	Keys  map[string]struct{}
	Label string
}

type yieldTimedRecord struct {
	At     time.Time
	Record InvestmentRecord
}

type yieldCashFlow struct {
	At       time.Time
	Amount   float64
	External bool
}

var yieldSelections = make(map[string]bool)

func syncYieldSelections(options []string) {
	allowed := make(map[string]struct{}, len(options))
	for _, name := range options {
		allowed[name] = struct{}{}
	}
	for name := range yieldSelections {
		if _, ok := allowed[name]; !ok {
			delete(yieldSelections, name)
		}
	}

	selected := 0
	for _, name := range options {
		if yieldSelections[name] {
			selected++
		}
	}
	if selected == 0 && len(options) > 0 {
		yieldSelections[options[0]] = true
	}
}

func buildYieldChartData(records []InvestmentRecord, selections map[string]bool, start, end time.Time) yieldChartData {
	start = normalizeTrendMonth(start)
	end = normalizeTrendMonth(end)
	if start.After(end) {
		return yieldChartData{Message: "开始月份不能晚于结束月份"}
	}

	requestedMonths := trendMonthRange(start, end)
	selection, ok := resolveYieldSelection(records, selections)
	if !ok {
		return yieldChartData{Months: requestedMonths, Message: "请选择左侧资产"}
	}

	monthly := buildMonthlyTrendRecords(records)
	startRecord, baseValue, lastMonth, ok := yieldDataBounds(requestedMonths, monthly, selection)
	if !ok {
		return yieldChartData{Months: requestedMonths, SelectionLabel: selection.Label, Message: "所选时间周期内无法取到可测算数据"}
	}

	months := trendMonthRange(start, lastMonth)
	timedRecords := sortedYieldRecords(records)
	points := make([]yieldPoint, 0, len(months))
	for _, month := range months {
		point := yieldPoint{Month: month}
		if month.Before(startRecord.Month) {
			point.Present = true
			points = append(points, point)
			continue
		}

		monthlyRecord, ok := monthly[month]
		if !ok || monthlyRecord.ArchivedAt.Before(startRecord.ArchivedAt) {
			points = append(points, point)
			continue
		}

		endValue, ok := yieldRecordBeforeAmount(monthlyRecord.Record, selection)
		if !ok {
			points = append(points, point)
			continue
		}

		cashFlows, buyTotal, sellTotal := yieldCashFlowsForPoint(timedRecords, selection, startRecord.ArchivedAt, monthlyRecord.ArchivedAt, baseValue, endValue)
		point.Profit = roundMoney(endValue + sellTotal - baseValue - buyTotal)
		point.Rate, point.AnnualizedRate = yieldRates(cashFlows, startRecord.ArchivedAt, monthlyRecord.ArchivedAt, baseValue, buyTotal, sellTotal, endValue)
		point.Present = true
		points = append(points, point)
	}

	message := ""
	if !yieldPointsHaveAny(points) {
		message = "当前范围没有可测算数据"
	}
	return yieldChartData{
		Months:         months,
		Points:         points,
		SelectionLabel: selection.Label,
		Message:        message,
		StartAt:        startRecord.ArchivedAt,
		EndAt:          latestYieldPointTime(points, monthly),
	}
}

func yieldDataBounds(months []time.Time, monthly map[time.Time]trendMonthlyRecord, selection yieldSelection) (trendMonthlyRecord, float64, time.Time, bool) {
	var firstRecord trendMonthlyRecord
	var firstValue float64
	var lastMonth time.Time
	found := false
	for _, month := range months {
		monthlyRecord, ok := monthly[month]
		if !ok {
			continue
		}
		value, ok := yieldRecordBeforeAmount(monthlyRecord.Record, selection)
		if !ok {
			continue
		}
		if !found {
			firstRecord = monthlyRecord
			firstValue = value
			found = true
		}
		lastMonth = month
	}
	return firstRecord, firstValue, lastMonth, found
}

func resolveYieldSelection(records []InvestmentRecord, selections map[string]bool) (yieldSelection, bool) {
	options := trendSeriesOptions(records)
	names := selectedTrendSeriesNames(options, selections)
	for _, name := range names {
		if name == trendTotalSeries {
			return yieldSelection{Total: true, Label: trendTotalSeries}, true
		}
	}
	if len(names) == 0 {
		return yieldSelection{}, false
	}

	keys := make(map[string]struct{}, len(names))
	for _, name := range names {
		keys[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return yieldSelection{
		Names: names,
		Keys:  keys,
		Label: yieldSelectionLabel(names),
	}, true
}

func yieldSelectionLabel(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	return names[0] + " 等 " + formatFlexibleNumber(float64(len(names)), 0) + " 项"
}

func yieldRecordBeforeAmount(record InvestmentRecord, selection yieldSelection) (float64, bool) {
	if selection.Total {
		total := 0.0
		for _, asset := range record.Assets {
			total += asset.CurrentAmount
		}
		if len(record.Assets) > 0 {
			return roundMoney(total), true
		}
		return record.CurrentTotal, record.CurrentTotal > moneyEpsilon
	}

	total := 0.0
	found := false
	for _, asset := range record.Assets {
		key := strings.ToLower(strings.TrimSpace(asset.Name))
		if _, ok := selection.Keys[key]; !ok {
			continue
		}
		total += asset.CurrentAmount
		found = true
	}
	return roundMoney(total), found
}

func yieldRecordBuyAmount(record InvestmentRecord, selection yieldSelection) float64 {
	if !isBuyRecord(record) {
		return 0
	}
	total := 0.0
	if selection.Total {
		for _, asset := range record.Assets {
			total += asset.BuyAmount
		}
		if len(record.Assets) == 0 {
			if record.AllocatedCash > moneyEpsilon {
				return roundMoney(record.AllocatedCash)
			}
			return roundMoney(record.InvestAmount)
		}
		return roundMoney(total)
	}

	for _, asset := range record.Assets {
		key := strings.ToLower(strings.TrimSpace(asset.Name))
		if _, ok := selection.Keys[key]; ok {
			total += asset.BuyAmount
		}
	}
	return roundMoney(total)
}

func yieldRecordSellAmount(record InvestmentRecord, selection yieldSelection) float64 {
	if !isSellRecord(record) {
		return 0
	}
	total := 0.0
	if selection.Total {
		for _, asset := range record.Assets {
			total += asset.SellAmount
		}
		if total <= moneyEpsilon {
			total = record.SellAmount
		}
		return roundMoney(total)
	}

	for _, asset := range record.Assets {
		key := strings.ToLower(strings.TrimSpace(asset.Name))
		if _, ok := selection.Keys[key]; ok {
			total += asset.SellAmount
		}
	}
	return roundMoney(total)
}

func sortedYieldRecords(records []InvestmentRecord) []yieldTimedRecord {
	timed := make([]yieldTimedRecord, 0, len(records))
	for _, record := range records {
		archivedAt, ok := parseArchiveTime(record.ArchivedAt)
		if !ok {
			continue
		}
		timed = append(timed, yieldTimedRecord{
			At:     archivedAt,
			Record: record,
		})
	}
	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].At.Before(timed[j].At)
	})
	return timed
}

func yieldCashFlowsForPoint(records []yieldTimedRecord, selection yieldSelection, startAt, endAt time.Time, baseValue, endValue float64) ([]yieldCashFlow, float64, float64) {
	cashFlows := make([]yieldCashFlow, 0, len(records)+2)
	if math.Abs(baseValue) > moneyEpsilon {
		cashFlows = append(cashFlows, yieldCashFlow{At: startAt, Amount: -baseValue})
	}

	buyTotal := 0.0
	sellTotal := 0.0
	for _, record := range records {
		if record.At.Before(startAt) || !record.At.Before(endAt) {
			continue
		}
		buyAmount := yieldRecordBuyAmount(record.Record, selection)
		if buyAmount > moneyEpsilon {
			buyTotal += buyAmount
			cashFlows = append(cashFlows, yieldCashFlow{At: record.At, Amount: -buyAmount, External: true})
		}
		sellAmount := yieldRecordSellAmount(record.Record, selection)
		if sellAmount > moneyEpsilon {
			sellTotal += sellAmount
			cashFlows = append(cashFlows, yieldCashFlow{At: record.At, Amount: sellAmount, External: true})
		}
	}
	if math.Abs(endValue) > moneyEpsilon {
		cashFlows = append(cashFlows, yieldCashFlow{At: endAt, Amount: endValue})
	}
	return cashFlows, roundMoney(buyTotal), roundMoney(sellTotal)
}

func yieldRates(cashFlows []yieldCashFlow, startAt, endAt time.Time, baseValue, buyTotal, sellTotal, endValue float64) (float64, float64) {
	if !endAt.After(startAt) {
		return 0, 0
	}
	if annualized, ok := solveXIRR(cashFlows); ok {
		years := yearFraction(startAt, endAt)
		if years > 0 && annualized > -1 {
			return finiteFloat(math.Pow(1+annualized, years)-1, 0), annualized
		}
	}

	periodRate, ok := modifiedDietzRate(startAt, endAt, baseValue, buyTotal, sellTotal, endValue, cashFlows)
	if !ok {
		return 0, 0
	}
	return periodRate, annualizePeriodRate(periodRate, startAt, endAt)
}

func solveXIRR(cashFlows []yieldCashFlow) (float64, bool) {
	if len(cashFlows) < 2 || !cashFlowsHaveBothSigns(cashFlows) {
		return 0, false
	}

	rate := 0.1
	for range 50 {
		value, derivative := xirrNPVAndDerivative(cashFlows, rate)
		if math.Abs(value) < 0.000001 {
			return rate, true
		}
		if math.Abs(derivative) < 1e-12 {
			break
		}
		next := rate - value/derivative
		if !isFinite(next) || next <= -0.999999 {
			break
		}
		if math.Abs(next-rate) < 1e-10 {
			return next, true
		}
		rate = next
	}

	low := -0.999999
	high := 1.0
	lowValue := xirrNPV(cashFlows, low)
	highValue := xirrNPV(cashFlows, high)
	if math.Abs(lowValue) < 0.000001 {
		return low, true
	}
	if math.Abs(highValue) < 0.000001 {
		return high, true
	}
	for sameSign(lowValue, highValue) && high < 1_000_000 {
		high = high*2 + 1
		highValue = xirrNPV(cashFlows, high)
		if math.Abs(highValue) < 0.000001 {
			return high, true
		}
	}
	if sameSign(lowValue, highValue) {
		return 0, false
	}
	for range 100 {
		mid := (low + high) / 2
		midValue := xirrNPV(cashFlows, mid)
		if math.Abs(midValue) < 0.000001 {
			return mid, true
		}
		if sameSign(lowValue, midValue) {
			low = mid
			lowValue = midValue
		} else {
			high = mid
		}
	}
	return (low + high) / 2, true
}

func xirrNPV(cashFlows []yieldCashFlow, rate float64) float64 {
	value, _ := xirrNPVAndDerivative(cashFlows, rate)
	return value
}

func xirrNPVAndDerivative(cashFlows []yieldCashFlow, rate float64) (float64, float64) {
	baseDate := cashFlows[0].At
	value := 0.0
	derivative := 0.0
	for _, cashFlow := range cashFlows {
		years := yearFraction(baseDate, cashFlow.At)
		denominator := math.Pow(1+rate, years)
		value += cashFlow.Amount / denominator
		if years > 0 {
			derivative -= years * cashFlow.Amount / math.Pow(1+rate, years+1)
		}
	}
	return value, derivative
}

func modifiedDietzRate(startAt, endAt time.Time, baseValue, buyTotal, sellTotal, endValue float64, cashFlows []yieldCashFlow) (float64, bool) {
	periodDays := endAt.Sub(startAt).Hours() / 24
	if periodDays <= 0 {
		return 0, false
	}

	weightedCapital := baseValue
	for _, cashFlow := range cashFlows {
		if !cashFlow.External {
			continue
		}
		if cashFlow.At.Before(startAt) || !cashFlow.At.Before(endAt) {
			continue
		}
		weight := endAt.Sub(cashFlow.At).Hours() / 24 / periodDays
		weightedCapital += -cashFlow.Amount * weight
	}
	if weightedCapital <= moneyEpsilon {
		return 0, false
	}

	profit := endValue + sellTotal - baseValue - buyTotal
	return finiteFloat(profit/weightedCapital, 0), true
}

func annualizePeriodRate(periodRate float64, startAt, endAt time.Time) float64 {
	years := yearFraction(startAt, endAt)
	if years <= 0 {
		return 0
	}
	if periodRate <= -1 {
		return -1
	}
	return finiteFloat(math.Pow(1+periodRate, 1/years)-1, 0)
}

func yieldPointsHaveAny(points []yieldPoint) bool {
	for _, point := range points {
		if point.Present {
			return true
		}
	}
	return false
}

func latestYieldPoint(points []yieldPoint) (yieldPoint, bool) {
	for i := len(points) - 1; i >= 0; i-- {
		if points[i].Present {
			return points[i], true
		}
	}
	return yieldPoint{}, false
}

func latestYieldPointTime(points []yieldPoint, monthly map[time.Time]trendMonthlyRecord) time.Time {
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].Present {
			continue
		}
		if record, ok := monthly[points[i].Month]; ok {
			return record.ArchivedAt
		}
	}
	return time.Time{}
}

func yieldValueRange(points []yieldPoint) (float64, float64) {
	minValue := 0.0
	maxValue := 0.0
	initialized := false
	for _, point := range points {
		if !point.Present {
			continue
		}
		if !initialized || point.Rate < minValue {
			minValue = point.Rate
		}
		if !initialized || point.Rate > maxValue {
			maxValue = point.Rate
		}
		initialized = true
	}
	if !initialized {
		return -0.01, 0.01
	}
	if math.Abs(maxValue-minValue) < 0.0001 {
		padding := math.Max(0.01, math.Abs(maxValue)*0.18)
		return minValue - padding, maxValue + padding
	}
	padding := (maxValue - minValue) * 0.18
	return minValue - padding, maxValue + padding
}

func cashFlowsHaveBothSigns(cashFlows []yieldCashFlow) bool {
	hasPositive := false
	hasNegative := false
	for _, cashFlow := range cashFlows {
		if cashFlow.Amount > moneyEpsilon {
			hasPositive = true
		}
		if cashFlow.Amount < -moneyEpsilon {
			hasNegative = true
		}
	}
	return hasPositive && hasNegative
}

func sameSign(a, b float64) bool {
	return a > 0 && b > 0 || a < 0 && b < 0
}

func yearFraction(startAt, endAt time.Time) float64 {
	return endAt.Sub(startAt).Hours() / 24 / 365.2425
}

func finiteFloat(value, fallback float64) float64 {
	if !isFinite(value) {
		return fallback
	}
	return value
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
