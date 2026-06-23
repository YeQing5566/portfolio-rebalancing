package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

const (
	dashPageCalc = iota
	dashPageHistory
	dashPageTrend
)

const (
	dashActionButton = "button"
	dashActionNav    = "nav"
	dashActionSplit  = "split"
	dashActionSelect = "select"
	dashActionToggle = "toggle"
	dashActionScroll = "scroll"
)

type dashboardUI struct {
	mw          *walk.MainWindow
	canvas      *walk.CustomWidget
	editorLayer *walk.Composite
	editor      *walk.LineEdit

	fontTiny  *walk.Font
	fontSmall *walk.Font
	fontMono  *walk.Font
	fontBody  *walk.Font
	fontBold  *walk.Font
	fontTitle *walk.Font
	fontBig   *walk.Font
	fontHuge  *walk.Font

	page               int
	assets             []AssetInput
	investAmount       float64
	selectedAsset      int
	assetScroll        int
	resultScroll       int
	resultText         string
	statusText         string
	hoverAction        string
	historyScroll      int
	historyAssetScroll int
	trendOptionScroll  int
	trendStartText     string
	trendEndText       string
	regionWidth        int
	regionHeight       int
	dragMode           string
	dragStartX         int
	dragStartY         int
	dragStartBounds    walk.Rectangle
	dragStartTable     int
	dragStartScroll    int
	dragScrollTotal    int
	dragScrollVisible  int
	dragScrollTrackH   int
	dragScrollThumbH   int
	assetTableHeight   int
	cursorMode         string
	lastTitleClickAt   time.Time
	lastTitleClickX    int
	lastTitleClickY    int
	activeField        *dashField
	actions            []dashAction
	fields             []dashField
	scrollbars         []dashScrollbar
}

type dashAction struct {
	Rect  walk.Rectangle
	Kind  string
	Key   string
	Index int
	Name  string
}

type dashField struct {
	Rect        walk.Rectangle
	Key         string
	Index       int
	Value       string
	Placeholder string
	Numeric     bool
	Suffix      string
}

type dashScrollbar struct {
	Key     string
	Track   walk.Rectangle
	Thumb   walk.Rectangle
	Total   int
	Visible int
	Offset  int
}

type dashPalette struct {
	bg            walk.Color
	shell         walk.Color
	sidebar       walk.Color
	panel         walk.Color
	panel2        walk.Color
	card          walk.Color
	card2         walk.Color
	line          walk.Color
	line2         walk.Color
	text          walk.Color
	muted         walk.Color
	faint         walk.Color
	accent        walk.Color
	accentHover   walk.Color
	accentPressed walk.Color
	warning       walk.Color
	danger        walk.Color
	white         walk.Color
}

type dashMetrics struct {
	sidebarWidth int
	pagePad      int
	cardRadius   int
	panelRadius  int
	buttonRadius int
	buttonHeight int
	cardPad      int
	gap          int
	resizeEdge   int
	minWidth     int
	minHeight    int
}

var dashColors = dashPalette{
	bg:            walk.RGB(5, 5, 5),
	shell:         walk.RGB(8, 8, 8),
	sidebar:       walk.RGB(0, 0, 0),
	panel:         walk.RGB(18, 18, 18),
	panel2:        walk.RGB(24, 24, 24),
	card:          walk.RGB(18, 18, 18),
	card2:         walk.RGB(31, 31, 31),
	line:          walk.RGB(42, 42, 42),
	line2:         walk.RGB(58, 58, 58),
	text:          walk.RGB(245, 245, 245),
	muted:         walk.RGB(163, 163, 163),
	faint:         walk.RGB(115, 115, 115),
	accent:        walk.RGB(255, 153, 0),
	accentHover:   walk.RGB(255, 177, 59),
	accentPressed: walk.RGB(230, 134, 0),
	warning:       walk.RGB(251, 191, 36),
	danger:        walk.RGB(239, 68, 68),
	white:         walk.RGB(255, 255, 255),
}

var dashStyle = dashMetrics{
	sidebarWidth: 248,
	pagePad:      28,
	cardRadius:   14,
	panelRadius:  14,
	buttonRadius: 10,
	buttonHeight: 42,
	cardPad:      18,
	gap:          14,
	resizeEdge:   9,
	minWidth:     1100,
	minHeight:    720,
}

var (
	createRoundRectRgnProc = syscall.NewLazyDLL("gdi32.dll").NewProc("CreateRoundRectRgn")
	setWindowRgnProc       = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowRgn")
)

func main() {
	runDashboardApp()
}

func runDashboardApp() {
	walk.AppendToWalkInit(func() {
		walk.FocusEffect = nil
		walk.ValidationErrorEffect = nil
	})

	ui := &dashboardUI{
		page:          dashPageCalc,
		investAmount:  5000,
		selectedAsset: -1,
		resultText:    initialResultText(),
	}
	ui.initFonts()
	if err := ui.loadPortfolioConfig(); err != nil {
		ui.statusText = "当前资产配置读取失败：" + err.Error()
	}

	window := MainWindow{
		AssignTo:   &ui.mw,
		Title:      "投资组合再平衡助手",
		MinSize:    Size{Width: dashStyle.minWidth, Height: dashStyle.minHeight},
		Size:       Size{Width: 1360, Height: 860},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 11},
		Background: SolidColorBrush{Color: dashColors.shell},
		Layout: VBox{
			MarginsZero: true,
			SpacingZero: true,
		},
		Children: []Widget{
			CustomWidget{
				AssignTo:            &ui.canvas,
				InvalidatesOnResize: true,
				PaintMode:           PaintBuffered,
				PaintPixels:         ui.paint,
				StretchFactor:       1,
			},
		},
	}

	if err := window.Create(); err != nil {
		showStartupError(err)
	}
	mainWindow = ui.mw
	dashboard = ui
	ui.installBorderlessWindow()
	ui.installEditor()
	ui.attachEvents()
	if err := loadInvestmentRecords(); err != nil {
		ui.statusText = "历史记录读取失败：" + err.Error()
	}
	ui.ensureHistorySelection()
	ui.ensureTrendRange()
	ui.invalidate()
	ui.mw.Run()
}

var dashboard *dashboardUI

func (ui *dashboardUI) initFonts() {
	ui.fontTiny, _ = walk.NewFont("Microsoft YaHei UI", 9, 0)
	ui.fontSmall, _ = walk.NewFont("Microsoft YaHei UI", 10, 0)
	ui.fontMono, _ = walk.NewFont("NSimSun", 10, 0)
	ui.fontBody, _ = walk.NewFont("Microsoft YaHei UI", 11, 0)
	ui.fontBold, _ = walk.NewFont("Microsoft YaHei UI", 11, walk.FontBold)
	ui.fontTitle, _ = walk.NewFont("Microsoft YaHei UI", 17, walk.FontBold)
	ui.fontBig, _ = walk.NewFont("Microsoft YaHei UI", 23, 0)
	ui.fontHuge, _ = walk.NewFont("Microsoft YaHei UI", 28, 0)
}

func (ui *dashboardUI) installBorderlessWindow() {
	hwnd := ui.mw.Handle()
	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	style &^= win.WS_CAPTION | win.WS_THICKFRAME
	win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func (ui *dashboardUI) applyRoundedRegion(width, height int) {
	if ui.mw == nil || width <= 0 || height <= 0 {
		return
	}
	if ui.regionWidth == width && ui.regionHeight == height {
		return
	}
	rgn, _, _ := createRoundRectRgnProc.Call(0, 0, uintptr(width+1), uintptr(height+1), 28, 28)
	if rgn == 0 {
		return
	}
	ret, _, _ := setWindowRgnProc.Call(uintptr(ui.mw.Handle()), rgn, 1)
	if ret == 0 {
		win.DeleteObject(win.HGDIOBJ(rgn))
		return
	}
	ui.regionWidth = width
	ui.regionHeight = height
}

func (ui *dashboardUI) installEditor() {
	layer, err := walk.NewCompositeWithStyle(ui.mw, 0)
	if err != nil {
		return
	}
	ui.editorLayer = layer
	ui.editorLayer.SetVisible(false)
	_ = ui.editorLayer.SetBoundsPixels(rect(0, 0, 1, 1))
	ui.editorLayer.SetBackground(solidBrush(fieldFillColor()))

	editor, err := walk.NewLineEdit(ui.editorLayer)
	if err != nil {
		return
	}
	ui.editor = editor
	ui.editor.SetVisible(false)
	ui.makeEditorBorderless()
	ui.editor.SetBackground(solidBrush(fieldFillColor()))
	ui.editor.SetTextColor(dashColors.text)
	ui.editor.SetFont(ui.fontSmall)
	ui.editor.FocusedChanged().Attach(func() {
		if ui.activeField != nil && !ui.editor.Focused() {
			ui.commitEditor()
		}
	})
	ui.editor.KeyDown().Attach(func(key walk.Key) {
		switch key {
		case walk.KeyReturn:
			ui.commitEditor()
		case walk.KeyEscape:
			ui.cancelEditor()
		}
	})
}

func (ui *dashboardUI) makeEditorBorderless() {
	if ui.editor == nil {
		return
	}
	hwnd := ui.editor.Handle()
	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	style &^= win.WS_BORDER
	win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
	exStyle := uint32(win.GetWindowLong(hwnd, win.GWL_EXSTYLE))
	exStyle &^= win.WS_EX_CLIENTEDGE
	win.SetWindowLong(hwnd, win.GWL_EXSTYLE, int32(exStyle))
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	win.SendMessage(hwnd, win.EM_SETMARGINS, uintptr(0x0001|0x0002), 0)
}

func (ui *dashboardUI) attachEvents() {
	ui.canvas.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		ui.handleMouseDown(x, y, button)
	})
	ui.canvas.MouseMove().Attach(func(x, y int, button walk.MouseButton) {
		ui.handleMouseMove(x, y, button)
	})
	ui.canvas.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		ui.handleMouseUp(x, y, button)
	})
	ui.canvas.MouseWheel().Attach(func(x, y int, button walk.MouseButton) {
		ui.handleMouseWheel(x, y, button)
	})
}

func (ui *dashboardUI) invalidate() {
	if ui.canvas != nil {
		_ = ui.canvas.Invalidate()
	}
}

