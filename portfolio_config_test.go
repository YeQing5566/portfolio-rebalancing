package main

import (
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
