package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	recordsFileName           = "investment_records.json"
	recordsFileVersion        = 1
	archiveTimeFmt            = "2006-01-02 15:04:05"
	jsonFileFilter            = "JSON 文件 (*.json)|*.json|所有文件 (*.*)|*.*"
	saveArchiveSuccessMessage = "保存成功，可在历史投资记录中查看"
	recordTypeSell            = "sell"
)

type InvestmentRecordsFile struct {
	Version int                `json:"version"`
	Records []InvestmentRecord `json:"records"`
}

type InvestmentRecord struct {
	ID            string                  `json:"id"`
	RecordType    string                  `json:"record_type,omitempty"`
	ArchivedAt    string                  `json:"archived_at"`
	Notes         string                  `json:"notes,omitempty"`
	InvestAmount  float64                 `json:"invest_amount"`
	SellAmount    float64                 `json:"sell_amount,omitempty"`
	CurrentTotal  float64                 `json:"current_total"`
	AfterTotal    float64                 `json:"after_total"`
	AllocatedCash float64                 `json:"allocated_cash"`
	RemainingCash float64                 `json:"remaining_cash,omitempty"`
	Assets        []InvestmentAssetRecord `json:"assets"`
}

type InvestmentAssetRecord struct {
	Name         string  `json:"name"`
	TargetPct    float64 `json:"target_pct"`
	BeforeAmount float64 `json:"before_amount"`
	BeforePct    float64 `json:"before_pct"`
	BuyAmount    float64 `json:"buy_amount"`
	SellAmount   float64 `json:"sell_amount,omitempty"`
	AfterAmount  float64 `json:"after_amount"`
	AfterPct     float64 `json:"after_pct"`
	LowLine      float64 `json:"low_line"`
	HighLine     float64 `json:"high_line"`
	Status       string  `json:"status"`
}

var (
	investmentRecords    []InvestmentRecord
	selectedHistoryIndex = -1
	selectedAssetIndex   = -1
	selectedHistoryDraft InvestmentRecord
)

func recordFromResult(result *PortfolioResult) InvestmentRecord {
	now := time.Now()
	record := InvestmentRecord{
		ID:            fmt.Sprintf("%d", now.UnixNano()),
		ArchivedAt:    now.Format(archiveTimeFmt),
		InvestAmount:  result.AllocatedCash,
		CurrentTotal:  result.CurrentTotal,
		AfterTotal:    result.CurrentTotal + result.AllocatedCash,
		AllocatedCash: result.AllocatedCash,
		RemainingCash: 0,
		Assets:        make([]InvestmentAssetRecord, 0, len(result.Assets)),
	}
	for _, asset := range result.Assets {
		record.Assets = append(record.Assets, InvestmentAssetRecord{
			Name:         asset.Name,
			TargetPct:    asset.TargetPct,
			BeforeAmount: asset.CurrentAmount,
			BeforePct:    asset.CurrentPct,
			BuyAmount:    asset.BuyAmount,
			AfterAmount:  asset.CurrentAmount + asset.BuyAmount,
			AfterPct:     asset.AfterPct,
			LowLine:      asset.LowLine,
			HighLine:     asset.HighLine,
			Status:       asset.Status,
		})
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
		amounts = append(amounts, asset.BuyAmount)
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

	record := recordFromResult(result)
	actualInvest := 0.0
	for i, buyAmount := range buyAmounts {
		if buyAmount < 0 {
			return InvestmentRecord{}, fmt.Errorf("%s 的真实买入金额不能为负数", record.Assets[i].Name)
		}
		record.Assets[i].BuyAmount = roundMoney(buyAmount)
		actualInvest += record.Assets[i].BuyAmount
	}
	record.InvestAmount = roundMoney(actualInvest)
	recalculateInvestmentRecord(&record)
	return record, nil
}

func sellRecordFromAssets(assets []AssetInput, soldAt time.Time) InvestmentRecord {
	record := InvestmentRecord{
		ID:         fmt.Sprintf("%d", soldAt.UnixNano()),
		RecordType: recordTypeSell,
		ArchivedAt: soldAt.Format(archiveTimeFmt),
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

func finalizedSellRecord(record InvestmentRecord) (InvestmentRecord, error) {
	record.RecordType = recordTypeSell
	archivedAt := strings.TrimSpace(record.ArchivedAt)
	parsed, err := time.ParseInLocation(archiveTimeFmt, archivedAt, time.Local)
	if err != nil {
		return InvestmentRecord{}, fmt.Errorf("卖出时间格式应为：2026-05-29 11:41:16")
	}
	record.ArchivedAt = parsed.Format(archiveTimeFmt)
	if strings.TrimSpace(record.ID) == "" {
		record.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	filtered := make([]InvestmentAssetRecord, 0, len(record.Assets))
	for i, asset := range record.Assets {
		amount := roundMoney(asset.SellAmount)
		if amount <= moneyEpsilon {
			continue
		}
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			return InvestmentRecord{}, fmt.Errorf("第 %d 项资产名称不能为空", i+1)
		}
		filtered = append(filtered, InvestmentAssetRecord{
			Name:       name,
			SellAmount: amount,
		})
	}
	record.Assets = filtered
	recalculateInvestmentRecord(&record)
	if record.SellAmount <= moneyEpsilon {
		return InvestmentRecord{}, fmt.Errorf("没有填写卖出金额，不生成卖出记录")
	}
	if err := validateInvestmentRecord(record); err != nil {
		return InvestmentRecord{}, err
	}
	return record, nil
}

func appendInvestmentRecord(record InvestmentRecord) error {
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
	path, err := recordsFilePath()
	if err != nil {
		return err
	}

	records, err := readInvestmentRecordsFile(path)
	if os.IsNotExist(err) {
		investmentRecords = nil
		return nil
	}
	if err != nil {
		return err
	}

	investmentRecords = records
	return nil
}

func saveInvestmentRecords() error {
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

	var file InvestmentRecordsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("记录文件格式错误：%w", err)
	}

	records := append([]InvestmentRecord(nil), file.Records...)
	for i := range records {
		recalculateInvestmentRecord(&records[i])
	}
	sortInvestmentRecords(records)
	return records, nil
}

func writeInvestmentRecordsFile(path string, records []InvestmentRecord) error {
	data, err := json.MarshalIndent(InvestmentRecordsFile{
		Version: recordsFileVersion,
		Records: records,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("生成记录文件失败：%w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入记录文件失败：%w", err)
	}
	return nil
}

func sortInvestmentRecords(records []InvestmentRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].ArchivedAt > records[j].ArchivedAt
	})
}

