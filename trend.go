package main

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lxn/walk"
)

const (
	trendTotalSeries = "总资产"
	trendMonthFmt    = "2006-01"
	trendDataHint    = "数据为当月最新记录中的买入前金额"
)

type trendMonthlyRecord struct {
	Month      time.Time
	ArchivedAt time.Time
	Record     InvestmentRecord
}

type trendPoint struct {
	Month   time.Time
	Value   float64
	Pct     float64
	Present bool
}

type trendSeries struct {
	Name   string
	Color  walk.Color
	Points []trendPoint
}

type trendChartData struct {
	Months  []time.Time
	Series  []trendSeries
	Message string
}

var (
	trendSelections = make(map[string]bool)
	trendPalette    = []walk.Color{
		walk.RGB(255, 153, 0),
		walk.RGB(56, 189, 248),
		walk.RGB(34, 197, 94),
		walk.RGB(244, 114, 182),
		walk.RGB(168, 85, 247),
		walk.RGB(239, 68, 68),
		walk.RGB(20, 184, 166),
		walk.RGB(250, 204, 21),
		walk.RGB(129, 140, 248),
		walk.RGB(245, 245, 245),
	}
)

func syncTrendSelections(options []string) {
	allowed := make(map[string]struct{}, len(options))
	for _, name := range options {
		allowed[name] = struct{}{}
	}
	for name := range trendSelections {
		if _, ok := allowed[name]; !ok {
			delete(trendSelections, name)
		}
	}

	selected := 0
	for _, name := range options {
		if trendSelections[name] {
			selected++
		}
	}
	if selected == 0 && len(options) > 0 {
		trendSelections[options[0]] = true
	}
}

