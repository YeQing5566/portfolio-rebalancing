package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type AssetTableModel struct {
	walk.TableModelBase
	items []AssetInput
}

func (m *AssetTableModel) RowCount() int {
	return len(m.items)
}

func (m *AssetTableModel) Value(row, col int) interface{} {
	item := m.items[row]
	isBlank := strings.TrimSpace(item.Name) == "" && item.TargetPct == 0 && item.CurrentAmount == 0
	switch col {
	case 0:
		return row + 1
	case 1:
		return item.Name
	case 2:
		if isBlank {
			return ""
		}
		return formatPercent(item.TargetPct)
	case 3:
		if isBlank {
			return ""
		}
		return formatMoney(item.CurrentAmount) + " 元"
	case 4:
		if isBlank {
			return ""
		}
		return formatPercent(currentPctForInputs(m.items, row))
	default:
		return ""
	}
}

func (m *AssetTableModel) Add(item AssetInput) {
	index := len(m.items)
	m.items = append(m.items, item)
	m.PublishRowsInserted(index, index)
}

func (m *AssetTableModel) Update(index int, item AssetInput) {
	m.items[index] = item
	m.PublishRowChanged(index)
}

func (m *AssetTableModel) Remove(index int) {
	m.items = append(m.items[:index], m.items[index+1:]...)
	m.PublishRowsRemoved(index, index)
	m.PublishRowsReset()
}

func (m *AssetTableModel) SetItems(items []AssetInput) {
	m.items = append([]AssetInput(nil), items...)
	m.PublishRowsReset()
}

func (m *AssetTableModel) ItemsCopy() []AssetInput {
	return append([]AssetInput(nil), m.items...)
}

func (m *AssetTableModel) RefreshAll() {
	if len(m.items) > 0 {
		m.PublishRowsChanged(0, len(m.items)-1)
	}
}

var (
	mainWindow        *walk.MainWindow
	mainTabs          *walk.TabWidget
	investAmountEdit  *walk.NumberEdit
	assetTable        *walk.TableView
	assetModel        = &AssetTableModel{}
	assetSummaryLabel *walk.Label
	inlineEditor      *walk.Composite
	editorStateLabel  *walk.Label
	assetNameEdit     *walk.LineEdit
	assetTargetEdit   *walk.NumberEdit
	assetAmountEdit   *walk.NumberEdit
	resultEdit        *walk.TextEdit
	statusBarItem     *walk.StatusBarItem
	editingIndex      = -1
	editorOriginal    AssetInput
	editorIsNew       bool
	loadingEditor     bool
	loadingPortfolio  bool
	defaultTextColor  = walk.RGB(245, 245, 245)
	mutedTextColor    = walk.RGB(163, 163, 163)
	accentColor       = walk.RGB(255, 153, 0)
	secondaryColor    = walk.RGB(255, 177, 59)
	warningColor      = walk.RGB(251, 191, 36)
	dangerColor       = walk.RGB(239, 68, 68)
	windowBackground  = SolidColorBrush{Color: walk.RGB(8, 8, 8)}
	panelBackground   = SolidColorBrush{Color: walk.RGB(18, 18, 18)}
	headerBackground  = SolidColorBrush{Color: walk.RGB(0, 0, 0)}
	resultBackground  = SolidColorBrush{Color: walk.RGB(18, 18, 18)}
	fieldBackground   = SolidColorBrush{Color: walk.RGB(31, 31, 31)}
	tableBackground   = SolidColorBrush{Color: walk.RGB(18, 18, 18)}
	tableRowColor     = walk.RGB(21, 21, 21)
	tableAltRowColor  = walk.RGB(26, 26, 26)
	tableSelectColor  = walk.RGB(49, 34, 12)
)

