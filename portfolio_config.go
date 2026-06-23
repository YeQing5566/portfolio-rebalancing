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
	portfolioConfigFileName    = "portfolio_config.json"
	portfolioConfigFileVersion = 2
)

type portfolioConfigFile struct {
	Version      int           `json:"version"`
	InvestAmount flexibleFloat `json:"invest_amount"`
	Assets       []AssetInput  `json:"assets"`
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
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法确定程序目录：%w", err)
	}
	return filepath.Join(filepath.Dir(executable), fileName), nil
}

func portfolioConfigPath() (string, error) {
	return appDataFilePath(portfolioConfigFileName)
}

func loadPortfolioConfig(defaultInvestAmount float64) (float64, []AssetInput, error) {
	path, err := portfolioConfigPath()
	if err != nil {
		return defaultInvestAmount, nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultInvestAmount, nil, nil
	}
	if err != nil {
		return defaultInvestAmount, nil, err
	}

	var file portfolioConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return defaultInvestAmount, nil, fmt.Errorf("当前资产配置文件格式错误：%w", err)
	}
	investAmount := float64(file.InvestAmount)
	if investAmount < 0 {
		investAmount = defaultInvestAmount
	}
	return investAmount, clonePortfolioAssets(file.Assets), nil
}

func savePortfolioConfig(investAmount float64, assets []AssetInput) error {
	path, err := portfolioConfigPath()
	if err != nil {
		return err
	}
	if investAmount < 0 {
		investAmount = 0
	}
	file := portfolioConfigFile{
		Version:      portfolioConfigFileVersion,
		InvestAmount: flexibleFloat(investAmount),
		Assets:       filledPortfolioAssets(assets),
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
	assets := make([]AssetInput, 0, len(record.Assets))
	for _, asset := range record.Assets {
		item := AssetInput{
			Name:          strings.TrimSpace(asset.Name),
			TargetPct:     asset.TargetPct,
			CurrentAmount: roundMoney(asset.AfterAmount),
		}
		if isBlankAsset(item) {
			continue
		}
		assets = append(assets, item)
	}
	return assets
}
