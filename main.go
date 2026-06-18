package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const configFile = "portfolio_config.json"

type Config struct {
	CurrentTotal string `json:"current_total"`
	InvestAmount string `json:"invest_amount"`
	MaxBuyCount  string `json:"max_buy_count"`
	AssetsText   string `json:"assets_text"`
}

type Asset struct {
	Name          string
	TargetPct     float64
	CurrentPct    float64
	PremiumPct    float64
	PremiumLimit  float64
	CurrentAmount float64
	TargetAmount  float64
	Gap           float64
	BuyAmount     float64
	AfterPct      float64
	Status        string
	Skipped       bool
}

var (
	currentTotalEdit *walk.LineEdit
	investAmountEdit *walk.LineEdit
	maxBuyCountEdit  *walk.LineEdit
	assetsEdit       *walk.TextEdit
	resultEdit       *walk.TextEdit
)

func main() {
	defaultAssets := `标普500ETF,33,0,0,7
纳指100ETF,17,0,0,10
中证A500ETF,33,0,0,0
红利低波ETF,17,0,0,0`

	exitCode, err := (MainWindow{
		Title:   "投资再平衡计算器",
		MinSize: Size{Width: 1100, Height: 720},
		Layout:  VBox{},
		Children: []Widget{
			Composite{
				Layout: Grid{Columns: 6},
				Children: []Widget{
					Label{Text: "当前投资总市值"},
					LineEdit{AssignTo: &currentTotalEdit, Text: "0"},
					Label{Text: "本次可投入金额"},
					LineEdit{AssignTo: &investAmountEdit, Text: "5000"},
					Label{Text: "最多买入资产数"},
					LineEdit{AssignTo: &maxBuyCountEdit, Text: "2"},
				},
			},
			Label{
				Text: "资产格式：资产名称,目标仓位%,当前仓位%,QDII溢价%,溢价保护线%。普通资产后两项填0。新增资产直接新增一行。",
			},
			TextEdit{
				AssignTo: &assetsEdit,
				Text:     defaultAssets,
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "计算买入建议",
						OnClicked: func() {
							result, err := calculate()
							if err != nil {
								walk.MsgBox(nil, "错误", err.Error(), walk.MsgBoxIconError)
								return
							}
							resultEdit.SetText(result)
						},
					},
					PushButton{
						Text: "保存配置",
						OnClicked: func() {
							if err := saveConfig(); err != nil {
								walk.MsgBox(nil, "错误", err.Error(), walk.MsgBoxIconError)
								return
							}
							walk.MsgBox(nil, "保存成功", "配置已保存到 "+configFile, walk.MsgBoxIconInformation)
						},
					},
					PushButton{
						Text: "读取配置",
						OnClicked: func() {
							if err := loadConfig(); err != nil {
								walk.MsgBox(nil, "错误", err.Error(), walk.MsgBoxIconError)
								return
							}
							walk.MsgBox(nil, "读取成功", "配置已读取", walk.MsgBoxIconInformation)
						},
					},
				},
			},
			Label{Text: "计算结果"},
			TextEdit{
				AssignTo: &resultEdit,
				ReadOnly: true,
			},
		},
	}).Run()
	if err != nil {
		fmt.Println("窗口启动失败：", err)
		fmt.Println("按回车退出...")
		fmt.Scanln()
		os.Exit(1)
	}

	_ = exitCode
}