func runClassicApp() {
	walk.AppendToWalkInit(func() {
		walk.FocusEffect, _ = walk.NewBorderGlowEffect(accentColor)
		walk.ValidationErrorEffect, _ = walk.NewBorderGlowEffect(walk.RGB(210, 55, 55))
	})

	window := MainWindow{
		AssignTo:   &mainWindow,
		Title:      "投资组合再平衡助手",
		MinSize:    Size{Width: 1100, Height: 720},
		Size:       Size{Width: 1260, Height: 860},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 10},
		Background: windowBackground,
		Layout: VBox{
			Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 10},
			Spacing: 10,
		},
		StatusBarItems: []StatusBarItem{
			{
				AssignTo: &statusBarItem,
				Text:     "",
				Width:    780,
			},
		},
		Children: []Widget{
			buildHeader(),
			TabWidget{
				AssignTo:           &mainTabs,
				ContentMarginsZero: true,
				Background:         windowBackground,
				StretchFactor:      1,
				OnCurrentIndexChanged: func() {
					switch mainTabs.CurrentIndex() {
					case 1:
						refreshHistoryView()
					case 2:
						refreshTrendView()
					}
				},
				Pages: []TabPage{
					{
						Title:      "平衡买入计算",
						Background: windowBackground,
						Layout:     VBox{Margins: Margins{Left: 0, Top: 8, Right: 0, Bottom: 0}},
						Children: []Widget{
							buildCalculatorPage(),
						},
					},
					{
						Title:      "历史投资记录",
						Background: windowBackground,
						Layout:     VBox{Margins: Margins{Left: 0, Top: 8, Right: 0, Bottom: 0}},
						Children: []Widget{
							buildHistoryPage(),
						},
					},
					{
						Title:      "历史资产趋势",
						Background: windowBackground,
						Layout:     VBox{Margins: Margins{Left: 0, Top: 8, Right: 0, Bottom: 0}},
						Children: []Widget{
							buildTrendPage(),
						},
					},
				},
			},
		},
	}

	if err := window.Create(); err != nil {
		showStartupError(err)
	}

	loadPortfolioIntoClassic()
	refreshAssetSummary()
	if err := loadInvestmentRecords(); err != nil {
		statusBarItem.SetText("历史记录读取失败：" + err.Error())
	}
	refreshTrendView()
	mainWindow.Run()
}

func buildCalculatorPage() Widget {
	return VSplitter{
		HandleWidth:   8,
		StretchFactor: 1,
		Background:    windowBackground,
		Children: []Widget{
			Composite{
				StretchFactor: 2,
				Layout: VBox{
					MarginsZero: true,
					Spacing:     10,
				},
				Children: []Widget{
					Composite{
						StretchFactor: 1,
						Background:    windowBackground,
						Layout: HBox{
							MarginsZero: true,
							Spacing:     10,
						},
						Children: []Widget{
							buildOverviewPanel(),
							buildAssetPanel(),
						},
					},
					buildActionBar(),
				},
			},
			Composite{
				MinSize:       Size{Height: 160},
				StretchFactor: 3,
				Background:    panelBackground,
				Border:        true,
				Layout: VBox{
					Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 14},
					Spacing: 8,
				},
				Children: []Widget{
					Composite{
						Background: panelBackground,
						Layout: HBox{
							MarginsZero: true,
							Spacing:     8,
						},
						Children: []Widget{
							Label{
								Text:      "再平衡建议",
								TextColor: defaultTextColor,
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
							},
							Label{
								Text:      "买入金额、预计仓位和偏离提醒",
								TextColor: mutedTextColor,
							},
							HSpacer{},
							Label{
								Text:      "结果区",
								TextColor: accentColor,
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
							},
						},
					},
					TextEdit{
						AssignTo:      &resultEdit,
						ReadOnly:      true,
						VScroll:       true,
						Background:    resultBackground,
						TextColor:     defaultTextColor,
						Font:          Font{Family: "NSimSun", PointSize: 10},
						StretchFactor: 1,
						Text:          initialResultText(),
					},
				},
			},
		},
	}
}

func buildHeader() Widget {
	return Composite{
		MinSize:    Size{Height: 70},
		Background: headerBackground,
		Border:     true,
		Layout: HBox{
			Margins: Margins{Left: 18, Top: 12, Right: 18, Bottom: 12},
			Spacing: 16,
		},
		Children: []Widget{
			Label{
				Text:      "▰",
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 19, Bold: true},
				TextColor: accentColor,
				MinSize:   Size{Width: 24},
			},
			Composite{
				Background: headerBackground,
				Layout: VBox{
					MarginsZero: true,
					Spacing:     2,
				},
				Children: []Widget{
					Label{
						Text:      "组合再平衡工作台",
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 16, Bold: true},
						TextColor: defaultTextColor,
					},
					Label{
						Text:      "面向长期定投与仓位校准：先录入资产，再计算本次买入方案。",
						TextColor: mutedTextColor,
					},
				},
			},
			HSpacer{},
			Label{
				Text:      "Windows 本地版",
				TextColor: secondaryColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
			},
		},
	}
}