func (ui *dashboardUI) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	bounds := ui.canvas.ClientBoundsPixels()
	ui.applyRoundedRegion(bounds.Width, bounds.Height)
	ui.actions = ui.actions[:0]
	ui.fields = ui.fields[:0]
	ui.scrollbars = ui.scrollbars[:0]
	fill(canvas, dashColors.shell, bounds)
	_ = canvas.GradientFillRectanglePixels(dashColors.bg, dashColors.shell, walk.Vertical, bounds)

	ui.paintChrome(canvas, bounds)
	switch ui.page {
	case dashPageHistory:
		ui.paintHistory(canvas, bounds)
	case dashPageTrend:
		ui.paintTrend(canvas, bounds)
	default:
		ui.paintCalculator(canvas, bounds)
	}
	ui.drawWindowFrame(canvas, bounds)
	return nil
}

func (ui *dashboardUI) paintChrome(canvas *walk.Canvas, bounds walk.Rectangle) {
	sidebar := walk.Rectangle{X: 0, Y: 0, Width: dashStyle.sidebarWidth, Height: bounds.Height}
	fill(canvas, dashColors.sidebar, sidebar)
	drawLine(canvas, dashColors.line, sidebar.X+sidebar.Width, 0, sidebar.X+sidebar.Width, bounds.Height, 1)

	drawText(canvas, "●", ui.fontBig, dashColors.accent, rect(28, 30, 24, 28), walk.TextLeft|walk.TextVCenter)
	drawText(canvas, "Rebalance", ui.fontTitle, dashColors.text, rect(58, 27, 158, 30), walk.TextLeft|walk.TextVCenter)
	drawText(canvas, "投资组合再平衡助手", ui.fontSmall, dashColors.muted, rect(28, 64, 188, 20), walk.TextLeft|walk.TextVCenter)

	ui.drawInfoCard(canvas, rect(22, 112, 204, 72))
	ui.drawNav(canvas, 22, 216, "平衡买入计算", "nav-calc", dashPageCalc)
	ui.drawNav(canvas, 22, 280, "历史投资记录", "nav-history", dashPageHistory)
	ui.drawNav(canvas, 22, 344, "资产趋势图表", "nav-trend", dashPageTrend)

	ui.drawWindowButton(canvas, rect(bounds.Width-128, 18, 28, 28), "win-min")
	ui.drawWindowButton(canvas, rect(bounds.Width-88, 18, 28, 28), "win-max")
	ui.drawWindowButton(canvas, rect(bounds.Width-48, 18, 28, 28), "win-close")
}

func (ui *dashboardUI) drawWindowFrame(canvas *walk.Canvas, bounds walk.Rectangle) {
	if bounds.Width <= 8 || bounds.Height <= 8 {
		return
	}
	outer := rect(1, 1, bounds.Width-2, bounds.Height-2)
	drawRoundStroke(canvas, walk.RGB(70, 70, 70), outer, 14, 2)
	inner := rect(3, 3, bounds.Width-6, bounds.Height-6)
	drawRoundStroke(canvas, walk.RGB(22, 22, 22), inner, 12, 1)
	drawLine(canvas, walk.RGB(96, 64, 16), 28, 2, bounds.Width-28, 2, 1)
}

func (ui *dashboardUI) isMaximized() bool {
	return ui.mw != nil && win.IsZoomed(ui.mw.Handle())
}

func (ui *dashboardUI) toggleMaximize() {
	if ui.mw == nil {
		return
	}
	if ui.isMaximized() {
		win.ShowWindow(ui.mw.Handle(), win.SW_RESTORE)
	} else {
		win.ShowWindow(ui.mw.Handle(), win.SW_MAXIMIZE)
	}
	ui.regionWidth = 0
	ui.regionHeight = 0
	ui.invalidate()
}

func (ui *dashboardUI) drawInfoCard(canvas *walk.Canvas, r walk.Rectangle) {
	roundFill(canvas, dashColors.panel, r, dashStyle.cardRadius)
	drawRoundStroke(canvas, dashColors.line, r, dashStyle.cardRadius, 1)
	roundFill(canvas, walk.RGB(54, 37, 12), rect(r.X+14, r.Y+17, 38, 38), 19)
	drawText(canvas, "本", ui.fontBold, dashColors.accent, rect(r.X+14, r.Y+17, 38, 38), walk.TextCenter|walk.TextVCenter)
	drawText(canvas, "本地数据", ui.fontBold, dashColors.text, rect(r.X+66, r.Y+17, 112, 20), walk.TextLeft|walk.TextVCenter)
	drawText(canvas, "记录保存在本机", ui.fontSmall, dashColors.muted, rect(r.X+66, r.Y+40, 126, 18), walk.TextLeft|walk.TextVCenter)
}

func (ui *dashboardUI) drawNav(canvas *walk.Canvas, x, y int, title, key string, page int) {
	active := ui.page == page
	r := rect(x, y, 204, 50)
	bg := dashColors.panel
	fg := dashColors.muted
	if active {
		bg = walk.RGB(34, 24, 10)
		fg = dashColors.text
	} else if ui.hoverAction == key {
		bg = dashColors.card2
		fg = dashColors.text
	}
	roundFill(canvas, bg, r, dashStyle.buttonRadius)
	drawRoundStroke(canvas, chooseColor(active, walk.RGB(88, 57, 8), dashColors.line), r, dashStyle.buttonRadius, 1)
	if active {
		roundFill(canvas, dashColors.accent, rect(r.X, r.Y+10, 4, r.Height-20), 2)
	}
	drawText(canvas, title, ui.fontBold, fg, rect(r.X+14, r.Y, r.Width-28, r.Height), walk.TextCenter|walk.TextVCenter)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionNav, Key: key})
}

func (ui *dashboardUI) drawSmallAction(canvas *walk.Canvas, r walk.Rectangle, label, key string) {
	roundFill(canvas, dashColors.panel2, r, 8)
	drawRoundStroke(canvas, dashColors.line, r, 8, 1)
	drawText(canvas, label, ui.fontSmall, dashColors.text, inset(r, 12, 0), walk.TextLeft|walk.TextVCenter)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: key})
}

func (ui *dashboardUI) drawWindowButton(canvas *walk.Canvas, r walk.Rectangle, key string) {
	bg := walk.RGB(44, 44, 44)
	border := walk.RGB(96, 96, 96)
	if key == "win-close" {
		bg = walk.RGB(76, 28, 28)
		border = walk.RGB(150, 58, 58)
	}
	if ui.hoverAction == key {
		if key == "win-close" {
			bg = walk.RGB(126, 34, 34)
			border = walk.RGB(210, 86, 86)
		} else {
			bg = walk.RGB(70, 70, 70)
			border = walk.RGB(140, 140, 140)
		}
	}
	roundFill(canvas, bg, r, 14)
	drawRoundStroke(canvas, border, r, 14, 1)
	ui.drawWindowIcon(canvas, r, key)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: key})
}

func (ui *dashboardUI) drawWindowIcon(canvas *walk.Canvas, r walk.Rectangle, key string) {
	color := walk.RGB(255, 255, 255)
	if key == "win-close" {
		color = walk.RGB(255, 238, 238)
	}
	cx := r.X + r.Width/2
	cy := r.Y + r.Height/2
	switch key {
	case "win-min":
		drawLine(canvas, color, cx-5, cy, cx+5, cy, 2)
	case "win-max":
		if ui.isMaximized() {
			drawRoundStroke(canvas, color, rect(cx-3, cy-6, 9, 9), 1, 1)
			drawRoundStroke(canvas, color, rect(cx-6, cy-3, 9, 9), 1, 1)
		} else {
			drawRoundStroke(canvas, color, rect(cx-5, cy-5, 10, 10), 1, 1)
		}
	case "win-close":
		drawLine(canvas, color, cx-5, cy-5, cx+5, cy+5, 2)
		drawLine(canvas, color, cx+5, cy-5, cx-5, cy+5, 2)
	}
}

func (ui *dashboardUI) paintCalculator(canvas *walk.Canvas, bounds walk.Rectangle) {
	left := dashStyle.sidebarWidth + dashStyle.pagePad
	right := bounds.Width - dashStyle.pagePad
	contentW := maxInt(0, right-left)
	top := 30

	drawText(canvas, "平衡买入计算", ui.fontTitle, dashColors.text, rect(left, top, 220, 34), walk.TextLeft|walk.TextVCenter)

	cardGap := dashStyle.gap
	mainY := 84
	mainH := maxInt(0, bounds.Height-mainY-dashStyle.pagePad)
	minTopH := 236
	maxTopH := maxInt(minTopH, mainH-cardGap-240)
	defaultResultH := bounds.Height / 2
	defaultTopH := clampInt(mainH-cardGap-defaultResultH, minTopH, maxTopH)
	if ui.assetTableHeight <= 0 {
		ui.assetTableHeight = defaultTopH
	}
	ui.assetTableHeight = clampInt(ui.assetTableHeight, minTopH, maxTopH)
	topH := ui.assetTableHeight
	resultH := maxInt(220, mainH-topH-cardGap)

	sideW := maxInt(0, (contentW-cardGap)/4)
	overview := rect(left, mainY, sideW, topH)
	table := rect(overview.X+overview.Width+cardGap, mainY, contentW-sideW-cardGap, topH)

	ui.drawPortfolioPanel(canvas, overview)
	ui.drawPanel(canvas, table, "资产条目", "")
	actionX := table.X + table.Width - 238
	ui.drawButton(canvas, rect(actionX, table.Y+10, 104, dashStyle.buttonHeight), "添加资产", "calc-add", false)
	ui.drawButton(canvas, rect(actionX+116, table.Y+10, 104, dashStyle.buttonHeight), "删除选中", "calc-delete", false)
	ui.drawAssetTable(canvas, table)

	resultY := mainY + topH + cardGap
	ui.drawSplitHandle(canvas, rect(left, resultY-cardGap/2-3, contentW, 6), "split-table")

	result := rect(left, resultY, contentW, resultH)
	ui.drawPanel(canvas, result, "再平衡建议", "")
	buttonW := 104
	ui.drawButton(canvas, rect(result.X+result.Width-buttonW*2-30, result.Y+10, buttonW, dashStyle.buttonHeight), "计算建议", "calc-run", true)
	ui.drawButton(canvas, rect(result.X+result.Width-buttonW-18, result.Y+10, buttonW, dashStyle.buttonHeight), "保存归档", "calc-save", false)
	ui.drawResultText(canvas, inset(result, 18, 62))
}

