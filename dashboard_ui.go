package main

import (
	"fmt"
	"image"
	"image/color"
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
	dashPageYield
)

// rsrc assigns the manifest ID first, then the icon group ID.
const appIconResourceID = 2

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
	appIcon     *walk.Icon

	fontTiny  *walk.Font
	fontSmall *walk.Font
	fontMono  *walk.Font
	fontBody  *walk.Font
	fontBold  *walk.Font
	fontBrand *walk.Font
	fontTitle *walk.Font
	fontBig   *walk.Font
	fontHuge  *walk.Font

	page               int
	assets             []AssetInput
	investAmount       float64
	selectedAsset      int
	assetScroll        int
	resultScroll       int
	archiveDraft       *InvestmentRecord
	archiveSuggested   []float64
	archiveScroll      int
	sellDraft          *InvestmentRecord
	sellScroll         int
	resultText         string
	statusText         string
	hoverAction        string
	historyScroll      int
	historyAssetScroll int
	trendOptionScroll  int
	trendStartText     string
	trendEndText       string
	trendDisplayData   trendChartData
	trendHoverSeries   int
	trendHoverIndex    int
	trendPlot          walk.Rectangle
	yieldOptionScroll  int
	yieldStartText     string
	yieldEndText       string
	yieldData          yieldChartData
	yieldCalculated    bool
	yieldHoverIndex    int
	yieldPlot          walk.Rectangle
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
		page:             dashPageCalc,
		investAmount:     5000,
		selectedAsset:    -1,
		trendHoverSeries: -1,
		trendHoverIndex:  -1,
		yieldHoverIndex:  -1,
		resultText:       initialResultText(),
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
	if err := ui.applyWindowIcon(); err != nil {
		ui.statusText = "程序图标加载失败：" + err.Error()
	}
	ui.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		ui.disposeWindowIcon()
	})
	dashboard = ui
	ui.installBorderlessWindow()
	ui.installEditor()
	ui.attachEvents()
	if err := loadInvestmentRecords(); err != nil {
		ui.statusText = "历史记录读取失败：" + err.Error()
	}
	ui.ensureHistorySelection()
	ui.ensureTrendRange()
	ui.ensureYieldRange()
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
	ui.fontBrand, _ = walk.NewFont("Microsoft YaHei UI", 13, walk.FontBold)
	ui.fontTitle, _ = walk.NewFont("Microsoft YaHei UI", 17, walk.FontBold)
	ui.fontBig, _ = walk.NewFont("Microsoft YaHei UI", 23, 0)
	ui.fontHuge, _ = walk.NewFont("Microsoft YaHei UI", 28, 0)
}

func (ui *dashboardUI) applyWindowIcon() error {
	if ui.mw == nil {
		return nil
	}
	icon, err := walk.NewIconFromResourceId(appIconResourceID)
	if err != nil {
		return err
	}
	if err := ui.mw.SetIcon(icon); err != nil {
		icon.Dispose()
		return err
	}
	if ui.appIcon != nil {
		ui.appIcon.Dispose()
	}
	ui.appIcon = icon
	return nil
}

func (ui *dashboardUI) disposeWindowIcon() {
	if ui.appIcon != nil {
		ui.appIcon.Dispose()
		ui.appIcon = nil
	}
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
	case dashPageYield:
		ui.paintYield(canvas, bounds)
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

	ui.drawHeaderBrand(canvas, rect(22, 26, 204, 52))
	drawText(canvas, "投资组合再平衡助手", ui.fontSmall, dashColors.muted, rect(22, 94, 204, 20), walk.TextCenter|walk.TextVCenter)
	drawText(canvas, "记录数据保存在本机", ui.fontSmall, dashColors.muted, rect(22, 120, 204, 20), walk.TextCenter|walk.TextVCenter)

	ui.drawNav(canvas, 22, 168, "平衡买入计算", "nav-calc", dashPageCalc)
	ui.drawNav(canvas, 22, 232, "历史投资记录", "nav-history", dashPageHistory)
	ui.drawNav(canvas, 22, 296, "资产趋势图表", "nav-trend", dashPageTrend)
	ui.drawNav(canvas, 22, 360, "收益数据测算", "nav-yield", dashPageYield)

	ui.drawWindowButton(canvas, rect(bounds.Width-128, 18, 28, 28), "win-min")
	ui.drawWindowButton(canvas, rect(bounds.Width-88, 18, 28, 28), "win-max")
	ui.drawWindowButton(canvas, rect(bounds.Width-48, 18, 28, 28), "win-close")
}

