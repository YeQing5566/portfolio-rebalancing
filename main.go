package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const configFile = "portfolio_config.json"

type Config struct {
	Version      int                `json:"version,omitempty"`
	InvestAmount string             `json:"invest_amount"`
	Assets       []AssetInput       `json:"assets,omitempty"`
	CurrentTotal string             `json:"current_total,omitempty"`
	CurrentPcts  map[string]float64 `json:"current_pcts,omitempty"`
	AssetsText   string             `json:"assets_text,omitempty"`
}

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
	defaultTextColor  = walk.RGB(35, 46, 66)
	mutedTextColor    = walk.RGB(99, 115, 138)
	accentColor       = walk.RGB(31, 111, 235)
	windowBackground  = SolidColorBrush{Color: walk.RGB(245, 247, 251)}
	panelBackground   = SolidColorBrush{Color: walk.RGB(255, 255, 255)}
	headerBackground  = SolidColorBrush{Color: walk.RGB(30, 53, 87)}
	resultBackground  = SolidColorBrush{Color: walk.RGB(250, 252, 255)}
)

func main() {
	walk.AppendToWalkInit(func() {
		walk.FocusEffect, _ = walk.NewBorderGlowEffect(accentColor)
		walk.ValidationErrorEffect, _ = walk.NewBorderGlowEffect(walk.RGB(210, 55, 55))
	})

	window := MainWindow{
		AssignTo:   &mainWindow,
		Title:      "投资组合再平衡助手",
		MinSize:    Size{Width: 1020, Height: 700},
		Size:       Size{Width: 1180, Height: 840},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 10},
		Background: windowBackground,
		Layout: VBox{
			Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 8},
			Spacing: 8,
		},
		StatusBarItems: []StatusBarItem{
			{
				AssignTo: &statusBarItem,
				Text:     "先添加至少两项资产，并使目标仓位合计为 100%",
				Width:    780,
			},
		},
		Children: []Widget{
			buildHeader(),
			VSplitter{
				HandleWidth:   7,
				StretchFactor: 1,
				Children: []Widget{
					Composite{
						StretchFactor: 2,
						Layout: VBox{
							MarginsZero: true,
							Spacing:     8,
						},
						Children: []Widget{
							Composite{
								StretchFactor: 1,
								Layout: HBox{
									MarginsZero: true,
									Spacing:     8,
								},
								Children: []Widget{
									buildOverviewPanel(),
									buildAssetPanel(),
								},
							},
							buildActionBar(),
						},
					},
					GroupBox{
						Title:         "再平衡建议",
						MinSize:       Size{Height: 160},
						StretchFactor: 3,
						Background:    panelBackground,
						Layout: VBox{
							Margins: Margins{Left: 10, Top: 12, Right: 10, Bottom: 10},
						},
						Children: []Widget{
							TextEdit{
								AssignTo:      &resultEdit,
								ReadOnly:      true,
								VScroll:       true,
								Background:    resultBackground,
								TextColor:     defaultTextColor,
								Font:          Font{Family: "Microsoft YaHei UI", PointSize: 10},
								StretchFactor: 1,
								Text:          initialResultText(),
							},
						},
					},
				},
			},
		},
	}

	if err := window.Create(); err != nil {
		showStartupError(err)
	}

	_ = investAmountEdit.SetValue(5000)
	refreshAssetSummary()
	mainWindow.Run()
}

func buildHeader() Widget {
	return Composite{
		MinSize:    Size{Height: 54},
		Background: headerBackground,
		Layout: HBox{
			Margins: Margins{Left: 18, Top: 9, Right: 18, Bottom: 9},
			Spacing: 14,
		},
		Children: []Widget{
			Label{
				Text:      "投资组合再平衡助手",
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 15, Bold: true},
				TextColor: walk.RGB(255, 255, 255),
			},
			Label{
				Text:      "按买入后的组合总额计算目标缺口，让所有资产尽量靠近设定仓位。",
				TextColor: walk.RGB(213, 224, 240),
			},
			HSpacer{},
		},
	}
}

