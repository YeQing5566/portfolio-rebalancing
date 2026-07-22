package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lxn/walk"
	"github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

type transactionDialogUI struct {
	owner        *dashboardUI
	dlg          *walk.Dialog
	canvas       *walk.CustomWidget
	editorLayer  *walk.Composite
	editor       *walk.LineEdit
	record       InvestmentRecord
	result       InvestmentRecord
	isSell       bool
	title        string
	amountTitle  string
	saved        bool
	scroll       int
	hoverAction  string
	actions      []dashAction
	fields       []dashField
	activeField  *dashField
	regionWidth  int
	regionHeight int
}

const (
	transactionDialogWidth          = 620
	transactionDialogRowHeight      = 40
	transactionDialogVerticalChrome = 200
)

// showTransactionRecordDialog uses the same self-painted visual system as the
// dashboard. The modal remains independent from the recommendation card.
func showTransactionRecordDialog(owner *dashboardUI, record InvestmentRecord) (InvestmentRecord, bool, error) {
	if owner == nil || owner.mw == nil {
		return InvestmentRecord{}, false, fmt.Errorf("主程序窗口尚未就绪")
	}
	if len(record.Assets) == 0 {
		return InvestmentRecord{}, false, fmt.Errorf("请先填写资产名称")
	}

	ui := &transactionDialogUI{
		owner:       owner,
		record:      cloneInvestmentRecord(record),
		isSell:      isSellRecord(record),
		title:       "记录买入",
		amountTitle: "买入金额",
	}
	if ui.isSell {
		ui.title = "记录卖出"
		ui.amountTitle = "卖出金额"
	}

	width, height := transactionDialogSize(owner)
	dialogDef := declarative.Dialog{
		AssignTo:   &ui.dlg,
		Title:      ui.title,
		FixedSize:  true,
		Size:       declarative.Size{Width: width, Height: height},
		MinSize:    declarative.Size{Width: width, Height: height},
		MaxSize:    declarative.Size{Width: width, Height: height},
		Font:       declarative.Font{Family: "Microsoft YaHei UI", PointSize: 10},
		Background: declarative.SolidColorBrush{Color: dashColors.shell},
		Layout:     declarative.VBox{MarginsZero: true, SpacingZero: true},
		Children: []declarative.Widget{
			declarative.CustomWidget{
				AssignTo:            &ui.canvas,
				InvalidatesOnResize: true,
				PaintMode:           declarative.PaintBuffered,
				PaintPixels:         ui.paint,
				StretchFactor:       1,
			},
		},
	}
	if err := dialogDef.Create(owner.mw); err != nil {
		return InvestmentRecord{}, false, err
	}
	ui.installBorderlessWindow()
	if err := ui.dlg.SetMinMaxSizePixels(walk.Size{Width: width, Height: height}, walk.Size{Width: width, Height: height}); err != nil {
		ui.dlg.Dispose()
		return InvestmentRecord{}, false, err
	}
	if err := ui.dlg.SetClientSizePixels(walk.Size{Width: width, Height: height}); err != nil {
		ui.dlg.Dispose()
		return InvestmentRecord{}, false, err
	}
	ui.installEditor()
	ui.attachEvents()
	if icon, err := walk.NewIconFromResourceId(appIconResourceID); err == nil {
		_ = ui.dlg.SetIcon(icon)
		defer icon.Dispose()
	}
	ui.invalidate()
	ui.dlg.Run()
	return ui.result, ui.saved, nil
}

func transactionDialogSize(owner *dashboardUI) (int, int) {
	cardHeight := owner.assetTableHeight
	if cardHeight <= 0 {
		cardHeight = defaultCalculatorTopCardHeight
	}
	cardHeight = maxInt(calculatorMinTopCardHeight, cardHeight)
	visibleRows := assetTableVisibleRows(cardHeight)
	return transactionDialogWidth, transactionDialogHeightForRows(visibleRows)
}

func transactionDialogHeightForRows(visibleRows int) int {
	return transactionDialogVerticalChrome + maxInt(1, visibleRows)*transactionDialogRowHeight
}

func transactionDialogVisibleRowsForHeight(height int) int {
	return maxInt(1, maxInt(0, height-transactionDialogVerticalChrome)/transactionDialogRowHeight)
}