func buildOverviewPanel() Widget {
	return Composite{
		MinSize:    Size{Width: 265, Height: 205},
		MaxSize:    Size{Width: 310, Height: 1000},
		Background: panelBackground,
		Border:     true,
		Layout: VBox{
			Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 14},
			Spacing: 10,
		},
		Children: []Widget{
			Label{
				Text:      "本次投入",
				TextColor: defaultTextColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
			},
			Label{
				Text:      "本次可投入金额",
				TextColor: mutedTextColor,
			},
			NumberEdit{
				AssignTo:           &investAmountEdit,
				Value:              5000,
				Decimals:           2,
				Increment:          500,
				MinValue:           0,
				MaxValue:           1_000_000_000,
				SpinButtonsVisible: false,
				Suffix:             " 元",
				ToolTipText:        "本次准备投入组合的新增资金。",
				Background:         fieldBackground,
				TextColor:          defaultTextColor,
				Font:               Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true},
				MinSize:            Size{Height: 34},
				OnValueChanged: func() {
					saveCurrentPortfolioFromClassic()
				},
			},
			VSpacer{Size: 2},
			Label{
				Text:      "资产概览",
				TextColor: defaultTextColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
			},
			Label{
				AssignTo:  &assetSummaryLabel,
				Text:      "资产数量：0 项\r\n目标合计：0%\r\n当前总额：0 元",
				TextColor: accentColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 11, Bold: true},
				MinSize:   Size{Height: 78},
			},
			VSpacer{},
		},
	}
}

func buildAssetPanel() Widget {
	return Composite{
		MinSize:       Size{Height: 205},
		StretchFactor: 4,
		Background:    panelBackground,
		Border:        true,
		Layout: VBox{
			Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 12},
			Spacing: 8,
		},
		Children: []Widget{
			Composite{
				Background: panelBackground,
				Layout: HBox{
					MarginsZero: true,
					Spacing:     8,
				},
				Children: []Widget{
					Label{
						Text:      "资产条目",
						TextColor: defaultTextColor,
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
					},
					Label{
						Text:      "双击行可编辑",
						TextColor: mutedTextColor,
					},
					HSpacer{},
					PushButton{
						Text:    "＋ 添加资产",
						MinSize: Size{Width: 112, Height: 28},
						Font:    Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
						OnClicked: func() {
							addAsset()
						},
					},
					PushButton{
						Text:    "删除",
						MinSize: Size{Width: 76, Height: 28},
						OnClicked: func() {
							removeSelectedAsset()
						},
					},
				},
			},
			TableView{
				AssignTo:                    &assetTable,
				Model:                       assetModel,
				Background:                  tableBackground,
				AlternatingRowBG:            true,
				LastColumnStretched:         true,
				NotSortableByHeaderClick:    true,
				SelectionHiddenWithoutFocus: false,
				CustomRowHeight:             30,
				MinSize:                     Size{Height: 118},
				Columns: []TableViewColumn{
					{Title: "#", Width: 44, Alignment: AlignCenter},
					{Title: "资产名称", Width: 260},
					{Title: "目标仓位", Width: 140, Alignment: AlignFar},
					{Title: "当前持有金额", Width: 190, Alignment: AlignFar},
					{Title: "自动计算当前仓位", Width: 190, Alignment: AlignFar},
				},
				StyleCell: styleAssetTableCell,
				OnItemActivated: func() {
					editSelectedAsset()
				},
			},
			buildInlineEditor(),
		},
	}
}

func styleAssetTableCell(style *walk.CellStyle) {
	styleDarkTableCell(style, currentTableIndex(assetTable))
	row := style.Row()
	if row < 0 || row >= len(assetModel.items) {
		return
	}
	item := assetModel.items[row]
	if isBlankAsset(item) {
		style.TextColor = mutedTextColor
		return
	}
	switch style.Col() {
	case 2, 4:
		style.TextColor = accentColor
	case 3:
		style.TextColor = secondaryColor
	}
}

func styleDarkTableCell(style *walk.CellStyle, currentIndex int) {
	row := style.Row()
	if row < 0 {
		return
	}
	if row == currentIndex {
		style.BackgroundColor = tableSelectColor
		style.TextColor = defaultTextColor
		return
	}
	if row%2 == 0 {
		style.BackgroundColor = tableRowColor
	} else {
		style.BackgroundColor = tableAltRowColor
	}
	style.TextColor = defaultTextColor
}

func currentTableIndex(table *walk.TableView) int {
	if table == nil {
		return -1
	}
	return table.CurrentIndex()
}

