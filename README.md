## portfolio-rebalancing

一个由 codex 用 Go 编写的投资组合再平衡项目。

## 功能

- 投资组合根据资产金额自动计算当前仓位
- 根据当前需投入金额和目标仓位，计算各资产再平衡投入金额
- Windows 排版

## 构建

```powershell
go build -o .\rebalance-test.exe .
