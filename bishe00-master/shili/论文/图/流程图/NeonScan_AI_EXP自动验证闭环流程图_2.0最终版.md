# NeonScan AI-EXP 自动验证闭环流程图 2.0 最终版

本文件单独刻画当前 2.0 代码里最有代表性的闭环能力：`POC结果 -> ExpSpec -> AI生成Python EXP -> 自动验证 -> 失败分析 -> AI/规则修正 -> 重试`。

## 证据来源

- `ai_exp.go`
- `ai_exp_batch.go`
- `ai_exp_verify.go`
- `main_autoscan_exploit_verify.go`

## PlantUML

```plantuml
@startuml NeonScan_AI_EXP_自动验证闭环_2_0最终版

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E3F2FD
skinparam activityBorderColor #1E88E5
skinparam activityStartColor #43A047
skinparam activityEndColor #E53935
skinparam activityDiamondBackgroundColor #FFF9C4
skinparam activityDiamondBorderColor #FBC02D

title NeonScan 2.0 AI Python EXP 自动验证闭环流程图

start

:接收 ExpSpec / POC结果 / 目标URL;
:DetectVulnCategory;
:构造分类提示词;
:调用 AI Provider 生成初始 Python EXP;

if (是否启用 autoVerify?) then (否)
  :直接返回 Python EXP;
  stop
else (是)
endif

:构建 ExpVerifyConfig;
:设置重试次数与超时;
:保存到 %TEMP%/neonscan_exp;

repeat
  :保存 exp_attempt_n.py;
  :调用 python --target --cmd;
  :检查 VULNERABLE 标记;
  :尝试提取命令输出;

  if (验证成功?) then (是)
    :记录 verifyLogs / verifyAttempts;
    :返回已验证脚本;
    stop
  else (否)
    :analyzeFailure;

    if (达到最大重试次数?) then (是)
      :返回失败结果;
      :保留最后脚本与日志;
      stop
    else (否)
    endif

    if (AI返回修正版本?) then (是)
      :requestExpCorrection;
      :替换 currentCode;
    else (否)
      :tryAlternativeFix;
      :语法修复 / 输出提取增强 / 标记补齐;
    endif
  endif

repeat while (继续下一次重试?)

@enduml
```

## 修正点

- 明确 `autoVerify=true` 才进入自动验证闭环
- 明确输出并不是一次完成，而是 `生成 -> 执行 -> 分析 -> 修正 -> 重试`
- 明确两种修正来源：
  - `AI 返回修正版本`
  - `规则级备用修复`
- 明确这条闭环既可来自单个 `/ai/exp/python`，也可来自 `AI 自动扫描` 后的批量生成验证