func statusTextColor(status string) walk.Color {
	switch {
	case strings.Contains(status, "严重超配"):
		return dangerColor
	case strings.Contains(status, "严重低配"):
		return warningColor
	case strings.Contains(status, "略高"):
		return warningColor
	case strings.Contains(status, "略低"):
		return secondaryColor
	case strings.Contains(status, "接近"):
		return accentColor
	default:
		return defaultTextColor
	}
}

func buildInlineEditor() Widget {
	return Composite{
		AssignTo:   &inlineEditor,
		Visible:    false,
		Background: fieldBackground,
		Border:     true,
		Layout: HBox{
			Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 8},
			Spacing: 8,
		},
		Children: []Widget{
			Label{
				AssignTo:  &editorStateLabel,
				Text:      "编辑",
				TextColor: accentColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
				MinSize:   Size{Width: 64},
			},
			LineEdit{
				AssignTo:      &assetNameEdit,
				CueBanner:     "资产名称",
				MinSize:       Size{Width: 210},
				StretchFactor: 2,
				Background:    resultBackground,
				TextColor:     defaultTextColor,
				OnTextChanged: syncInlineEditor,
			},
			NumberEdit{
				AssignTo:           &assetTargetEdit,
				Decimals:           2,
				Increment:          1,
				MinValue:           0,
				MaxValue:           100,
				SpinButtonsVisible: false,
				Suffix:             " % 目标",
				MinSize:            Size{Width: 135},
				Background:         resultBackground,
				TextColor:          defaultTextColor,
				OnValueChanged:     syncInlineEditor,
			},
			NumberEdit{
				AssignTo:           &assetAmountEdit,
				Decimals:           2,
				Increment:          1000,
				MinValue:           0,
				MaxValue:           1_000_000_000,
				SpinButtonsVisible: false,
				MinSize:            Size{Width: 185},
				StretchFactor:      1,
				Background:         resultBackground,
				TextColor:          defaultTextColor,
				OnValueChanged:     syncInlineEditor,
			},
			Label{
				Text:      "元",
				TextColor: mutedTextColor,
			},
			PushButton{
				Text:    "完成",
				MinSize: Size{Width: 76, Height: 28},
				OnClicked: func() {
					finishInlineEdit()
				},
			},
			PushButton{
				Text:    "撤销",
				MinSize: Size{Width: 76, Height: 28},
				OnClicked: func() {
					cancelInlineEdit()
				},
			},
		},
	}
}