func (ui *dashboardUI) drawAssetTable(canvas *walk.Canvas, panel walk.Rectangle) {
	x := panel.X + dashStyle.cardPad
	y := panel.Y + 60
	w := panel.Width - dashStyle.cardPad*2
	headers := []string{"#", "资产名称", "目标仓位", "当前持有金额", "当前仓位"}
	widths := []int{44, maxInt(170, w-44-120-160-130), 120, 160, 130}
	drawTableHeader(canvas, ui.fontTiny, x, y, widths, headers)
	rowY := y + 32
	rowH := 40
	visible := assetTableVisibleRows(panel.Height)
	ui.clampAssetScroll(visible)
	for i := 0; i < visible; i++ {
		index := ui.assetScroll + i
		if index >= len(ui.assets) {
			break
		}
		item := ui.assets[index]
		rowRect := rect(x, rowY+i*rowH, w, rowH-6)
		bg := dashColors.panel2
		if index == ui.selectedAsset {
			bg = walk.RGB(49, 34, 12)
		} else if i%2 == 1 {
			bg = walk.RGB(21, 21, 21)
		}
		roundFill(canvas, bg, rowRect, 9)
		ui.actions = append(ui.actions, dashAction{Rect: rowRect, Kind: dashActionSelect, Key: "asset-row", Index: index})

		cx := x
		drawText(canvas, strconv.Itoa(index+1), ui.fontSmall, dashColors.muted, rect(cx, rowRect.Y, widths[0], rowRect.Height), walk.TextCenter|walk.TextVCenter)
		cx += widths[0]
		ui.drawField(canvas, rect(cx+4, rowRect.Y+5, widths[1]-8, 26), "asset-name", index, item.Name, "资产名称", false, "")
		cx += widths[1]
		targetField := rect(cx+4, rowRect.Y+5, widths[2]-30, 26)
		ui.drawField(canvas, targetField, "asset-target", index, formatMaybeFloat(item.TargetPct), "0", true, "")
		drawText(canvas, "%", ui.fontSmall, dashColors.muted, rect(targetField.X+targetField.Width+6, rowRect.Y, 18, rowRect.Height), walk.TextLeft|walk.TextVCenter)
		cx += widths[2]
		amountField := rect(cx+4, rowRect.Y+5, widths[3]-30, 26)
		ui.drawField(canvas, amountField, "asset-current", index, formatMaybeFloat(item.CurrentAmount), "0", true, "")
		drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(amountField.X+amountField.Width+6, rowRect.Y, 18, rowRect.Height), walk.TextLeft|walk.TextVCenter)
		cx += widths[3]
		pct := "0%"
		if !isBlankAsset(item) {
			pct = formatPercent(currentPctForInputs(ui.assets, index))
		}
		drawText(canvas, pct, ui.fontSmall, dashColors.accent, rect(cx+8, rowRect.Y, widths[4]-16, rowRect.Height), walk.TextLeft|walk.TextVCenter)
	}
	if len(ui.assets) == 0 {
		drawText(canvas, "暂无资产，点击右上角‘添加资产’开始。", ui.fontBody, dashColors.faint, rect(x, rowY+44, w, 40), walk.TextCenter|walk.TextVCenter)
	}
	ui.drawScrollBar(canvas, "scroll-assets", rect(panel.X+panel.Width-12, rowY, 6, maxInt(0, panel.Y+panel.Height-rowY-16)), len(ui.assets), visible, ui.assetScroll)
}

func assetTableVisibleRows(panelHeight int) int {
	const assetTableHeaderOffset = 60 + 32 + 16
	const assetRowHeight = 40
	return maxInt(1, maxInt(0, panelHeight-assetTableHeaderOffset)/assetRowHeight)
}

func (ui *dashboardUI) drawPortfolioPanel(canvas *walk.Canvas, r walk.Rectangle) {
	ui.drawPanel(canvas, r, "组合概览", "")
	x := r.X + dashStyle.cardPad
	y := r.Y + 58
	total := currentAmountSum(ui.assets)
	targetSum := targetPctSum(ui.assets)
	canCalculate := calculationReady(ui.assets)

	drawText(canvas, "本次投入", ui.fontSmall, dashColors.muted, rect(x, y, r.Width-dashStyle.cardPad*2, 18), walk.TextLeft|walk.TextVCenter)
	investField := rect(x, y+24, r.Width-dashStyle.cardPad*2-30, 38)
	ui.drawField(canvas, investField, "invest", -1, formatPlainMoney(ui.investAmount), "0", true, "")
	drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(investField.X+investField.Width+8, investField.Y, 22, investField.Height), walk.TextLeft|walk.TextVCenter)

	rowY := y + 74
	innerW := r.Width - dashStyle.cardPad*2
	colGap := 12
	colW := maxInt(0, (innerW-colGap)/2)
	ui.drawOverviewRow(canvas, x, rowY, colW, "当前资产总额", formatMoney(total)+" 元", dashColors.text)
	ui.drawOverviewRow(canvas, x+colW+colGap, rowY, colW, "目标仓位合计", formatPercent(targetSum), targetStatusColor(targetSum))
	ui.drawOverviewRow(canvas, x, rowY+48, colW, "资产数量", fmt.Sprintf("%d 项", len(ui.assets)), dashColors.text)
	conditionColor := dashColors.warning
	conditionText := "未满足"
	if canCalculate {
		conditionColor = dashColors.accent
		conditionText = "满足"
	}
	ui.drawOverviewRow(canvas, x+colW+colGap, rowY+48, colW, "计算条件", conditionText, conditionColor)
}

func (ui *dashboardUI) drawOverviewRow(canvas *walk.Canvas, x, y, w int, label, value string, color walk.Color) {
	drawText(canvas, label, ui.fontSmall, dashColors.muted, rect(x, y, w, 16), walk.TextLeft|walk.TextVCenter)
	drawText(canvas, value, ui.fontBold, color, rect(x, y+18, w, 20), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
}

func targetStatusColor(total float64) walk.Color {
	if math.Abs(total-100) <= 0.01 {
		return dashColors.accent
	}
	if total > 100.01 {
		return dashColors.danger
	}
	return dashColors.warning
}

func calculationReady(assets []AssetInput) bool {
	return len(assets) >= 2 && math.Abs(targetPctSum(assets)-100) <= 0.01
}

func (ui *dashboardUI) drawResultText(canvas *walk.Canvas, r walk.Rectangle) {
	lines := resultLines(ui.resultText)
	lineH := 21
	visible := maxInt(1, r.Height/lineH)
	ui.clampResultScroll(visible, len(lines))
	y := r.Y
	for i := 0; i < visible; i++ {
		index := ui.resultScroll + i
		if index >= len(lines) {
			break
		}
		line := lines[index]
		color := dashColors.text
		font := ui.fontMono
		if resultHeadingLine(line) {
			color = dashColors.accent
			font = ui.fontBold
		} else if strings.Contains(line, "严重") {
			color = dashColors.warning
		}
		drawText(canvas, line, font, color, rect(r.X, y, r.Width, 20), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		y += lineH
	}
	ui.drawScrollBar(canvas, "scroll-result", rect(r.X+r.Width+8, r.Y, 6, maxInt(0, r.Height-4)), len(lines), visible, ui.resultScroll)
}

func resultLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func resultHeadingLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	switch trimmed {
	case "建议买入", "严重偏离提醒":
		return true
	default:
		return false
	}
}

func (ui *dashboardUI) paintHistory(canvas *walk.Canvas, bounds walk.Rectangle) {
	left := dashStyle.sidebarWidth + dashStyle.pagePad
	top := 88
	gap := dashStyle.gap
	contentW := bounds.Width - left - dashStyle.pagePad
	listW := maxInt(0, (contentW-gap)/4)
	detailX := left + listW + gap
	detailW := contentW - listW - gap
	height := bounds.Height - top - dashStyle.pagePad

	drawText(canvas, "历史投资记录", ui.fontTitle, dashColors.text, rect(left, 30, 220, 34), walk.TextLeft|walk.TextVCenter)

	listPanel := rect(left, top, listW, height)
	ui.drawPanel(canvas, listPanel, "记录列表", "")
	ui.drawHistoryList(canvas, listPanel)

	detail := rect(detailX, top, detailW, height)
	ui.drawPanel(canvas, detail, "投资记录详情", "")
	ui.drawHistoryDetail(canvas, detail)
}

func (ui *dashboardUI) drawHistoryList(canvas *walk.Canvas, panel walk.Rectangle) {
	x := panel.X + 14
	y := panel.Y + 54
	w := panel.Width - 28
	rowH := 62
	bottomControlsH := dashStyle.buttonHeight + 32
	visible := maxInt(1, (panel.Y+panel.Height-y-bottomControlsH)/rowH)
	ui.clampHistoryScroll(visible)
	for i := 0; i < visible; i++ {
		index := ui.historyScroll + i
		if index >= len(investmentRecords) {
			break
		}
		record := investmentRecords[index]
		r := rect(x, y+i*rowH, w, rowH-8)
		bg := dashColors.panel2
		if index == selectedHistoryIndex {
			bg = walk.RGB(49, 34, 12)
		}
		roundFill(canvas, bg, r, 8)
		drawRoundStroke(canvas, dashColors.line, r, 8, 1)
		drawText(canvas, record.ArchivedAt, ui.fontSmall, dashColors.text, rect(r.X+12, r.Y+8, r.Width-24, 18), walk.TextLeft|walk.TextVCenter)
		drawText(canvas, "买入金额 "+formatMoney(record.InvestAmount)+" 元", ui.fontTiny, dashColors.accent, rect(r.X+12, r.Y+29, r.Width-24, 16), walk.TextLeft|walk.TextVCenter)
		ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionSelect, Key: "history-row", Index: index})
	}
	if len(investmentRecords) == 0 {
		drawText(canvas, "暂无历史记录。计算页保存后会出现在这里。", ui.fontBody, dashColors.muted, rect(x, y+36, w, 60), walk.TextCenter|walk.TextVCenter|walk.TextWordbreak)
	}
	ui.drawScrollBar(canvas, "scroll-history-list", rect(panel.X+panel.Width-12, y, 6, maxInt(0, panel.Y+panel.Height-y-bottomControlsH)), len(investmentRecords), visible, ui.historyScroll)
	buttonY := panel.Y + panel.Height - dashStyle.buttonHeight - 16
	buttonW := (w - 12) / 2
	ui.drawButton(canvas, rect(x, buttonY, buttonW, dashStyle.buttonHeight), "导入", "hist-import", false)
	ui.drawButton(canvas, rect(x+buttonW+12, buttonY, buttonW, dashStyle.buttonHeight), "导出", "hist-export", false)
}

