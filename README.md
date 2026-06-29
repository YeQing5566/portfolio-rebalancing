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

<img width="1359" height="858" alt="image" src="https://github.com/user-attachments/assets/27c1520f-bd08-4eca-9c5c-01e96b38666b" />
<img width="1359" height="858" alt="image" src="https://github.com/user-attachments/assets/9d041906-ec8d-4030-a101-0f9b5d6ed0ab" />
<img width="1359" height="858" alt="image" src="https://github.com/user-attachments/assets/6aa8b255-5f58-494e-801b-33ae8810cb9b" />
<img width="1359" height="858" alt="image" src="https://github.com/user-attachments/assets/f6d3251c-ce6f-4fdc-8553-f4fd11383ec1" />

## 构建

```powershell
go build -o .\portfolio-rebalancing.exe .
