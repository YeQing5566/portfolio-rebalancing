package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

const (
	trendTotalSeries = "总资产"
	trendMonthFmt    = "2006-01"
)

type trendMonthlyRecord struct {
	Month      time.Time
	ArchivedAt time.Time
	Record     InvestmentRecord
}

type trendPoint struct {
	Month   time.Time
	Value   float64
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
	trendSeriesPanel     *walk.Composite
	trendChartWidget     *walk.CustomWidget
	trendStartEdit       *walk.LineEdit
	trendEndEdit         *walk.LineEdit
	trendRangeLabel      *walk.Label
	trendSelections      = make(map[string]bool)
	trendData            trendChartData
	trendRangeUserSet    bool
	trendRefreshingCards bool
	trendPalette         = []walk.Color{
		walk.RGB(31, 111, 235),
		walk.RGB(230, 126, 34),
		walk.RGB(46, 160, 67),
		walk.RGB(186, 104, 200),
		walk.RGB(214, 68, 68),
		walk.RGB(0, 151, 167),
		walk.RGB(133, 88, 255),
		walk.RGB(102, 153, 0),
		walk.RGB(204, 85, 0),
		walk.RGB(0, 102, 204),
	}
)

func buildTrendPage() Widget {
	return Composite{
		Layout: HBox{
			MarginsZero: true,
			Spacing:     8,
		},
		Children: []Widget{
			GroupBox{
				Title:      "资产选择",
				MinSize:    Size{Width: 260},
				MaxSize:    Size{Width: 300, Height: 2000},
				Background: panelBackground,
				Layout: VBox{
					Margins: Margins{Left: 8, Top: 10, Right: 8, Bottom: 8},
					Spacing: 8,
				},
				Children: []Widget{
					Label{
						Text:      "勾选要显示的资产趋势",
						TextColor: mutedTextColor,
					},
					ScrollView{
						HorizontalFixed: true,
						StretchFactor:   1,
						Layout: VBox{
							MarginsZero: true,
							Spacing:     6,
						},
						Children: []Widget{
							Composite{
								AssignTo: &trendSeriesPanel,
								Layout: VBox{
									MarginsZero: true,
									Spacing:     6,
								},
							},
						},
					},
					Label{
						Text:      "资产名称按历史记录去重",
						TextColor: mutedTextColor,
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 8},
					},
				},
			},
			GroupBox{
				Title:         "资产金额变化趋势",
				StretchFactor: 1,
				Background:    panelBackground,
				Layout: VBox{
					Margins: Margins{Left: 10, Top: 12, Right: 10, Bottom: 10},
					Spacing: 8,
				},
				Children: []Widget{
					Composite{
						Layout: HBox{
							MarginsZero: true,
							Spacing:     8,
						},
						Children: []Widget{
							Label{
								Text:      "开始月份",
								TextColor: mutedTextColor,
							},
							LineEdit{
								AssignTo:          &trendStartEdit,
								CueBanner:         "YYYY-MM",
								ToolTipText:       "示例：2026-01",
								MinSize:           Size{Width: 96},
								MaxSize:           Size{Width: 112, Height: 100},
								OnEditingFinished: applyTrendRangeFromInputs,
							},
							Label{
								Text:      "结束月份",
								TextColor: mutedTextColor,
							},
							LineEdit{
								AssignTo:          &trendEndEdit,
								CueBanner:         "YYYY-MM",
								ToolTipText:       "示例：2026-12",
								MinSize:           Size{Width: 96},
								MaxSize:           Size{Width: 112, Height: 100},
								OnEditingFinished: applyTrendRangeFromInputs,
							},
							PushButton{
								Text:    "应用范围",
								MinSize: Size{Width: 88, Height: 28},
								OnClicked: func() {
									applyTrendRangeFromInputs()
								},
							},
							PushButton{
								Text:    "查看最近一年",
								MinSize: Size{Width: 110, Height: 28},
								OnClicked: func() {
									setTrendRecentYear()
								},
							},
							HSpacer{},
						},
					},
					Label{
						AssignTo:  &trendRangeLabel,
						Text:      "暂无历史记录",
						TextColor: mutedTextColor,
					},
					CustomWidget{
						AssignTo:            &trendChartWidget,
						Background:          resultBackground,
						InvalidatesOnResize: true,
						MinSize:             Size{Height: 360},
						PaintMode:           PaintBuffered,
						PaintPixels:         paintTrendChart,
						StretchFactor:       1,
					},
					Label{
						Text:      "趋势图基于月度数据",
						TextColor: mutedTextColor,
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 8},
					},
				},
			},
		},
	}
}