func calculate() (string, error) {
	currentTotal, err := parseFloat(currentTotalEdit.Text())
	if err != nil {
		return "", fmt.Errorf("当前投资总市值填写错误")
	}

	investAmount, err := parseFloat(investAmountEdit.Text())
	if err != nil {
		return "", fmt.Errorf("本次可投入金额填写错误")
	}

	maxBuyFloat, err := parseFloat(maxBuyCountEdit.Text())
	if err != nil {
		return "", fmt.Errorf("最多买入资产数填写错误")
	}
	maxBuyCount := int(maxBuyFloat)

	if currentTotal < 0 || investAmount < 0 {
		return "", fmt.Errorf("金额不能为负数")
	}

	assets, warning, err := parseAssets(assetsEdit.Text(), currentTotal)
	if err != nil {
		return "", err
	}

	var targetSum float64
	var currentPctSum float64

	for _, a := range assets {
		targetSum += a.TargetPct
		currentPctSum += a.CurrentPct
	}

	if math.Abs(targetSum-100) > 0.01 {
		return "", fmt.Errorf("目标仓位合计必须为100%%，当前为 %.2f%%", targetSum)
	}

	if currentTotal > 0 && math.Abs(currentPctSum-100) > 0.5 {
		return "", fmt.Errorf("当前仓位合计建议为100%%，当前为 %.2f%%", currentPctSum)
	}

	afterTotal := currentTotal + investAmount
	if afterTotal <= 0 {
		return "", fmt.Errorf("当前总市值和投入金额不能同时为0")
	}

	for _, a := range assets {
		a.TargetAmount = afterTotal * a.TargetPct / 100
		a.Gap = a.TargetAmount - a.CurrentAmount

		lowLine := a.TargetPct * 0.75
		highLine := a.TargetPct * 1.25

		switch {
		case a.CurrentPct > highLine:
			a.Status = "高配，半年检查时提示"
		case a.CurrentPct < lowLine:
			a.Status = "明显低配，新增资金优先补"
		case a.Gap > 0:
			a.Status = "低配，可买入"
		default:
			a.Status = "达标或高配，本次不买"
		}

		if a.Gap > 0 && a.PremiumLimit > 0 && a.PremiumPct > a.PremiumLimit {
			if a.CurrentPct >= lowLine {
				a.Skipped = true
				a.Status += "；QDII溢价超过保护线，本次跳过"
			} else {
				a.Status += "；QDII溢价高，但严重低配，可少量补"
			}
		}
	}

	candidates := make([]*Asset, 0)
	for _, a := range assets {
		if a.Gap > 0 && !a.Skipped {
			candidates = append(candidates, a)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Gap > candidates[j].Gap
	})

	if maxBuyCount > 0 && len(candidates) > maxBuyCount {
		candidates = candidates[:maxBuyCount]
	}

	var selectedGapSum float64
	for _, c := range candidates {
		selectedGapSum += c.Gap
	}

	remainingCash := investAmount

	if selectedGapSum > 0 && investAmount > 0 {
		if investAmount >= selectedGapSum {
			for _, c := range candidates {
				c.BuyAmount = c.Gap
				remainingCash -= c.BuyAmount
			}
		} else {
			for _, c := range candidates {
				c.BuyAmount = investAmount * c.Gap / selectedGapSum
				remainingCash -= c.BuyAmount
			}
		}
	}

	for _, a := range assets {
		a.AfterPct = (a.CurrentAmount + a.BuyAmount) / afterTotal * 100
	}

	var b strings.Builder

	b.WriteString("========== 买入建议 ==========\r\n\r\n")

	if warning != "" {
		b.WriteString("【QDII溢价提示】\r\n")
		b.WriteString(warning)
		b.WriteString("\r\n")
	}

	b.WriteString(fmt.Sprintf("当前投资总市值：%.2f 元\r\n", currentTotal))
	b.WriteString(fmt.Sprintf("本次可投入金额：%.2f 元\r\n", investAmount))
	b.WriteString(fmt.Sprintf("买入后总市值：%.2f 元\r\n", afterTotal))
	b.WriteString(fmt.Sprintf("最多买入资产数：%d，0表示不限\r\n", maxBuyCount))
	b.WriteString(fmt.Sprintf("未分配现金：%.2f 元\r\n\r\n", math.Max(0, remainingCash)))

	b.WriteString("【本次建议买入】\r\n")
	hasBuy := false
	for _, a := range assets {
		if a.BuyAmount > 0.005 {
			hasBuy = true
			b.WriteString(fmt.Sprintf(
				"- %s：买入 %.2f 元；当前 %.2f%% → 买后 %.2f%%\r\n",
				a.Name, a.BuyAmount, a.CurrentPct, a.AfterPct,
			))
		}
	}
	if !hasBuy {
		b.WriteString("- 本次没有符合规则的买入资产，资金可暂存现金或货币基金。\r\n")
	}

	b.WriteString("\r\n【全部资产状态】\r\n")
	for _, a := range assets {
		b.WriteString(fmt.Sprintf(
			"- %s：目标 %.2f%%，当前 %.2f%%，买后 %.2f%%，缺口 %.2f 元，状态：%s\r\n",
			a.Name, a.TargetPct, a.CurrentPct, a.AfterPct, a.Gap, a.Status,
		))
	}

	b.WriteString("\r\n【半年检查参考】\r\n")
	for _, a := range assets {
		lowLine := a.TargetPct * 0.75
		highLine := a.TargetPct * 1.25
		b.WriteString(fmt.Sprintf(
			"- %s：明显低配线 %.2f%%，高配提示线 %.2f%%\r\n",
			a.Name, lowLine, highLine,
		))
	}

	return b.String(), nil
}