func (ui *dashboardUI) drawHistoryDetail(canvas *walk.Canvas, panel walk.Rectangle) {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		drawText(canvas, "请选择左侧记录", ui.fontBig, dashColors.muted, inset(panel, 18, 74), walk.TextCenter|walk.TextVCenter)
		return
	}
	recalculateInvestmentRecord(&selectedHistoryDraft)
	x := panel.X + 18
	y := panel.Y + 58
	w := panel.Width - 36
	ui.drawField(canvas, rect(x, y, 220, 38), "hist-archive", -1, selectedHistoryDraft.ArchivedAt, "记录时间", false, "")
	historyInvestField := rect(x+236, y, 128, 38)
	ui.drawField(canvas, historyInvestField, "hist-invest", -1, formatMaybeFloat(selectedHistoryDraft.InvestAmount), "当次投入", true, "")
	drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(historyInvestField.X+historyInvestField.Width+8, historyInvestField.Y, 22, historyInvestField.Height), walk.TextLeft|walk.TextVCenter)
	ui.drawField(canvas, rect(x+408, y, w-408, 38), "hist-notes", -1, selectedHistoryDraft.Notes, "备注", false, "")

	summaryY := y + 52
	summary := fmt.Sprintf("买入前 %s 元  |  投入 %s 元  |  买入后 %s 元  |  未分配 %s 元",
		formatMoney(selectedHistoryDraft.CurrentTotal),
		formatMoney(selectedHistoryDraft.InvestAmount),
		formatMoney(selectedHistoryDraft.AfterTotal),
		formatMoney(selectedHistoryDraft.RemainingCash))
	drawText(canvas, summary, ui.fontBold, dashColors.accent, rect(x, summaryY, w, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	tableY := summaryY + 38
	tableBottom := panel.Y + panel.Height - dashStyle.buttonHeight - 34
	table := rect(x, tableY, w, maxInt(190, tableBottom-tableY))
	ui.drawHistoryAssetTable(canvas, table)

	ui.drawButton(canvas, rect(panel.X+panel.Width-270, panel.Y+panel.Height-58, 118, dashStyle.buttonHeight), "读取记录", "hist-load-to-calc", false)
	ui.drawDangerButton(canvas, rect(panel.X+panel.Width-140, panel.Y+panel.Height-58, 118, dashStyle.buttonHeight), "删除记录", "hist-delete")
}

func (ui *dashboardUI) drawHistoryAssetTable(canvas *walk.Canvas, table walk.Rectangle) {
	headers := []string{"资产", "目标", "买入前", "买入", "买入后", "状态"}
	widths := []int{maxInt(140, table.Width-86-132-112-132-130), 86, 132, 112, 132, 130}
	drawTableHeader(canvas, ui.fontTiny, table.X, table.Y, widths, headers)
	rowY := table.Y + 28
	visible := maxInt(1, (table.Y+table.Height-rowY)/34)
	ui.clampHistoryAssetScroll(visible)
	for i := 0; i < visible; i++ {
		index := ui.historyAssetScroll + i
		if index >= len(selectedHistoryDraft.Assets) {
			break
		}
		asset := selectedHistoryDraft.Assets[index]
		r := rect(table.X, rowY+i*34, table.Width, 30)
		bg := dashColors.panel2
		if index == selectedAssetIndex {
			bg = walk.RGB(49, 34, 12)
		} else if i%2 == 1 {
			bg = walk.RGB(21, 21, 21)
		}
		roundFill(canvas, bg, r, 7)
		ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionSelect, Key: "hasset-row", Index: index})
		cx := table.X
		ui.drawField(canvas, rect(cx+4, r.Y+3, widths[0]-8, 24), "hasset-name", index, asset.Name, "资产名称", false, "")
		cx += widths[0]
		targetField := rect(cx+4, r.Y+3, widths[1]-28, 24)
		ui.drawField(canvas, targetField, "hasset-target", index, formatMaybeFloat(asset.TargetPct), "0", true, "")
		drawText(canvas, "%", ui.fontSmall, dashColors.muted, rect(targetField.X+targetField.Width+6, r.Y, 18, r.Height), walk.TextLeft|walk.TextVCenter)
		cx += widths[1]
		ui.drawField(canvas, rect(cx+4, r.Y+3, widths[2]-8, 24), "hasset-before", index, formatMaybeFloat(asset.BeforeAmount), "0", true, "")
		cx += widths[2]
		ui.drawField(canvas, rect(cx+4, r.Y+3, widths[3]-8, 24), "hasset-buy", index, formatMaybeFloat(asset.BuyAmount), "0", true, "")
		cx += widths[3]
		drawText(canvas, formatMoney(asset.AfterAmount), ui.fontSmall, dashColors.text, rect(cx+8, r.Y, widths[4]-16, r.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[4]
		drawText(canvas, asset.Status, ui.fontSmall, statusTextColor(asset.Status), rect(cx+8, r.Y, widths[5]-16, r.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	}
	ui.drawScrollBar(canvas, "scroll-history-detail", rect(table.X+table.Width+8, rowY, 6, maxInt(0, table.Y+table.Height-rowY)), len(selectedHistoryDraft.Assets), visible, ui.historyAssetScroll)
}

func (ui *dashboardUI) paintTrend(canvas *walk.Canvas, bounds walk.Rectangle) {
	left := dashStyle.sidebarWidth + dashStyle.pagePad
	top := 88
	gap := dashStyle.gap
	contentW := bounds.Width - left - dashStyle.pagePad
	optionsW := maxInt(0, (contentW-gap)/4)
	chartX := left + optionsW + gap
	chartW := contentW - optionsW - gap
	height := bounds.Height - top - dashStyle.pagePad

	drawText(canvas, "资产趋势图表", ui.fontTitle, dashColors.text, rect(left, 30, 240, 34), walk.TextLeft|walk.TextVCenter)

	options := rect(left, top, optionsW, height)
	ui.drawPanel(canvas, options, "资产选择", "")
	ui.drawTrendOptions(canvas, options)

	chartPanel := rect(chartX, top, chartW, height)
	ui.drawPanel(canvas, chartPanel, "资产金额变化趋势", "")
	ui.drawField(canvas, rect(chartPanel.X+18, chartPanel.Y+58, 118, 38), "trend-start", -1, ui.trendStartText, "YYYY-MM", false, "")
	ui.drawField(canvas, rect(chartPanel.X+150, chartPanel.Y+58, 118, 38), "trend-end", -1, ui.trendEndText, "YYYY-MM", false, "")
	ui.drawButton(canvas, rect(chartPanel.X+284, chartPanel.Y+58, 118, 38), "最近一年", "trend-year", false)

	chart := rect(chartPanel.X+18, chartPanel.Y+116, chartPanel.Width-36, chartPanel.Height-138)
	ui.drawTrendChart(canvas, chart)
}

func (ui *dashboardUI) drawTrendOptions(canvas *walk.Canvas, panel walk.Rectangle) {
	options := trendSeriesOptions(investmentRecords)
	syncTrendSelections(options)
	x := panel.X + 16
	y := panel.Y + 54
	w := panel.Width - 32
	visible := maxInt(1, (panel.Y+panel.Height-y-18)/48)
	ui.clampTrendOptionScroll(visible, len(options))
	for i := 0; i < visible; i++ {
		index := ui.trendOptionScroll + i
		if index >= len(options) {
			break
		}
		name := options[index]
		r := rect(x, y+i*48, w, 40)
		roundFill(canvas, dashColors.panel2, r, 9)
		drawRoundStroke(canvas, dashColors.line, r, 9, 1)
		color := trendColorForIndex(index)
		roundFill(canvas, color, rect(r.X+12, r.Y+14, 12, 12), 6)
		box := rect(r.X+r.Width-34, r.Y+10, 20, 20)
		drawRoundStroke(canvas, dashColors.line2, box, 5, 1)
		if trendSelections[name] {
			roundFill(canvas, dashColors.accent, inset(box, 4, 4), 4)
		}
		drawText(canvas, name, ui.fontSmall, dashColors.text, rect(r.X+34, r.Y+8, r.Width-80, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionToggle, Key: "trend-toggle", Index: index, Name: name})
	}
	if len(options) == 0 {
		drawText(canvas, "暂无历史记录可生成趋势", ui.fontBody, dashColors.muted, rect(x, y+36, w, 60), walk.TextCenter|walk.TextVCenter)
	}
	ui.drawScrollBar(canvas, "scroll-trend-options", rect(panel.X+panel.Width-12, y, 6, maxInt(0, panel.Y+panel.Height-y-18)), len(options), visible, ui.trendOptionScroll)
}

func (ui *dashboardUI) drawTrendChart(canvas *walk.Canvas, r walk.Rectangle) {
	roundFill(canvas, walk.RGB(12, 14, 16), r, 10)
	drawRoundStroke(canvas, dashColors.line, r, 10, 1)
	start, end, err := ui.trendRange()
	if err != nil {
		drawText(canvas, err.Error(), ui.fontBody, dashColors.muted, r, walk.TextCenter|walk.TextVCenter)
		return
	}
	data := buildTrendChartData(investmentRecords, trendSelections, start, end)
	if len(data.Months) == 0 || len(data.Series) == 0 || data.Message != "" && !trendSeriesHasAnyPoint(data.Series) {
		message := data.Message
		if message == "" {
			message = "暂无历史记录可生成趋势图"
		}
		drawText(canvas, message, ui.fontBody, dashColors.muted, r, walk.TextCenter|walk.TextVCenter)
		return
	}
	legendY := r.Y + 14
	legendX := r.X + 18
	for _, series := range data.Series {
		if legendX+128 > r.X+r.Width-20 {
			legendX = r.X + 18
			legendY += 22
		}
		color := series.Color
		roundFill(canvas, color, rect(legendX, legendY+6, 18, 4), 2)
		drawText(canvas, series.Name, ui.fontTiny, dashColors.text, rect(legendX+24, legendY, 96, 16), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		legendX += 128
	}
	plot := rect(r.X+84, legendY+34, r.Width-110, r.Height-(legendY-r.Y)-82)
	if plot.Height < 120 || plot.Width < 180 {
		drawText(canvas, "窗口空间不足，无法绘制趋势图", ui.fontBody, dashColors.muted, r, walk.TextCenter|walk.TextVCenter)
		return
	}
	minValue, maxValue := trendValueRange(data.Series)
	drawTrendGridCustom(canvas, ui.fontTiny, plot, minValue, maxValue)
	drawTrendLinesCustom(canvas, plot, data, minValue, maxValue)
	drawTrendAxisCustom(canvas, ui.fontTiny, plot, data.Months)
}

func drawTrendGridCustom(canvas *walk.Canvas, font *walk.Font, plot walk.Rectangle, minValue, maxValue float64) {
	for i := 0; i <= 4; i++ {
		y := plot.Y + plot.Height - i*plot.Height/4
		drawLine(canvas, walk.RGB(36, 42, 46), plot.X, y, plot.X+plot.Width, y, 1)
		value := minValue + (maxValue-minValue)*float64(i)/4
		drawText(canvas, formatCompactMoney(value), font, dashColors.muted, rect(plot.X-78, y-10, 68, 18), walk.TextRight|walk.TextVCenter)
	}
	drawLine(canvas, walk.RGB(60, 70, 76), plot.X, plot.Y, plot.X, plot.Y+plot.Height, 1)
	drawLine(canvas, walk.RGB(60, 70, 76), plot.X, plot.Y+plot.Height, plot.X+plot.Width, plot.Y+plot.Height, 1)
}

func drawTrendLinesCustom(canvas *walk.Canvas, plot walk.Rectangle, data trendChartData, minValue, maxValue float64) {
	for _, series := range data.Series {
		pen, penBrush := chartPen(series.Color, 3)
		brush := solidBrush(series.Color)
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
			_ = canvas.FillEllipsePixels(brush, rect(current.X-4, current.Y-4, 8, 8))
			cp := current
			previous = &cp
		}
		brush.Dispose()
		pen.Dispose()
		penBrush.Dispose()
	}
}

func drawTrendAxisCustom(canvas *walk.Canvas, font *walk.Font, plot walk.Rectangle, months []time.Time) {
	if len(months) == 0 {
		return
	}
	step := maxInt(1, int(math.Ceil(float64(len(months))/8)))
	for i, month := range months {
		if i%step != 0 && i != len(months)-1 {
			continue
		}
		x := trendPointX(plot, i, len(months))
		drawLine(canvas, walk.RGB(60, 70, 76), x, plot.Y+plot.Height, x, plot.Y+plot.Height+5, 1)
		label := month.Format("06-01")
		if len(months) <= 12 {
			label = month.Format(trendMonthFmt)
		}
		drawText(canvas, label, font, dashColors.muted, rect(x-34, plot.Y+plot.Height+8, 68, 20), walk.TextCenter|walk.TextVCenter)
	}
}

func (ui *dashboardUI) drawPanel(canvas *walk.Canvas, r walk.Rectangle, title, sub string) {
	roundFill(canvas, dashColors.panel, r, dashStyle.panelRadius)
	drawRoundStroke(canvas, dashColors.line, r, dashStyle.panelRadius, 1)
	drawText(canvas, title, ui.fontBold, dashColors.text, rect(r.X+dashStyle.cardPad, r.Y+16, r.Width-dashStyle.cardPad*2, 24), walk.TextLeft|walk.TextVCenter)
	if sub != "" {
		drawText(canvas, sub, ui.fontTiny, dashColors.muted, rect(r.X+dashStyle.cardPad, r.Y+40, r.Width-dashStyle.cardPad*2, 16), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	}
}

func (ui *dashboardUI) drawButton(canvas *walk.Canvas, r walk.Rectangle, label, key string, primary bool) {
	bg := dashColors.card2
	fg := dashColors.text
	border := dashColors.line2
	if primary {
		bg = dashColors.accent
		fg = walk.RGB(8, 8, 8)
		border = dashColors.accent
	}
	if ui.hoverAction == key && !primary {
		bg = walk.RGB(44, 44, 44)
	} else if ui.hoverAction == key && primary {
		bg = dashColors.accentHover
		border = dashColors.accentHover
	}
	roundFill(canvas, bg, r, dashStyle.buttonRadius)
	drawRoundStroke(canvas, border, r, dashStyle.buttonRadius, 1)
	drawText(canvas, label, ui.fontBold, fg, r, walk.TextCenter|walk.TextVCenter|walk.TextEndEllipsis)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: key})
}

func (ui *dashboardUI) drawDangerButton(canvas *walk.Canvas, r walk.Rectangle, label, key string) {
	bg := dashColors.panel2
	if ui.hoverAction == key {
		bg = walk.RGB(43, 24, 24)
	}
	roundFill(canvas, bg, r, dashStyle.buttonRadius)
	drawRoundStroke(canvas, walk.RGB(94, 42, 42), r, dashStyle.buttonRadius, 1)
	drawText(canvas, label, ui.fontBold, dashColors.danger, r, walk.TextCenter|walk.TextVCenter|walk.TextEndEllipsis)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: key})
}