func refreshTrendView() {
	if trendChartWidget == nil {
		return
	}

	options := trendSeriesOptions(investmentRecords)
	syncTrendSelections(options)
	ensureTrendDateInputs(investmentRecords)
	rebuildTrendSeriesCards(options)
	redrawTrendChart()
}

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

func ensureTrendDateInputs(records []InvestmentRecord) {
	start, end, ok := trendMonthBounds(records)
	if !ok {
		if !trendRangeUserSet {
			trendStartEdit.SetText("")
			trendEndEdit.SetText("")
		}
		return
	}
	if trendRangeUserSet && strings.TrimSpace(trendStartEdit.Text()) != "" && strings.TrimSpace(trendEndEdit.Text()) != "" {
		return
	}

	trendStartEdit.SetText(start.Format(trendMonthFmt))
	trendEndEdit.SetText(end.Format(trendMonthFmt))
}

func rebuildTrendSeriesCards(options []string) {
	if trendSeriesPanel == nil || trendRefreshingCards {
		return
	}

	trendRefreshingCards = true
	defer func() {
		trendRefreshingCards = false
	}()

	trendSeriesPanel.SetSuspended(true)
	defer trendSeriesPanel.SetSuspended(false)

	children := trendSeriesPanel.Children()
	for i := children.Len() - 1; i >= 0; i-- {
		children.At(i).Dispose()
	}
	_ = children.Clear()

	for i, name := range options {
		addTrendSeriesCard(name, trendColorForIndex(i))
	}
	trendSeriesPanel.RequestLayout()
}

func addTrendSeriesCard(name string, color walk.Color) {
	card, err := walk.NewCompositeWithStyle(trendSeriesPanel, win.WS_BORDER)
	if err != nil {
		return
	}
	if err := card.SetMinMaxSize(walk.Size{Width: 218, Height: 42}, walk.Size{Width: 1000, Height: 42}); err != nil {
		return
	}
	bg, err := walk.NewSolidColorBrush(walk.RGB(250, 252, 255))
	if err == nil {
		card.SetBackground(bg)
		card.AddDisposable(bg)
	}
	layout := walk.NewHBoxLayout()
	_ = layout.SetMargins(walk.Margins{8, 5, 8, 5})
	_ = layout.SetSpacing(8)
	_ = card.SetLayout(layout)

	swatch, err := walk.NewLabel(card)
	if err == nil {
		_ = swatch.SetText("■")
		swatch.SetTextColor(color)
		_ = swatch.SetMinMaxSize(walk.Size{Width: 16, Height: 26}, walk.Size{Width: 16, Height: 26})
	}

	checkBox, err := walk.NewCheckBox(card)
	if err != nil {
		return
	}
	_ = checkBox.SetText(name)
	checkBox.SetChecked(trendSelections[name])
	_ = checkBox.SetMinMaxSize(walk.Size{Width: 168, Height: 26}, walk.Size{})
	checkBox.CheckedChanged().Attach(func() {
		if trendRefreshingCards {
			return
		}
		trendSelections[name] = checkBox.Checked()
		redrawTrendChart()
	})
	card.MouseDown().Attach(func(_ int, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			checkBox.SetChecked(!checkBox.Checked())
		}
	})
}

func applyTrendRangeFromInputs() {
	if trendStartEdit == nil || trendEndEdit == nil {
		return
	}

	start, end, err := trendRangeFromInputs()
	if err != nil {
		walk.MsgBox(mainWindow, "时间范围有误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}
	trendRangeUserSet = true
	trendStartEdit.SetText(start.Format(trendMonthFmt))
	trendEndEdit.SetText(end.Format(trendMonthFmt))
	redrawTrendChart()
}

func setTrendRecentYear() {
	_, latest, ok := trendMonthBounds(investmentRecords)
	if !ok {
		latest = normalizeTrendMonth(time.Now())
	}
	start := latest.AddDate(0, -11, 0)
	trendRangeUserSet = true
	trendStartEdit.SetText(start.Format(trendMonthFmt))
	trendEndEdit.SetText(latest.Format(trendMonthFmt))
	redrawTrendChart()
}

func redrawTrendChart() {
	if trendChartWidget == nil {
		return
	}

	start, end, err := trendRangeFromInputs()
	if err != nil {
		trendData = trendChartData{Message: err.Error()}
		updateTrendRangeLabel("")
		_ = trendChartWidget.Invalidate()
		return
	}

	trendData = buildTrendChartData(investmentRecords, trendSelections, start, end)
	updateTrendRangeLabel(fmt.Sprintf("%s 至 %s", start.Format(trendMonthFmt), end.Format(trendMonthFmt)))
	_ = trendChartWidget.Invalidate()
}

func updateTrendRangeLabel(rangeText string) {
	if trendRangeLabel == nil {
		return
	}
	if rangeText == "" {
		trendRangeLabel.SetText("暂无可显示范围")
		return
	}

	selected := selectedTrendSeriesNames(trendSeriesOptions(investmentRecords), trendSelections)
	trendRangeLabel.SetText(fmt.Sprintf("显示范围：%s｜已选 %d 条趋势", rangeText, len(selected)))
}

func trendRangeFromInputs() (time.Time, time.Time, error) {
	startText := strings.TrimSpace(trendStartEdit.Text())
	endText := strings.TrimSpace(trendEndEdit.Text())
	if startText == "" || endText == "" {
		start, end, ok := trendMonthBounds(investmentRecords)
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("暂无历史记录可生成趋势图")
		}
		return start, end, nil
	}

	start, err := parseTrendMonth(startText)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("开始月份格式应为 YYYY-MM")
	}
	end, err := parseTrendMonth(endText)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("结束月份格式应为 YYYY-MM")
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("开始月份不能晚于结束月份")
	}
	return start, end, nil
}

