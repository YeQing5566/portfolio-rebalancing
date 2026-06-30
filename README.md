## 投资组合再平衡工具

一个由 Codex 用 Go 编写的投资组合再平衡项目，轻量本地化，基于 Windows 排版。

## 功能

- 自定义投资组合，及各资产目标仓位
- 输入持仓金额、预估投入金额、目标仓位，自动计算各资产再平衡投入金额
- 支持保存此次的买入前后数据作为历史投资记录到本地
- 支持修改、导入导出历史投资记录
- 支持查看历史各资产趋势
- 支持通过历史买入数据测算收益率、年化收益率（考虑定投的多次买入情况）

## 策略

- 优先通过新增资金买入补足低配资产，若买入后仍出现严重低配（低于0.75倍目标仓位）或严重高配（高于1.25倍目标仓位），将高亮提醒，用户可考虑卖出，或者多次通过增量资金再平衡

## 界面

<img width="1357" height="858" alt="image" src="https://github.com/user-attachments/assets/d63635a5-fa83-4c89-b9ac-ec877b1d551e" />
<img width="1356" height="850" alt="image" src="https://github.com/user-attachments/assets/ace31576-2065-4111-aeb8-bfaa4f71c951" />
<img width="1355" height="854" alt="image" src="https://github.com/user-attachments/assets/92ed6d45-5b17-4fa9-b3d6-8cffa800ca68" />
<img width="1354" height="852" alt="image" src="https://github.com/user-attachments/assets/4b76573b-e3a4-4d64-a19a-4f7ecac13ad2" />

## 构建

```powershell
go build -o .\portfolio-rebalancing.exe .
