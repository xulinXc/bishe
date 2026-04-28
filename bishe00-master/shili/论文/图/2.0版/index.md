# NeonScan 2.0 图集说明

本图集依据 `bishe00-master` 当前 2.0 代码整理，重点修正了 1.0 图中与实际实现不一致的部分，尤其是 `AI 自动扫描`、`AI 生成 Python EXP`、`自动验证与迭代修正`、`批量生成 EXP`、`JADX MCP 会话` 相关能力。

## 证据来源

- `main.go`
  - 路由注册、`Task`/`SSEMessage`、`aiAutoScanHandler`
- `ai_exp.go`
  - 单个 Python EXP 生成、`autoVerify`
- `ai_exp_batch.go`
  - 批量 Python EXP 生成
- `ai_exp_verify.go`
  - `DetectVulnCategory`、`GenerateAndVerifyExp`、失败修正闭环
- `main_autoscan_exploit_verify.go`
  - 自动扫描结束后批量生成并验证 Python EXP
- `exp_result_spec.go`
  - 从 POC 结果归一化出 `ExpSpec`
- `internal/mcp/*.go`
  - `AIProvider`、`BaseChatSession`、`JADXMCPConnection`
- `internal/shouji/shouji.go`
  - `FFUF`、`URLFinder`、`Packer-Fuzzer` 接入
- `upload.go`
  - 上传批次与 `uploads/u-*` 文件落盘逻辑
- `web/*.html`、`web/app.js`
  - 前端多页面、`localStorage` 报告汇总和导出逻辑

## 图清单

| 文件 | 图名 | 章节建议 | 说明 |
|---|---|---|---|
| `specs/00-architecture.json` | NeonScan 2.0 功能模块框图 | 总体设计 | 展示前端、Go 核心服务、扫描模块、AI、Shouji、JADX MCP 与资源层关系 |
| `specs/01-use-case.json` | NeonScan 2.0 系统用例图 | 系统分析 | 用用户目标而不是内部函数名描述 2.0 能力 |
| `specs/02-class.json` | NeonScan 2.0 AI-EXP 与会话核心类图 | 详细设计 | 聚焦 2.0 改动最大的 AI EXP 闭环和会话模型 |
| `specs/03-system-flow.json` | NeonScan 2.0 总体运行流程图 | 系统设计 | 描述前端、HTTP、Task、SSE、报告汇总的整体运行链路 |
| `specs/04-ai-exp-flow.json` | AI Python EXP 自动验证闭环流程图 | 详细设计 | 单独刻画生成、验证、失败分析、AI修正、规则修复、重试 |
| `specs/05-logical-er.json` | NeonScan 2.0 逻辑数据模型图 | 数据设计 | 说明这是运行时与文件资源逻辑模型，不是数据库表结构 |

## 建模说明

- `00-architecture`、`01-use-case`、`03-system-flow`、`04-ai-exp-flow` 为已确认模型，直接来自现有路由和执行逻辑。
- `02-class` 只保留稳定且能支撑论文描述的核心类型，没有把所有辅助函数和临时结构塞进一张图。
- `05-logical-er` 明确使用“逻辑数据模型”口径，因为项目当前没有数据库持久化层，旧版 ER 图中大量“表”并不存在。

## 与 1.0 图的主要差异

- 不再把 `IDA Pro` 画成当前主流程中的已落地模块；当前代码主路由实际落地的是 `JADX MCP`。
- 不再把 AI 仅画成“报告分析”；2.0 已包含 `AI 自动扫描编排`、`AI 生成 Python EXP`、`自动验证与修正`、`批量生成`。
- 不再把 ER 图画成传统数据库表设计；当前系统主要是 `Go 单体 + Task/SSE + uploads + localStorage + MCP 会话`。
- 资产收集模块明确落到 `FFUF`、`URLFinder`、`Packer-Fuzzer` 三个外部工具，而不是笼统写“信息收集”。