func (ui *dashboardUI) drawSplitHandle(canvas *walk.Canvas, r walk.Rectangle, key string) {
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionSplit, Key: key})
	if r.Height > r.Width {
		x := r.X + r.Width/2
		drawLine(canvas, dashColors.line, x, r.Y+12, x, r.Y+r.Height-12, 1)
	} else {
		y := r.Y + r.Height/2
		drawLine(canvas, dashColors.line, r.X+12, y, r.X+r.Width-12, y, 1)
	}
	if ui.hoverAction == key || ui.dragMode == key {
		roundFill(canvas, dashColors.line2, r, 3)
	}
}

func (ui *dashboardUI) drawField(canvas *walk.Canvas, r walk.Rectangle, key string, index int, value, placeholder string, numeric bool, suffix string) {
	roundFill(canvas, fieldFillColor(), r, 8)
	border := dashColors.line
	if ui.activeField != nil && sameField(*ui.activeField, dashField{Key: key, Index: index}) {
		border = dashColors.line2
	}
	drawRoundStroke(canvas, border, r, 8, 1)
	display := strings.TrimSpace(value)
	color := dashColors.text
	if display == "" {
		display = placeholder
		color = dashColors.faint
	}
	if suffix != "" && strings.TrimSpace(value) != "" {
		display += " " + suffix
	}
	format := walk.TextLeft | walk.TextVCenter | walk.TextEndEllipsis
	drawText(canvas, display, ui.fontSmall, color, fieldTextRect(r), format)
	ui.fields = append(ui.fields, dashField{Rect: r, Key: key, Index: index, Value: value, Placeholder: placeholder, Numeric: numeric, Suffix: suffix})
}

func fieldTextRect(r walk.Rectangle) walk.Rectangle {
	return rect(r.X+8, r.Y, maxInt(0, r.Width-16), r.Height)
}

func fieldFillColor() walk.Color {
	return walk.RGB(22, 22, 22)
}