func buildActionBar() Widget {
	return Composite{
		Background: panelBackground,
		Border:     true,
		Layout: HBox{
			Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 10},
			Spacing: 10,
		},
		Children: []Widget{
			PushButton{
				Text:    "计算建议",
				MinSize: Size{Width: 118, Height: 30},
				Font:    Font{Family: "Microsoft YaHei UI", PointSize: 11, Bold: true},
				OnClicked: func() {
					result, err := calculateFromForm()
					if err != nil {
						walk.MsgBox(mainWindow, "输入有误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
						statusBarItem.SetText("请修正资产条目后重新计算")
						return
					}
					resultEdit.SetText(result)
					statusBarItem.SetText("计算完成：所有目标金额均基于买入后的组合总额")
				},
			},
			Label{
				Text:      "目标合计需为 100%，计算结果可直接归档到历史记录。",
				TextColor: mutedTextColor,
			},
			HSpacer{},
			PushButton{
				Text:    "保存归档",
				MinSize: Size{Width: 118, Height: 30},
				OnClicked: func() {
					if err := archiveCurrentInvestment(); err != nil {
						walk.MsgBox(mainWindow, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
						return
					}
					statusBarItem.SetText(saveArchiveSuccessMessage)
					walk.MsgBox(mainWindow, "保存成功", saveArchiveSuccessMessage, walk.MsgBoxOK|walk.MsgBoxIconInformation)
				},
			},
			PushButton{
				Text:    "清空资产",
				MinSize: Size{Width: 96, Height: 30},
				OnClicked: func() {
					closeInlineEditor()
					assetModel.SetItems(nil)
					refreshAssetSummary()
					resultEdit.SetText(initialResultText())
					if saveCurrentPortfolioFromClassic() {
						statusBarItem.SetText("资产列表已清空")
					}
				},
			},
		},
	}
}

func addAsset() {
	if editingIndex >= 0 && isBlankAsset(assetModel.items[editingIndex]) {
		beginInlineEdit(editingIndex, true)
		return
	}

	assetModel.Add(AssetInput{})
	index := len(assetModel.items) - 1
	_ = assetTable.SetCurrentIndex(index)
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	saved := saveCurrentPortfolioFromClassic()
	beginInlineEdit(index, true)
	if saved {
		statusBarItem.SetText("已添加空白行，请在资产区下方直接填写")
	}
}

func editSelectedAsset() {
	index := assetTable.CurrentIndex()
	if index < 0 || index >= len(assetModel.items) {
		statusBarItem.SetText("请先选择要编辑的资产行")
		return
	}
	beginInlineEdit(index, false)
}

func removeSelectedAsset() {
	index := assetTable.CurrentIndex()
	if index < 0 || index >= len(assetModel.items) {
		statusBarItem.SetText("请先选择要删除的资产行")
		return
	}

	name := assetModel.items[index].Name
	if isBlankAsset(assetModel.items[index]) {
		if editingIndex == index {
			closeInlineEditor()
		}
		assetModel.Remove(index)
		refreshAssetSummary()
		if saveCurrentPortfolioFromClassic() {
			statusBarItem.SetText("已删除空白资产行")
		}
		return
	}
	if walk.MsgBox(
		mainWindow,
		"确认删除",
		"确定删除“"+name+"”吗？",
		walk.MsgBoxYesNo|walk.MsgBoxIconQuestion,
	) != walk.DlgCmdYes {
		return
	}

	if editingIndex == index {
		closeInlineEditor()
	}
	assetModel.Remove(index)
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	if saveCurrentPortfolioFromClassic() {
		statusBarItem.SetText("已删除资产：" + name)
	}
}

func beginInlineEdit(index int, isNew bool) {
	if index < 0 || index >= len(assetModel.items) {
		return
	}

	editingIndex = index
	editorOriginal = assetModel.items[index]
	editorIsNew = isNew
	loadingEditor = true
	item := assetModel.items[index]
	assetNameEdit.SetText(item.Name)
	_ = assetTargetEdit.SetValue(item.TargetPct)
	_ = assetAmountEdit.SetValue(item.CurrentAmount)
	editorStateLabel.SetText(fmt.Sprintf("编辑 #%d", index+1))
	inlineEditor.SetVisible(true)
	loadingEditor = false
	_ = assetNameEdit.SetFocus()
	statusBarItem.SetText("正在编辑第 " + strconv.Itoa(index+1) + " 项，修改会实时保存到表格")
}

func syncInlineEditor() {
	if loadingEditor || editingIndex < 0 || editingIndex >= len(assetModel.items) {
		return
	}

	assetModel.items[editingIndex] = AssetInput{
		Name:          strings.TrimSpace(assetNameEdit.Text()),
		TargetPct:     assetTargetEdit.Value(),
		CurrentAmount: assetAmountEdit.Value(),
	}
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	saveCurrentPortfolioFromClassic()
}

func finishInlineEdit() {
	if editingIndex < 0 || editingIndex >= len(assetModel.items) {
		return
	}
	if err := validateAssetAt(editingIndex); err != nil {
		statusBarItem.SetText(err.Error())
		_ = assetNameEdit.SetFocus()
		return
	}

	name := assetModel.items[editingIndex].Name
	closeInlineEditor()
	statusBarItem.SetText("已完成编辑：" + name)
}

func cancelInlineEdit() {
	if editingIndex < 0 || editingIndex >= len(assetModel.items) {
		closeInlineEditor()
		return
	}

	index := editingIndex
	if editorIsNew {
		assetModel.Remove(index)
	} else {
		assetModel.items[index] = editorOriginal
		assetModel.RefreshAll()
	}
	closeInlineEditor()
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	saveCurrentPortfolioFromClassic()
	statusBarItem.SetText("已撤销本次编辑")
}

func loadPortfolioIntoClassic() {
	if investAmountEdit == nil {
		return
	}
	loadingPortfolio = true
	defer func() {
		loadingPortfolio = false
	}()

	investAmount, assets, err := loadPortfolioConfig(5000)
	if err != nil {
		statusBarItem.SetText("当前资产配置读取失败：" + err.Error())
		investAmount = 5000
	}
	_ = investAmountEdit.SetValue(investAmount)
	assetModel.SetItems(assets)
	if len(assets) > 0 && assetTable != nil {
		_ = assetTable.SetCurrentIndex(0)
	}
}

func saveCurrentPortfolioFromClassic() bool {
	if loadingPortfolio || investAmountEdit == nil {
		return true
	}
	if err := savePortfolioConfig(investAmountEdit.Value(), assetModel.ItemsCopy()); err != nil {
		statusBarItem.SetText("当前资产配置保存失败：" + err.Error())
		return false
	}
	return true
}

func loadSelectedHistoryToCalculator() {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		statusBarItem.SetText("请先选择要读取的历史记录")
		return
	}
	record := cloneInvestmentRecord(selectedHistoryDraft)
	if record.ID == "" {
		record = cloneInvestmentRecord(investmentRecords[selectedHistoryIndex])
	}
	recalculateInvestmentRecord(&record)
	assets := portfolioAssetsFromHistory(record)
	if len(assets) == 0 {
		statusBarItem.SetText("该历史记录没有可读取的资产条目")
		return
	}

	closeInlineEditor()
	assetModel.SetItems(assets)
	_ = assetTable.SetCurrentIndex(0)
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	saved := saveCurrentPortfolioFromClassic()
	if mainTabs != nil {
		_ = mainTabs.SetCurrentIndex(0)
	}
	if saved {
		statusBarItem.SetText("已读取历史记录，当前资产金额使用买入后金额")
	}
}

