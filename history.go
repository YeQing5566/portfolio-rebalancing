package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	recordsFileName            = "investment_records.json"
	recordsFileVersion         = 3
	archiveTimeFmt             = "2006-01-02 15:04:05"
	jsonFileFilter             = "JSON 文件 (*.json)|*.json|所有文件 (*.*)|*.*"
	saveArchiveSuccessMessage  = "保存成功，可在历史投资记录中查看"
	recordTypeBuy              = "buy"
	recordTypeSell             = "sell"
	recordTypeValuation        = "valuation"
	recordsBackupPrefix        = "investment_records_"
	recordsBackupTimeFmt       = "20060102_150405_000000000"
	maxInvestmentRecordBackups = 3
)

type InvestmentRecordsFile struct {
	Version int                `json:"version"`
	Records []InvestmentRecord `json:"records"`
}

// InvestmentRecord stores source events only. Aggregate values are rebuilt in
// recalculateInvestmentRecord after loading and are intentionally excluded
// from investment_records.json.
type InvestmentRecord struct {
	ID            string                  `json:"id"`
	RecordType    string                  `json:"record_type"`
	ArchivedAt    string                  `json:"archived_at"`
	Notes         string                  `json:"notes,omitempty"`
	InvestAmount  float64                 `json:"-"`
	SellAmount    float64                 `json:"-"`
	CurrentTotal  float64                 `json:"-"`
	AfterTotal    float64                 `json:"-"`
	AllocatedCash float64                 `json:"-"`
	RemainingCash float64                 `json:"-"`
	Assets        []InvestmentAssetRecord `json:"assets"`
}

// Only the fields relevant to a record type are populated, so omitempty keeps
// each persisted event compact: buy_amount for buys, sell_amount for sells,
// and target_pct/current_amount for valuation snapshots.
type InvestmentAssetRecord struct {
	Name          string  `json:"name"`
	TargetPct     float64 `json:"target_pct,omitempty"`
	CurrentAmount float64 `json:"current_amount,omitempty"`
	BuyAmount     float64 `json:"buy_amount,omitempty"`
	SellAmount    float64 `json:"sell_amount,omitempty"`
	CurrentPct    float64 `json:"-"`
	BeforeAmount  float64 `json:"-"`
	BeforePct     float64 `json:"-"`
	AfterAmount   float64 `json:"-"`
	AfterPct      float64 `json:"-"`
	LowLine       float64 `json:"-"`
	HighLine      float64 `json:"-"`
	Status        string  `json:"-"`
}

var (
	investmentRecords               []InvestmentRecord
	selectedHistoryIndex            = -1
	selectedAssetIndex              = -1
	selectedHistoryDraft            InvestmentRecord
	investmentRecordsWriteEnabled   bool
	investmentRecordsStartupWarning string
)

func transactionRecordFromAssets(recordType string, assets []AssetInput, occurredAt time.Time) InvestmentRecord {
	record := InvestmentRecord{
		ID:         fmt.Sprintf("%d", occurredAt.UnixNano()),
		RecordType: recordType,
		ArchivedAt: occurredAt.Format(archiveTimeFmt),
		Assets:     make([]InvestmentAssetRecord, 0, len(assets)),
	}
	for _, asset := range assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			continue
		}
		record.Assets = append(record.Assets, InvestmentAssetRecord{Name: name})
	}
	recalculateInvestmentRecord(&record)
	return record
}

func buyRecordFromAssets(assets []AssetInput, boughtAt time.Time) InvestmentRecord {
	return transactionRecordFromAssets(recordTypeBuy, assets, boughtAt)
}

func sellRecordFromAssets(assets []AssetInput, soldAt time.Time) InvestmentRecord {
	return transactionRecordFromAssets(recordTypeSell, assets, soldAt)
}

func valuationRecordFromAssets(assets []AssetInput, valuedAt time.Time) (InvestmentRecord, error) {
	record := InvestmentRecord{
		ID:         fmt.Sprintf("%d", valuedAt.UnixNano()),
		RecordType: recordTypeValuation,
		ArchivedAt: valuedAt.Format(archiveTimeFmt),
		Assets:     make([]InvestmentAssetRecord, 0, len(assets)),
	}
	for _, asset := range assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			continue
		}
		record.Assets = append(record.Assets, InvestmentAssetRecord{
			Name:          name,
			TargetPct:     asset.TargetPct,
			CurrentAmount: roundMoney(asset.CurrentAmount),
		})
	}
	recalculateInvestmentRecord(&record)
	if err := validateInvestmentRecord(record); err != nil {
		return InvestmentRecord{}, err
	}
	return record, nil
}