func trendSeriesOptions(records []InvestmentRecord) []string {
	assets := make([]string, 0)
	seen := map[string]struct{}{
		strings.ToLower(trendTotalSeries): {},
	}
	for _, record := range records {
		if isSellRecord(record) {
			continue
		}
		for _, asset := range record.Assets {
			name := strings.TrimSpace(asset.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			assets = append(assets, name)
		}
	}
	sort.Strings(assets)

	options := make([]string, 0, len(assets)+1)
	options = append(options, trendTotalSeries)
	options = append(options, assets...)
	return options
}

func selectedTrendSeriesNames(options []string, selections map[string]bool) []string {
	selected := make([]string, 0, len(options))
	for _, name := range options {
		if selections[name] {
			selected = append(selected, name)
		}
	}
	return selected
}

func buildTrendChartData(records []InvestmentRecord, selections map[string]bool, start, end time.Time) trendChartData {
	start = normalizeTrendMonth(start)
	end = normalizeTrendMonth(end)
	if start.After(end) {
		return trendChartData{Message: "开始月份不能晚于结束月份"}
	}

	months := trendMonthRange(start, end)
	options := trendSeriesOptions(records)
	names := selectedTrendSeriesNames(options, selections)
	if len(names) == 0 {
		return trendChartData{Months: months, Message: "请选择左侧资产"}
	}

	monthly := buildMonthlyTrendRecords(records)
	series := make([]trendSeries, 0, len(names))
	for _, name := range names {
		points := make([]trendPoint, 0, len(months))
		for _, month := range months {
			point := trendPoint{Month: month}
			if monthlyRecord, ok := monthly[month]; ok {
				if name == trendTotalSeries {
					point.Value = trendRecordCurrentTotal(monthlyRecord.Record)
					point.Pct = 100
					point.Present = true
				} else if amount, pct, ok := trendAssetCurrentPosition(monthlyRecord.Record, name); ok {
					point.Value = amount
					point.Pct = pct
					point.Present = true
				}
			}
			points = append(points, point)
		}
		series = append(series, trendSeries{
			Name:   name,
			Color:  trendColorForIndex(optionIndex(options, name)),
			Points: points,
		})
	}

	message := ""
	if !trendSeriesHasAnyPoint(series) {
		message = "当前范围没有可显示数据"
	}
	return trendChartData{
		Months:  months,
		Series:  series,
		Message: message,
	}
}

func buildMonthlyTrendRecords(records []InvestmentRecord) map[time.Time]trendMonthlyRecord {
	monthly := make(map[time.Time]trendMonthlyRecord)
	for _, record := range records {
		if isSellRecord(record) {
			continue
		}
		archivedAt, ok := parseArchiveTime(record.ArchivedAt)
		if !ok {
			continue
		}
		month := normalizeTrendMonth(archivedAt)
		current, exists := monthly[month]
		if !exists || archivedAt.After(current.ArchivedAt) {
			monthly[month] = trendMonthlyRecord{
				Month:      month,
				ArchivedAt: archivedAt,
				Record:     cloneInvestmentRecord(record),
			}
		}
	}
	return monthly
}

func trendMonthBounds(records []InvestmentRecord) (time.Time, time.Time, bool) {
	monthly := buildMonthlyTrendRecords(records)
	if len(monthly) == 0 {
		return time.Time{}, time.Time{}, false
	}

	var start time.Time
	var end time.Time
	first := true
	for month := range monthly {
		if first || month.Before(start) {
			start = month
		}
		if first || month.After(end) {
			end = month
		}
		first = false
	}
	return start, end, true
}

func trendMonthRange(start, end time.Time) []time.Time {
	start = normalizeTrendMonth(start)
	end = normalizeTrendMonth(end)
	if start.After(end) {
		return nil
	}

	months := make([]time.Time, 0)
	for month := start; !month.After(end); month = month.AddDate(0, 1, 0) {
		months = append(months, month)
	}
	return months
}

func parseArchiveTime(value string) (time.Time, bool) {
	parsed, err := time.ParseInLocation(archiveTimeFmt, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func parseTrendMonth(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation(trendMonthFmt, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeTrendMonth(parsed), nil
}

func normalizeTrendMonth(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.Local)
}

func trendAssetCurrentAmount(record InvestmentRecord, name string) (float64, bool) {
	amount, _, ok := trendAssetCurrentPosition(record, name)
	return amount, ok
}

func trendAssetCurrentPosition(record InvestmentRecord, name string) (float64, float64, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	total := trendRecordCurrentTotal(record)
	for _, asset := range record.Assets {
		if strings.ToLower(strings.TrimSpace(asset.Name)) == key {
			pct := 0.0
			if total > moneyEpsilon {
				pct = asset.BeforeAmount / total * 100
			}
			return asset.BeforeAmount, pct, true
		}
	}
	return 0, 0, false
}

func trendRecordCurrentTotal(record InvestmentRecord) float64 {
	if record.CurrentTotal > moneyEpsilon {
		return record.CurrentTotal
	}
	total := 0.0
	for _, asset := range record.Assets {
		total += asset.BeforeAmount
	}
	if len(record.Assets) > 0 {
		return roundMoney(total)
	}
	return record.CurrentTotal
}

func trendSeriesHasAnyPoint(series []trendSeries) bool {
	for _, item := range series {
		for _, point := range item.Points {
			if point.Present {
				return true
			}
		}
	}
	return false
}

func optionIndex(options []string, name string) int {
	for i, option := range options {
		if option == name {
			return i
		}
	}
	return 0
}

func trendColorForIndex(index int) walk.Color {
	if index < 0 {
		index = 0
	}
	return trendPalette[index%len(trendPalette)]
}

func trendValueRange(series []trendSeries) (float64, float64) {
	minValue := 0.0
	maxValue := 0.0
	initialized := false
	for _, item := range series {
		for _, point := range item.Points {
			if !point.Present {
				continue
			}
			if !initialized || point.Value < minValue {
				minValue = point.Value
			}
			if !initialized || point.Value > maxValue {
				maxValue = point.Value
			}
			initialized = true
		}
	}
	if !initialized {
		return 0, 1
	}
	if math.Abs(maxValue-minValue) < 0.01 {
		padding := math.Max(1000, math.Abs(maxValue)*0.08)
		return math.Max(0, minValue-padding), maxValue + padding
	}
	padding := (maxValue - minValue) * 0.12
	return math.Max(0, minValue-padding), maxValue + padding
}

func trendPointX(plot walk.Rectangle, index, count int) int {
	if count <= 1 {
		return plot.X + plot.Width/2
	}
	return plot.X + index*plot.Width/(count-1)
}

func trendPointY(plot walk.Rectangle, minValue, maxValue, value float64) int {
	if maxValue <= minValue {
		return plot.Y + plot.Height/2
	}
	ratio := (value - minValue) / (maxValue - minValue)
	return plot.Y + plot.Height - int(math.Round(ratio*float64(plot.Height)))
}

func formatCompactMoney(value float64) string {
	abs := math.Abs(value)
	switch {
	case abs >= 100000000:
		return formatFlexibleNumber(value/100000000, 2) + "亿"
	case abs >= 10000:
		return formatFlexibleNumber(value/10000, 2) + "万"
	default:
		return formatMoney(value)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