func closeInlineEditor() {
	loadingEditor = true
	editingIndex = -1
	editorIsNew = false
	if inlineEditor != nil {
		inlineEditor.SetVisible(false)
	}
	loadingEditor = false
}

func validateAssetAt(index int) error {
	if index < 0 || index >= len(assetModel.items) {
		return fmt.Errorf("资产行不存在")
	}
	item := assetModel.items[index]
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("第 %d 项资产名称不能为空", index+1)
	}
	if item.TargetPct <= 0 || item.TargetPct > 100 {
		return fmt.Errorf("%s 的目标仓位必须大于 0%% 且不超过 100%%", item.Name)
	}
	if assetNameExists(item.Name, index) {
		return fmt.Errorf("资产名称不能重复：%s", item.Name)
	}
	return nil
}

func isBlankAsset(item AssetInput) bool {
	return strings.TrimSpace(item.Name) == "" && item.TargetPct == 0 && item.CurrentAmount == 0
}

func assetNameExists(name string, exceptIndex int) bool {
	for i, item := range assetModel.items {
		if i != exceptIndex && strings.EqualFold(strings.TrimSpace(item.Name), name) {
			return true
		}
	}
	return false
}

func refreshAssetSummary() {
	if assetSummaryLabel == nil {
		return
	}
	assetSummaryLabel.SetText(fmt.Sprintf(
		"资产数量：%d 项\r\n目标合计：%s\r\n当前总额：%s 元",
		len(assetModel.items),
		formatPercent(targetPctSum(assetModel.items)),
		formatMoney(currentAmountSum(assetModel.items)),
	))
	assetModel.RefreshAll()
}

func currentAmountSum(items []AssetInput) float64 {
	var total float64
	for _, item := range items {
		total += item.CurrentAmount
	}
	return total
}

func targetPctSum(items []AssetInput) float64 {
	var total float64
	for _, item := range items {
		total += item.TargetPct
	}
	return total
}

func currentPctForInputs(items []AssetInput, index int) float64 {
	total := currentAmountSum(items)
	if total <= moneyEpsilon || index < 0 || index >= len(items) {
		return 0
	}
	return items[index].CurrentAmount / total * 100
}

func calculateFromForm() (string, error) {
	result, err := CalculatePortfolio(investAmountEdit.Value(), assetModel.ItemsCopy())
	if err != nil {
		return "", err
	}
	return FormatResult(result), nil
}

func validateDraftAssets(items []AssetInput) error {
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("配置中第 %d 项资产名称为空", i+1)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("配置中存在重复资产：%s", name)
		}
		seen[key] = struct{}{}
		if item.TargetPct <= 0 || item.TargetPct > 100 {
			return fmt.Errorf("%s 的目标仓位超出允许范围", name)
		}
		if item.CurrentAmount < 0 {
			return fmt.Errorf("%s 的当前持有金额不能为负数", name)
		}
	}
	return nil
}

func initialResultText() string {
	return "先点击“添加资产”录入名称、目标仓位和当前持有金额。\r\n\r\n" +
		"至少需要两项资产，且目标仓位合计为 100%。当前总额和当前仓位会根据持有金额自动更新。"
}

func showStartupError(err error) {
	walk.MsgBox(nil, "启动失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
	os.Exit(1)
}