func finalizedTransactionRecord(record InvestmentRecord, recordType string) (InvestmentRecord, error) {
	record.RecordType = recordType
	record.ArchivedAt = strings.TrimSpace(record.ArchivedAt)
	if _, ok := parseArchiveTime(record.ArchivedAt); !ok {
		label := "买入"
		if recordType == recordTypeSell {
			label = "卖出"
		}
		return InvestmentRecord{}, fmt.Errorf("%s时间格式应为：2026-05-29 11:41:16", label)
	}

	assets := make([]InvestmentAssetRecord, 0, len(record.Assets))
	for i, asset := range record.Assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			return InvestmentRecord{}, fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		amount := asset.BuyAmount
		if recordType == recordTypeSell {
			amount = asset.SellAmount
		}
		amount = roundMoney(amount)
		if amount <= moneyEpsilon {
			continue
		}
		item := InvestmentAssetRecord{Name: name}
		if recordType == recordTypeSell {
			item.SellAmount = amount
		} else {
			item.BuyAmount = amount
		}
		assets = append(assets, item)
	}
	record.Assets = assets
	recalculateInvestmentRecord(&record)
	if err := validateInvestmentRecord(record); err != nil {
		return InvestmentRecord{}, err
	}
	return record, nil
}

func finalizedBuyRecord(record InvestmentRecord) (InvestmentRecord, error) {
	return finalizedTransactionRecord(record, recordTypeBuy)
}

func finalizedSellRecord(record InvestmentRecord) (InvestmentRecord, error) {
	return finalizedTransactionRecord(record, recordTypeSell)
}

// These compatibility helpers are kept for calculation-focused tests and
// internal callers. The calculator UI no longer archives its result directly.
func recordFromResult(result *PortfolioResult) InvestmentRecord {
	now := time.Now()
	record := InvestmentRecord{
		ID:         fmt.Sprintf("%d", now.UnixNano()),
		RecordType: recordTypeBuy,
		ArchivedAt: now.Format(archiveTimeFmt),
		Assets:     make([]InvestmentAssetRecord, 0, len(result.Assets)),
	}
	for _, asset := range result.Assets {
		if asset.BuyAmount > moneyEpsilon {
			record.Assets = append(record.Assets, InvestmentAssetRecord{Name: asset.Name, BuyAmount: roundMoney(asset.BuyAmount)})
		}
	}
	recalculateInvestmentRecord(&record)
	return record
}

func suggestedBuyAmounts(result *PortfolioResult) []float64 {
	if result == nil {
		return nil
	}
	amounts := make([]float64, 0, len(result.Assets))
	for _, asset := range result.Assets {
		amounts = append(amounts, math.Max(0, asset.BuyAmount))
	}
	return amounts
}

func recordFromResultWithActualBuys(result *PortfolioResult, buyAmounts []float64) (InvestmentRecord, error) {
	if result == nil {
		return InvestmentRecord{}, fmt.Errorf("没有可归档的计算结果")
	}
	if len(buyAmounts) != len(result.Assets) {
		return InvestmentRecord{}, fmt.Errorf("真实买入金额数量与资产数量不一致")
	}
	record := InvestmentRecord{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		RecordType: recordTypeBuy,
		ArchivedAt: time.Now().Format(archiveTimeFmt),
		Assets:     make([]InvestmentAssetRecord, 0, len(result.Assets)),
	}
	for i, amount := range buyAmounts {
		if amount < 0 {
			return InvestmentRecord{}, fmt.Errorf("%s 的真实买入金额不能为负数", result.Assets[i].Name)
		}
		if amount > moneyEpsilon {
			record.Assets = append(record.Assets, InvestmentAssetRecord{Name: result.Assets[i].Name, BuyAmount: roundMoney(amount)})
		}
	}
	recalculateInvestmentRecord(&record)
	return record, nil
}

func appendInvestmentRecord(record InvestmentRecord) error {
	recalculateInvestmentRecord(&record)
	if err := validateInvestmentRecord(record); err != nil {
		return err
	}
	previous := cloneInvestmentRecords(investmentRecords)
	investmentRecords = append(investmentRecords, cloneInvestmentRecord(record))
	sortInvestmentRecords(investmentRecords)
	if err := saveInvestmentRecords(); err != nil {
		investmentRecords = previous
		return err
	}
	return nil
}

func recordsFilePath() (string, error) {
	return appDataFilePath(recordsFileName)
}

