package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvestmentRecordsJSONOmitsDerivedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.json")
	record := InvestmentRecord{
		ID:            "record-1",
		RecordType:    recordTypeBuy,
		ArchivedAt:    "2026-07-21 12:00:00",
		InvestAmount:  1000,
		CurrentTotal:  10000,
		AfterTotal:    11000,
		AllocatedCash: 1000,
		Assets: []InvestmentAssetRecord{{
			Name:         "资产A",
			BeforeAmount: 10000,
			BeforePct:    100,
			BuyAmount:    1000,
			AfterAmount:  11000,
			AfterPct:     100,
			LowLine:      80,
			HighLine:     120,
			Status:       "接近目标",
		}},
	}
	if err := writeInvestmentRecordsFile(path, []InvestmentRecord{record}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, key := range []string{
		"invest_amount", "sell_amount", "current_total", "after_total", "allocated_cash", "remaining_cash",
		"target_pct", "current_amount", "before_amount", "before_pct", "after_amount", "after_pct", "low_line", "high_line", "status", "relative_deviation_threshold_pct",
	} {
		if strings.Contains(text, `"`+key+`"`) {
			t.Fatalf("derived key %q should not be persisted:\n%s", key, text)
		}
	}
	for _, key := range []string{"version", "id", "record_type", "archived_at", "assets", "name", "buy_amount"} {
		if !strings.Contains(text, `"`+key+`"`) {
			t.Fatalf("source key %q should be persisted:\n%s", key, text)
		}
	}

	loaded, err := readInvestmentRecordsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].InvestAmount != 1000 || loaded[0].AllocatedCash != 1000 {
		t.Fatalf("derived values were not rebuilt after load: %#v", loaded)
	}
}

func TestInvestmentRecordsRejectLegacyVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"records":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readInvestmentRecordsFile(path); err == nil {
		t.Fatal("legacy history version should be rejected")
	}
}