func (ui *transactionDialogUI) installBorderlessWindow() {
	hwnd := ui.dlg.Handle()
	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	style &^= win.WS_CAPTION | win.WS_THICKFRAME | win.WS_SYSMENU | win.WS_MINIMIZEBOX | win.WS_MAXIMIZEBOX
	style |= win.WS_POPUP
	win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func (ui *transactionDialogUI) installEditor() {
	layer, err := walk.NewCompositeWithStyle(ui.dlg, 0)
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
	ui.editor.SetFont(ui.owner.fontSmall)
	ui.editor.KeyDown().Attach(func(key walk.Key) {
		switch key {
		case walk.KeyReturn:
			ui.commitEditor()
		case walk.KeyEscape:
			ui.cancelEditor()
		}
	})
}

func (ui *transactionDialogUI) makeEditorBorderless() {
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

func (ui *transactionDialogUI) attachEvents() {
	ui.canvas.MouseDown().Attach(ui.handleMouseDown)
	ui.canvas.MouseMove().Attach(ui.handleMouseMove)
	ui.canvas.MouseWheel().Attach(ui.handleMouseWheel)
}

func (ui *transactionDialogUI) invalidate() {
	if ui.canvas != nil {
		_ = ui.canvas.Invalidate()
	}
}

func (ui *transactionDialogUI) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	bounds := ui.canvas.ClientBoundsPixels()
	ui.applyRoundedRegion(bounds)
	ui.actions = ui.actions[:0]
	ui.fields = ui.fields[:0]
	fill(canvas, dashColors.shell, bounds)
	_ = canvas.GradientFillRectanglePixels(dashColors.bg, dashColors.shell, walk.Vertical, bounds)

	drawText(canvas, ui.title, ui.owner.fontTitle, dashColors.text, rect(18, 10, bounds.Width-82, 36), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	ui.drawCloseButton(canvas, rect(bounds.Width-46, 12, 28, 28))

	contentX := dashStyle.cardPad
	contentW := maxInt(0, bounds.Width-dashStyle.cardPad*2)
	timeY := 52
	drawText(canvas, "记录时间", ui.owner.fontSmall, dashColors.muted, rect(contentX, timeY, 78, 34), walk.TextLeft|walk.TextVCenter)
	ui.drawField(canvas, rect(contentX+82, timeY, maxInt(0, contentW-82), 34), "transaction-time", -1, ui.record.ArchivedAt, "2026-05-29 11:41:16", false)

	tableY := 102
	tableW := contentW
	amountW := 190
	widths := []int{44, maxInt(170, tableW-44-amountW), amountW}
	drawTableHeader(canvas, ui.owner.fontTiny, contentX, tableY, widths, []string{"#", "资产名称", ui.amountTitle})

	rowY := tableY + 32
	rowH := transactionDialogRowHeight
	buttonY := bounds.Height - dashStyle.buttonHeight - 16
	visible := transactionDialogVisibleRowsForHeight(bounds.Height)
	ui.clampScroll(visible)
	for i := 0; i < visible; i++ {
		index := ui.scroll + i
		if index >= len(ui.record.Assets) {
			break
		}
		asset := ui.record.Assets[index]
		row := rect(contentX, rowY+i*rowH, sumInts(widths), rowH-6)
		bg := dashColors.panel2
		if i%2 == 1 {
			bg = walk.RGB(21, 21, 21)
		}
		roundFill(canvas, bg, row, 9)
		cx := contentX
		drawText(canvas, strconv.Itoa(index+1), ui.owner.fontSmall, dashColors.muted, rect(cx, row.Y, widths[0], row.Height), walk.TextCenter|walk.TextVCenter)
		cx += widths[0]
		drawText(canvas, asset.Name, ui.owner.fontSmall, dashColors.text, rect(cx+8, row.Y, widths[1]-16, row.Height), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
		cx += widths[1]
		amount := asset.BuyAmount
		if ui.isSell {
			amount = asset.SellAmount
		}
		field := rect(cx+4, row.Y+4, maxInt(0, widths[2]-38), 26)
		ui.drawField(canvas, field, "transaction-amount", index, formatMaybeFloat(amount), "0", true)
		drawText(canvas, "元", ui.owner.fontSmall, dashColors.muted, rect(field.X+field.Width+6, row.Y, 24, row.Height), walk.TextLeft|walk.TextVCenter)
	}
	ui.drawScrollBar(canvas, rect(bounds.Width-11, rowY, 5, maxInt(0, buttonY-rowY-8)), len(ui.record.Assets), visible)

	buttonW := 104
	confirmX := bounds.Width - dashStyle.cardPad - buttonW
	cancelX := confirmX - 12 - buttonW
	ui.drawButton(canvas, rect(cancelX, buttonY, buttonW, dashStyle.buttonHeight), "取消", "transaction-cancel")
	ui.drawButton(canvas, rect(confirmX, buttonY, buttonW, dashStyle.buttonHeight), "确认保存", "transaction-confirm")

	ui.owner.drawWindowFrame(canvas, bounds)
	return nil
}

func (ui *transactionDialogUI) applyRoundedRegion(bounds walk.Rectangle) {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	if ui.regionWidth == bounds.Width && ui.regionHeight == bounds.Height {
		return
	}
	rgn, _, _ := createRoundRectRgnProc.Call(0, 0, uintptr(bounds.Width+1), uintptr(bounds.Height+1), 28, 28)
	if rgn == 0 {
		return
	}
	ret, _, _ := setWindowRgnProc.Call(uintptr(ui.dlg.Handle()), rgn, 1)
	if ret == 0 {
		win.DeleteObject(win.HGDIOBJ(rgn))
		return
	}
	ui.regionWidth = bounds.Width
	ui.regionHeight = bounds.Height
}

func (ui *transactionDialogUI) drawCloseButton(canvas *walk.Canvas, r walk.Rectangle) {
	bg := walk.RGB(24, 24, 24)
	border := dashColors.line2
	if ui.hoverAction == "transaction-close" {
		bg = walk.RGB(54, 54, 54)
		border = walk.RGB(110, 110, 110)
	}
	roundFill(canvas, bg, r, 14)
	drawRoundStroke(canvas, border, r, 14, 1)
	cx := r.X + r.Width/2
	cy := r.Y + r.Height/2
	drawLine(canvas, dashColors.white, cx-5, cy-5, cx+5, cy+5, 2)
	drawLine(canvas, dashColors.white, cx+5, cy-5, cx-5, cy+5, 2)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: "transaction-close"})
}

func (ui *transactionDialogUI) drawButton(canvas *walk.Canvas, r walk.Rectangle, label, key string) {
	bg := dashColors.card2
	border := dashColors.line2
	if ui.hoverAction == key {
		bg = walk.RGB(44, 44, 44)
		border = walk.RGB(78, 78, 78)
	}
	roundFill(canvas, bg, r, dashStyle.buttonRadius)
	drawRoundStroke(canvas, border, r, dashStyle.buttonRadius, 1)
	drawText(canvas, label, ui.owner.fontBold, dashColors.text, r, walk.TextCenter|walk.TextVCenter|walk.TextEndEllipsis)
	ui.actions = append(ui.actions, dashAction{Rect: r, Kind: dashActionButton, Key: key})
}

func (ui *transactionDialogUI) drawField(canvas *walk.Canvas, r walk.Rectangle, key string, index int, value, placeholder string, numeric bool) {
	roundFill(canvas, fieldFillColor(), r, 8)
	border := dashColors.line
	field := dashField{Key: key, Index: index}
	if ui.activeField != nil && sameField(*ui.activeField, field) {
		border = dashColors.line2
	}
	drawRoundStroke(canvas, border, r, 8, 1)
	display := strings.TrimSpace(value)
	textColor := dashColors.text
	if display == "" {
		display = placeholder
		textColor = dashColors.faint
	}
	drawText(canvas, display, ui.owner.fontSmall, textColor, fieldTextRect(r), walk.TextLeft|walk.TextVCenter|walk.TextEndEllipsis)
	ui.fields = append(ui.fields, dashField{Rect: r, Key: key, Index: index, Value: value, Placeholder: placeholder, Numeric: numeric})
}

func (ui *transactionDialogUI) drawScrollBar(canvas *walk.Canvas, track walk.Rectangle, total, visible int) {
	if total <= visible || visible <= 0 || track.Height <= 0 {
		return
	}
	maxOffset := total - visible
	thumbH := clampInt(track.Height*visible/total, minInt(24, track.Height), track.Height)
	span := maxInt(1, track.Height-thumbH)
	thumbY := track.Y
	if maxOffset > 0 {
		thumbY += span * ui.scroll / maxOffset
	}
	roundFill(canvas, walk.RGB(32, 32, 32), track, 3)
	roundFill(canvas, walk.RGB(112, 112, 112), rect(track.X, thumbY, track.Width, thumbH), 3)
}

func (ui *transactionDialogUI) handleMouseDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	for _, field := range ui.fields {
		if contains(field.Rect, x, y) {
			if ui.activeField != nil && !sameField(*ui.activeField, field) && !ui.commitEditor() {
				return
			}
			ui.beginEdit(field)
			return
		}
	}
	for _, action := range ui.actions {
		if !contains(action.Rect, x, y) {
			continue
		}
		switch action.Key {
		case "transaction-close", "transaction-cancel":
			ui.cancelEditor()
			ui.dlg.Cancel()
		case "transaction-confirm":
			if !ui.commitEditor() {
				return
			}
			ui.accept()
		}
		return
	}
	if !ui.commitEditor() {
		return
	}
	if y < 48 {
		win.ReleaseCapture()
		win.SendMessage(ui.dlg.Handle(), win.WM_NCLBUTTONDOWN, uintptr(win.HTCAPTION), 0)
	}
}

