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

var assetDefinitions = []AssetDefinition{
	{Name: "标普500ETF", TargetPct: 33},
	{Name: "纳指100ETF", TargetPct: 17},
	{Name: "中证A500ETF", TargetPct: 33},
	{Name: "红利低波ETF", TargetPct: 17},
}

type Config struct {
	CurrentTotal string             `json:"current_total"`
	InvestAmount string             `json:"invest_amount"`
	CurrentPcts  map[string]float64 `json:"current_pcts,omitempty"`
	AssetsText   string             `json:"assets_text,omitempty"`
}

var (
	mainWindow       *walk.MainWindow
	currentTotalEdit *walk.NumberEdit
	investAmountEdit *walk.NumberEdit
	currentPctEdits  = make([]*walk.NumberEdit, len(assetDefinitions))
	resultEdit       *walk.TextEdit
	statusBarItem    *walk.StatusBarItem
	defaultTextColor = walk.RGB(35, 46, 66)
	mutedTextColor   = walk.RGB(99, 115, 138)
	accentColor      = walk.RGB(31, 111, 235)
	windowBackground = SolidColorBrush{Color: walk.RGB(245, 247, 251)}
	panelBackground  = SolidColorBrush{Color: walk.RGB(255, 255, 255)}
	headerBackground = SolidColorBrush{Color: walk.RGB(30, 53, 87)}
	resultBackground = SolidColorBrush{Color: walk.RGB(250, 252, 255)}
)

