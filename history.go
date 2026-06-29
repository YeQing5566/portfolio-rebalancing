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

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const (
	recordsFileName           = "investment_records.json"
	recordsFileVersion        = 1
	archiveTimeFmt            = "2006-01-02 15:04:05"
	jsonFileFilter            = "JSON 文件 (*.json)|*.json|所有文件 (*.*)|*.*"
	saveArchiveSuccessMessage = "保存成功，可在历史投资记录中查看"
)

type InvestmentRecordsFile struct {
	Version int                `json:"version"`
	Records []InvestmentRecord `json:"records"`
}

type InvestmentRecord struct {
	ID            string                  `json:"id"`
	ArchivedAt    string                  `json:"archived_at"`
	Notes         string                  `json:"notes,omitempty"`
	InvestAmount  float64                 `json:"invest_amount"`
	CurrentTotal  float64                 `json:"current_total"`
	AfterTotal    float64                 `json:"after_total"`
	AllocatedCash float64                 `json:"allocated_cash"`
	RemainingCash float64                 `json:"remaining_cash"`
	Assets        []InvestmentAssetRecord `json:"assets"`
}

type InvestmentAssetRecord struct {
	Name         string  `json:"name"`
	TargetPct    float64 `json:"target_pct"`
	BeforeAmount float64 `json:"before_amount"`
	BeforePct    float64 `json:"before_pct"`
	BuyAmount    float64 `json:"buy_amount"`
	AfterAmount  float64 `json:"after_amount"`
	AfterPct     float64 `json:"after_pct"`
	LowLine      float64 `json:"low_line"`
	HighLine     float64 `json:"high_line"`
	Status       string  `json:"status"`
}

type HistoryListModel struct {
	walk.TableModelBase
}

func (m *HistoryListModel) RowCount() int {
	return len(investmentRecords)
}

func (m *HistoryListModel) Value(row, col int) interface{} {
	record := investmentRecords[row]
	switch col {
	case 0:
		return record.ArchivedAt
	case 1:
		return formatMoney(record.AfterTotal) + " 元"
	case 2:
		return formatMoney(record.InvestAmount) + " 元"
	default:
		return ""
	}
}

type HistoryAssetModel struct {
	walk.TableModelBase
}

func (m *HistoryAssetModel) RowCount() int {
	return len(selectedHistoryDraft.Assets)
}

func (m *HistoryAssetModel) Value(row, col int) interface{} {
	asset := selectedHistoryDraft.Assets[row]
	switch col {
	case 0:
		return asset.Name
	case 1:
		return formatPercent(asset.TargetPct)
	case 2:
		return formatMoney(asset.BeforeAmount)
	case 3:
		return formatPercent(asset.BeforePct)
	case 4:
		return formatMoney(asset.BuyAmount)
	case 5:
		return formatMoney(asset.AfterAmount)
	case 6:
		return formatPercent(asset.AfterPct)
	case 7:
		return asset.Status
	default:
		return ""
	}
}

var (
	investmentRecords    []InvestmentRecord
	historyListModel     = &HistoryListModel{}
	historyAssetModel    = &HistoryAssetModel{}
	historyTable         *walk.TableView
	historyAssetTable    *walk.TableView
	historyDetailPanel   *walk.Composite
	historyArchiveEdit   *walk.LineEdit
	historyNotesEdit     *walk.LineEdit
	historyInvestEdit    *walk.NumberEdit
	historySummaryLabel  *walk.Label
	historyFileLabel     *walk.Label
	historyAssetNameEdit *walk.LineEdit
	historyTargetEdit    *walk.NumberEdit
	historyBeforeEdit    *walk.NumberEdit
	historyBuyEdit       *walk.NumberEdit
	selectedHistoryIndex = -1
	selectedAssetIndex   = -1
	selectedHistoryDraft InvestmentRecord
	loadingHistoryEditor bool
)