func (ui *dashboardUI) drawHeaderBrand(canvas *walk.Canvas, r walk.Rectangle) {
	first := "Portfolio"
	second := "Rebalancing"
	fonts := []*walk.Font{ui.fontBrand, ui.fontBold, ui.fontSmall, ui.fontTiny}
	font := ui.fontTiny
	padX := 5
	firstInset := 5
	gap := 2
	firstW, secondW, textH := 0, 0, 18
	for _, candidate := range fonts {
		if candidate == nil {
			continue
		}
		candidatePadX := 5
		if candidate == ui.fontTiny {
			candidatePadX = 4
		}
		firstSize := measureTextSize(canvas, first, candidate)
		secondSize := measureTextSize(canvas, second, candidate)
		totalW := candidatePadX + firstSize.Width + gap + secondSize.Width + candidatePadX*2
		if totalW <= r.Width || font == nil {
			font = candidate
			padX = candidatePadX
			firstInset = candidatePadX
			firstW = firstSize.Width
			secondW = secondSize.Width
			textH = maxInt(firstSize.Height, secondSize.Height)
			if totalW <= r.Width {
				break
			}
		}
	}
	if font == nil {
		font = ui.fontBold
	}
	if firstW == 0 {
		firstW = measureTextWidth(canvas, first, font)
	}
	if secondW == 0 {
		secondW = measureTextWidth(canvas, second, font)
	}
	if textH == 0 {
		textH = 18
	}

	blockH := clampInt(textH+6, 22, minInt(32, r.Height))
	blockW := minInt(r.Width, secondW+padX*2)
	textY := r.Y + (r.Height-blockH)/2
	blockY := clampInt(textY+2, r.Y, r.Y+r.Height-blockH)
	block := rect(r.X+r.Width-blockW, blockY, blockW, blockH)
	firstX := r.X + firstInset
	drawText(canvas, first, font, dashColors.white, rect(firstX, textY, maxInt(0, block.X-firstX-gap), block.Height), walk.TextLeft|walk.TextVCenter)
	roundFill(canvas, dashColors.accent, block, 4)
	drawText(canvas, second, font, walk.RGB(0, 0, 0), inset(rect(block.X, textY, block.Width, block.Height), padX, 0), walk.TextCenter|walk.TextVCenter)
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
	if ui.archiveDraft != nil {
		ui.drawArchiveConfirmPanel(canvas, result)
		return
	}
	if ui.sellDraft != nil {
		ui.drawSellConfirmPanel(canvas, result)
		return
	}
	ui.drawPanel(canvas, result, "再平衡建议", "")
	buttonW := 104
	saveX := result.X + result.Width - buttonW - 18
	runX := saveX - buttonW - 12
	sellX := runX - buttonW - 12
	ui.drawDangerButton(canvas, rect(sellX, result.Y+10, buttonW, dashStyle.buttonHeight), "期间卖出", "calc-sell")
	ui.drawButton(canvas, rect(runX, result.Y+10, buttonW, dashStyle.buttonHeight), "计算建议", "calc-run", true)
	ui.drawButton(canvas, rect(saveX, result.Y+10, buttonW, dashStyle.buttonHeight), "保存归档", "calc-save", false)
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

func relativeTargetDeviationColor(deviationPct float64) walk.Color {
	absDeviation := math.Abs(deviationPct)
	switch {
	case absDeviation >= 20:
		return dashColors.danger
	case absDeviation >= 10:
		return dashColors.warning
	default:
		return dashColors.white
	}
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

func (ui *dashboardUI) drawArchiveConfirmPanel(canvas *walk.Canvas, r walk.Rectangle) {
	record := ui.archiveDraft
	if record == nil {
		return
	}

	ui.drawPanel(canvas, r, "确认真实买入金额", "")
	buttonW := 104
	buttonY := r.Y + 10
	confirmButtonX := r.X + r.Width - buttonW*2 - 30
	ui.drawButton(canvas, rect(confirmButtonX, buttonY, buttonW, dashStyle.buttonHeight), "确认保存", "archive-confirm", true)
	ui.drawButton(canvas, rect(r.X+r.Width-buttonW-18, buttonY, buttonW, dashStyle.buttonHeight), "取消", "archive-cancel", false)

	x := r.X + dashStyle.cardPad
	w := r.Width - dashStyle.cardPad*2
	summary := "真实买入合计 " + formatMoney(record.AllocatedCash) + " 元"
	summaryX := x + 150
	drawText(canvas, summary, ui.fontSmall, dashColors.accent, rect(summaryX, r.Y+16, maxInt(0, confirmButtonX-summaryX-16), 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	buttonReserve := buttonW*2 + 48
	drawText(canvas, "默认填入计算建议，可按实际成交金额修改。", ui.fontTiny, dashColors.muted, rect(x, r.Y+42, maxInt(0, w-buttonReserve), 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	tableY := r.Y + 72
	headers := []string{"资产", "建议买入", "真实买入", "买入后金额", "买入后仓位"}
	widths := []int{maxInt(130, w-120-150-150-110), 120, 150, 150, 110}
	drawTableHeader(canvas, ui.fontTiny, x, tableY, widths, headers)

	rowY := tableY + 32
	rowH := 38
	visible := maxInt(1, maxInt(0, r.Y+r.Height-rowY-12)/rowH)
	ui.clampArchiveScroll(visible, len(record.Assets))
	for i := 0; i < visible; i++ {
		index := ui.archiveScroll + i
		if index >= len(record.Assets) {
			break
		}
		asset := record.Assets[index]
		rowRect := rect(x, rowY+i*rowH, sumInts(widths), rowH-6)
		if i%2 == 0 {
			roundFill(canvas, dashColors.panel2, rowRect, 7)
		}
		cx := x
		drawText(canvas, asset.Name, ui.fontSmall, dashColors.text, rect(cx+8, rowRect.Y, widths[0]-16, rowRect.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[0]
		suggested := 0.0
		if index < len(ui.archiveSuggested) {
			suggested = ui.archiveSuggested[index]
		}
		drawText(canvas, formatMoney(suggested)+" 元", ui.fontSmall, dashColors.muted, rect(cx+8, rowRect.Y, widths[1]-16, rowRect.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[1]
		unitW := 22
		unitGap := 6
		buyField := rect(cx+6, rowRect.Y+4, maxInt(0, widths[2]-12-unitGap-unitW), 26)
		ui.drawField(canvas, buyField, "archive-buy", index, formatMaybeFloat(asset.BuyAmount), "0", true, "")
		drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(buyField.X+buyField.Width+unitGap, rowRect.Y, unitW, rowRect.Height), walk.TextLeft|walk.TextVCenter)
		cx += widths[2]
		drawText(canvas, formatMoney(asset.AfterAmount)+" 元", ui.fontSmall, dashColors.text, rect(cx+8, rowRect.Y, widths[3]-16, rowRect.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[3]
		drawText(canvas, formatPercent(asset.AfterPct), ui.fontSmall, statusTextColor(asset.Status), rect(cx+8, rowRect.Y, widths[4]-16, rowRect.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	}
	ui.drawScrollBar(canvas, "scroll-archive-buy", rect(r.X+r.Width-10, rowY, 6, maxInt(0, r.Y+r.Height-rowY-12)), len(record.Assets), visible, ui.archiveScroll)
}

func (ui *dashboardUI) drawSellConfirmPanel(canvas *walk.Canvas, r walk.Rectangle) {
	record := ui.sellDraft
	if record == nil {
		return
	}

	ui.drawPanel(canvas, r, "记录期间卖出", "")
	buttonW := 104
	buttonY := r.Y + 10
	confirmButtonX := r.X + r.Width - buttonW*2 - 30
	ui.drawButton(canvas, rect(confirmButtonX, buttonY, buttonW, dashStyle.buttonHeight), "确认保存", "sell-confirm", true)
	ui.drawButton(canvas, rect(r.X+r.Width-buttonW-18, buttonY, buttonW, dashStyle.buttonHeight), "取消", "sell-cancel", false)

	x := r.X + dashStyle.cardPad
	w := r.Width - dashStyle.cardPad*2
	summary := "卖出合计 " + formatMoney(recordSellTotal(*record)) + " 元"
	summaryX := x + 128
	drawText(canvas, summary, ui.fontSmall, dashColors.danger, rect(summaryX, r.Y+16, maxInt(0, confirmButtonX-summaryX-16), 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	buttonReserve := buttonW*2 + 48
	drawText(canvas, "卖出时间以第一行填写为准，其他资产同步使用同一时间。", ui.fontTiny, dashColors.muted, rect(x, r.Y+42, maxInt(0, w-buttonReserve), 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	tableY := r.Y + 72
	headers := []string{"资产名称", "卖出金额", "卖出时间"}
	widths := []int{maxInt(180, w-170-220), 170, 220}
	drawTableHeader(canvas, ui.fontTiny, x, tableY, widths, headers)

	rowY := tableY + 32
	rowH := 38
	visible := maxInt(1, maxInt(0, r.Y+r.Height-rowY-12)/rowH)
	ui.clampSellScroll(visible, len(record.Assets))
	for i := 0; i < visible; i++ {
		index := ui.sellScroll + i
		if index >= len(record.Assets) {
			break
		}
		asset := record.Assets[index]
		rowRect := rect(x, rowY+i*rowH, sumInts(widths), rowH-6)
		if i%2 == 0 {
			roundFill(canvas, dashColors.panel2, rowRect, 7)
		}
		cx := x
		ui.drawField(canvas, rect(cx+6, rowRect.Y+4, widths[0]-12, 26), "sell-name", index, asset.Name, "资产名称", false, "")
		cx += widths[0]
		unitW := 22
		unitGap := 6
		sellField := rect(cx+6, rowRect.Y+4, maxInt(0, widths[1]-12-unitGap-unitW), 26)
		ui.drawField(canvas, sellField, "sell-amount", index, formatMaybeFloat(asset.SellAmount), "0", true, "")
		drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(sellField.X+sellField.Width+unitGap, rowRect.Y, unitW, rowRect.Height), walk.TextLeft|walk.TextVCenter)
		cx += widths[1]
		timeField := rect(cx+6, rowRect.Y+4, widths[2]-12, 26)
		if index == 0 {
			ui.drawField(canvas, timeField, "sell-time", 0, record.ArchivedAt, "2026-05-29 11:41:16", false, "")
		} else {
			ui.drawReadOnlyField(canvas, timeField, record.ArchivedAt, "卖出时间")
		}
	}
	ui.drawScrollBar(canvas, "scroll-sell", rect(r.X+r.Width-10, rowY, 6, maxInt(0, r.Y+r.Height-rowY-12)), len(record.Assets), visible, ui.sellScroll)
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
		amountLabel := "买入金额 " + formatMoney(record.InvestAmount) + " 元"
		amountColor := dashColors.accent
		if isSellRecord(record) {
			amountLabel = "卖出金额 " + formatMoney(recordSellTotal(record)) + " 元"
			amountColor = dashColors.danger
		}
		drawText(canvas, amountLabel, ui.fontTiny, amountColor, rect(r.X+12, r.Y+29, r.Width-24, 16), walk.TextLeft|walk.TextVCenter)
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
	if isSellRecord(selectedHistoryDraft) {
		ui.drawSellHistoryDetail(canvas, panel, x, y, w)
		return
	}
	ui.drawField(canvas, rect(x, y, 220, 38), "hist-archive", -1, selectedHistoryDraft.ArchivedAt, "记录时间", false, "")
	historyInvestField := rect(x+236, y, 128, 38)
	ui.drawReadOnlyField(canvas, historyInvestField, formatMoney(selectedHistoryDraft.InvestAmount), "真实投入")
	drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(historyInvestField.X+historyInvestField.Width+8, historyInvestField.Y, 22, historyInvestField.Height), walk.TextLeft|walk.TextVCenter)
	ui.drawField(canvas, rect(x+408, y, w-408, 38), "hist-notes", -1, selectedHistoryDraft.Notes, "备注", false, "")

	summaryY := y + 52
	summary := fmt.Sprintf("买入前 %s 元  |  真实投入 %s 元  |  买入后 %s 元",
		formatMoney(selectedHistoryDraft.CurrentTotal),
		formatMoney(selectedHistoryDraft.InvestAmount),
		formatMoney(selectedHistoryDraft.AfterTotal))
	drawText(canvas, summary, ui.fontBold, dashColors.accent, rect(x, summaryY, w, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	tableY := summaryY + 38
	tableBottom := panel.Y + panel.Height - dashStyle.buttonHeight - 34
	table := rect(x, tableY, w, maxInt(190, tableBottom-tableY))
	ui.drawHistoryAssetTable(canvas, table)

	ui.drawButton(canvas, rect(panel.X+panel.Width-270, panel.Y+panel.Height-58, 118, dashStyle.buttonHeight), "读取记录", "hist-load-to-calc", false)
	ui.drawDangerButton(canvas, rect(panel.X+panel.Width-140, panel.Y+panel.Height-58, 118, dashStyle.buttonHeight), "删除记录", "hist-delete")
}

func (ui *dashboardUI) drawSellHistoryDetail(canvas *walk.Canvas, panel walk.Rectangle, x, y, w int) {
	ui.drawField(canvas, rect(x, y, 220, 38), "hist-archive", -1, selectedHistoryDraft.ArchivedAt, "记录时间", false, "")
	sellField := rect(x+236, y, 128, 38)
	ui.drawReadOnlyField(canvas, sellField, formatMoney(recordSellTotal(selectedHistoryDraft)), "卖出金额")
	drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(sellField.X+sellField.Width+8, sellField.Y, 22, sellField.Height), walk.TextLeft|walk.TextVCenter)
	ui.drawField(canvas, rect(x+408, y, w-408, 38), "hist-notes", -1, selectedHistoryDraft.Notes, "备注", false, "")

	summaryY := y + 52
	summary := "卖出总金额 " + formatMoney(recordSellTotal(selectedHistoryDraft)) + " 元"
	drawText(canvas, summary, ui.fontBold, dashColors.danger, rect(x, summaryY, w, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	tableY := summaryY + 38
	tableBottom := panel.Y + panel.Height - dashStyle.buttonHeight - 34
	table := rect(x, tableY, w, maxInt(190, tableBottom-tableY))
	ui.drawSellHistoryAssetTable(canvas, table)

	ui.drawDangerButton(canvas, rect(panel.X+panel.Width-140, panel.Y+panel.Height-58, 118, dashStyle.buttonHeight), "删除记录", "hist-delete")
}

func (ui *dashboardUI) drawHistoryAssetTable(canvas *walk.Canvas, table walk.Rectangle) {
	headers := []string{"资产", "目标", "买入前", "买入", "买入后", "买入后仓位", "相对目标偏离"}
	widths := []int{maxInt(126, table.Width-76-118-106-120-100-122), 76, 118, 106, 120, 100, 122}
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
		drawText(canvas, formatPercent(asset.AfterPct), ui.fontSmall, dashColors.accent, rect(cx+8, r.Y, widths[5]-16, r.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[5]
		deviationText := "-"
		deviationColor := dashColors.muted
		if deviationPct, ok := relativeTargetDeviationPct(asset); ok {
			deviationText = formatSignedPercent(deviationPct)
			deviationColor = relativeTargetDeviationColor(deviationPct)
		}
		drawText(canvas, deviationText, ui.fontSmall, deviationColor, rect(cx+8, r.Y, widths[6]-16, r.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	}
	ui.drawScrollBar(canvas, "scroll-history-detail", rect(table.X+table.Width+8, rowY, 6, maxInt(0, table.Y+table.Height-rowY)), len(selectedHistoryDraft.Assets), visible, ui.historyAssetScroll)
}

func (ui *dashboardUI) drawSellHistoryAssetTable(canvas *walk.Canvas, table walk.Rectangle) {
	headers := []string{"资产名称", "卖出金额"}
	widths := []int{maxInt(220, table.Width-180), 180}
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
		r := rect(table.X, rowY+i*34, sumInts(widths), 30)
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
		unitW := 22
		unitGap := 6
		sellField := rect(cx+4, r.Y+3, maxInt(0, widths[1]-8-unitGap-unitW), 24)
		ui.drawField(canvas, sellField, "hasset-sell", index, formatMaybeFloat(asset.SellAmount), "0", true, "")
		drawText(canvas, "元", ui.fontSmall, dashColors.muted, rect(sellField.X+sellField.Width+unitGap, r.Y, unitW, r.Height), walk.TextLeft|walk.TextVCenter)
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
	drawText(canvas, trendDataHint, ui.fontTiny, dashColors.muted, rect(chartPanel.X+chartPanel.Width-318, chartPanel.Y+18, 300, 18), walk.TextRight|walk.TextVCenter|walk.TextEndEllipsis)
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
	ui.trendPlot = walk.Rectangle{}
	ui.trendDisplayData = trendChartData{}
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
	ui.trendDisplayData = data
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
	ui.trendPlot = plot
	minValue, maxValue := trendValueRange(data.Series)
	drawTrendGridCustom(canvas, ui.fontTiny, plot, minValue, maxValue)
	drawTrendLinesCustom(canvas, plot, data, minValue, maxValue)
	drawTrendAxisCustom(canvas, ui.fontTiny, plot, data.Months)
	ui.drawTrendTooltip(canvas, r, plot, minValue, maxValue)
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
	if drawTrendLineLayer(canvas, plot, data, minValue, maxValue) {
		drawTrendDataPoints(canvas, plot, data, minValue, maxValue)
		return
	}

	drawTrendLinesFallback(canvas, plot, data, minValue, maxValue)
	drawTrendDataPoints(canvas, plot, data, minValue, maxValue)
}

func drawTrendLineLayer(canvas *walk.Canvas, plot walk.Rectangle, data trendChartData, minValue, maxValue float64) bool {
	if plot.Width <= 0 || plot.Height <= 0 {
		return false
	}

	layer := image.NewRGBA(image.Rect(0, 0, plot.Width, plot.Height))
	for _, series := range data.Series {
		var previous *chartPointF
		lineColor := rgbaFromWalkColor(series.Color)
		for i, point := range series.Points {
			if !point.Present {
				continue
			}
			current := chartPointF{
				X: float64(trendPointX(plot, i, len(data.Months)) - plot.X),
				Y: float64(trendPointY(plot, minValue, maxValue, point.Value) - plot.Y),
			}
			if previous != nil {
				drawAntialiasedLine(layer, *previous, current, 2, lineColor)
			}
			cp := current
			previous = &cp
		}
	}

	bmp, err := walk.NewBitmapFromImageForDPI(layer, canvas.DPI())
	if err != nil {
		return false
	}
	defer bmp.Dispose()

	if err := canvas.DrawBitmapWithOpacityPixels(bmp, plot, 255); err != nil {
		return false
	}
	return true
}

func drawTrendDataPoints(canvas *walk.Canvas, plot walk.Rectangle, data trendChartData, minValue, maxValue float64) {
	for _, series := range data.Series {
		brush := solidBrush(series.Color)
		for i, point := range series.Points {
			if !point.Present {
				continue
			}
			current := walk.Point{
				X: trendPointX(plot, i, len(data.Months)),
				Y: trendPointY(plot, minValue, maxValue, point.Value),
			}
			_ = canvas.FillEllipsePixels(brush, rect(current.X-4, current.Y-4, 8, 8))
		}
		brush.Dispose()
	}
}

func drawTrendLinesFallback(canvas *walk.Canvas, plot walk.Rectangle, data trendChartData, minValue, maxValue float64) {
	for _, series := range data.Series {
		pen, penBrush := chartPen(series.Color, 2)
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
			cp := current
			previous = &cp
		}
		pen.Dispose()
		penBrush.Dispose()
	}
}

func (ui *dashboardUI) drawTrendTooltip(canvas *walk.Canvas, chart, plot walk.Rectangle, minValue, maxValue float64) {
	if ui.trendHoverSeries < 0 || ui.trendHoverSeries >= len(ui.trendDisplayData.Series) {
		return
	}
	series := ui.trendDisplayData.Series[ui.trendHoverSeries]
	if ui.trendHoverIndex < 0 || ui.trendHoverIndex >= len(series.Points) {
		return
	}
	point := series.Points[ui.trendHoverIndex]
	if !point.Present {
		return
	}
	px := trendPointX(plot, ui.trendHoverIndex, len(ui.trendDisplayData.Months))
	py := trendPointY(plot, minValue, maxValue, point.Value)
	tipW := 226
	tipH := 86
	tx := px + 14
	if tx+tipW > chart.X+chart.Width-10 {
		tx = px - tipW - 14
	}
	ty := clampInt(py-tipH/2, chart.Y+10, chart.Y+chart.Height-tipH-10)
	tip := rect(tx, ty, tipW, tipH)
	roundFill(canvas, walk.RGB(24, 24, 24), tip, 9)
	drawRoundStroke(canvas, series.Color, tip, 9, 1)
	drawText(canvas, series.Name+"  "+point.Month.Format(trendMonthFmt), ui.fontBold, dashColors.text, rect(tip.X+12, tip.Y+8, tip.Width-24, 20), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	drawText(canvas, "金额 "+formatMoney(point.Value)+" 元", ui.fontSmall, dashColors.accent, rect(tip.X+12, tip.Y+34, tip.Width-24, 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	drawText(canvas, "仓位 "+formatPercent(point.Pct), ui.fontSmall, dashColors.text, rect(tip.X+12, tip.Y+56, tip.Width-24, 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
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

func (ui *dashboardUI) paintYield(canvas *walk.Canvas, bounds walk.Rectangle) {
	left := dashStyle.sidebarWidth + dashStyle.pagePad
	top := 88
	gap := dashStyle.gap
	contentW := bounds.Width - left - dashStyle.pagePad
	optionsW := maxInt(0, (contentW-gap)/4)
	chartX := left + optionsW + gap
	chartW := contentW - optionsW - gap
	height := bounds.Height - top - dashStyle.pagePad

	drawText(canvas, "收益数据测算", ui.fontTitle, dashColors.text, rect(left, 30, 240, 34), walk.TextLeft|walk.TextVCenter)

	options := rect(left, top, optionsW, height)
	ui.drawPanel(canvas, options, "资产选择", "")
	ui.drawYieldOptions(canvas, options)

	chartPanel := rect(chartX, top, chartW, height)
	ui.drawPanel(canvas, chartPanel, "收益率测算", "")
	drawText(canvas, yieldDataHint, ui.fontTiny, dashColors.muted, rect(chartPanel.X+128, chartPanel.Y+12, chartPanel.Width-146, 38), walk.TextRight|walk.TextVCenter|walk.TextWordbreak)
	ui.drawField(canvas, rect(chartPanel.X+18, chartPanel.Y+58, 118, 38), "yield-start", -1, ui.yieldStartText, "YYYY-MM", false, "")
	ui.drawField(canvas, rect(chartPanel.X+150, chartPanel.Y+58, 118, 38), "yield-end", -1, ui.yieldEndText, "YYYY-MM", false, "")
	ui.drawButton(canvas, rect(chartPanel.X+284, chartPanel.Y+58, 118, 38), "最近一年", "yield-year", false)
	buttonW := 118
	ui.drawButton(canvas, rect(chartPanel.X+chartPanel.Width-dashStyle.cardPad-buttonW, chartPanel.Y+58, buttonW, 38), "测算收益", "yield-run", true)

	summary := ui.yieldSummaryText()
	drawText(canvas, summary, ui.fontTiny, dashColors.muted, rect(chartPanel.X+18, chartPanel.Y+102, chartPanel.Width-36, 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)

	chart := rect(chartPanel.X+18, chartPanel.Y+132, chartPanel.Width-36, chartPanel.Height-154)
	ui.drawYieldChart(canvas, chart)
}

func (ui *dashboardUI) drawYieldOptions(canvas *walk.Canvas, panel walk.Rectangle) {
	options := trendSeriesOptions(investmentRecords)
	syncYieldSelections(options)
	x := panel.X + 16
	y := panel.Y + 54
	w := panel.Width - 32
	visible := maxInt(1, (panel.Y+panel.Height-y-18)/48)
	ui.clampYieldOptionScroll(visible, len(options))
	for i := 0; i < visible; i++ {
		index := ui.yieldOptionScroll + i
		if index >= len(options) {
			break
		}
		name := options[index]
		r := rect(x, y+i*48, w, 40)
		roundFill(canvas, dashColors.panel2, r, 9)
		drawRoundStroke(canvas, dashColors.line, r, 9, 1)
		box := rect(r.X+r.Width-34, r.Y+10, 20, 20)
		drawRoundStroke(canvas, dashColors.line2, box, 5, 1)
		if yieldSelections[name] {
			roundFill(canvas, dashColors.accent, inset(box, 4, 4), 4)
		}
		drawText(canvas, name, ui.fontSmall, dashColors.text, rect(r.X+14, r.Y+8, r.Width-62, 24), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionToggle, Key: "yield-toggle", Index: index, Name: name})
	}
	if len(options) == 0 {
		drawText(canvas, "暂无历史记录可测算收益", ui.fontBody, dashColors.muted, rect(x, y+36, w, 60), walk.TextCenter|walk.TextVCenter)
	}
	ui.drawScrollBar(canvas, "scroll-yield-options", rect(panel.X+panel.Width-12, y, 6, maxInt(0, panel.Y+panel.Height-y-18)), len(options), visible, ui.yieldOptionScroll)
}

func (ui *dashboardUI) drawYieldChart(canvas *walk.Canvas, r walk.Rectangle) {
	roundFill(canvas, walk.RGB(12, 14, 16), r, 10)
	drawRoundStroke(canvas, dashColors.line, r, 10, 1)
	ui.yieldPlot = walk.Rectangle{}
	if !ui.yieldCalculated {
		centerY := r.Y + r.Height/2 - 22
		drawText(canvas, yieldInitialMessage, ui.fontBody, dashColors.muted, rect(r.X+20, centerY, r.Width-40, 24), walk.TextCenter|walk.TextVCenter|walk.TextEndEllipsis)
		drawText(canvas, yieldSellHint, ui.fontSmall, dashColors.muted, rect(r.X+20, centerY+28, r.Width-40, 24), walk.TextCenter|walk.TextVCenter|walk.TextEndEllipsis)
		return
	}

	data := ui.yieldData
	if len(data.Months) == 0 || len(data.Points) == 0 || data.Message != "" && !yieldPointsHaveAny(data.Points) {
		message := data.Message
		if message == "" {
			message = "暂无历史记录可测算收益"
		}
		drawText(canvas, message, ui.fontBody, dashColors.muted, r, walk.TextCenter|walk.TextVCenter)
		return
	}

	plot := rect(r.X+84, r.Y+28, r.Width-112, r.Height-80)
	if plot.Height < 120 || plot.Width < 180 {
		drawText(canvas, "窗口空间不足，无法绘制收益图", ui.fontBody, dashColors.muted, r, walk.TextCenter|walk.TextVCenter)
		return
	}
	ui.yieldPlot = plot
	minValue, maxValue := yieldValueRange(data.Points)
	drawYieldGridCustom(canvas, ui.fontTiny, plot, minValue, maxValue)
	drawYieldLineCustom(canvas, plot, data, minValue, maxValue)
	drawTrendAxisCustom(canvas, ui.fontTiny, plot, data.Months)
	ui.drawYieldTooltip(canvas, r, plot, minValue, maxValue)
}

func drawYieldGridCustom(canvas *walk.Canvas, font *walk.Font, plot walk.Rectangle, minValue, maxValue float64) {
	for i := 0; i <= 4; i++ {
		y := plot.Y + plot.Height - i*plot.Height/4
		drawLine(canvas, walk.RGB(36, 42, 46), plot.X, y, plot.X+plot.Width, y, 1)
		value := minValue + (maxValue-minValue)*float64(i)/4
		drawText(canvas, formatYieldRate(value), font, dashColors.muted, rect(plot.X-78, y-10, 68, 18), walk.TextRight|walk.TextVCenter)
	}
	drawLine(canvas, walk.RGB(60, 70, 76), plot.X, plot.Y, plot.X, plot.Y+plot.Height, 1)
	drawLine(canvas, walk.RGB(60, 70, 76), plot.X, plot.Y+plot.Height, plot.X+plot.Width, plot.Y+plot.Height, 1)
}

func drawYieldLineCustom(canvas *walk.Canvas, plot walk.Rectangle, data yieldChartData, minValue, maxValue float64) {
	pen, penBrush := chartPen(walk.RGB(245, 245, 245), 2)
	brush := solidBrush(dashColors.accent)
	defer pen.Dispose()
	defer penBrush.Dispose()
	defer brush.Dispose()

	var previous *walk.Point
	points := make([]walk.Point, 0, len(data.Points))
	for i, point := range data.Points {
		if !point.Present {
			continue
		}
		current := walk.Point{
			X: trendPointX(plot, i, len(data.Months)),
			Y: trendPointY(plot, minValue, maxValue, point.Rate),
		}
		if previous != nil {
			_ = canvas.DrawLinePixels(pen, *previous, current)
		}
		points = append(points, current)
		cp := current
		previous = &cp
	}
	for _, point := range points {
		_ = canvas.FillEllipsePixels(brush, rect(point.X-4, point.Y-4, 8, 8))
	}
}

func (ui *dashboardUI) drawYieldTooltip(canvas *walk.Canvas, chart, plot walk.Rectangle, minValue, maxValue float64) {
	if ui.yieldHoverIndex < 0 || ui.yieldHoverIndex >= len(ui.yieldData.Points) {
		return
	}
	point := ui.yieldData.Points[ui.yieldHoverIndex]
	if !point.Present {
		return
	}
	px := trendPointX(plot, ui.yieldHoverIndex, len(ui.yieldData.Months))
	py := trendPointY(plot, minValue, maxValue, point.Rate)
	tipW := 224
	tipH := 78
	tx := px + 14
	if tx+tipW > chart.X+chart.Width-10 {
		tx = px - tipW - 14
	}
	ty := clampInt(py-tipH/2, chart.Y+10, chart.Y+chart.Height-tipH-10)
	tip := rect(tx, ty, tipW, tipH)
	roundFill(canvas, walk.RGB(24, 24, 24), tip, 9)
	drawRoundStroke(canvas, walk.RGB(88, 57, 8), tip, 9, 1)
	drawText(canvas, point.Month.Format(trendMonthFmt), ui.fontBold, dashColors.text, rect(tip.X+12, tip.Y+8, tip.Width-24, 20), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	drawText(canvas, "总收益金额 "+formatMoney(point.Profit)+" 元", ui.fontSmall, dashColors.accent, rect(tip.X+12, tip.Y+32, tip.Width-24, 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	drawText(canvas, "收益率 "+formatYieldRate(point.Rate), ui.fontSmall, dashColors.text, rect(tip.X+12, tip.Y+52, tip.Width-24, 18), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
}

func (ui *dashboardUI) yieldSummaryText() string {
	if !ui.yieldCalculated {
		return yieldInitialMessage
	}
	if ui.yieldData.Message != "" && !yieldPointsHaveAny(ui.yieldData.Points) {
		return ui.yieldData.Message
	}
	latest, ok := latestYieldPoint(ui.yieldData.Points)
	if !ok {
		return "暂无可显示收益数据"
	}
	return fmt.Sprintf(
		"测算对象：%s｜最新点：%s｜总收益 %s 元｜收益率 %s｜年化 %s",
		ui.yieldData.SelectionLabel,
		latest.Month.Format(trendMonthFmt),
		formatMoney(latest.Profit),
		formatYieldRate(latest.Rate),
		formatYieldRate(latest.AnnualizedRate),
	)
}

func formatYieldRate(value float64) string {
	return formatPercent(value * 100)
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

func (ui *dashboardUI) drawReadOnlyField(canvas *walk.Canvas, r walk.Rectangle, value, placeholder string) {
	roundFill(canvas, walk.RGB(26, 26, 26), r, 8)
	drawRoundStroke(canvas, dashColors.line, r, 8, 1)
	display := strings.TrimSpace(value)
	color := dashColors.text
	if display == "" {
		display = placeholder
		color = dashColors.faint
	}
	drawText(canvas, display, ui.fontSmall, color, fieldTextRect(r), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
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
	oldTrendHoverSeries := ui.trendHoverSeries
	oldTrendHoverIndex := ui.trendHoverIndex
	if ui.page == dashPageTrend {
		ui.trendHoverSeries, ui.trendHoverIndex = ui.trendPointAt(x, y)
	} else {
		ui.trendHoverSeries = -1
		ui.trendHoverIndex = -1
	}
	oldYieldHover := ui.yieldHoverIndex
	if ui.page == dashPageYield {
		ui.yieldHoverIndex = ui.yieldPointIndexAt(x, y)
	} else {
		ui.yieldHoverIndex = -1
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
	if hover != ui.hoverAction || oldYieldHover != ui.yieldHoverIndex || oldTrendHoverSeries != ui.trendHoverSeries || oldTrendHoverIndex != ui.trendHoverIndex {
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
			if ui.archiveDraft != nil {
				archiveVisible := maxInt(1, maxInt(0, resultH-116)/38)
				ui.archiveScroll += step
				ui.clampArchiveScroll(archiveVisible, len(ui.archiveDraft.Assets))
			} else if ui.sellDraft != nil {
				sellVisible := maxInt(1, maxInt(0, resultH-116)/38)
				ui.sellScroll += step
				ui.clampSellScroll(sellVisible, len(ui.sellDraft.Assets))
			} else {
				ui.resultScroll += step
				ui.clampResultScroll(visible, len(resultLines(ui.resultText)))
			}
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
	case dashPageYield:
		bounds := ui.canvas.ClientBoundsPixels()
		left := dashStyle.sidebarWidth + dashStyle.pagePad
		contentW := bounds.Width - left - dashStyle.pagePad
		yieldBoundary := left + maxInt(0, (contentW-dashStyle.gap)/4) + dashStyle.gap/2
		if x < yieldBoundary {
			ui.yieldOptionScroll += step
			ui.clampYieldOptionScroll(7, len(trendSeriesOptions(investmentRecords)))
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
	case "nav-yield":
		ui.page = dashPageYield
		ui.ensureYieldRange()
	case "calc-add":
		ui.clearArchiveDraft()
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
	case "calc-sell":
		ui.openSellConfirmation()
	case "calc-save":
		ui.openArchiveConfirmation()
	case "archive-confirm":
		ui.confirmArchive()
	case "archive-cancel":
		ui.clearArchiveDraft()
		ui.statusText = "已取消保存归档"
	case "sell-confirm":
		ui.confirmSellRecord()
	case "sell-cancel":
		ui.clearArchiveDraft()
		ui.statusText = "已取消期间卖出记录"
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
		ui.trendHoverSeries = -1
		ui.trendHoverIndex = -1
	case "trend-year":
		ui.setTrendRecentYear()
		ui.trendHoverSeries = -1
		ui.trendHoverIndex = -1
	case "yield-toggle":
		yieldSelections[action.Name] = !yieldSelections[action.Name]
		ui.markYieldDirty()
	case "yield-year":
		ui.setYieldRecentYear()
	case "yield-run":
		ui.runYieldCalculation()
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
			ui.clearArchiveDraft()
			ui.investAmount = value
			ui.autoSavePortfolioConfig()
		}
	case "asset-name":
		if field.Index >= 0 && field.Index < len(ui.assets) {
			ui.clearArchiveDraft()
			ui.assets[field.Index].Name = strings.TrimSpace(text)
			ui.autoSavePortfolioConfig()
		}
	case "asset-target":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(ui.assets) {
			ui.clearArchiveDraft()
			ui.assets[field.Index].TargetPct = value
			ui.autoSavePortfolioConfig()
		}
	case "asset-current":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(ui.assets) {
			ui.clearArchiveDraft()
			ui.assets[field.Index].CurrentAmount = value
			ui.autoSavePortfolioConfig()
		}
	case "hist-archive":
		selectedHistoryDraft.ArchivedAt = text
		ui.autoSaveHistoryDraft()
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
	case "hasset-sell":
		if value, ok := parse(); ok && field.Index >= 0 && field.Index < len(selectedHistoryDraft.Assets) {
			selectedAssetIndex = field.Index
			selectedHistoryDraft.Assets[field.Index].SellAmount = roundMoney(value)
			recalculateInvestmentRecord(&selectedHistoryDraft)
			ui.autoSaveHistoryDraft()
		}
	case "archive-buy":
		if ui.archiveDraft == nil || field.Index < 0 || field.Index >= len(ui.archiveDraft.Assets) {
			return
		}
		value := 0.0
		if strings.TrimSpace(text) != "" {
			parsed, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
			if err != nil {
				ui.statusText = "请输入有效数字"
				return
			}
			if parsed < 0 {
				ui.statusText = "真实买入金额不能为负数"
				return
			}
			value = parsed
		}
		ui.archiveDraft.Assets[field.Index].BuyAmount = roundMoney(value)
		recalculateInvestmentRecord(ui.archiveDraft)
		ui.statusText = "已更新真实买入金额"
	case "sell-name":
		if ui.sellDraft == nil || field.Index < 0 || field.Index >= len(ui.sellDraft.Assets) {
			return
		}
		ui.sellDraft.Assets[field.Index].Name = strings.TrimSpace(text)
		recalculateInvestmentRecord(ui.sellDraft)
		ui.statusText = "已更新卖出资产名称"
	case "sell-amount":
		if ui.sellDraft == nil || field.Index < 0 || field.Index >= len(ui.sellDraft.Assets) {
			return
		}
		value := 0.0
		if strings.TrimSpace(text) != "" {
			parsed, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
			if err != nil {
				ui.statusText = "请输入有效数字"
				return
			}
			if parsed < 0 {
				ui.statusText = "卖出金额不能为负数"
				return
			}
			value = parsed
		}
		ui.sellDraft.Assets[field.Index].SellAmount = roundMoney(value)
		recalculateInvestmentRecord(ui.sellDraft)
		ui.statusText = "已更新卖出金额"
	case "sell-time":
		if ui.sellDraft == nil {
			return
		}
		ui.sellDraft.ArchivedAt = strings.TrimSpace(text)
		ui.statusText = "已更新卖出时间"
	case "trend-start":
		ui.trendStartText = text
	case "trend-end":
		ui.trendEndText = text
	case "yield-start":
		ui.yieldStartText = text
		ui.markYieldDirty()
	case "yield-end":
		ui.yieldEndText = text
		ui.markYieldDirty()
	}
}

func (ui *dashboardUI) deleteSelectedAsset() {
	if ui.selectedAsset < 0 || ui.selectedAsset >= len(ui.assets) {
		ui.statusText = "请先选择要删除的资产"
		return
	}
	ui.clearArchiveDraft()
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
	ui.clearArchiveDraft()
	result, err := CalculatePortfolio(ui.investAmount, ui.assets)
	if err != nil {
		ui.statusText = "输入有误：" + err.Error()
		walk.MsgBox(ui.mw, "输入有误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}
	ui.resultText = FormatResult(result)
	ui.statusText = "计算完成：所有目标金额均基于买入后的组合总额"
}

func (ui *dashboardUI) openArchiveConfirmation() {
	result, err := CalculatePortfolio(ui.investAmount, ui.assets)
	if err != nil {
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	suggested := suggestedBuyAmounts(result)
	record, err := recordFromResultWithActualBuys(result, suggested)
	if err != nil {
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	ui.archiveDraft = &record
	ui.archiveSuggested = append([]float64(nil), suggested...)
	ui.archiveScroll = 0
	ui.resultText = FormatResult(result)
	ui.resultScroll = 0
	ui.statusText = "请确认真实买入金额后点击确认保存"
}

func (ui *dashboardUI) openSellConfirmation() {
	record := sellRecordFromAssets(ui.assets, time.Now())
	if len(record.Assets) == 0 {
		ui.statusText = "请先填写资产名称后再记录期间卖出"
		walk.MsgBox(ui.mw, "无法记录期间卖出", "请先填写资产名称后再记录期间卖出", walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}
	ui.clearArchiveDraft()
	ui.sellDraft = &record
	ui.sellScroll = 0
	ui.statusText = "请填写期间卖出金额和卖出时间后点击确认保存"
}

func (ui *dashboardUI) confirmArchive() {
	if ui.archiveDraft == nil {
		ui.openArchiveConfirmation()
		return
	}
	record := cloneInvestmentRecord(*ui.archiveDraft)
	recalculateInvestmentRecord(&record)
	if err := appendInvestmentRecord(record); err != nil {
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	summary := fmt.Sprintf(
		"%s\r\n\r\n真实买入合计：%s 元\r\n买入后总额：%s 元",
		saveArchiveSuccessMessage,
		formatMoney(record.AllocatedCash),
		formatMoney(record.AfterTotal),
	)
	ui.clearArchiveDraft()
	ui.resultText = summary
	ui.resultScroll = 0
	ui.selectHistoryByID(record.ID)
	ui.ensureTrendRange()
	ui.ensureYieldRange()
	ui.markYieldDirty()
	ui.statusText = saveArchiveSuccessMessage
	walk.MsgBox(ui.mw, "保存成功", saveArchiveSuccessMessage, walk.MsgBoxOK|walk.MsgBoxIconInformation)
}

func (ui *dashboardUI) confirmSellRecord() {
	if ui.sellDraft == nil {
		ui.openSellConfirmation()
		return
	}
	record, err := finalizedSellRecord(cloneInvestmentRecord(*ui.sellDraft))
	if err != nil {
		if err.Error() == "没有填写卖出金额，不生成卖出记录" {
			ui.statusText = err.Error()
			walk.MsgBox(ui.mw, "期间卖出", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconInformation)
			return
		}
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	if err := appendInvestmentRecord(record); err != nil {
		ui.statusText = "保存失败：" + err.Error()
		walk.MsgBox(ui.mw, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	summary := fmt.Sprintf(
		"卖出记录已保存，可在历史投资记录中查看\r\n\r\n卖出总金额：%s 元\r\n卖出时间：%s",
		formatMoney(recordSellTotal(record)),
		record.ArchivedAt,
	)
	ui.clearArchiveDraft()
	ui.resultText = summary
	ui.resultScroll = 0
	ui.selectHistoryByID(record.ID)
	ui.ensureTrendRange()
	ui.ensureYieldRange()
	ui.markYieldDirty()
	ui.statusText = "卖出记录已保存"
	walk.MsgBox(ui.mw, "保存成功", "卖出记录已保存，可在历史投资记录中查看", walk.MsgBoxOK|walk.MsgBoxIconInformation)
}

func (ui *dashboardUI) clearArchiveDraft() {
	ui.archiveDraft = nil
	ui.archiveSuggested = nil
	ui.archiveScroll = 0
	ui.sellDraft = nil
	ui.sellScroll = 0
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
	if isSellRecord(record) {
		ui.statusText = "卖出记录不能读取到平衡买入计算"
		return
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

func (ui *dashboardUI) selectHistoryByID(id string) {
	for i := range investmentRecords {
		if investmentRecords[i].ID == id {
			ui.selectHistory(i)
			return
		}
	}
	ui.ensureHistorySelection()
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
	ui.ensureYieldRange()
	ui.markYieldDirty()
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
	ui.ensureTrendRange()
	ui.ensureYieldRange()
	ui.markYieldDirty()
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
	ui.ensureYieldRange()
	ui.markYieldDirty()
	ui.statusText = "历史投资记录已导入：" + dlg.FilePath
}

func (ui *dashboardUI) ensureTrendRange() {
	start, end, ok := trendMonthBounds(investmentRecords)
	if !ok {
		if ui.trendStartText == "" {
			start, end := recentYearMonthRange(time.Now())
			ui.trendStartText = start.Format(trendMonthFmt)
			ui.trendEndText = end.Format(trendMonthFmt)
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
	start, end := recentYearMonthRange(time.Now())
	ui.trendStartText = start.Format(trendMonthFmt)
	ui.trendEndText = end.Format(trendMonthFmt)
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

func (ui *dashboardUI) ensureYieldRange() {
	start, end, ok := trendMonthBounds(investmentRecords)
	if !ok {
		if ui.yieldStartText == "" {
			start, end := recentYearMonthRange(time.Now())
			ui.yieldStartText = start.Format(trendMonthFmt)
			ui.yieldEndText = end.Format(trendMonthFmt)
		}
		syncYieldSelections(trendSeriesOptions(investmentRecords))
		return
	}
	if ui.yieldStartText == "" || ui.yieldEndText == "" {
		ui.yieldStartText = start.Format(trendMonthFmt)
		ui.yieldEndText = end.Format(trendMonthFmt)
	}
	syncYieldSelections(trendSeriesOptions(investmentRecords))
}

func (ui *dashboardUI) setYieldRecentYear() {
	start, end := recentYearMonthRange(time.Now())
	ui.yieldStartText = start.Format(trendMonthFmt)
	ui.yieldEndText = end.Format(trendMonthFmt)
	ui.markYieldDirty()
	ui.statusText = "已切换到最近一年收益测算范围"
}

func recentYearMonthRange(now time.Time) (time.Time, time.Time) {
	end := normalizeTrendMonth(now)
	return end.AddDate(-1, 0, 0), end
}

func (ui *dashboardUI) yieldRange() (time.Time, time.Time, error) {
	startText := strings.TrimSpace(ui.yieldStartText)
	endText := strings.TrimSpace(ui.yieldEndText)
	if startText == "" || endText == "" {
		start, end, ok := trendMonthBounds(investmentRecords)
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("暂无历史记录可测算收益")
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

func (ui *dashboardUI) runYieldCalculation() {
	start, end, err := ui.yieldRange()
	if err != nil {
		ui.yieldCalculated = true
		ui.yieldData = yieldChartData{Message: err.Error()}
		ui.yieldHoverIndex = -1
		ui.statusText = "收益测算失败：" + err.Error()
		walk.MsgBox(ui.mw, "收益测算失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}

	options := trendSeriesOptions(investmentRecords)
	syncYieldSelections(options)
	ui.yieldData = buildYieldChartData(investmentRecords, yieldSelections, start, end)
	ui.yieldCalculated = true
	ui.yieldHoverIndex = -1
	if ui.yieldData.Message != "" && !yieldPointsHaveAny(ui.yieldData.Points) {
		ui.statusText = "收益测算失败：" + ui.yieldData.Message
		return
	}
	latest, ok := latestYieldPoint(ui.yieldData.Points)
	if !ok {
		ui.statusText = "收益测算完成：暂无可显示收益数据"
		return
	}
	ui.statusText = fmt.Sprintf("收益测算完成：%s 最新收益率 %s，年化 %s", ui.yieldData.SelectionLabel, formatYieldRate(latest.Rate), formatYieldRate(latest.AnnualizedRate))
}

func (ui *dashboardUI) markYieldDirty() {
	ui.yieldCalculated = false
	ui.yieldData = yieldChartData{}
	ui.yieldHoverIndex = -1
}

func (ui *dashboardUI) trendPointAt(x, y int) (int, int) {
	if !contains(ui.trendPlot, x, y) || len(ui.trendDisplayData.Months) == 0 {
		return -1, -1
	}
	minValue, maxValue := trendValueRange(ui.trendDisplayData.Series)
	bestSeries := -1
	bestIndex := -1
	bestDistance := 0
	for seriesIndex, series := range ui.trendDisplayData.Series {
		for pointIndex, point := range series.Points {
			if !point.Present {
				continue
			}
			px := trendPointX(ui.trendPlot, pointIndex, len(ui.trendDisplayData.Months))
			py := trendPointY(ui.trendPlot, minValue, maxValue, point.Value)
			dx := x - px
			dy := y - py
			distance := dx*dx + dy*dy
			if distance > 100 {
				continue
			}
			if bestSeries < 0 || distance < bestDistance {
				bestSeries = seriesIndex
				bestIndex = pointIndex
				bestDistance = distance
			}
		}
	}
	return bestSeries, bestIndex
}

func (ui *dashboardUI) yieldPointIndexAt(x, y int) int {
	if !ui.yieldCalculated || !contains(ui.yieldPlot, x, y) || len(ui.yieldData.Months) == 0 {
		return -1
	}
	minValue, maxValue := yieldValueRange(ui.yieldData.Points)
	for i, point := range ui.yieldData.Points {
		if !point.Present {
			continue
		}
		px := trendPointX(ui.yieldPlot, i, len(ui.yieldData.Months))
		py := trendPointY(ui.yieldPlot, minValue, maxValue, point.Rate)
		if absInt(x-px) <= 8 && absInt(y-py) <= 8 {
			return i
		}
	}
	return -1
}

func (ui *dashboardUI) clampAssetScroll(visible int) {
	ui.assetScroll = clampInt(ui.assetScroll, 0, maxInt(0, len(ui.assets)-visible))
}

func (ui *dashboardUI) clampResultScroll(visible, total int) {
	ui.resultScroll = clampInt(ui.resultScroll, 0, maxInt(0, total-visible))
}

func (ui *dashboardUI) clampArchiveScroll(visible, total int) {
	ui.archiveScroll = clampInt(ui.archiveScroll, 0, maxInt(0, total-visible))
}

func (ui *dashboardUI) clampSellScroll(visible, total int) {
	ui.sellScroll = clampInt(ui.sellScroll, 0, maxInt(0, total-visible))
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

func (ui *dashboardUI) clampYieldOptionScroll(visible, total int) {
	ui.yieldOptionScroll = clampInt(ui.yieldOptionScroll, 0, maxInt(0, total-visible))
}

func (ui *dashboardUI) setScrollOffset(key string, offset, total, visible int) {
	offset = clampInt(offset, 0, maxInt(0, total-visible))
	switch key {
	case "scroll-assets":
		ui.assetScroll = offset
	case "scroll-result":
		ui.resultScroll = offset
	case "scroll-archive-buy":
		ui.archiveScroll = offset
	case "scroll-sell":
		ui.sellScroll = offset
	case "scroll-history-list":
		ui.historyScroll = offset
	case "scroll-history-detail":
		ui.historyAssetScroll = offset
	case "scroll-trend-options":
		ui.trendOptionScroll = offset
	case "scroll-yield-options":
		ui.yieldOptionScroll = offset
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

func measureTextWidth(canvas *walk.Canvas, text string, font *walk.Font) int {
	if canvas == nil || font == nil || text == "" {
		return 0
	}
	measured, _, err := canvas.MeasureTextPixels(text, font, walk.Rectangle{Width: 999999}, walk.TextCalcRect)
	if err != nil {
		return 0
	}
	return measured.Width
}

func measureTextSize(canvas *walk.Canvas, text string, font *walk.Font) walk.Size {
	if canvas == nil || font == nil || text == "" {
		return walk.Size{}
	}
	measured, _, err := canvas.MeasureTextPixels(text, font, walk.Rectangle{Width: 999999}, walk.TextCalcRect)
	if err != nil {
		return walk.Size{}
	}
	return walk.Size{Width: measured.Width, Height: measured.Height}
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

type chartPointF struct {
	X float64
	Y float64
}

func drawAntialiasedLine(img *image.RGBA, from, to chartPointF, width float64, color color.RGBA) {
	dx := to.X - from.X
	dy := to.Y - from.Y
	lengthSq := dx*dx + dy*dy
	if lengthSq <= 0.000001 {
		drawAntialiasedDot(img, from, width/2, color)
		return
	}

	radius := width / 2
	minX := int(math.Floor(math.Min(from.X, to.X) - radius - 1))
	maxX := int(math.Ceil(math.Max(from.X, to.X) + radius + 1))
	minY := int(math.Floor(math.Min(from.Y, to.Y) - radius - 1))
	maxY := int(math.Ceil(math.Max(from.Y, to.Y) + radius + 1))

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			t := ((px-from.X)*dx + (py-from.Y)*dy) / lengthSq
			t = clampFloat(t, 0, 1)
			closestX := from.X + t*dx
			closestY := from.Y + t*dy
			distance := math.Hypot(px-closestX, py-closestY)
			coverage := clampFloat(radius+0.5-distance, 0, 1)
			blendImagePixel(img, x, y, color, coverage)
		}
	}
}

func drawAntialiasedDot(img *image.RGBA, center chartPointF, radius float64, color color.RGBA) {
	minX := int(math.Floor(center.X - radius - 1))
	maxX := int(math.Ceil(center.X + radius + 1))
	minY := int(math.Floor(center.Y - radius - 1))
	maxY := int(math.Ceil(center.Y + radius + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			distance := math.Hypot(px-center.X, py-center.Y)
			coverage := clampFloat(radius+0.5-distance, 0, 1)
			blendImagePixel(img, x, y, color, coverage)
		}
	}
}

func blendImagePixel(img *image.RGBA, x, y int, color color.RGBA, coverage float64) {
	if coverage <= 0 || !image.Pt(x, y).In(img.Bounds()) {
		return
	}

	offset := img.PixOffset(x, y)
	srcA := float64(color.A) / 255 * coverage
	dstA := float64(img.Pix[offset+3]) / 255
	outA := srcA + dstA*(1-srcA)
	if outA <= 0 {
		return
	}

	dstR := float64(img.Pix[offset])
	dstG := float64(img.Pix[offset+1])
	dstB := float64(img.Pix[offset+2])
	img.Pix[offset] = byte(math.Round((float64(color.R)*srcA + dstR*dstA*(1-srcA)) / outA))
	img.Pix[offset+1] = byte(math.Round((float64(color.G)*srcA + dstG*dstA*(1-srcA)) / outA))
	img.Pix[offset+2] = byte(math.Round((float64(color.B)*srcA + dstB*dstA*(1-srcA)) / outA))
	img.Pix[offset+3] = byte(math.Round(outA * 255))
}

func rgbaFromWalkColor(value walk.Color) color.RGBA {
	return color.RGBA{R: value.R(), G: value.G(), B: value.B(), A: 255}
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
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
