package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppDataFilePathUsesPortfolioRebalancingDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)

	configPath, err := portfolioConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	recordsPath, err := recordsFilePath()
	if err != nil {
		t.Fatal(err)
	}

	wantDir := filepath.Join(base, appDataDirName)
	if configPath != filepath.Join(wantDir, portfolioConfigFileName) {
		t.Fatalf("config path = %q, want under %q", configPath, wantDir)
	}
	if recordsPath != filepath.Join(wantDir, recordsFileName) {
		t.Fatalf("records path = %q, want under %q", recordsPath, wantDir)
	}
	if info, err := os.Stat(wantDir); err != nil || !info.IsDir() {
		t.Fatalf("app data dir was not created: info=%v err=%v", info, err)
	}
}

func TestPortfolioConfigPersistsCalculatorPreferences(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	wantAssets := []AssetInput{{Name: "现金", TargetPct: 40, CurrentAmount: 1200}}
	want := portfolioConfigState{
		InvestAmount:                  3000,
		Assets:                        wantAssets,
		RelativeDeviationThresholdPct: 35,
		CalculatorTopCardHeight:       418,
		WindowWidth:                   1500,
		WindowHeight:                  950,
	}
	if err := savePortfolioConfig(want); err != nil {
		t.Fatal(err)
	}

	got, err := loadPortfolioConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.InvestAmount != want.InvestAmount {
		t.Fatalf("invest amount = %v, want %v", got.InvestAmount, want.InvestAmount)
	}
	if len(got.Assets) != 1 || got.Assets[0] != wantAssets[0] {
		t.Fatalf("assets = %#v, want %#v", got.Assets, wantAssets)
	}
	if got.CalculatorTopCardHeight != want.CalculatorTopCardHeight {
		t.Fatalf("top card height = %d, want %d", got.CalculatorTopCardHeight, want.CalculatorTopCardHeight)
	}
	if got.RelativeDeviationThresholdPct != want.RelativeDeviationThresholdPct {
		t.Fatalf("relative deviation threshold = %v, want %v", got.RelativeDeviationThresholdPct, want.RelativeDeviationThresholdPct)
	}
	if got.WindowWidth != want.WindowWidth || got.WindowHeight != want.WindowHeight {
		t.Fatalf("window size = %dx%d, want %dx%d", got.WindowWidth, got.WindowHeight, want.WindowWidth, want.WindowHeight)
	}
}

func TestPortfolioConfigUsesBundledDefaults(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	got, err := loadPortfolioConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.InvestAmount != 2000 {
		t.Fatalf("default invest amount = %v, want 2000", got.InvestAmount)
	}
	wantAssets := []AssetInput{
		{Name: "标普500", TargetPct: 25, CurrentAmount: 0},
		{Name: "纳指100", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "中证A500", TargetPct: 25, CurrentAmount: 0},
		{Name: "红利低波", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "0-3年政金债", TargetPct: 12.5, CurrentAmount: 0},
		{Name: "7-10年政金债", TargetPct: 12.5, CurrentAmount: 0},
	}
	if len(got.Assets) != len(wantAssets) {
		t.Fatalf("default assets count = %d, want %d", len(got.Assets), len(wantAssets))
	}
	for i := range wantAssets {
		if got.Assets[i] != wantAssets[i] {
			t.Fatalf("default asset %d = %#v, want %#v", i, got.Assets[i], wantAssets[i])
		}
	}
	if got.RelativeDeviationThresholdPct != 20 {
		t.Fatalf("default threshold = %v, want 20", got.RelativeDeviationThresholdPct)
	}
	if got.CalculatorTopCardHeight != 350 {
		t.Fatalf("default top card height = %d, want 350", got.CalculatorTopCardHeight)
	}
	if got.WindowWidth != 1360 || got.WindowHeight != 860 {
		t.Fatalf("default window size = %dx%d, want 1360x860", got.WindowWidth, got.WindowHeight)
	}
}

func TestLoadLegacyPortfolioConfigUsesDefaultDeviationThreshold(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	path, err := portfolioConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	legacy := portfolioConfigFile{
		Version:                 3,
		InvestAmount:            flexibleFloat(2000),
		Assets:                  []AssetInput{{Name: "债券", TargetPct: 100, CurrentAmount: 2000}},
		CalculatorTopCardHeight: 375,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := loadPortfolioConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.CalculatorTopCardHeight != 375 {
		t.Fatalf("legacy top card height = %d, want 375", got.CalculatorTopCardHeight)
	}
	if got.RelativeDeviationThresholdPct != defaultRelativeDeviationThresholdPct {
		t.Fatalf("legacy relative deviation threshold = %v, want %v", got.RelativeDeviationThresholdPct, defaultRelativeDeviationThresholdPct)
	}
	if got.WindowWidth != defaultWindowWidth || got.WindowHeight != defaultWindowHeight {
		t.Fatalf("legacy window size = %dx%d, want defaults", got.WindowWidth, got.WindowHeight)
	}
}