func buildHistoryPage() Widget {
	return Composite{
		Background: windowBackground,
		Layout: HBox{
			MarginsZero: true,
			Spacing:     10,
		},
		Children: []Widget{
			Composite{
				MinSize:    Size{Width: 350},
				MaxSize:    Size{Width: 400, Height: 2000},
				Background: panelBackground,
				Border:     true,
				Layout: VBox{
					Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 12},
					Spacing: 8,
				},
				Children: []Widget{
					Label{
						Text:      "历史投资记录",
						TextColor: defaultTextColor,
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
					},
					Label{
						Text:      "选择一条记录，在右侧查看、调整或删除。",
						TextColor: mutedTextColor,
					},
					TableView{
						AssignTo:                    &historyTable,
						Model:                       historyListModel,
						Background:                  tableBackground,
						AlternatingRowBG:            true,
						SelectionHiddenWithoutFocus: false,
						NotSortableByHeaderClick:    true,
						LastColumnStretched:         true,
						CustomRowHeight:             30,
						StretchFactor:               1,
						Columns: []TableViewColumn{
							{Title: "归档时间", Width: 145},
							{Title: "投资总额", Width: 105, Alignment: AlignFar},
							{Title: "当次投入", Width: 100, Alignment: AlignFar},
						},
						StyleCell:             styleHistoryListCell,
						OnCurrentIndexChanged: showSelectedHistoryRecord,
						OnItemActivated:       showSelectedHistoryRecord,
					},
					Label{
						AssignTo:  &historyFileLabel,
						Text:      "记录文件：",
						TextColor: mutedTextColor,
						Font:      Font{Family: "Microsoft YaHei UI", PointSize: 8},
					},
					Composite{
						Background: panelBackground,
						Layout:     HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{
								Text:    "导出",
								MinSize: Size{Width: 96, Height: 30},
								OnClicked: func() {
									exportInvestmentRecords()
								},
							},
							PushButton{
								Text:    "导入",
								MinSize: Size{Width: 96, Height: 30},
								OnClicked: func() {
									importInvestmentRecords()
								},
							},
							HSpacer{},
						},
					},
				},
			},
			Composite{
				StretchFactor: 3,
				Background:    panelBackground,
				Border:        true,
				Layout: VBox{
					Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 12},
					Spacing: 9,
				},
				Children: []Widget{
					Composite{
						Background: panelBackground,
						Layout: HBox{
							MarginsZero: true,
							Spacing:     8,
						},
						Children: []Widget{
							Label{
								Text:      "投资记录详情",
								TextColor: defaultTextColor,
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
							},
							Label{
								Text:      "资产明细、备注和当次投入可直接维护",
								TextColor: mutedTextColor,
							},
							HSpacer{},
						},
					},
					Composite{
						AssignTo:   &historyDetailPanel,
						Enabled:    false,
						Background: panelBackground,
						Layout: VBox{
							MarginsZero: true,
							Spacing:     9,
						},
						Children: []Widget{
							Composite{
								Background: panelBackground,
								Layout: Grid{
									Columns:     4,
									MarginsZero: true,
									Spacing:     8,
								},
								Children: []Widget{
									Label{Text: "归档时间", TextColor: mutedTextColor},
									LineEdit{
										AssignTo:          &historyArchiveEdit,
										ToolTipText:       "格式：2026-06-18 15:30:00",
										MinSize:           Size{Width: 190},
										MaxSize:           Size{Width: 220, Height: 100},
										Background:        fieldBackground,
										TextColor:         defaultTextColor,
										OnEditingFinished: syncHistoryRecordFields,
									},
									Label{Text: "当次投入", TextColor: mutedTextColor},
									Composite{
										Background: panelBackground,
										Layout:     HBox{MarginsZero: true, Spacing: 6},
										Children: []Widget{
											NumberEdit{
												AssignTo:           &historyInvestEdit,
												Decimals:           2,
												Increment:          500,
												MinValue:           0,
												MaxValue:           1_000_000_000,
												SpinButtonsVisible: false,
												MinSize:            Size{Width: 130},
												MaxSize:            Size{Width: 160, Height: 100},
												Background:         fieldBackground,
												TextColor:          defaultTextColor,
												OnValueChanged:     syncHistoryRecordFields,
											},
											Label{Text: "元", TextColor: mutedTextColor},
										},
									},
									Label{Text: "备注", TextColor: mutedTextColor},
									LineEdit{
										AssignTo:      &historyNotesEdit,
										ColumnSpan:    3,
										CueBanner:     "可填写本次投资说明",
										Background:    fieldBackground,
										TextColor:     defaultTextColor,
										OnTextChanged: syncHistoryRecordFields,
									},
								},
							},
							Label{
								AssignTo:  &historySummaryLabel,
								Text:      "请选择左侧记录",
								TextColor: accentColor,
								Font:      Font{Family: "Microsoft YaHei UI", PointSize: 10, Bold: true},
							},
							TableView{
								AssignTo:                    &historyAssetTable,
								Model:                       historyAssetModel,
								Background:                  tableBackground,
								AlternatingRowBG:            true,
								SelectionHiddenWithoutFocus: false,
								NotSortableByHeaderClick:    true,
								LastColumnStretched:         true,
								CustomRowHeight:             30,
								StretchFactor:               1,
								Columns: []TableViewColumn{
									{Title: "资产", Width: 150},
									{Title: "目标", Width: 70},
									{Title: "买入前金额", Width: 115},
									{Title: "买入前仓位", Width: 90},
									{Title: "买入金额", Width: 105},
									{Title: "买入后金额", Width: 115},
									{Title: "买入后仓位", Width: 90},
									{Title: "状态", Width: 110},
								},
								StyleCell:             styleHistoryAssetCell,
								OnCurrentIndexChanged: loadSelectedHistoryAsset,
								OnItemActivated:       loadSelectedHistoryAsset,
							},
							Composite{
								Background: fieldBackground,
								Border:     true,
								Layout: HBox{
									Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 8},
									Spacing: 8,
								},
								Children: []Widget{
									Label{Text: "修改资产", TextColor: accentColor, Font: Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true}},
									LineEdit{
										AssignTo:      &historyAssetNameEdit,
										CueBanner:     "资产名称",
										MinSize:       Size{Width: 150},
										StretchFactor: 2,
										Background:    resultBackground,
										TextColor:     defaultTextColor,
										OnTextChanged: syncHistoryAssetEditor,
									},
									NumberEdit{
										AssignTo:           &historyTargetEdit,
										Decimals:           2,
										MinValue:           0,
										MaxValue:           100,
										SpinButtonsVisible: false,
										Suffix:             " % 目标",
										MinSize:            Size{Width: 125},
										Background:         resultBackground,
										TextColor:          defaultTextColor,
										OnValueChanged:     syncHistoryAssetEditor,
									},
									NumberEdit{
										AssignTo:           &historyBeforeEdit,
										Decimals:           2,
										MinValue:           0,
										MaxValue:           1_000_000_000,
										SpinButtonsVisible: false,
										Suffix:             " 元 买入前",
										MinSize:            Size{Width: 165},
										Background:         resultBackground,
										TextColor:          defaultTextColor,
										OnValueChanged:     syncHistoryAssetEditor,
									},
									NumberEdit{
										AssignTo:           &historyBuyEdit,
										Decimals:           2,
										MinValue:           0,
										MaxValue:           1_000_000_000,
										SpinButtonsVisible: false,
										Suffix:             " 元 买入",
										MinSize:            Size{Width: 150},
										Background:         resultBackground,
										TextColor:          defaultTextColor,
										OnValueChanged:     syncHistoryAssetEditor,
									},
								},
							},
							Composite{
								Background: panelBackground,
								Layout:     HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									PushButton{
										Text:    "读取记录",
										MinSize: Size{Width: 112, Height: 32},
										OnClicked: func() {
											loadSelectedHistoryToCalculator()
										},
									},
									PushButton{
										Text:    "删除该记录",
										MinSize: Size{Width: 112, Height: 32},
										OnClicked: func() {
											deleteSelectedHistoryRecord()
										},
									},
									HSpacer{},
									Label{
										Text:      "修改后会自动保存，金额、仓位与提醒会自动重算",
										TextColor: mutedTextColor,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func styleHistoryListCell(style *walk.CellStyle) {
	styleDarkTableCell(style, currentTableIndex(historyTable))
	switch style.Col() {
	case 1:
		style.TextColor = accentColor
	case 2:
		style.TextColor = secondaryColor
	}
}

func styleHistoryAssetCell(style *walk.CellStyle) {
	styleDarkTableCell(style, currentTableIndex(historyAssetTable))
	row := style.Row()
	if row < 0 || row >= len(selectedHistoryDraft.Assets) {
		return
	}
	asset := selectedHistoryDraft.Assets[row]
	switch style.Col() {
	case 1, 6:
		style.TextColor = accentColor
	case 4:
		if asset.BuyAmount > 0 {
			style.TextColor = accentColor
		} else {
			style.TextColor = mutedTextColor
		}
	case 7:
		style.TextColor = statusTextColor(asset.Status)
	}
}

func archiveCurrentInvestment() error {
	result, err := CalculatePortfolio(investAmountEdit.Value(), assetModel.ItemsCopy())
	if err != nil {
		return err
	}

	record := recordFromResult(result)
	investmentRecords = append([]InvestmentRecord{record}, investmentRecords...)
	if err := saveInvestmentRecords(); err != nil {
		investmentRecords = investmentRecords[1:]
		return err
	}

	resultEdit.SetText(FormatResult(result))
	historyListModel.PublishRowsReset()
	refreshTrendView()
	return nil
}

func recordFromResult(result *PortfolioResult) InvestmentRecord {
	now := time.Now()
	record := InvestmentRecord{
		ID:            fmt.Sprintf("%d", now.UnixNano()),
		ArchivedAt:    now.Format(archiveTimeFmt),
		InvestAmount:  result.InvestAmount,
		CurrentTotal:  result.CurrentTotal,
		AfterTotal:    result.AfterTotal,
		AllocatedCash: result.AllocatedCash,
		RemainingCash: result.RemainingCash,
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
	return record
}

func recordsFilePath() (string, error) {
	return appDataFilePath(recordsFileName)
}

func loadInvestmentRecords() error {
	path, err := recordsFilePath()
	if err != nil {
		return err
	}
	if historyFileLabel != nil {
		historyFileLabel.SetText("记录文件：" + path)
	}

	records, err := readInvestmentRecordsFile(path)
	if os.IsNotExist(err) {
		investmentRecords = nil
		historyListModel.PublishRowsReset()
		refreshTrendView()
		return nil
	}
	if err != nil {
		return err
	}

	investmentRecords = records
	historyListModel.PublishRowsReset()
	refreshTrendView()
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

func exportInvestmentRecords() {
	basePath, err := recordsFilePath()
	if err != nil {
		walk.MsgBox(mainWindow, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}

	fileName := "investment_records_export_" + time.Now().Format("20060102_150405") + ".json"
	dlg := walk.FileDialog{
		Title:          "导出历史投资记录",
		FilePath:       fileName,
		Filter:         jsonFileFilter,
		FilterIndex:    1,
		InitialDirPath: filepath.Dir(basePath),
	}
	accepted, err := dlg.ShowSave(mainWindow)
	if err != nil {
		walk.MsgBox(mainWindow, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	if !accepted {
		return
	}

	targetPath := strings.TrimSpace(dlg.FilePath)
	if strings.EqualFold(filepath.Ext(targetPath), "") {
		targetPath += ".json"
	}
	if _, err := os.Stat(targetPath); err == nil {
		if walk.MsgBox(
			mainWindow,
			"覆盖导出文件",
			"导出文件已存在，是否覆盖？\r\n"+targetPath,
			walk.MsgBoxYesNo|walk.MsgBoxIconQuestion,
		) != walk.DlgCmdYes {
			return
		}
	} else if !os.IsNotExist(err) {
		walk.MsgBox(mainWindow, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}

	if err := writeInvestmentRecordsFile(targetPath, investmentRecords); err != nil {
		walk.MsgBox(mainWindow, "导出失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	statusBarItem.SetText("历史投资记录已导出：" + targetPath)
	walk.MsgBox(mainWindow, "导出完成", "历史投资记录已导出到：\r\n"+targetPath, walk.MsgBoxOK|walk.MsgBoxIconInformation)
}

func importInvestmentRecords() {
	basePath, err := recordsFilePath()
	if err != nil {
		walk.MsgBox(mainWindow, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}

	dlg := walk.FileDialog{
		Title:          "选择要导入的历史记录 JSON 文件",
		Filter:         jsonFileFilter,
		FilterIndex:    1,
		InitialDirPath: filepath.Dir(basePath),
	}
	accepted, err := dlg.ShowOpen(mainWindow)
	if err != nil {
		walk.MsgBox(mainWindow, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	if !accepted {
		return
	}

	records, err := readInvestmentRecordsFile(dlg.FilePath)
	if err != nil {
		walk.MsgBox(mainWindow, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}

	message := fmt.Sprintf(
		"将从以下文件导入 %d 条历史记录，并覆盖当前记录文件：\r\n%s\r\n\r\n当前记录文件：\r\n%s\r\n\r\n是否继续？",
		len(records),
		dlg.FilePath,
		basePath,
	)
	if walk.MsgBox(mainWindow, "导入历史记录", message, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}

	if err := writeInvestmentRecordsFile(basePath, records); err != nil {
		walk.MsgBox(mainWindow, "导入失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	investmentRecords = records
	resetHistorySelection()
	historyListModel.PublishRowsReset()
	refreshTrendView()
	if len(investmentRecords) > 0 && historyTable != nil {
		_ = historyTable.SetCurrentIndex(0)
		showSelectedHistoryRecord()
	}
	statusBarItem.SetText("历史投资记录已导入：" + dlg.FilePath)
	walk.MsgBox(mainWindow, "导入完成", "历史投资记录已导入到程序目录。", walk.MsgBoxOK|walk.MsgBoxIconInformation)
}

func resetHistorySelection() {
	selectedHistoryIndex = -1
	selectedAssetIndex = -1
	selectedHistoryDraft = InvestmentRecord{}
	if historyAssetModel != nil {
		historyAssetModel.PublishRowsReset()
	}
	if historyDetailPanel != nil {
		historyDetailPanel.SetEnabled(false)
	}
}

func refreshHistoryView() {
	if historyFileLabel == nil {
		return
	}
	path, err := recordsFilePath()
	if err == nil {
		historyFileLabel.SetText("记录文件：" + path)
	}
	historyListModel.PublishRowsReset()
	if len(investmentRecords) > 0 && historyTable.CurrentIndex() < 0 {
		_ = historyTable.SetCurrentIndex(0)
		showSelectedHistoryRecord()
	}
}

func showSelectedHistoryRecord() {
	index := historyTable.CurrentIndex()
	if index < 0 || index >= len(investmentRecords) {
		selectedHistoryIndex = -1
		historyDetailPanel.SetEnabled(false)
		return
	}

	selectedHistoryIndex = index
	selectedHistoryDraft = cloneInvestmentRecord(investmentRecords[index])
	loadingHistoryEditor = true
	historyArchiveEdit.SetText(selectedHistoryDraft.ArchivedAt)
	historyNotesEdit.SetText(selectedHistoryDraft.Notes)
	_ = historyInvestEdit.SetValue(selectedHistoryDraft.InvestAmount)
	historyDetailPanel.SetEnabled(true)
	historyAssetModel.PublishRowsReset()
	loadingHistoryEditor = false
	refreshHistorySummary()

	if len(selectedHistoryDraft.Assets) > 0 {
		_ = historyAssetTable.SetCurrentIndex(0)
		loadSelectedHistoryAsset()
	}
}

func loadSelectedHistoryAsset() {
	index := historyAssetTable.CurrentIndex()
	if index < 0 || index >= len(selectedHistoryDraft.Assets) {
		selectedAssetIndex = -1
		return
	}
	selectedAssetIndex = index
	asset := selectedHistoryDraft.Assets[index]
	loadingHistoryEditor = true
	historyAssetNameEdit.SetText(asset.Name)
	_ = historyTargetEdit.SetValue(asset.TargetPct)
	_ = historyBeforeEdit.SetValue(asset.BeforeAmount)
	_ = historyBuyEdit.SetValue(asset.BuyAmount)
	loadingHistoryEditor = false
}

func syncHistoryAssetEditor() {
	if loadingHistoryEditor || selectedAssetIndex < 0 || selectedAssetIndex >= len(selectedHistoryDraft.Assets) {
		return
	}
	asset := &selectedHistoryDraft.Assets[selectedAssetIndex]
	asset.Name = strings.TrimSpace(historyAssetNameEdit.Text())
	asset.TargetPct = historyTargetEdit.Value()
	asset.BeforeAmount = historyBeforeEdit.Value()
	asset.BuyAmount = historyBuyEdit.Value()
	recalculateInvestmentRecord(&selectedHistoryDraft)
	historyAssetModel.PublishRowsReset()
	_ = historyAssetTable.SetCurrentIndex(selectedAssetIndex)
	refreshHistorySummary()
	autoSaveHistoryChanges()
}

func syncHistoryRecordFields() {
	if loadingHistoryEditor || selectedHistoryIndex < 0 {
		return
	}
	selectedHistoryDraft.ArchivedAt = strings.TrimSpace(historyArchiveEdit.Text())
	selectedHistoryDraft.InvestAmount = historyInvestEdit.Value()
	selectedHistoryDraft.Notes = strings.TrimSpace(historyNotesEdit.Text())
	recalculateInvestmentRecord(&selectedHistoryDraft)
	historyAssetModel.PublishRowsReset()
	refreshHistorySummary()
	autoSaveHistoryChanges()
}

func refreshHistorySummary() {
	recalculateInvestmentRecord(&selectedHistoryDraft)
	historySummaryLabel.SetText(fmt.Sprintf(
		"买入前总额 %s 元｜当次投入 %s 元｜买入总额 %s 元｜未分配 %s 元｜买入后总额 %s 元",
		formatMoney(selectedHistoryDraft.CurrentTotal),
		formatMoney(selectedHistoryDraft.InvestAmount),
		formatMoney(selectedHistoryDraft.AllocatedCash),
		formatMoney(selectedHistoryDraft.RemainingCash),
		formatMoney(selectedHistoryDraft.AfterTotal),
	))
}

func recalculateInvestmentRecord(record *InvestmentRecord) {
	var currentTotal float64
	var allocated float64
	for _, asset := range record.Assets {
		currentTotal += asset.BeforeAmount
		allocated += asset.BuyAmount
	}
	record.CurrentTotal = roundMoney(currentTotal)
	record.AllocatedCash = roundMoney(allocated)
	record.AfterTotal = roundMoney(record.CurrentTotal + record.InvestAmount)
	record.RemainingCash = roundMoney(math.Max(0, record.InvestAmount-record.AllocatedCash))

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

func saveHistoryChanges() error {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		return fmt.Errorf("请先选择历史记录")
	}

	archivedAt := strings.TrimSpace(historyArchiveEdit.Text())
	parsed, err := time.ParseInLocation(archiveTimeFmt, archivedAt, time.Local)
	if err != nil {
		return fmt.Errorf("归档时间格式应为：2026-06-18 15:30:00")
	}
	selectedHistoryDraft.ArchivedAt = parsed.Format(archiveTimeFmt)
	selectedHistoryDraft.Notes = strings.TrimSpace(historyNotesEdit.Text())
	selectedHistoryDraft.InvestAmount = historyInvestEdit.Value()
	recalculateInvestmentRecord(&selectedHistoryDraft)

	if err := validateInvestmentRecord(selectedHistoryDraft); err != nil {
		return err
	}

	recordID := investmentRecords[selectedHistoryIndex].ID
	selectedHistoryDraft.ID = recordID
	investmentRecords[selectedHistoryIndex] = cloneInvestmentRecord(selectedHistoryDraft)
	sort.SliceStable(investmentRecords, func(i, j int) bool {
		return investmentRecords[i].ArchivedAt > investmentRecords[j].ArchivedAt
	})
	if err := saveInvestmentRecords(); err != nil {
		return err
	}

	historyListModel.PublishRowsReset()
	refreshTrendView()
	for i := range investmentRecords {
		if investmentRecords[i].ID == recordID {
			selectedHistoryIndex = i
			_ = historyTable.SetCurrentIndex(i)
			break
		}
	}
	return nil
}

func autoSaveHistoryChanges() {
	if err := saveHistoryChanges(); err != nil {
		statusBarItem.SetText("自动保存失败：" + err.Error())
		return
	}
	statusBarItem.SetText("历史投资记录已自动保存")
}

func validateInvestmentRecord(record InvestmentRecord) error {
	if len(record.Assets) < 2 {
		return fmt.Errorf("历史记录至少需要两项资产")
	}
	if record.AllocatedCash > record.InvestAmount+0.01 {
		return fmt.Errorf("资产买入金额合计不能超过当次投入金额")
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

func deleteSelectedHistoryRecord() {
	if selectedHistoryIndex < 0 || selectedHistoryIndex >= len(investmentRecords) {
		return
	}
	if walk.MsgBox(
		mainWindow,
		"删除历史记录",
		"确定删除 "+investmentRecords[selectedHistoryIndex].ArchivedAt+" 的投资记录吗？",
		walk.MsgBoxYesNo|walk.MsgBoxIconQuestion,
	) != walk.DlgCmdYes {
		return
	}

	investmentRecords = append(
		investmentRecords[:selectedHistoryIndex],
		investmentRecords[selectedHistoryIndex+1:]...,
	)
	if err := saveInvestmentRecords(); err != nil {
		walk.MsgBox(mainWindow, "删除失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	selectedHistoryIndex = -1
	selectedAssetIndex = -1
	historyListModel.PublishRowsReset()
	historyAssetModel.PublishRowsReset()
	historyDetailPanel.SetEnabled(false)
	refreshTrendView()
	statusBarItem.SetText("历史投资记录已删除")
}

func cloneInvestmentRecord(record InvestmentRecord) InvestmentRecord {
	clone := record
	clone.Assets = append([]InvestmentAssetRecord(nil), record.Assets...)
	return clone
}
