package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadInvestmentRecordsCreatesBackupOnlyAfterValidation(t *testing.T) {
	restore := preserveInvestmentRecordGlobals()
	defer restore()
	t.Setenv("APPDATA", t.TempDir())
	path, err := recordsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeInvestmentRecordsFile(path, sampleInvestmentRecords()); err != nil {
		t.Fatal(err)
	}

	if err := loadInvestmentRecords(); err != nil {
		t.Fatal(err)
	}
	backups, err := investmentRecordBackupPaths(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("got %d startup backups, want 1", len(backups))
	}
	if _, err := readInvestmentRecordsFile(backups[0]); err != nil {
		t.Fatalf("startup backup is invalid: %v", err)
	}
	if !investmentRecordsWriteEnabled || investmentRecordsStartupWarning != "" {
		t.Fatalf("unexpected startup state: writeEnabled=%v warning=%q", investmentRecordsWriteEnabled, investmentRecordsStartupWarning)
	}
}

func TestInvestmentRecordBackupsKeepLatestThree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recordsFileName)
	if err := writeInvestmentRecordsFile(path, sampleInvestmentRecords()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 22, 9, 0, 0, 0, time.Local)
	for i := 0; i < 4; i++ {
		if err := backupInvestmentRecordsData(path, data, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := investmentRecordBackupPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != maxInvestmentRecordBackups {
		t.Fatalf("got %d backups, want %d", len(backups), maxInvestmentRecordBackups)
	}
	if strings.Contains(filepath.Base(backups[0]), base.Format(recordsBackupTimeFmt)) {
		t.Fatalf("oldest backup was not removed: %v", backups)
	}
}

func TestCorruptOfficialFileDoesNotCreateOrPruneBackups(t *testing.T) {
	restore := preserveInvestmentRecordGlobals()
	defer restore()
	t.Setenv("APPDATA", t.TempDir())
	path, err := recordsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":3,"records":[`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := loadInvestmentRecords(); err == nil {
		t.Fatal("corrupt official file should fail when no valid backup exists")
	}
	backups, err := investmentRecordBackupPaths(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("corrupt official file must not create a backup: %v", backups)
	}
	if investmentRecordsWriteEnabled {
		t.Fatal("writes must remain disabled after unrecoverable startup validation failure")
	}
}

func TestCorruptOfficialFileRestoresLatestValidBackup(t *testing.T) {
	restore := preserveInvestmentRecordGlobals()
	defer restore()
	t.Setenv("APPDATA", t.TempDir())
	path, err := recordsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeInvestmentRecordsFile(path, sampleInvestmentRecords()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backupAt := time.Date(2026, 7, 22, 9, 30, 0, 0, time.Local)
	if err := backupInvestmentRecordsData(path, data, backupAt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := loadInvestmentRecords(); err != nil {
		t.Fatal(err)
	}
	backupName := recordsBackupPrefix + backupAt.Format(recordsBackupTimeFmt) + ".json"
	if !strings.Contains(investmentRecordsStartupWarning, backupName) || !strings.Contains(investmentRecordsStartupWarning, "请确认是否丢失部分历史投资记录") {
		t.Fatalf("recovery warning is incomplete: %q", investmentRecordsStartupWarning)
	}
	if !investmentRecordsWriteEnabled {
		t.Fatal("writes should be enabled after the official file is restored atomically")
	}
	loaded, err := readInvestmentRecordsFile(path)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("restored official file is invalid: records=%d err=%v", len(loaded), err)
	}
}

func TestRecoverySkipsInvalidNewestBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recordsFileName)
	if err := writeInvestmentRecordsFile(path, sampleInvestmentRecords()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 7, 22, 9, 0, 0, 0, time.Local)
	newer := older.Add(time.Hour)
	if err := backupInvestmentRecordsData(path, data, older); err != nil {
		t.Fatal(err)
	}
	invalidNewest := filepath.Join(dir, recordsBackupPrefix+newer.Format(recordsBackupTimeFmt)+".json")
	if err := os.WriteFile(invalidNewest, []byte(`broken`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`broken official`), 0644); err != nil {
		t.Fatal(err)
	}

	_, backupName, found, err := restoreInvestmentRecordsFromLatestBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	wantName := recordsBackupPrefix + older.Format(recordsBackupTimeFmt) + ".json"
	if !found || backupName != wantName {
		t.Fatalf("restored backup = %q found=%v, want %q", backupName, found, wantName)
	}
}

func TestAtomicWriteRejectsInvalidReplacementAndKeepsOfficialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recordsFileName)
	if err := writeInvestmentRecordsFile(path, sampleInvestmentRecords()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []InvestmentRecord{{ID: "invalid", RecordType: "unknown", ArchivedAt: "2026-07-22 09:00:00"}}
	if err := writeInvestmentRecordsFile(path, invalid); err == nil {
		t.Fatal("invalid replacement should be rejected")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("official file changed after an invalid atomic replacement")
	}
	temps, err := filepath.Glob(filepath.Join(dir, "."+recordsFileName+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", temps)
	}
}

func sampleInvestmentRecords() []InvestmentRecord {
	return []InvestmentRecord{{
		ID:         "sample-buy",
		RecordType: recordTypeBuy,
		ArchivedAt: "2026-07-22 09:00:00",
		Assets:     []InvestmentAssetRecord{{Name: "资产A", BuyAmount: 100}},
	}}
}

func preserveInvestmentRecordGlobals() func() {
	records := cloneInvestmentRecords(investmentRecords)
	writeEnabled := investmentRecordsWriteEnabled
	warning := investmentRecordsStartupWarning
	return func() {
		investmentRecords = records
		investmentRecordsWriteEnabled = writeEnabled
		investmentRecordsStartupWarning = warning
	}
}