func loadInvestmentRecords() error {
	investmentRecords = nil
	investmentRecordsWriteEnabled = false
	investmentRecordsStartupWarning = ""
	path, err := recordsFilePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		records, backupName, found, restoreErr := restoreInvestmentRecordsFromLatestBackup(path)
		if restoreErr != nil {
			return fmt.Errorf("正式历史记录文件不存在，且备份恢复失败：%w", restoreErr)
		}
		if !found {
			investmentRecordsWriteEnabled = true
			return nil
		}
		investmentRecords = records
		investmentRecordsWriteEnabled = true
		investmentRecordsStartupWarning = recoveredInvestmentRecordsMessage(backupName, "正式历史记录文件不存在")
		return nil
	}
	if err != nil {
		return err
	}
	records, err := decodeInvestmentRecords(data)
	if err != nil {
		recovered, backupName, found, restoreErr := restoreInvestmentRecordsFromLatestBackup(path)
		if restoreErr != nil {
			return fmt.Errorf("正式历史记录文件损坏（%v），且备份恢复失败：%w", err, restoreErr)
		}
		if !found {
			return fmt.Errorf("正式历史记录文件损坏（%w），未找到可用的有效备份", err)
		}
		investmentRecords = recovered
		investmentRecordsWriteEnabled = true
		investmentRecordsStartupWarning = recoveredInvestmentRecordsMessage(backupName, "正式历史记录文件未通过 JSON、版本或记录内容校验")
		return nil
	}
	investmentRecords = records
	if err := backupInvestmentRecordsData(path, data, time.Now()); err != nil {
		return fmt.Errorf("历史记录已通过校验，但启动备份失败，已禁止覆盖正式文件：%w", err)
	}
	investmentRecordsWriteEnabled = true
	return nil
}

func saveInvestmentRecords() error {
	if !investmentRecordsWriteEnabled {
		return fmt.Errorf("启动时未能完成历史记录校验和备份，已禁止覆盖正式文件")
	}
	path, err := recordsFilePath()
	if err != nil {
		return err
	}
	return writeInvestmentRecordsFile(path, investmentRecords)
}

func readInvestmentRecordsFile(path string) ([]InvestmentRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeInvestmentRecords(data)
}

func decodeInvestmentRecords(data []byte) ([]InvestmentRecord, error) {
	var file InvestmentRecordsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("记录文件格式错误：%w", err)
	}
	if file.Version != recordsFileVersion {
		return nil, fmt.Errorf("历史记录文件版本不受支持：当前为 %d，需要版本 %d", file.Version, recordsFileVersion)
	}
	records := append([]InvestmentRecord(nil), file.Records...)
	for i := range records {
		recalculateInvestmentRecord(&records[i])
		if err := validateInvestmentRecord(records[i]); err != nil {
			return nil, fmt.Errorf("第 %d 条历史记录无效：%w", i+1, err)
		}
	}
	sortInvestmentRecords(records)
	return records, nil
}

func writeInvestmentRecordsFile(path string, records []InvestmentRecord) error {
	data, err := json.MarshalIndent(InvestmentRecordsFile{Version: recordsFileVersion, Records: records}, "", "  ")
	if err != nil {
		return fmt.Errorf("生成记录文件失败：%w", err)
	}
	if err := atomicWriteInvestmentRecordsData(path, data); err != nil {
		return fmt.Errorf("原子写入记录文件失败：%w", err)
	}
	return nil
}

