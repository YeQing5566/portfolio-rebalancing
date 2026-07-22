package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	portfolioConfigFileName        = "portfolio_config.json"
	portfolioConfigFileVersion     = 5
	appDataDirName                 = "PortfolioRebalancing"
	defaultInvestAmount            = 2000.0
	defaultCalculatorTopCardHeight = 350
	defaultWindowWidth             = 1360
	defaultWindowHeight            = 860
)

type portfolioConfigFile struct {
	Version                       int           `json:"version"`
	InvestAmount                  flexibleFloat `json:"invest_amount"`
	Assets                        []AssetInput  `json:"assets"`
	RelativeDeviationThresholdPct flexibleFloat `json:"relative_deviation_threshold_pct,omitempty"`
	CalculatorTopCardHeight       int           `json:"calculator_top_card_height,omitempty"`
	WindowWidth                   int           `json:"window_width,omitempty"`
	WindowHeight                  int           `json:"window_height,omitempty"`
}

type portfolioConfigState struct {
	InvestAmount                  float64
	Assets                        []AssetInput
	RelativeDeviationThresholdPct float64
	CalculatorTopCardHeight       int
	WindowWidth                   int
	WindowHeight                  int
}

type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*f = flexibleFloat(value)
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	if text == "" {
		*f = 0
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*f = flexibleFloat(parsed)
	return nil
}

func (f flexibleFloat) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(f))
}

func appDataFilePath(fileName string) (string, error) {
	dir, err := portfolioAppDataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建数据目录失败：%w", err)
	}
	return filepath.Join(dir, fileName), nil
}

func portfolioAppDataDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("APPDATA"))
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("无法确定 APPDATA 目录：%w", err)
		}
	}
	return filepath.Join(base, appDataDirName), nil
}

func portfolioConfigPath() (string, error) {
	return appDataFilePath(portfolioConfigFileName)
}

func defaultPortfolioAssets() []AssetInput {
	return []AssetInput{
		{Name: "标普500", TargetPct: 25, CurrentAmount: 0},
		{Name: "纳指100", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "中证A500", TargetPct: 25, CurrentAmount: 0},
		{Name: "红利低波", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "0-3年政金债", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "7-10年政金债", TargetPct: 12.5, CurrentAmount: 0},
	}
}

func defaultPortfolioConfigState() portfolioConfigState {
	return portfolioConfigState{
		InvestAmount:                  defaultInvestAmount,
		Assets:                        defaultPortfolioAssets(),
		RelativeDeviationThresholdPct: defaultRelativeDeviationThresholdPct,
		CalculatorTopCardHeight:       defaultCalculatorTopCardHeight,
		WindowWidth:                   defaultWindowWidth,
		WindowHeight:                  defaultWindowHeight,
	}
}

func loadPortfolioConfig() (portfolioConfigState, error) {
	state := defaultPortfolioConfigState()
	path, err := portfolioConfigPath()
	if err != nil {
		return state, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}

	var file portfolioConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return state, fmt.Errorf("当前资产配置文件格式错误：%w", err)
	}
	if investAmount := float64(file.InvestAmount); investAmount >= 0 {
		state.InvestAmount = investAmount
	}
	state.Assets = clonePortfolioAssets(file.Assets)
	state.RelativeDeviationThresholdPct = normalizedRelativeDeviationThresholdPct(float64(file.RelativeDeviationThresholdPct))
	if file.CalculatorTopCardHeight > 0 {
		state.CalculatorTopCardHeight = file.CalculatorTopCardHeight
	}
	if file.WindowWidth > 0 {
		state.WindowWidth = file.WindowWidth
	}
	if file.WindowHeight > 0 {
		state.WindowHeight = file.WindowHeight
	}
	return state, nil
}

func savePortfolioConfig(state portfolioConfigState) error {
	path, err := portfolioConfigPath()
	if err != nil {
		return err
	}
	if state.InvestAmount < 0 {
		state.InvestAmount = 0
	}
	if state.CalculatorTopCardHeight <= 0 {
		state.CalculatorTopCardHeight = defaultCalculatorTopCardHeight
	}
	if state.WindowWidth <= 0 {
		state.WindowWidth = defaultWindowWidth
	}
	if state.WindowHeight <= 0 {
		state.WindowHeight = defaultWindowHeight
	}
	state.RelativeDeviationThresholdPct = normalizedRelativeDeviationThresholdPct(state.RelativeDeviationThresholdPct)
	file := portfolioConfigFile{
		Version:                       portfolioConfigFileVersion,
		InvestAmount:                  flexibleFloat(state.InvestAmount),
		Assets:                        filledPortfolioAssets(state.Assets),
		RelativeDeviationThresholdPct: flexibleFloat(state.RelativeDeviationThresholdPct),
		CalculatorTopCardHeight:       state.CalculatorTopCardHeight,
		WindowWidth:                   state.WindowWidth,
		WindowHeight:                  state.WindowHeight,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("生成当前资产配置失败：%w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入当前资产配置失败：%w", err)
	}
	return nil
}

func filledPortfolioAssets(assets []AssetInput) []AssetInput {
	items := make([]AssetInput, 0, len(assets))
	for _, asset := range assets {
		normalized := AssetInput{
			Name:          strings.TrimSpace(asset.Name),
			TargetPct:     asset.TargetPct,
			CurrentAmount: asset.CurrentAmount,
		}
		if isBlankAsset(normalized) {
			continue
		}
		items = append(items, normalized)
	}
	return items
}

func clonePortfolioAssets(assets []AssetInput) []AssetInput {
	return append([]AssetInput(nil), assets...)
}

func portfolioAssetsFromHistory(record InvestmentRecord) []AssetInput {
	if !isValuationRecord(record) {
		return nil
	}
	assets := make([]AssetInput, 0, len(record.Assets))
	for _, asset := range record.Assets {
		item := AssetInput{
			Name:          strings.TrimSpace(asset.Name),
			TargetPct:     asset.TargetPct,
			CurrentAmount: roundMoney(asset.CurrentAmount),
		}
		if isBlankAsset(item) {
			continue
		}
		assets = append(assets, item)
	}
	return assets
}