func drawTableHeader(canvas *walk.Canvas, font *walk.Font, x, y int, widths []int, headers []string) {
	cx := x
	for i, header := range headers {
		w := widths[i]
		drawText(canvas, header, font, dashColors.muted, rect(cx+8, y, w-16, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += w
	}
	drawLine(canvas, dashColors.line, x, y+28, x+sumInts(widths), y+28, 1)
}

func (ui *dashboardUI) drawScrollBar(canvas *walk.Canvas, key string, track walk.Rectangle, total, visible, offset int) {
	if total <= visible || visible <= 0 || track.Width <= 0 || track.Height <= 0 {
		return
	}
	maxOffset := maxInt(0, total-visible)
	offset = clampInt(offset, 0, maxOffset)
	thumbH := int(math.Round(float64(track.Height) * float64(visible) / float64(total)))
	thumbH = clampInt(thumbH, minInt(24, track.Height), track.Height)
	span := maxInt(1, track.Height-thumbH)
	thumbY := track.Y
	if maxOffset > 0 {
		thumbY = track.Y + int(math.Round(float64(span)*float64(offset)/float64(maxOffset)))
	}
	thumb := rect(track.X, thumbY, track.Width, thumbH)
	active := ui.hoverAction == key || ui.dragMode == scrollDragMode(key)
	trackColor := walk.RGB(32, 32, 32)
	thumbColor := walk.RGB(112, 112, 112)
	if active {
		trackColor = walk.RGB(42, 42, 42)
		thumbColor = walk.RGB(168, 168, 168)
	}
	roundFill(canvas, trackColor, track, maxInt(2, track.Width/2))
	roundFill(canvas, thumbColor, thumb, maxInt(2, thumb.Width/2))
	ui.scrollbars = append(ui.scrollbars, dashScrollbar{
		Key:     key,
		Track:   track,
		Thumb:   thumb,
		Total:   total,
		Visible: visible,
		Offset:  offset,
	})
	ui.actions = append(ui.actions, dashAction{Rect: track, Kind: dashActionScroll, Key: key})
}

func (ui *dashboardUI) handleMouseDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	if ui.titleDoubleClick(x, y) {
		ui.commitEditor()
		ui.toggleMaximize()
		return
	}
	if mode := resizeModeForPoint(ui.canvas.ClientBoundsPixels(), x, y); mode != "" {
		ui.commitEditor()
		ui.beginDrag(mode)
		return
	}
	for _, action := range ui.actions {
		if action.Kind == dashActionSplit && contains(action.Rect, x, y) {
			ui.commitEditor()
			ui.beginDrag(action.Key)
			return
		}
	}
	if bar := ui.scrollBarAt(x, y); bar != nil {
		ui.commitEditor()
		ui.beginScrollDrag(*bar, x, y)
		return
	}
	for _, field := range ui.fields {
		if contains(field.Rect, x, y) {
			if ui.activeField != nil && !sameField(*ui.activeField, field) {
				ui.commitEditor()
			}
			ui.beginEdit(field)
			return
		}
	}
	ui.commitEditor()
	for _, action := range ui.actions {
		if contains(action.Rect, x, y) {
			ui.handleAction(action)
			return
		}
	}
	if y < 64 && x > dashStyle.sidebarWidth {
		win.ReleaseCapture()
		win.SendMessage(ui.mw.Handle(), win.WM_NCLBUTTONDOWN, uintptr(win.HTCAPTION), 0)
	}
}

func sameField(a, b dashField) bool {
	return a.Key == b.Key && a.Index == b.Index
}

func (ui *dashboardUI) titleDoubleClick(x, y int) bool {
	if y >= 64 || x <= dashStyle.sidebarWidth || ui.actionAt(x, y) != nil {
		return false
	}
	now := time.Now()
	double := !ui.lastTitleClickAt.IsZero() &&
		now.Sub(ui.lastTitleClickAt) <= 450*time.Millisecond &&
		absInt(x-ui.lastTitleClickX) <= 8 &&
		absInt(y-ui.lastTitleClickY) <= 8
	if double {
		ui.lastTitleClickAt = time.Time{}
		return true
	}
	ui.lastTitleClickAt = now
	ui.lastTitleClickX = x
	ui.lastTitleClickY = y
	return false
}

func (ui *dashboardUI) handleMouseMove(x, y int, _ walk.MouseButton) {
	if ui.dragMode != "" {
		ui.updateDrag()
		return
	}
	hover := ""
	if bar := ui.scrollBarAt(x, y); bar != nil {
		hover = bar.Key
	} else {
		for _, action := range ui.actions {
			if contains(action.Rect, x, y) {
				hover = action.Key
				break
			}
		}
	}
	ui.updateCursor(x, y, hover)
	if hover != ui.hoverAction {
		ui.hoverAction = hover
		ui.invalidate()
	}
}

func (ui *dashboardUI) handleMouseUp(_, _ int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	if ui.dragMode != "" {
		ui.dragMode = ""
		win.ReleaseCapture()
		ui.setCursorMode("")
		ui.invalidate()
	}
}

func (ui *dashboardUI) actionAt(x, y int) *dashAction {
	for i := range ui.actions {
		if contains(ui.actions[i].Rect, x, y) {
			return &ui.actions[i]
		}
	}
	return nil
}

func (ui *dashboardUI) scrollBarAt(x, y int) *dashScrollbar {
	for i := len(ui.scrollbars) - 1; i >= 0; i-- {
		if contains(ui.scrollbars[i].Track, x, y) {
			return &ui.scrollbars[i]
		}
	}
	return nil
}

func (ui *dashboardUI) updateCursor(x, y int, hover string) {
	if mode := resizeModeForPoint(ui.canvas.ClientBoundsPixels(), x, y); mode != "" {
		ui.setCursorMode(cursorModeForDrag(mode))
		return
	}
	if hover == "split-table" {
		ui.setCursorMode("size-ns")
		return
	}
	if strings.HasPrefix(hover, "scroll-") {
		ui.setCursorMode("size-ns")
		return
	}
	if hover != "" {
		ui.setCursorMode("hand")
		return
	}
	ui.setCursorMode("")
}

func (ui *dashboardUI) setCursorMode(mode string) {
	if mode == ui.cursorMode || ui.canvas == nil {
		return
	}
	ui.cursorMode = mode
	switch mode {
	case "size-we":
		ui.canvas.SetCursor(walk.CursorSizeWE())
	case "size-ns":
		ui.canvas.SetCursor(walk.CursorSizeNS())
	case "size-nwse":
		ui.canvas.SetCursor(walk.CursorSizeNWSE())
	case "size-nesw":
		ui.canvas.SetCursor(walk.CursorSizeNESW())
	case "hand":
		ui.canvas.SetCursor(walk.CursorHand())
	default:
		ui.canvas.SetCursor(walk.CursorArrow())
	}
}

func cursorModeForDrag(mode string) string {
	switch mode {
	case "resize-left", "resize-right":
		return "size-we"
	case "split-table", "resize-top", "resize-bottom":
		return "size-ns"
	case "resize-left-top", "resize-right-bottom":
		return "size-nwse"
	case "resize-right-top", "resize-left-bottom":
		return "size-nesw"
	default:
		return ""
	}
}

func (ui *dashboardUI) beginDrag(mode string) {
	x, y, ok := cursorScreenPoint()
	if !ok {
		return
	}
	ui.dragMode = mode
	ui.dragStartX = x
	ui.dragStartY = y
	ui.dragStartBounds = ui.mw.BoundsPixels()
	ui.dragStartTable = ui.assetTableHeight
	ui.setCursorMode(cursorModeForDrag(mode))
	win.SetCapture(ui.canvas.Handle())
}

func (ui *dashboardUI) beginScrollDrag(bar dashScrollbar, x, y int) {
	_, screenY, ok := cursorScreenPoint()
	if !ok {
		return
	}
	offset := bar.Offset
	if !contains(bar.Thumb, x, y) {
		offset = scrollOffsetForY(bar, y)
		ui.setScrollOffset(bar.Key, offset, bar.Total, bar.Visible)
	}
	ui.dragMode = scrollDragMode(bar.Key)
	ui.dragStartY = screenY
	ui.dragStartScroll = offset
	ui.dragScrollTotal = bar.Total
	ui.dragScrollVisible = bar.Visible
	ui.dragScrollTrackH = bar.Track.Height
	ui.dragScrollThumbH = bar.Thumb.Height
	ui.setCursorMode("size-ns")
	win.SetCapture(ui.canvas.Handle())
	ui.invalidate()
}

func scrollDragMode(key string) string {
	return "scroll:" + key
}

func scrollOffsetForY(bar dashScrollbar, y int) int {
	maxOffset := maxInt(0, bar.Total-bar.Visible)
	span := maxInt(1, bar.Track.Height-bar.Thumb.Height)
	rel := clampInt(y-bar.Track.Y-bar.Thumb.Height/2, 0, span)
	return int(math.Round(float64(rel) * float64(maxOffset) / float64(span)))
}

func (ui *dashboardUI) updateDrag() {
	x, y, ok := cursorScreenPoint()
	if !ok {
		return
	}
	dx := x - ui.dragStartX
	dy := y - ui.dragStartY
	if strings.HasPrefix(ui.dragMode, "scroll:") {
		maxOffset := maxInt(0, ui.dragScrollTotal-ui.dragScrollVisible)
		span := maxInt(1, ui.dragScrollTrackH-ui.dragScrollThumbH)
		next := ui.dragStartScroll + int(math.Round(float64(dy)*float64(maxOffset)/float64(span)))
		ui.setScrollOffset(strings.TrimPrefix(ui.dragMode, "scroll:"), next, ui.dragScrollTotal, ui.dragScrollVisible)
		ui.invalidate()
		return
	}
	switch ui.dragMode {
	case "split-table":
		bounds := ui.canvas.ClientBoundsPixels()
		mainY := 84
		mainH := maxInt(0, bounds.Height-mainY-dashStyle.pagePad)
		minTableH := 236
		maxTableH := maxInt(minTableH, mainH-dashStyle.gap-240)
		ui.assetTableHeight = clampInt(ui.dragStartTable+dy, minTableH, maxTableH)
		ui.invalidate()
	default:
		ui.resizeWindow(dx, dy)
	}
}

func (ui *dashboardUI) resizeWindow(dx, dy int) {
	b := ui.dragStartBounds
	next := b
	if strings.Contains(ui.dragMode, "right") {
		next.Width = maxInt(dashStyle.minWidth, b.Width+dx)
	}
	if strings.Contains(ui.dragMode, "bottom") {
		next.Height = maxInt(dashStyle.minHeight, b.Height+dy)
	}
	if strings.Contains(ui.dragMode, "left") {
		next.Width = maxInt(dashStyle.minWidth, b.Width-dx)
		next.X = b.X + b.Width - next.Width
	}
	if strings.Contains(ui.dragMode, "top") {
		next.Height = maxInt(dashStyle.minHeight, b.Height-dy)
		next.Y = b.Y + b.Height - next.Height
	}
	_ = ui.mw.SetBoundsPixels(next)
	ui.invalidate()
}

func resizeModeForPoint(bounds walk.Rectangle, x, y int) string {
	edge := dashStyle.resizeEdge
	left := x < edge
	right := x >= bounds.Width-edge
	top := y < edge
	bottom := y >= bounds.Height-edge
	switch {
	case left && top:
		return "resize-left-top"
	case right && top:
		return "resize-right-top"
	case left && bottom:
		return "resize-left-bottom"
	case right && bottom:
		return "resize-right-bottom"
	case left:
		return "resize-left"
	case right:
		return "resize-right"
	case top:
		return "resize-top"
	case bottom:
		return "resize-bottom"
	default:
		return ""
	}
}

func cursorScreenPoint() (int, int, bool) {
	var point win.POINT
	if !win.GetCursorPos(&point) {
		return 0, 0, false
	}
	return int(point.X), int(point.Y), true
}

func (ui *dashboardUI) wheelClientPoint(screenX, screenY int) (int, int) {
	if ui.canvas == nil {
		return screenX, screenY
	}
	point := win.POINT{X: int32(screenX), Y: int32(screenY)}
	if !win.ScreenToClient(ui.canvas.Handle(), &point) {
		return screenX, screenY
	}
	return int(point.X), int(point.Y)
}

func (ui *dashboardUI) handleMouseWheel(x, y int, button walk.MouseButton) {
	x, y = ui.wheelClientPoint(x, y)
	delta := walk.MouseWheelEventDelta(button)
	step := 1
	if delta < 0 {
		step = 1
	} else {
		step = -1
	}
	switch ui.page {
	case dashPageCalc:
		bounds := ui.canvas.ClientBoundsPixels()
		mainY := 84
		mainH := maxInt(0, bounds.Height-mainY-dashStyle.pagePad)
		minTopH := 236
		topH := minTopH
		if ui.assetTableHeight > 0 {
			maxTopH := maxInt(minTopH, mainH-dashStyle.gap-240)
			topH = clampInt(ui.assetTableHeight, minTopH, maxTopH)
		}
		resultY := mainY + topH + dashStyle.gap
		if y >= resultY {
			resultH := maxInt(220, mainH-topH-dashStyle.gap)
			visible := maxInt(1, maxInt(0, resultH-124)/21)
			ui.resultScroll += step
			ui.clampResultScroll(visible, len(resultLines(ui.resultText)))
		} else {
			ui.assetScroll += step
			ui.clampAssetScroll(assetTableVisibleRows(topH))
		}
	case dashPageHistory:
		bounds := ui.canvas.ClientBoundsPixels()
		left := dashStyle.sidebarWidth + dashStyle.pagePad
		contentW := bounds.Width - left - dashStyle.pagePad
		historyBoundary := left + maxInt(0, (contentW-dashStyle.gap)/4) + dashStyle.gap/2
		if x < historyBoundary {
			ui.historyScroll += step
			ui.clampHistoryScroll(7)
		} else {
			ui.historyAssetScroll += step
			ui.clampHistoryAssetScroll(7)
		}
	case dashPageTrend:
		bounds := ui.canvas.ClientBoundsPixels()
		left := dashStyle.sidebarWidth + dashStyle.pagePad
		contentW := bounds.Width - left - dashStyle.pagePad
		trendBoundary := left + maxInt(0, (contentW-dashStyle.gap)/4) + dashStyle.gap/2
		if x < trendBoundary {
			ui.trendOptionScroll += step
			ui.clampTrendOptionScroll(7, len(trendSeriesOptions(investmentRecords)))
		}
	}
	ui.invalidate()
}

func (ui *dashboardUI) handleAction(action dashAction) {
	switch action.Key {
	case "win-close":
		_ = ui.mw.Close()
	case "win-min":
		win.ShowWindow(ui.mw.Handle(), win.SW_MINIMIZE)
	case "win-max":
		ui.toggleMaximize()
	case "nav-calc":
		ui.page = dashPageCalc
	case "nav-history":
		ui.page = dashPageHistory
		ui.ensureHistorySelection()
	case "nav-trend":
		ui.page = dashPageTrend
		ui.ensureTrendRange()
	case "calc-add":
		ui.assets = append(ui.assets, AssetInput{})
		ui.selectedAsset = len(ui.assets) - 1
		if ui.autoSavePortfolioConfig() {
			ui.statusText = "已添加资产，请在表格中填写名称、目标仓位和当前金额"
		}
	case "asset-row":
		ui.selectedAsset = action.Index
	case "calc-delete":
		ui.deleteSelectedAsset()
	case "calc-run":
		ui.calculate()
	case "calc-save":
		ui.saveCurrentInvestment()
	case "history-row":
		ui.selectHistory(action.Index)
	case "hasset-row":
		selectedAssetIndex = action.Index
	case "hist-load-to-calc":
		ui.loadHistoryToCalculator()
	case "hist-delete":
		ui.deleteHistory()
	case "hist-import":
		ui.importHistory()
	case "hist-export":
		ui.exportHistory()
	case "trend-toggle":
		trendSelections[action.Name] = !trendSelections[action.Name]
	case "trend-year":
		ui.setTrendRecentYear()
	}
	ui.invalidate()
}

func (ui *dashboardUI) beginEdit(field dashField) {
	if ui.editor == nil || ui.editorLayer == nil {
		return
	}
	if strings.HasPrefix(field.Key, "hasset-") && field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
		selectedAssetIndex = field.Index
	}
	if strings.HasPrefix(field.Key, "asset-") && field.Index >= 0 && field.Index < len(ui.assets) {
		ui.selectedAsset = field.Index
	}
	copyField := field
	ui.activeField = &copyField
	_ = ui.editor.SetCueBanner(field.Placeholder)
	ui.editor.SetText(fieldEditorText(field))
	layerRect := fieldEditorRect(field.Rect)
	_ = ui.editorLayer.SetBoundsPixels(layerRect)
	_ = ui.editor.SetBoundsPixels(rect(0, 0, layerRect.Width, layerRect.Height))
	win.SetWindowPos(ui.editorLayer.Handle(), win.HWND_TOP, int32(layerRect.X), int32(layerRect.Y), int32(layerRect.Width), int32(layerRect.Height), win.SWP_NOACTIVATE)
	ui.editor.SetVisible(true)
	ui.editorLayer.SetVisible(true)
	_ = ui.editor.SetFocus()
	cursor := len([]rune(ui.editor.Text()))
	ui.editor.SetTextSelection(cursor, cursor)
}

