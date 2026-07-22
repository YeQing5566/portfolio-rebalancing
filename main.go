package main

import (
	"os"
	"strings"

	"github.com/lxn/walk"
)

var (
	defaultTextColor = walk.RGB(245, 245, 245)
	accentColor      = walk.RGB(255, 153, 0)
	secondaryColor   = walk.RGB(255, 177, 59)
	warningColor     = walk.RGB(251, 191, 36)
	dangerColor      = walk.RGB(239, 68, 68)
)

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

func isBlankAsset(item AssetInput) bool {
	return strings.TrimSpace(item.Name) == "" && item.TargetPct == 0 && item.CurrentAmount == 0
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

func initialResultText() string {
	return "计算建议前，可先更新各资产持有金额，并点击更新收益按钮保存各资产金额，结合买入卖出记录，可在收益数据测算界面计算出收益数据"
}

func showStartupError(err error) {
	walk.MsgBox(nil, "启动失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
	os.Exit(1)
}