func recalculateInvestmentRecord(record *InvestmentRecord) {
	if record == nil {
		return
	}
	if isSellRecord(*record) {
		var sold float64
		for i := range record.Assets {
			asset := &record.Assets[i]
			asset.Name = strings.TrimSpace(asset.Name)
			asset.SellAmount = roundMoney(asset.SellAmount)
			asset.BuyAmount = 0
			asset.BeforeAmount = 0
			asset.BeforePct = 0
			asset.AfterAmount = 0
			asset.AfterPct = 0
			asset.LowLine = 0
			asset.HighLine = 0
			asset.Status = ""
			sold += asset.SellAmount
		}
		record.SellAmount = roundMoney(sold)
		record.InvestAmount = 0
		record.CurrentTotal = 0
		record.AfterTotal = 0
		record.AllocatedCash = 0
		record.RemainingCash = 0
		return
	}

	var currentTotal float64
	var allocated float64
	for _, asset := range record.Assets {
		currentTotal += asset.BeforeAmount
		allocated += asset.BuyAmount
	}
	record.CurrentTotal = roundMoney(currentTotal)
	record.AllocatedCash = roundMoney(allocated)
	record.InvestAmount = record.AllocatedCash
	record.SellAmount = 0
	record.AfterTotal = roundMoney(record.CurrentTotal + record.AllocatedCash)
	record.RemainingCash = 0

	for i := range record.Assets {
		asset := &record.Assets[i]
		asset.BeforePct = 0
		if record.CurrentTotal > moneyEpsilon {
			asset.BeforePct = asset.BeforeAmount / record.CurrentTotal * 100
		}
		asset.AfterAmount = roundMoney(asset.BeforeAmount + asset.BuyAmount)
		asset.AfterPct = 0
		if record.AfterTotal > moneyEpsilon {
			asset.AfterPct = asset.AfterAmount / record.AfterTotal * 100
		}
		asset.LowLine = asset.TargetPct * lowAllocationRatio
		asset.HighLine = asset.TargetPct * highAllocationRatio
		asset.Status = allocationStatus(&AssetResult{
			TargetPct:      asset.TargetPct,
			AfterPct:       asset.AfterPct,
			LowLine:        asset.LowLine,
			HighLine:       asset.HighLine,
			IsSeverelyLow:  asset.AfterPct < asset.LowLine,
			IsSeverelyHigh: asset.AfterPct > asset.HighLine,
		})
	}
}

func relativeTargetDeviationPct(asset InvestmentAssetRecord) (float64, bool) {
	if asset.TargetPct <= moneyEpsilon {
		return 0, false
	}
	return (asset.AfterPct - asset.TargetPct) / asset.TargetPct * 100, true
}

func validateInvestmentRecord(record InvestmentRecord) error {
	if isSellRecord(record) {
		return validateSellRecord(record)
	}
	if len(record.Assets) < 2 {
		return fmt.Errorf("历史记录至少需要两项资产")
	}

	var targetSum float64
	seen := make(map[string]struct{}, len(record.Assets))
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
		targetSum += asset.TargetPct
	}
	if math.Abs(targetSum-100) > 0.01 {
		return fmt.Errorf("目标仓位合计必须为 100%%，当前为 %s", formatPercent(targetSum))
	}
	return nil
}

func validateSellRecord(record InvestmentRecord) error {
	if len(record.Assets) == 0 {
		return fmt.Errorf("没有填写卖出金额，不生成卖出记录")
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
		if asset.SellAmount < 0 {
			return fmt.Errorf("%s 的卖出金额不能为负数", name)
		}
		total += asset.SellAmount
	}
	if total <= moneyEpsilon {
		return fmt.Errorf("没有填写卖出金额，不生成卖出记录")
	}
	return nil
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

func isSellRecord(record InvestmentRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.RecordType), recordTypeSell)
}

func recordSellTotal(record InvestmentRecord) float64 {
	total := 0.0
	for _, asset := range record.Assets {
		total += asset.SellAmount
	}
	if total <= moneyEpsilon && record.SellAmount > moneyEpsilon {
		total = record.SellAmount
	}
	return roundMoney(total)
}