func (ui *transactionDialogUI) handleMouseMove(x, y int, _ walk.MouseButton) {
	hover := ""
	for _, action := range ui.actions {
		if contains(action.Rect, x, y) {
			hover = action.Key
			break
		}
	}
	if hover != ui.hoverAction {
		ui.hoverAction = hover
		ui.invalidate()
	}
}

func (ui *transactionDialogUI) handleMouseWheel(x, y int, button walk.MouseButton) {
	if ui.canvas != nil {
		point := win.POINT{X: int32(x), Y: int32(y)}
		if win.ScreenToClient(ui.canvas.Handle(), &point) {
			x, y = int(point.X), int(point.Y)
		}
	}
	_ = x
	if y < 100 {
		return
	}
	if !ui.commitEditor() {
		return
	}
	visible := ui.visibleRows()
	if walk.MouseWheelEventDelta(button) < 0 {
		ui.scroll++
	} else {
		ui.scroll--
	}
	ui.clampScroll(visible)
	ui.invalidate()
}

func (ui *transactionDialogUI) visibleRows() int {
	if ui.canvas == nil {
		return 1
	}
	return transactionDialogVisibleRowsForHeight(ui.canvas.ClientBoundsPixels().Height)
}

func (ui *transactionDialogUI) clampScroll(visible int) {
	ui.scroll = clampInt(ui.scroll, 0, maxInt(0, len(ui.record.Assets)-visible))
}