func buildOverviewPanel() Widget {
	return GroupBox{
		Title:      "本次投入",
		MinSize:    Size{Width: 265, Height: 205},
		MaxSize:    Size{Width: 290, Height: 1000},
		Background: panelBackground,
		Layout: VBox{
			Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 10},
			Spacing: 8,
		},
		Children: []Widget{
			Label{
				Text:      "本次可投入金额",
				TextColor: defaultTextColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
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
			},
			VSpacer{Size: 4},
			Label{
				Text:      "资产汇总",
				TextColor: mutedTextColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
			},
			Label{
				AssignTo:  &assetSummaryLabel,
				Text:      "资产数量：0 项\r\n目标合计：0.00%\r\n当前总额：0.00 元",
				TextColor: accentColor,
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
				MinSize:   Size{Height: 64},
			},
			VSpacer{},
		},
	}
}

func buildAssetPanel() Widget {
	return GroupBox{
		Title:         "资产条目",
		MinSize:       Size{Height: 205},
		StretchFactor: 4,
		Background:    panelBackground,
		Layout: VBox{
			Margins: Margins{Left: 10, Top: 12, Right: 10, Bottom: 9},
			Spacing: 6,
		},
		Children: []Widget{
			Composite{
				Layout: HBox{
					MarginsZero: true,
					Spacing:     8,
				},
				Children: []Widget{
					PushButton{
						Text:    "＋ 添加资产",
						MinSize: Size{Width: 112, Height: 28},
						Font:    Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
						OnClicked: func() {
							addAsset()
						},
					},
					PushButton{
						Text:    "删除选中",
						MinSize: Size{Width: 100, Height: 28},
						OnClicked: func() {
							removeSelectedAsset()
						},
					},
					HSpacer{},
					Label{
						Text:      "双击任意一行，在下方直接编辑",
						TextColor: mutedTextColor,
					},
				},
			},
			TableView{
				AssignTo:                    &assetTable,
				Model:                       assetModel,
				AlternatingRowBG:            true,
				LastColumnStretched:         true,
				NotSortableByHeaderClick:    true,
				SelectionHiddenWithoutFocus: false,
				CustomRowHeight:             26,
				MinSize:                     Size{Height: 92},
				Columns: []TableViewColumn{
					{Title: "#", Width: 44, Alignment: AlignCenter},
					{Title: "资产名称", Width: 260},
					{Title: "目标仓位", Width: 140, Alignment: AlignFar},
					{Title: "当前持有金额", Width: 190, Alignment: AlignFar},
					{Title: "自动计算当前仓位", Width: 190, Alignment: AlignFar},
				},
				OnItemActivated: func() {
					editSelectedAsset()
				},
			},
			buildInlineEditor(),
		},
	}
}