func (ui *dashboardUI) commitEditor() {
	if ui.activeField == nil || ui.editor == nil {
		return
	}
	field := *ui.activeField
	text := strings.TrimSpace(ui.editor.Text())
	if strings.TrimSpace(field.Value) == "" && text == strings.TrimSpace(field.Placeholder) {
		text = ""
	}
	text = stripFieldSuffix(text, field.Suffix)
	ui.editor.SetVisible(false)
	if ui.editorLayer != nil {
		ui.editorLayer.SetVisible(false)
	}
	ui.applyField(field, text)
	ui.activeField = nil
	ui.invalidate()
}

func fieldEditorText(field dashField) string {
	text := strings.TrimSpace(field.Value)
	if text == "" {
		return ""
	}
	if field.Suffix != "" {
		return text + " " + field.Suffix
	}
	return text
}

func fieldEditorRect(r walk.Rectangle) walk.Rectangle {
	text := fieldTextRect(r)
	height := clampInt(r.Height-4, 20, 24)
	if r.Height >= 34 {
		height = 24
	}
	y := r.Y + maxInt(0, (r.Height-height)/2)
	return rect(text.X, y, text.Width, height)
}

func stripFieldSuffix(text, suffix string) string {
	text = strings.TrimSpace(text)
	if suffix != "" {
		text = strings.TrimSpace(strings.TrimSuffix(text, suffix))
	}
	text = strings.TrimSpace(strings.TrimSuffix(text, "元"))
	text = strings.TrimSpace(strings.TrimSuffix(text, "%"))
	return text
}

func (ui *dashboardUI) cancelEditor() {
	if ui.editor != nil {
		ui.editor.SetVisible(false)
	}
	if ui.editorLayer != nil {
		ui.editorLayer.SetVisible(false)
	}
	ui.activeField = nil
	ui.invalidate()
}