func (ui *transactionDialogUI) beginEdit(field dashField) {
	if ui.editor == nil || ui.editorLayer == nil {
		return
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
	ui.invalidate()
}

func (ui *transactionDialogUI) commitEditor() bool {
	if ui.activeField == nil || ui.editor == nil {
		return true
	}
	field := *ui.activeField
	text := strings.TrimSpace(ui.editor.Text())
	if strings.TrimSpace(field.Value) == "" && text == strings.TrimSpace(field.Placeholder) {
		text = ""
	}
	valid := ui.applyField(field, text)
	if !valid {
		return false
	}
	ui.editor.SetVisible(false)
	if ui.editorLayer != nil {
		ui.editorLayer.SetVisible(false)
	}
	ui.activeField = nil
	ui.invalidate()
	return true
}

func (ui *transactionDialogUI) cancelEditor() {
	if ui.editor != nil {
		ui.editor.SetVisible(false)
	}
	if ui.editorLayer != nil {
		ui.editorLayer.SetVisible(false)
	}
	ui.activeField = nil
	ui.invalidate()
}

func (ui *transactionDialogUI) applyField(field dashField, text string) bool {
	switch field.Key {
	case "transaction-time":
		ui.record.ArchivedAt = strings.TrimSpace(text)
		return true
	case "transaction-amount":
		if field.Index < 0 || field.Index >= len(ui.record.Assets) {
			return false
		}
		amount := 0.0
		text = strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
		if text != "" {
			parsed, err := strconv.ParseFloat(text, 64)
			if err != nil || parsed < 0 {
				walk.MsgBox(ui.dlg, "输入有误", fmt.Sprintf("%s 的%s必须是非负数字", ui.record.Assets[field.Index].Name, ui.amountTitle), walk.MsgBoxOK|walk.MsgBoxIconWarning)
				return false
			}
			amount = roundMoney(parsed)
		}
		if ui.isSell {
			ui.record.Assets[field.Index].SellAmount = amount
		} else {
			ui.record.Assets[field.Index].BuyAmount = amount
		}
		return true
	}
	return false
}

func (ui *transactionDialogUI) accept() {
	if !ui.commitEditor() {
		return
	}
	candidate := cloneInvestmentRecord(ui.record)
	var err error
	if ui.isSell {
		candidate, err = finalizedSellRecord(candidate)
	} else {
		candidate, err = finalizedBuyRecord(candidate)
	}
	if err != nil {
		walk.MsgBox(ui.dlg, "无法保存", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return
	}
	ui.result = candidate
	ui.saved = true
	ui.dlg.Accept()
}