func buildInlineEditor() Widget {
	return Composite{
		AssignTo:   &inlineEditor,
		Visible:    false,
		Background: resultBackground,
		Border:     true,
		Layout: HBox{
			Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6},
			Spacing: 7,
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
				OnValueChanged:     syncInlineEditor,
			},
			NumberEdit{
				AssignTo:           &assetAmountEdit,
				Decimals:           2,
				Increment:          1000,
				MinValue:           0,
				MaxValue:           1_000_000_000,
				SpinButtonsVisible: false,
				Suffix:             " 元 持有",
				MinSize:            Size{Width: 185},
				StretchFactor:      1,
				OnValueChanged:     syncInlineEditor,
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
		Layout: HBox{
			Margins: Margins{Left: 10, Top: 6, Right: 10, Bottom: 6},
			Spacing: 8,
		},
		Children: []Widget{
			PushButton{
				Text:    "计算再平衡建议",
				MinSize: Size{Width: 165, Height: 30},
				Font:    Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
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
			HSpacer{},
			PushButton{
				Text:    "保存配置",
				MinSize: Size{Width: 100, Height: 28},
				OnClicked: func() {
					if err := saveConfig(); err != nil {
						walk.MsgBox(mainWindow, "保存失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
						return
					}
					statusBarItem.SetText("配置已保存到 " + configFile)
				},
			},
			PushButton{
				Text:    "读取配置",
				MinSize: Size{Width: 100, Height: 28},
				OnClicked: func() {
					if err := loadConfig(); err != nil {
						walk.MsgBox(mainWindow, "读取失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
						return
					}
					statusBarItem.SetText("配置已读取")
				},
			},
			PushButton{
				Text:    "清空资产",
				MinSize: Size{Width: 100, Height: 28},
				OnClicked: func() {
					closeInlineEditor()
					assetModel.SetItems(nil)
					refreshAssetSummary()
					resultEdit.SetText(initialResultText())
					statusBarItem.SetText("资产列表已清空")
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
	beginInlineEdit(index, true)
	statusBarItem.SetText("已添加空白行，请在资产区下方直接填写")
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
		statusBarItem.SetText("已删除空白资产行")
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
	statusBarItem.SetText("已删除资产：" + name)
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
	statusBarItem.SetText("已撤销本次编辑")
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
		"资产数量：%d 项\r\n目标合计：%.2f%%\r\n当前总额：%s 元",
		len(assetModel.items),
		targetPctSum(assetModel.items),
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

func saveConfig() error {
	cfg := Config{
		Version:      2,
		InvestAmount: formatNumber(investAmountEdit.Value()),
		Assets:       assetModel.ItemsCopy(),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("生成配置失败：%w", err)
	}
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("写入配置失败：%w", err)
	}
	return nil
}

func loadConfig() error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", configFile, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("配置文件格式错误：%w", err)
	}

	investAmount, err := parseNumber(cfg.InvestAmount)
	if err != nil {
		return fmt.Errorf("配置中的可投入金额无效")
	}

	items := cfg.Assets
	if len(items) == 0 {
		items, err = migrateLegacyConfig(cfg)
		if err != nil {
			return err
		}
	}
	if err := validateDraftAssets(items); err != nil {
		return err
	}

	if err := investAmountEdit.SetValue(investAmount); err != nil {
		return fmt.Errorf("配置中的可投入金额超出允许范围")
	}
	closeInlineEditor()
	assetModel.SetItems(items)
	refreshAssetSummary()
	resultEdit.SetText(initialResultText())
	return nil
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

func migrateLegacyConfig(cfg Config) ([]AssetInput, error) {
	currentTotal, err := parseNumber(cfg.CurrentTotal)
	if err != nil {
		return nil, fmt.Errorf("旧配置中的当前总资产无效")
	}

	if strings.TrimSpace(cfg.AssetsText) != "" {
		return parseLegacyAssetsText(cfg.AssetsText, currentTotal)
	}

	if len(cfg.CurrentPcts) > 0 {
		legacyTargets := map[string]float64{
			"标普500ETF":  33,
			"纳指100ETF":  17,
			"中证A500ETF": 33,
			"红利低波ETF":   17,
		}
		items := make([]AssetInput, 0, len(cfg.CurrentPcts))
		for name, currentPct := range cfg.CurrentPcts {
			targetPct, ok := legacyTargets[name]
			if !ok {
				return nil, fmt.Errorf("旧配置缺少 %s 的目标仓位，无法自动迁移", name)
			}
			items = append(items, AssetInput{
				Name:          name,
				TargetPct:     targetPct,
				CurrentAmount: currentTotal * currentPct / 100,
			})
		}
		return items, nil
	}

	return nil, fmt.Errorf("配置中没有资产条目")
}

func parseLegacyAssetsText(text string, currentTotal float64) ([]AssetInput, error) {
	items := make([]AssetInput, 0)
	for lineIndex, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			return nil, fmt.Errorf("旧配置第 %d 行格式错误", lineIndex+1)
		}

		targetPct, err := parseNumber(parts[1])
		if err != nil {
			return nil, fmt.Errorf("旧配置第 %d 行目标仓位无效", lineIndex+1)
		}
		currentPct, err := parseNumber(parts[2])
		if err != nil {
			return nil, fmt.Errorf("旧配置第 %d 行当前仓位无效", lineIndex+1)
		}

		items = append(items, AssetInput{
			Name:          strings.TrimSpace(parts[0]),
			TargetPct:     targetPct,
			CurrentAmount: currentTotal * currentPct / 100,
		})
	}
	return items, nil
}

func parseNumber(value string) (float64, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", "")
	value = strings.ReplaceAll(value, "，", "")
	value = strings.ReplaceAll(value, "%", "")
	value = strings.ReplaceAll(value, "％", "")
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func initialResultText() string {
	return "点击“添加资产”会直接插入空白行；双击任意资产行，可在表格下方直接修改。\r\n\r\n" +
		"至少需要两项资产，且目标仓位合计为 100%。当前总额和当前仓位由持有金额自动计算。"
}

func showStartupError(err error) {
	walk.MsgBox(nil, "启动失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
	os.Exit(1)
}
