## portfolio-rebalancing

一个由 Codex 用 Go 编写的投资组合再平衡项目，轻量本地化，基于 Windows 排版。

## 功能

- 自定义投资组合，及各资产仓位
- 根据输入的资产当前金额自动计算当前仓位
- 根据当前需投入金额和目标仓位，计算各资产再平衡投入金额
- 支持保存当前的仓位信息和计算出的再平衡策略到本地，并展示在历史投资记录界面
- 支持修改、导入导出历史投资记录
- 支持查看历史各资产趋势

## 界面

<img width="1357" height="855" alt="image" src="https://github.com/user-attachments/assets/c6fb5226-8e19-4876-9c0a-580d580fd369" />
<img width="1359" height="859" alt="image" src="https://github.com/user-attachments/assets/3ba1cf1b-f848-4624-998f-559fb0a2da3d" />
<img width="1359" height="858" alt="image" src="https://github.com/user-attachments/assets/01930ad0-74b8-45f9-9002-d377de049de4" />

## 构建

```powershell
go build -o .\portfolio-rebalancing.exe .