func trendSeriesOptions(records []InvestmentRecord) []string {
	assets := make([]string, 0)
	seen := map[string]struct{}{
		strings.ToLower(trendTotalSeries): {},
	}
	for _, record := range records {
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
					point.Value = monthlyRecord.Record.AfterTotal
					point.Present = true
				} else if amount, ok := trendAssetAmount(monthlyRecord.Record, name); ok {
					point.Value = amount
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
		archivedAt, ok := parseArchiveTime(record.ArchivedAt)
		if !ok {
			continue
		}
		month := normalizeTrendMonth(archivedAt)
		current, exists := monthly[month]
		if !exists || archivedAt.Before(current.ArchivedAt) {
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

func trendAssetAmount(record InvestmentRecord, name string) (float64, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, asset := range record.Assets {
		if strings.ToLower(strings.TrimSpace(asset.Name)) == key {
			return asset.AfterAmount, true
		}
	}
	return 0, false
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

func paintTrendChart(canvas *walk.Canvas, _ walk.Rectangle) error {
	if trendChartWidget == nil {
		return nil
	}

	bounds := trendChartWidget.ClientBoundsPixels()
	fillRect(canvas, walk.RGB(250, 252, 255), bounds)
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil
	}

	data := trendData
	if len(data.Months) == 0 || len(data.Series) == 0 || data.Message != "" && !trendSeriesHasAnyPoint(data.Series) {
		message := data.Message
		if message == "" {
			message = "暂无历史记录可生成趋势图"
		}
		return drawCenteredChartMessage(canvas, bounds, message)
	}

	left, top, right, bottom := 88, 16+trendLegendRows(bounds.Width, len(data.Series))*20+16, 28, 72
	plot := walk.Rectangle{
		X:      left,
		Y:      top,
		Width:  maxInt(1, bounds.Width-left-right),
		Height: maxInt(1, bounds.Height-top-bottom),
	}
	if plot.Width < 160 || plot.Height < 140 {
		return drawCenteredChartMessage(canvas, bounds, "窗口空间不足，无法绘制趋势图")
	}

	minValue, maxValue := trendValueRange(data.Series)
	drawTrendGrid(canvas, plot, minValue, maxValue)
	drawTrendXAxis(canvas, plot, data.Months)
	drawTrendLines(canvas, plot, data, minValue, maxValue)
	drawTrendLegend(canvas, bounds, data.Series)
	return nil
}

func drawCenteredChartMessage(canvas *walk.Canvas, bounds walk.Rectangle, message string) error {
	return canvas.DrawTextPixels(
		message,
		trendChartWidget.Font(),
		mutedTextColor,
		bounds,
		walk.TextCenter|walk.TextVCenter|walk.TextSingleLine,
	)
}

func drawTrendGrid(canvas *walk.Canvas, plot walk.Rectangle, minValue, maxValue float64) {
	axisPen, axisBrush := newChartPen(walk.RGB(160, 172, 188), 1)
	defer axisPen.Dispose()
	defer axisBrush.Dispose()
	gridPen, gridBrush := newChartPen(walk.RGB(224, 231, 240), 1)
	defer gridPen.Dispose()
	defer gridBrush.Dispose()

	_ = canvas.DrawLinePixels(axisPen, walk.Point{X: plot.X, Y: plot.Y}, walk.Point{X: plot.X, Y: plot.Y + plot.Height})
	_ = canvas.DrawLinePixels(axisPen, walk.Point{X: plot.X, Y: plot.Y + plot.Height}, walk.Point{X: plot.X + plot.Width, Y: plot.Y + plot.Height})

	for i := 0; i <= 4; i++ {
		y := plot.Y + plot.Height - i*plot.Height/4
		_ = canvas.DrawLinePixels(gridPen, walk.Point{X: plot.X, Y: y}, walk.Point{X: plot.X + plot.Width, Y: y})
		value := minValue + (maxValue-minValue)*float64(i)/4
		_ = canvas.DrawTextPixels(
			formatCompactMoney(value),
			trendChartWidget.Font(),
			mutedTextColor,
			walk.Rectangle{X: 0, Y: y - 9, Width: plot.X - 10, Height: 20},
			walk.TextRight|walk.TextVCenter|walk.TextSingleLine,
		)
	}
}

func drawTrendXAxis(canvas *walk.Canvas, plot walk.Rectangle, months []time.Time) {
	if len(months) == 0 {
		return
	}

	tickPen, tickBrush := newChartPen(walk.RGB(196, 206, 219), 1)
	defer tickPen.Dispose()
	defer tickBrush.Dispose()
	step := maxInt(1, int(math.Ceil(float64(len(months))/8)))
	for i, month := range months {
		x := trendPointX(plot, i, len(months))
		if i%step == 0 || i == len(months)-1 {
			_ = canvas.DrawLinePixels(tickPen, walk.Point{X: x, Y: plot.Y + plot.Height}, walk.Point{X: x, Y: plot.Y + plot.Height + 5})
			label := month.Format("06-01")
			if len(months) <= 12 {
				label = month.Format(trendMonthFmt)
			}
			_ = canvas.DrawTextPixels(
				label,
				trendChartWidget.Font(),
				mutedTextColor,
				walk.Rectangle{X: x - 34, Y: plot.Y + plot.Height + 8, Width: 68, Height: 22},
				walk.TextCenter|walk.TextVCenter|walk.TextSingleLine,
			)
		}
	}
}

func drawTrendLines(canvas *walk.Canvas, plot walk.Rectangle, data trendChartData, minValue, maxValue float64) {
	for _, series := range data.Series {
		pen, penBrush := newChartPen(series.Color, 2)
		brush, err := walk.NewSolidColorBrush(series.Color)
		if err != nil {
			pen.Dispose()
			penBrush.Dispose()
			continue
		}

		var previous *walk.Point
		for i, point := range series.Points {
			if !point.Present {
				continue
			}
			current := walk.Point{
				X: trendPointX(plot, i, len(data.Months)),
				Y: trendPointY(plot, minValue, maxValue, point.Value),
			}
			if previous != nil {
				_ = canvas.DrawLinePixels(pen, *previous, current)
			}
			_ = canvas.FillEllipsePixels(brush, walk.Rectangle{X: current.X - 3, Y: current.Y - 3, Width: 6, Height: 6})
			copyPoint := current
			previous = &copyPoint
		}
		brush.Dispose()
		pen.Dispose()
		penBrush.Dispose()
	}
}

func drawTrendLegend(canvas *walk.Canvas, bounds walk.Rectangle, series []trendSeries) {
	x := 88
	y := 16
	for _, item := range series {
		if x > bounds.Width-150 {
			x = 88
			y += 20
		}
		pen, brush := newChartPen(item.Color, 2)
		_ = canvas.DrawLinePixels(pen, walk.Point{X: x, Y: y + 8}, walk.Point{X: x + 18, Y: y + 8})
		pen.Dispose()
		brush.Dispose()
		_ = canvas.DrawTextPixels(
			item.Name,
			trendChartWidget.Font(),
			defaultTextColor,
			walk.Rectangle{X: x + 24, Y: y, Width: 120, Height: 18},
			walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis,
		)
		x += 150
	}
}

func trendLegendRows(width int, count int) int {
	if count <= 0 {
		return 0
	}
	available := maxInt(150, width-88-28)
	perRow := maxInt(1, available/150)
	return (count + perRow - 1) / perRow
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

func newChartPen(color walk.Color, width int) (*walk.GeometricPen, *walk.SolidColorBrush) {
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		panic(err)
	}
	pen, err := walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound|walk.PenJoinRound, width, brush)
	if err != nil {
		brush.Dispose()
		panic(err)
	}
	return pen, brush
}

func fillRect(canvas *walk.Canvas, color walk.Color, bounds walk.Rectangle) {
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		return
	}
	defer brush.Dispose()
	_ = canvas.FillRectanglePixels(brush, bounds)
}

func formatCompactMoney(value float64) string {
	abs := math.Abs(value)
	switch {
	case abs >= 100000000:
		return fmt.Sprintf("%.2f亿", value/100000000)
	case abs >= 10000:
		return fmt.Sprintf("%.2f万", value/10000)
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