func main() {
	walk.AppendToWalkInit(func() {
		walk.FocusEffect, _ = walk.NewBorderGlowEffect(accentColor)
		walk.ValidationErrorEffect, _ = walk.NewBorderGlowEffect(walk.RGB(210, 55, 55))
	})

	window := MainWindow{
		AssignTo:   &mainWindow,
		Title:      "投资组合再平衡助手",
		MinSize:    Size{Width: 980, Height: 760},
		Size:       Size{Width: 1120, Height: 860},
		Font:       Font{Family: "Microsoft YaHei UI", PointSize: 10},
		Background: windowBackground,
		Layout: VBox{
			Margins: Margins{Left: 16, Top: 16, Right: 16, Bottom: 12},
			Spacing: 12,
		},
		StatusBarItems: []StatusBarItem{
			{
				AssignTo: &statusBarItem,
				Text:     "规则：只买低配 · 不做择时 · 半年检查仅作卖出提醒",
				Width:    720,
			},
		},
		Children: []Widget{
			buildHeader(),
			Composite{
				Layout: HBox{
					MarginsZero: true,
					Spacing:     12,
				},
				Children: []Widget{
					buildAmountPanel(),
					buildPositionPanel(),
				},
			},
			buildActionBar(),
			GroupBox{
				Title:         "计算结果",
				StretchFactor: 1,
				Background:    panelBackground,
				Layout: VBox{
					Margins: Margins{Left: 12, Top: 14, Right: 12, Bottom: 12},
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
	}

	if err := window.Create(); err != nil {
		showStartupError(err)
	}

	_ = investAmountEdit.SetValue(5000)
	mainWindow.Run()
}

func buildHeader() Widget {
	return Composite{
		MinSize:    Size{Height: 72},
		Background: headerBackground,
		Layout: HBox{
			Margins: Margins{Left: 22, Top: 14, Right: 22, Bottom: 14},
			Spacing: 16,
		},
		Children: []Widget{
			Label{
				Text:      "投资组合再平衡助手",
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 16, Bold: true},
				TextColor: walk.RGB(255, 255, 255),
			},
			Label{
				Text:      "用新增资金优先补齐低配资产，让买入后的组合尽量靠近长期目标。",
				TextColor: walk.RGB(213, 224, 240),
			},
			HSpacer{},
		},
	}
}

func buildAmountPanel() Widget {
	return GroupBox{
		Title:         "① 本次投入",
		MinSize:       Size{Width: 330},
		StretchFactor: 1,
		Background:    panelBackground,
		Layout: Grid{
			Columns: 2,
			Margins: Margins{Left: 14, Top: 18, Right: 14, Bottom: 14},
			Spacing: 10,
		},
		Children: []Widget{
			Label{
				Text:      "当前已投资总资产",
				TextColor: defaultTextColor,
			},
			NumberEdit{
				AssignTo:           &currentTotalEdit,
				Value:              0,
				Decimals:           2,
				Increment:          1000,
				MinValue:           0,
				MaxValue:           1_000_000_000,
				SpinButtonsVisible: false,
				Suffix:             " 元",
				ToolTipText:        "当前四项资产合计市值，不含本次准备投入的金额。",
			},
			Label{
				Text:      "本次可投入金额",
				TextColor: defaultTextColor,
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
				ToolTipText:        "本次计划全部用于再平衡的新增资金。",
			},
			Label{
				ColumnSpan: 2,
				Text:       "资金按相对低配程度分层补齐；没有触发卖出时，月度操作只使用新增资金。",
				TextColor:  mutedTextColor,
				Font:       Font{Family: "Microsoft YaHei UI", PointSize: 9},
			},
		},
	}
}

func buildPositionPanel() Widget {
	children := []Widget{
		Label{
			Text:      "资产",
			Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
			TextColor: mutedTextColor,
		},
		Label{
			Text:          "目标仓位",
			Font:          Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
			TextColor:     mutedTextColor,
			TextAlignment: AlignCenter,
		},
		Label{
			Text:          "当前仓位",
			Font:          Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
			TextColor:     mutedTextColor,
			TextAlignment: AlignCenter,
		},
	}

	for i, definition := range assetDefinitions {
		children = append(children,
			Label{
				Text:      definition.Name,
				TextColor: defaultTextColor,
			},
			Label{
				Text:          formatPercent(definition.TargetPct),
				TextColor:     accentColor,
				Font:          Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
				TextAlignment: AlignCenter,
			},
			NumberEdit{
				AssignTo:           &currentPctEdits[i],
				Value:              0,
				Decimals:           2,
				Increment:          0.5,
				MinValue:           0,
				MaxValue:           100,
				SpinButtonsVisible: false,
				Suffix:             " %",
			},
		)
	}

	return GroupBox{
		Title:         "② 当前仓位",
		MinSize:       Size{Width: 560},
		StretchFactor: 2,
		Background:    panelBackground,
		Layout: Grid{
			Columns: 3,
			Margins: Margins{Left: 14, Top: 18, Right: 14, Bottom: 14},
			Spacing: 8,
		},
		Children: children,
	}
}

func buildActionBar() Widget {
	return Composite{
		Background: panelBackground,
		Layout: HBox{
			Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10},
			Spacing: 8,
		},
		Children: []Widget{
			PushButton{
				Text:    "计算买入建议",
				MinSize: Size{Width: 150, Height: 34},
				Font:    Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
				OnClicked: func() {
					result, err := calculateFromForm()
					if err != nil {
						walk.MsgBox(mainWindow, "输入有误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconWarning)
						statusBarItem.SetText("请修正输入后重新计算")
						return
					}
					resultEdit.SetText(result)
					statusBarItem.SetText("计算完成：买入金额已按相对低配程度分层分配")
				},
			},
			HSpacer{},
			PushButton{
				Text:    "保存配置",
				MinSize: Size{Width: 110, Height: 32},
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
				MinSize: Size{Width: 110, Height: 32},
				OnClicked: func() {
					if err := loadConfig(); err != nil {
						walk.MsgBox(mainWindow, "读取失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
						return
					}
					statusBarItem.SetText("配置已读取；旧版配置会自动迁移到四资产表单")
				},
			},
			PushButton{
				Text:    "清空仓位",
				MinSize: Size{Width: 110, Height: 32},
				OnClicked: func() {
					for _, edit := range currentPctEdits {
						_ = edit.SetValue(0)
					}
					resultEdit.SetText(initialResultText())
					statusBarItem.SetText("当前仓位已清空")
				},
			},
		},
	}
}

func calculateFromForm() (string, error) {
	currentPcts := make([]float64, len(assetDefinitions))
	for i, edit := range currentPctEdits {
		currentPcts[i] = edit.Value()
	}

	result, err := CalculatePortfolio(
		currentTotalEdit.Value(),
		investAmountEdit.Value(),
		assetDefinitions,
		currentPcts,
	)
	if err != nil {
		return "", err
	}

	return FormatResult(result), nil
}

func saveConfig() error {
	currentPcts := make(map[string]float64, len(assetDefinitions))
	for i, definition := range assetDefinitions {
		currentPcts[definition.Name] = currentPctEdits[i].Value()
	}

	cfg := Config{
		CurrentTotal: formatNumber(currentTotalEdit.Value()),
		InvestAmount: formatNumber(investAmountEdit.Value()),
		CurrentPcts:  currentPcts,
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

	currentTotal, err := parseNumber(cfg.CurrentTotal)
	if err != nil {
		return fmt.Errorf("配置中的当前总资产无效")
	}
	investAmount, err := parseNumber(cfg.InvestAmount)
	if err != nil {
		return fmt.Errorf("配置中的可投入金额无效")
	}

	currentPcts := cfg.CurrentPcts
	if len(currentPcts) == 0 && strings.TrimSpace(cfg.AssetsText) != "" {
		currentPcts, err = parseLegacyCurrentPcts(cfg.AssetsText)
		if err != nil {
			return err
		}
	}

	if err := currentTotalEdit.SetValue(currentTotal); err != nil {
		return fmt.Errorf("配置中的当前总资产超出允许范围")
	}
	if err := investAmountEdit.SetValue(investAmount); err != nil {
		return fmt.Errorf("配置中的可投入金额超出允许范围")
	}

	for i, definition := range assetDefinitions {
		value := currentPcts[definition.Name]
		if err := currentPctEdits[i].SetValue(value); err != nil {
			return fmt.Errorf("%s 的当前仓位超出 0%%—100%%", definition.Name)
		}
	}

	resultEdit.SetText(initialResultText())
	return nil
}

func parseLegacyCurrentPcts(text string) (map[string]float64, error) {
	values := make(map[string]float64)
	for lineIndex, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			return nil, fmt.Errorf("旧版配置第 %d 行格式错误", lineIndex+1)
		}

		name := strings.TrimSpace(parts[0])
		currentPct, err := parseNumber(parts[2])
		if err != nil {
			return nil, fmt.Errorf("旧版配置第 %d 行当前仓位无效", lineIndex+1)
		}
		values[name] = currentPct
	}
	return values, nil
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
	return "填写当前总资产、本次投入金额和四项当前仓位，然后点击“计算买入建议”。\r\n\r\n" +
		"程序会：\r\n" +
		"• 仅向买入后仍低配的资产分配新增资金；\r\n" +
		"• 优先补相对目标偏离最大的资产；\r\n" +
		"• 列出半年检查的高配卖出提醒线和明显低配参考线。"
}

func showStartupError(err error) {
	walk.MsgBox(nil, "启动失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
	os.Exit(1)
}