func atomicWriteInvestmentRecordsData(path string, data []byte) error {
	if _, err := decodeInvestmentRecords(data); err != nil {
		return fmt.Errorf("待写入数据校验失败：%w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建记录目录失败：%w", err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败：%w", err)
	}
	tempPath := temp.Name()
	replaced := false
	defer func() {
		_ = temp.Close()
		if !replaced {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0644); err != nil {
		return fmt.Errorf("设置临时文件权限失败：%w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("写入临时文件失败：%w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("同步临时文件失败：%w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败：%w", err)
	}
	if _, err := readInvestmentRecordsFile(tempPath); err != nil {
		return fmt.Errorf("临时文件落盘校验失败：%w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("替换正式文件失败：%w", err)
	}
	replaced = true
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func backupInvestmentRecordsData(recordsPath string, data []byte, now time.Time) error {
	if _, err := decodeInvestmentRecords(data); err != nil {
		return fmt.Errorf("正式文件未通过校验，不创建备份：%w", err)
	}
	dir := filepath.Dir(recordsPath)
	backupName := recordsBackupPrefix + now.Format(recordsBackupTimeFmt) + ".json"
	backupPath := filepath.Join(dir, backupName)
	if _, err := os.Stat(backupPath); err == nil {
		return fmt.Errorf("备份文件已存在：%s", backupName)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查备份文件失败：%w", err)
	}
	if err := atomicWriteInvestmentRecordsData(backupPath, data); err != nil {
		return fmt.Errorf("创建启动备份失败：%w", err)
	}
	if err := pruneInvestmentRecordBackups(dir, maxInvestmentRecordBackups); err != nil {
		return fmt.Errorf("清理旧备份失败：%w", err)
	}
	return nil
}

func restoreInvestmentRecordsFromLatestBackup(recordsPath string) ([]InvestmentRecord, string, bool, error) {
	backups, err := investmentRecordBackupPaths(filepath.Dir(recordsPath))
	if err != nil {
		return nil, "", false, err
	}
	if len(backups) == 0 {
		return nil, "", false, nil
	}
	invalid := 0
	for i := len(backups) - 1; i >= 0; i-- {
		backupPath := backups[i]
		data, err := os.ReadFile(backupPath)
		if err != nil {
			invalid++
			continue
		}
		records, err := decodeInvestmentRecords(data)
		if err != nil {
			invalid++
			continue
		}
		if err := atomicWriteInvestmentRecordsData(recordsPath, data); err != nil {
			return nil, "", false, fmt.Errorf("使用备份 %s 恢复正式文件失败：%w", filepath.Base(backupPath), err)
		}
		return records, filepath.Base(backupPath), true, nil
	}
	return nil, "", false, fmt.Errorf("共找到 %d 份备份，但均无法通过完整读取校验", invalid)
}

func recoveredInvestmentRecordsMessage(backupName, reason string) string {
	return fmt.Sprintf(
		"%s。\r\n\r\n程序已读取最近一次有效备份并恢复正式文件：\r\n%s\r\n\r\n请确认是否丢失部分历史投资记录。",
		reason,
		backupName,
	)
}

func pruneInvestmentRecordBackups(dir string, keep int) error {
	backups, err := investmentRecordBackupPaths(dir)
	if err != nil {
		return err
	}
	for len(backups) > maxInt(0, keep) {
		oldest := backups[0]
		if err := os.Remove(oldest); err != nil {
			return err
		}
		backups = backups[1:]
	}
	return nil
}

func investmentRecordBackupPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	backups := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isInvestmentRecordBackupName(name) {
			continue
		}
		backups = append(backups, filepath.Join(dir, name))
	}
	sort.Strings(backups)
	return backups, nil
}

func isInvestmentRecordBackupName(name string) bool {
	if !strings.HasPrefix(name, recordsBackupPrefix) || !strings.HasSuffix(name, ".json") {
		return false
	}
	timestamp := strings.TrimSuffix(strings.TrimPrefix(name, recordsBackupPrefix), ".json")
	_, err := time.Parse(recordsBackupTimeFmt, timestamp)
	return err == nil
}

func sortInvestmentRecords(records []InvestmentRecord) {
	sort.SliceStable(records, func(i, j int) bool { return records[i].ArchivedAt > records[j].ArchivedAt })
}

func recalculateInvestmentRecord(record *InvestmentRecord) {
	if record == nil {
		return
	}
	record.InvestAmount = 0
	record.SellAmount = 0
	record.CurrentTotal = 0
	record.AfterTotal = 0
	record.AllocatedCash = 0
	record.RemainingCash = 0

	switch {
	case isBuyRecord(*record):
		for i := range record.Assets {
			record.Assets[i].Name = strings.TrimSpace(record.Assets[i].Name)
			record.Assets[i].BuyAmount = roundMoney(record.Assets[i].BuyAmount)
			record.InvestAmount += record.Assets[i].BuyAmount
		}
		record.InvestAmount = roundMoney(record.InvestAmount)
		record.AllocatedCash = record.InvestAmount
	case isSellRecord(*record):
		for i := range record.Assets {
			record.Assets[i].Name = strings.TrimSpace(record.Assets[i].Name)
			record.Assets[i].SellAmount = roundMoney(record.Assets[i].SellAmount)
			record.SellAmount += record.Assets[i].SellAmount
		}
		record.SellAmount = roundMoney(record.SellAmount)
	case isValuationRecord(*record):
		for i := range record.Assets {
			asset := &record.Assets[i]
			asset.Name = strings.TrimSpace(asset.Name)
			asset.CurrentAmount = roundMoney(asset.CurrentAmount)
			record.CurrentTotal += asset.CurrentAmount
		}
		record.CurrentTotal = roundMoney(record.CurrentTotal)
		record.AfterTotal = record.CurrentTotal
		for i := range record.Assets {
			asset := &record.Assets[i]
			asset.CurrentPct = 0
			if record.CurrentTotal > moneyEpsilon {
				asset.CurrentPct = asset.CurrentAmount / record.CurrentTotal * 100
			}
			// Derived aliases keep shared formatting helpers simple.
			asset.BeforeAmount = asset.CurrentAmount
			asset.BeforePct = asset.CurrentPct
			asset.AfterAmount = asset.CurrentAmount
			asset.AfterPct = asset.CurrentPct
		}
	}
}

func recalculateInvestmentRecordWithDeviationThreshold(record *InvestmentRecord, _ float64) {
	recalculateInvestmentRecord(record)
}

func relativeTargetDeviationPct(asset InvestmentAssetRecord) (float64, bool) {
	if asset.TargetPct <= moneyEpsilon {
		return 0, false
	}
	pct := asset.CurrentPct
	if math.Abs(pct) <= moneyEpsilon {
		pct = asset.AfterPct
	}
	return (pct - asset.TargetPct) / asset.TargetPct * 100, true
}

func validateInvestmentRecord(record InvestmentRecord) error {
	if _, ok := parseArchiveTime(record.ArchivedAt); !ok {
		return fmt.Errorf("记录时间格式应为：2026-05-29 11:41:16")
	}
	switch {
	case isBuyRecord(record):
		return validateTransactionRecord(record, false)
	case isSellRecord(record):
		return validateTransactionRecord(record, true)
	case isValuationRecord(record):
		return validateValuationRecord(record)
	default:
		return fmt.Errorf("未知历史记录类型：%s", record.RecordType)
	}
}

func validateTransactionRecord(record InvestmentRecord, sell bool) error {
	label := "买入"
	if sell {
		label = "卖出"
	}
	if len(record.Assets) == 0 {
		return fmt.Errorf("没有填写%s金额，不生成%s记录", label, label)
	}
	seen := make(map[string]struct{}, len(record.Assets))
	total := 0.0
	for i, asset := range record.Assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			return fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("资产名称不能重复：%s", name)
		}
		seen[key] = struct{}{}
		amount := asset.BuyAmount
		if sell {
			amount = asset.SellAmount
		}
		if amount < 0 {
			return fmt.Errorf("%s 的%s金额不能为负数", name, label)
		}
		total += amount
	}
	if total <= moneyEpsilon {
		return fmt.Errorf("没有填写%s金额，不生成%s记录", label, label)
	}
	return nil
}

func validateValuationRecord(record InvestmentRecord) error {
	if len(record.Assets) == 0 {
		return fmt.Errorf("没有可保存的资产条目")
	}
	seen := make(map[string]struct{}, len(record.Assets))
	targetSum := 0.0
	for i, asset := range record.Assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			return fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("资产名称不能重复：%s", name)
		}
		seen[key] = struct{}{}
		if asset.TargetPct <= 0 || asset.TargetPct > 100 {
			return fmt.Errorf("%s 的目标仓位必须大于 0%% 且不超过 100%%", name)
		}
		if asset.CurrentAmount < 0 {
			return fmt.Errorf("%s 的当前持有金额不能为负数", name)
		}
		targetSum += asset.TargetPct
	}
	if math.Abs(targetSum-100) > 0.01 {
		return fmt.Errorf("目标仓位合计必须为 100%%，当前为 %s", formatPercent(targetSum))
	}
	return nil
}

func validateSellRecord(record InvestmentRecord) error {
	return validateTransactionRecord(record, true)
}

func cloneInvestmentRecord(record InvestmentRecord) InvestmentRecord {
	clone := record
	clone.Assets = append([]InvestmentAssetRecord(nil), record.Assets...)
	return clone
}

func cloneInvestmentRecords(records []InvestmentRecord) []InvestmentRecord {
	clone := make([]InvestmentRecord, len(records))
	for i, record := range records {
		clone[i] = cloneInvestmentRecord(record)
	}
	return clone
}

func isBuyRecord(record InvestmentRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.RecordType), recordTypeBuy)
}

func isSellRecord(record InvestmentRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.RecordType), recordTypeSell)
}

func isValuationRecord(record InvestmentRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.RecordType), recordTypeValuation)
}

func recordBuyTotal(record InvestmentRecord) float64 {
	total := 0.0
	for _, asset := range record.Assets {
		total += asset.BuyAmount
	}
	return roundMoney(total)
}

func recordSellTotal(record InvestmentRecord) float64 {
	total := 0.0
	for _, asset := range record.Assets {
		total += asset.SellAmount
	}
	return roundMoney(total)
}
