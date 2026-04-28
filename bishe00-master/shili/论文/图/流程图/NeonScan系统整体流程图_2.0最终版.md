# NeonScan 系统整体流程图 2.0 最终版

本文件依据当前 `bishe00-master` 代码、前端页面与 2.0 图谱规范整理，重点修正旧版流程图中路由错误、功能分支缺失、AI/资产收集/MCP 表达不准确等问题。

## 证据来源

- `main.go`
  - 路由注册、`Task`/`SSEMessage`、`aiAutoScanHandler`
- `web/*.html`、`web/app.js`
  - 多页面入口、报告汇总、localStorage 持久化
- `internal/shouji/shouji.go`
  - `FFUF`、`URLFinder`、`Packer-Fuzzer`
- `ai_exp.go`、`ai_exp_batch.go`、`ai_exp_verify.go`
  - Python EXP 生成、批量生成、自动验证与修正
- `main_autoscan_exploit_verify.go`
  - AI 自动扫描结束后批量生成并验证 Python EXP

## PlantUML

```plantuml
@startuml NeonScan系统整体流程图_2_0最终版

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E8F5E9
skinparam activityBorderColor #4CAF50
skinparam activityBarColor #FF9800
skinparam activityStartColor #4CAF50
skinparam activityEndColor #F44336
skinparam activityDiamondBackgroundColor #FFF9C4
skinparam activityDiamondBorderColor #FBC02D

title NeonScan 2.0 系统整体运行流程图

start

:打开多页面前端并选择功能模块;

if (是否需要上传字典/POC/报告/样本?) then (是)
  :调用 /upload;
  :保存到 uploads/u-*;
else (否)
endif

:向对应 HTTP 接口提交请求;

if (请求属于哪类能力?) then (常规扫描 / 批量生成)
  partition "标准任务流" {
    :创建 Task;
    :启动 goroutine 并发执行;
    :执行端口/目录/POC/EXP/WebProbe/WAF/Shouji;
    :通过 /events 实时推送进度与结果;
  }

elseif (AI 自动扫描)
  partition "AI 自动扫描流" {
    :创建 Task;
    :初始化 AI Provider;
    :AI 编排调用端口扫描/Web探针/POC检测;
    :汇总结果并继续决策;
    :根据 POC 结果批量生成并验证 Python EXP;
    :通过 /events 实时推送进度与结果;
  }

else (AI 分析 / JADX MCP)
  partition "流式会话流" {
    :建立流式会话;
    :执行 AI 安全分析或 JADX MCP Chat;
    :实时返回 token / 工具调用结果;
  }
endif

:前端汇总结果到 localStorage;
note right
  neonscan-report
  - 扫描结果聚合
  - 页面刷新恢复
  - 导出 Markdown / Python EXP
end note

:查看报告 / 导出结果 / 下载 Python EXP;

stop

@enduml
```

## 修正点

- 路由改为真实路径，如 `/scan/ports`、`/scan/poc`、`/events`、`/upload`
- 明确补入 `EXP验证`、`WAF绕过`、`资产收集`、`扫描报告`
- 区分三类主流程：常规扫描、AI 自动扫描、流式 AI/MCP 会话
- 明确 AI 自动扫描后还会进入 `批量生成并验证 Python EXP`
- 不再把 MCP 统称为泛化工具，当前 2.0 主落地为 `JADX MCP`