func parseAssets(text string, currentTotal float64) ([]*Asset, string, error) {
	lines := strings.Split(text, "\n")
	assets := make([]*Asset, 0)
	var warning strings.Builder

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			return nil, "", fmt.Errorf("第 %d 行格式错误，应为：资产名称,目标仓位,当前仓位,溢价,溢价保护线", i+1)
		}

		name := strings.TrimSpace(parts[0])
		targetPct, err := parseFloat(parts[1])
		if err != nil {
			return nil, "", fmt.Errorf("第 %d 行目标仓位错误", i+1)
		}

		currentPct, err := parseFloat(parts[2])
		if err != nil {
			return nil, "", fmt.Errorf("第 %d 行当前仓位错误", i+1)
		}

		premiumPct, err := parseFloat(parts[3])
		if err != nil {
			return nil, "", fmt.Errorf("第 %d 行QDII溢价错误", i+1)
		}

		premiumLimit, err := parseFloat(parts[4])
		if err != nil {
			return nil, "", fmt.Errorf("第 %d 行溢价保护线错误", i+1)
		}

		currentAmount := currentTotal * currentPct / 100

		if premiumLimit > 0 && premiumPct > premiumLimit {
			warning.WriteString(fmt.Sprintf(
				"%s 当前溢价 %.2f%%，超过保护线 %.2f%%。\r\n",
				name, premiumPct, premiumLimit,
			))
		}

		assets = append(assets, &Asset{
			Name:          name,
			TargetPct:     targetPct,
			CurrentPct:    currentPct,
			PremiumPct:    premiumPct,
			PremiumLimit:  premiumLimit,
			CurrentAmount: currentAmount,
		})
	}

	if len(assets) == 0 {
		return nil, "", fmt.Errorf("至少需要填写一个资产")
	}

	return assets, warning.String(), nil
}

func saveConfig() error {
	cfg := Config{
		CurrentTotal: currentTotalEdit.Text(),
		InvestAmount: investAmountEdit.Text(),
		MaxBuyCount:  maxBuyCountEdit.Text(),
		AssetsText:   assetsEdit.Text(),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, data, 0644)
}

func loadConfig() error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	currentTotalEdit.SetText(cfg.CurrentTotal)
	investAmountEdit.SetText(cfg.InvestAmount)
	maxBuyCountEdit.SetText(cfg.MaxBuyCount)
	assetsEdit.SetText(cfg.AssetsText)

	return nil
}

func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "，", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "％", "")

	if s == "" {
		return 0, nil
	}

	return strconv.ParseFloat(s, 64)
}