func (ui *dashboardUI) applyField(field dashField, text string) {
	parse := func() (float64, bool) {
		value, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
		if err != nil {
			ui.statusText = "请输入有效数字"
			return 0, false
		}
		if value < 0 {
			ui.statusText = "金额和仓位不能为负数"
			return 0, false
		}
		return value, true
	}

	switch field.Key {
	case "invest":
		if value, ok := parse(); ok {
			ui.investAmount = value
			ui.autoSavePortfolioConfig()
		}
	case "asset-name":
		if field.Index >= 0 && field.Index < len(ui.assets) {
			ui.assets[field.Index].Name = strings.TrimSpace(text)
			ui.autoSavePortfolioConfig()
		}
	case "asset-target":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(ui.assets) {
			ui.assets[field.Index].TargetPct = value
			ui.autoSavePortfolioConfig()
		}
	case "asset-current":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(ui.assets) {
			ui.assets[field.Index].CurrentAmount = value
			ui.autoSavePortfolioConfig()
		}
	case "hist-archive":
		selectedHistoryDraft.ArchivedAt = text
		ui.autoSaveHistoryDraft()
	case "hist-invest":
		if value, ok := parse(); ok {
			selectedHistoryDraft.InvestAmount = value
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "hist-notes":
		selectedHistoryDraft.Notes = text
		ui.autoSaveHistoryDraft()
	case "hasset-name":
		if field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
			selectedAssetIndex = field.Index
			selectedHistoryDraft.Assets[field.Index].Name = text
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "hasset-target":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
			selectedAssetIndex = field.Index
			selectedHistoryDraft.Assets[field.Index].TargetPct = value
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "hasset-before":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
			selectedAssetIndex = field.Index
			selectedHistoryDraft.Assets[field.Index].BeforeAmount = value
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "hasset-buy":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
			selectedAssetIndex = field.Index
			selectedHistoryDraft.Assets[field.Index].BuyAmount = value
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "trend-start":
		ui.trendStartText = text
	case "trend-end":
		ui.trendEndText = text
	}
}

func (ui *dashboardUI) deleteSelectedAsset() {
	if ui.selectedAsset < 0 || ui.selectedAsset >= len(ui.assets) {
		ui.statusText = "请先选择要删除的资产"
		return
	}
	ui.assets = append(ui.assets[:ui.selectedAsset], ui.assets[ui.selectedAsset+1:]...)
	if ui.selectedAsset >= len(ui.assets) {
		ui.selectedAsset = len(ui.assets) - 1
	}
	ui.resultText = initialResultText()
	if ui.autoSavePortfolioConfig() {
		ui.statusText = "已删除选中资产"
	}
}

func (ui *dashboardUI) calculate() {
	result, err := CalculatePortfolio(ui.investAmount, ui.assets)
	if err != nil {
		ui.statusText = "输入有误：" + err.Error()
		walk.MsgBox(ui.mw, "输入有误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}
	ui.resultText = FormatResult(result)
	ui.statusText = "计算完成：所有目标金额均基于买入后的组合总额"
}

func (ui *dashboardUI) saveCurrentInvestment() {
	result, err := CalculatePortfolio(ui.investAmount, ui.assets)
	if err != nil {
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	record := recordFromResult(result)
	investmentRecords = append([]InvestmentRecord{record}, investmentRecords...)
	if err := saveInvestmentRecords(); err != nil {
		investmentRecords = investmentRecords[1:]
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	ui.resultText = FormatResult(result)
	ui.selectHistory(0)
	ui.ensureTrendRange()
	ui.statusText = "本次投资信息已归档到程序目录"
}

func (ui *dashboardUI) loadPortfolioConfig() error {
	investAmount, assets, err := loadPortfolioConfig(ui.investAmount)
	if err != nil {
		return err
	}
	ui.investAmount = investAmount
	ui.assets = assets
	if len(ui.assets) > 0 {
		ui.selectedAsset = 0
	}
	return nil
}

func (ui *dashboardUI) autoSavePortfolioConfig() bool {
	if err := savePortfolioConfig(ui.investAmount, ui.assets); err != nil {
		ui.statusText = "当前资产配置保存失败：" + err.Error()
		return false
	}
	return true
}

func (ui *dashboardUI) loadHistoryToCalculator() {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		ui.statusText = "请先选择要读取的历史记录"
		return
	}
	record := cloneInvestmentRecord(selectedHistoryDraft)
	if record.ID == "" {
		record = cloneInvestmentRecord(investmentRecords[selectedHistoryIndex])
	}
	recalculateInvestmentRecord(&record)
	assets := portfolioAssetsFromHistory(record)
	if len(assets) == 0 {
		ui.statusText = "该历史记录没有可读取的资产条目"
		return
	}
	ui.assets = assets
	ui.selectedAsset = 0
	ui.assetScroll = 0
	ui.resultText = initialResultText()
	ui.page = dashPageCalc
	if ui.autoSavePortfolioConfig() {
		ui.statusText = "已读取历史记录，当前资产金额使用买入后金额"
	}
}

func (ui *dashboardUI) ensureHistorySelection() {
	if len(investmentRecords) == 0 {
		selectedHistoryIndex = -1
		selectedAssetIndex = -1
		selectedHistoryDraft = InvestmentRecord{}
		return
	}
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		ui.selectHistory(0)
	}
}

func (ui *dashboardUI) selectHistory(index int) {
	if index < 0 || index >= len(investmentRecords) {
		selectedHistoryIndex = -1
		selectedAssetIndex = -1
		selectedHistoryDraft = InvestmentRecord{}
		return
	}
	selectedHistoryIndex = index
	selectedHistoryDraft = cloneInvestmentRecord(investmentRecords[index])
	recalculateInvestmentRecord(&selectedHistoryDraft)
	if len(selectedHistoryDraft.Assets) > 0 {
		selectedAssetIndex = 0
	} else {
		selectedAssetIndex = -1
	}
}

func (ui *dashboardUI) autoSaveHistoryDraft() {
	if err := ui.saveHistoryDraft(); err != nil {
		ui.statusText = "自动保存失败：" + err.Error()
		return
	}
	ui.statusText = "历史投资记录已自动保存"
}

func (ui *dashboardUI) saveHistoryDraft() error {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		return fmt.Errorf("请先选择历史记录")
	}
	archivedAt := strings.TrimSpace(selectedHistoryDraft.ArchivedAt)
	parsed, err := time.ParseInLocation(archiveTimeFmt, archivedAt, time.Local)
	if err != nil {
		return fmt.Errorf("归档时间格式应为：2026-06-18 15:30:00")
	}
	selectedHistoryDraft.ArchivedAt = parsed.Format(archiveTimeFmt)
	recalculateInvestmentRecord(&selectedHistoryDraft)
	if err := validateInvestmentRecord(selectedHistoryDraft); err != nil {
		return err
	}
	recordID := investmentRecords[selectedHistoryIndex].ID
	selectedAsset := selectedAssetIndex
	selectedHistoryDraft.ID = recordID
	investmentRecords[selectedHistoryIndex] = cloneInvestmentRecord(selectedHistoryDraft)
	sort.SliceStable(investmentRecords, func(i, j int) bool {
		return investmentRecords[i].ArchivedAt > investmentRecords[j].ArchivedAt
	})
	if err := saveInvestmentRecords(); err != nil {
		return err
	}
	for i := range investmentRecords {
		if investmentRecords[i].ID == recordID {
			selectedHistoryIndex = i
			break
		}
	}
	selectedHistoryDraft = cloneInvestmentRecord(investmentRecords[selectedHistoryIndex])
	recalculateInvestmentRecord(&selectedHistoryDraft)
	if len(selectedHistoryDraft.Assets) == 0 {
		selectedAssetIndex = -1
	} else if selectedAsset >= 0 && selectedAsset < len(selectedHistoryDraft.Assets) {
		selectedAssetIndex = selectedAsset
	} else {
		selectedAssetIndex = 0
	}
	ui.ensureTrendRange()
	return nil
}

func (ui *dashboardUI) deleteHistory() {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		ui.statusText = "请先选择历史记录"
		return
	}
	if walk.MsgBox(ui.mw, "删除历史记录", "确定删除 "+investmentRecords[selectedHistoryIndex].ArchivedAt+" 的投资记录吗？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	investmentRecords = append(investmentRecords[:selectedHistoryIndex], investmentRecords[selectedHistoryIndex+1:]...)
	if err := saveInvestmentRecords(); err != nil {
		ui.statusText = "删除失败：" + err.Error()
		walk.MsgBox(ui.mw, "删除失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	selectedHistoryIndex = -1
	selectedAssetIndex = -1
	selectedHistoryDraft = InvestmentRecord{}
	ui.ensureHistorySelection()
	ui.statusText = "历史投资记录已删除"
}

func (ui *dashboardUI) exportHistory() {
	basePath, err := recordsFilePath()
	if err != nil {
		ui.statusText = "导出失败：" + err.Error()
		walk.MsgBox(ui.mw, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	fileName := "investment_records_export_" + time.Now().Format("20060102_150405") + ".json"
	dlg := walk.FileDialog{Title: "导出历史投资记录", FilePath: fileName, Filter: jsonFileFilter, FilterIndex: 1, InitialDirPath: filepath.Dir(basePath)}
	accepted, err := dlg.ShowSave(ui.mw)
	if err != nil || !accepted {
		if err != nil {
			ui.statusText = "导出失败：" + err.Error()
		}
		return
	}
	targetPath := strings.TrimSpace(dlg.FilePath)
	if filepath.Ext(targetPath) == "" {
		targetPath += ".json"
	}
	if _, err := os.Stat(targetPath); err == nil {
		if walk.MsgBox(ui.mw, "覆盖导出文件", "导出文件已存在，是否覆盖？\r\n"+targetPath, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
	}
	if err := writeInvestmentRecordsFile(targetPath, investmentRecords); err != nil {
		ui.statusText = "导出失败：" + err.Error()
		walk.MsgBox(ui.mw, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	ui.statusText = "历史投资记录已导出：" + targetPath
}

func (ui *dashboardUI) importHistory() {
	basePath, err := recordsFilePath()
	if err != nil {
		ui.statusText = "导入失败：" + err.Error()
		walk.MsgBox(ui.mw, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	dlg := walk.FileDialog{Title: "选择要导入的历史记录 JSON 文件", Filter: jsonFileFilter, FilterIndex: 1, InitialDirPath: filepath.Dir(basePath)}
	accepted, err := dlg.ShowOpen(ui.mw)
	if err != nil || !accepted {
		if err != nil {
			ui.statusText = "导入失败：" + err.Error()
		}
		return
	}
	records, err := readInvestmentRecordsFile(dlg.FilePath)
	if err != nil {
		ui.statusText = "导入失败：" + err.Error()
		walk.MsgBox(ui.mw, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	message := fmt.Sprintf("将从以下文件导入 %d 条历史记录，并覆盖当前记录文件：\r\n%s\r\n\r\n是否继续？", len(records), dlg.FilePath)
	if walk.MsgBox(ui.mw, "导入历史记录", message, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	if err := writeInvestmentRecordsFile(basePath, records); err != nil {
		ui.statusText = "导入失败：" + err.Error()
		walk.MsgBox(ui.mw, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	investmentRecords = records
	selectedHistoryIndex = -1
	selectedAssetIndex = -1
	ui.ensureHistorySelection()
	ui.ensureTrendRange()
	ui.statusText = "历史投资记录已导入：" + dlg.FilePath
}

func (ui *dashboardUI) ensureTrendRange() {
	start, end, ok := trendMonthBounds(investmentRecords)
	if !ok {
		if ui.trendStartText == "" {
			now := normalizeTrendMonth(time.Now())
			ui.trendStartText = now.AddDate(0, -11, 0).Format(trendMonthFmt)
			ui.trendEndText = now.Format(trendMonthFmt)
		}
		return
	}
	if ui.trendStartText == "" || ui.trendEndText == "" {
		ui.trendStartText = start.Format(trendMonthFmt)
		ui.trendEndText = end.Format(trendMonthFmt)
	}
	syncTrendSelections(trendSeriesOptions(investmentRecords))
}

func (ui *dashboardUI) setTrendRecentYear() {
	_, latest, ok := trendMonthBounds(investmentRecords)
	if !ok {
		latest = normalizeTrendMonth(time.Now())
	}
	ui.trendStartText = latest.AddDate(0, -11, 0).Format(trendMonthFmt)
	ui.trendEndText = latest.Format(trendMonthFmt)
	ui.statusText = "已切换到最近一年趋势范围"
}

func (ui *dashboardUI) trendRange() (time.Time, time.Time, error) {
	startText := strings.TrimSpace(ui.trendStartText)
	endText := strings.TrimSpace(ui.trendEndText)
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

func (ui *dashboardUI) clampAssetScroll(visible int) {
	ui.assetScroll = clampInt(ui.assetScroll, 0, maxInt(0, len(ui.assets)-visible))
}

func (ui *dashboardUI) clampResultScroll(visible, total int) {
	ui.resultScroll = clampInt(ui.resultScroll, 0, maxInt(0, total-visible))
}

func (ui *dashboardUI) clampHistoryScroll(visible int) {
	ui.historyScroll = clampInt(ui.historyScroll, 0, maxInt(0, len(investmentRecords)-visible))
}

func (ui *dashboardUI) clampHistoryAssetScroll(visible int) {
	ui.historyAssetScroll = clampInt(ui.historyAssetScroll, 0, maxInt(0, len(selectedHistoryDraft.Assets)-visible))
}

func (ui *dashboardUI) clampTrendOptionScroll(visible, total int) {
	ui.trendOptionScroll = clampInt(ui.trendOptionScroll, 0, maxInt(0, total-visible))
}

func (ui *dashboardUI) setScrollOffset(key string, offset, total, visible int) {
	offset = clampInt(offset, 0, maxInt(0, total-visible))
	switch key {
	case "scroll-assets":
		ui.assetScroll = offset
	case "scroll-result":
		ui.resultScroll = offset
	case "scroll-history-list":
		ui.historyScroll = offset
	case "scroll-history-detail":
		ui.historyAssetScroll = offset
	case "scroll-trend-options":
		ui.trendOptionScroll = offset
	}
}

func fill(canvas *walk.Canvas, color walk.Color, r walk.Rectangle) {
	brush := solidBrush(color)
	defer brush.Dispose()
	_ = canvas.FillRectanglePixels(brush, r)
}

func roundFill(canvas *walk.Canvas, color walk.Color, r walk.Rectangle, radius int) {
	brush := solidBrush(color)
	defer brush.Dispose()
	_ = canvas.FillRoundedRectanglePixels(brush, r, walk.Size{Width: radius * 2, Height: radius * 2})
}

func drawRoundStroke(canvas *walk.Canvas, color walk.Color, r walk.Rectangle, radius, width int) {
	pen, brush := chartPen(color, width)
	defer pen.Dispose()
	defer brush.Dispose()
	_ = canvas.DrawRoundedRectanglePixels(pen, r, walk.Size{Width: radius * 2, Height: radius * 2})
}

func drawLine(canvas *walk.Canvas, color walk.Color, x1, y1, x2, y2, width int) {
	pen, brush := chartPen(color, width)
	defer pen.Dispose()
	defer brush.Dispose()
	_ = canvas.DrawLinePixels(pen, walk.Point{X: x1, Y: y1}, walk.Point{X: x2, Y: y2})
}

func drawText(canvas *walk.Canvas, text string, font *walk.Font, color walk.Color, r walk.Rectangle, format walk.DrawTextFormat) {
	if format&walk.TextVCenter != 0 && format&walk.TextWordbreak == 0 {
		format |= walk.TextSingleLine
		r.Y += 2
	}
	_ = canvas.DrawTextPixels(text, font, color, r, format)
}

func solidBrush(color walk.Color) *walk.SolidColorBrush {
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		panic(err)
	}
	return brush
}

func chartPen(color walk.Color, width int) (*walk.GeometricPen, *walk.SolidColorBrush) {
	brush := solidBrush(color)
	pen, err := walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound|walk.PenJoinRound, width, brush)
	if err != nil {
		brush.Dispose()
		panic(err)
	}
	return pen, brush
}

func rect(x, y, w, h int) walk.Rectangle {
	return walk.Rectangle{X: x, Y: y, Width: w, Height: h}
}

func inset(r walk.Rectangle, x, y int) walk.Rectangle {
	return walk.Rectangle{X: r.X + x, Y: r.Y + y, Width: maxInt(0, r.Width-2*x), Height: maxInt(0, r.Height-2*y)}
}

func contains(r walk.Rectangle, x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

func chooseColor(ok bool, yes, no walk.Color) walk.Color {
	if ok {
		return yes
	}
	return no
}

func mutedForActive(active bool) walk.Color {
	if active {
		return walk.RGB(55, 58, 60)
	}
	return dashColors.muted
}

func formatPlainMoney(value float64) string {
	return strings.ReplaceAll(formatMoney(value), ",", "")
}

func formatMaybeFloat(value float64) string {
	if math.Abs(value) < moneyEpsilon {
		return ""
	}
	return formatFlexibleNumber(value, 2)
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
