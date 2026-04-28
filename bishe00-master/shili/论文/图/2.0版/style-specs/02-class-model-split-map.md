# 类图拆分连接说明

当前类图建议按 `4 个主类图 + 2 个补充类图` 使用。

## 文件列表

### 主类图

1. `02-class-model-main-task-requests.puml`
   - 任务管理
   - 扫描请求

2. `02-class-model-main-poc.puml`
   - 传统 POC
   - X-Ray POC
   - Nuclei POC

3. `02-class-model-main-exp.puml`
   - EXP 执行请求
   - AI 生成 Python EXP 请求
   - EXP 规范、步骤、验证、提取、验证结果

4. `02-class-model-main-mcp.puml`
   - MCPConnection
   - ChatSession
   - BaseChatSession
   - JADXMCPConnection
   - JADXChatSession
   - MCPTool

### 补充类图

5. `02-class-model-supplement-ai.puml`
   - ChatMessage
   - AIProvider / ChatStreamProvider
   - AITool / AIToolCall
   - OpenAI / DeepSeek / Anthropic / Ollama
   - StreamController

6. `02-class-model-supplement-waf-other.puml`
   - WAFBypassReq
   - PayloadVariant
   - FingerprintRule / FingerprintData
   - DictInfo

## 跨文件连接点

### A. 主类图-任务与扫描请求 -> 主类图-POC规则

- `PocScanReq` 是 POC 规则模块的输入请求结构。
- 连接方式：
  - 在论文说明里写“PocScanReq 驱动 POC/XRPOC/NucleiPOC 的加载与扫描”
  - 如果需要图上体现，可以在主类图-POC规则中增加一个引用类 `PocScanReq(见任务与扫描请求)`

### B. 主类图-任务与扫描请求 -> 主类图-EXP与AI-EXP

- `ExpExecReq` 直接依赖 `ExpSpec`
- `AIGenPythonFromExpReq` 直接依赖 `ExpSpec`
- `AIGenPythonFromExpBatchReq` 直接依赖 `ExpSpec`
- 这是请求层进入 EXP 语义层的主要桥接

### C. 主类图-MCP与JADX -> 补充类图-AI提供者与流式控制

- `BaseChatSession` 使用 `AIProvider`
- `BaseChatSession` 组合 `ChatMessage`
- `MCPConnection` 与 `AITool` 相关
- 这三个是最核心的跨文件连接点

### D. 主类图-任务与扫描请求 -> 补充类图-WAF与辅助数据

- `DirScanReq` 使用 `DictInfo`
- `WebProbeReq` 使用 `FingerprintData`
- `WAFBypassReq` 与 `PayloadVariant` 同属 WAF 模块

### E. 主类图-EXP与AI-EXP -> 补充类图-AI提供者与流式控制

- `AIGenPythonFromExpReq.Provider`
- `AIGenPythonFromExpBatchReq.Provider`
- `GenerateAndVerifyExp(config, provider, ...)`

虽然 `provider` 在详细 JSON 里作为运行时依赖来自 `mcp.AIProvider`，但为了避免主图过宽，具体 AIProvider 族被放在补充类图-AI 中展示。

## 推荐查看顺序

1. `02-class-model-main-task-requests.puml`
2. `02-class-model-main-poc.puml`
3. `02-class-model-main-exp.puml`
4. `02-class-model-main-mcp.puml`
5. `02-class-model-supplement-ai.puml`
6. `02-class-model-supplement-waf-other.puml`

## 如果写进论文

推荐写法：

- 图 X-1 主类图：任务管理与扫描请求
- 图 X-2 主类图：POC 规则模型
- 图 X-3 主类图：EXP 与 AI-EXP 模型
- 图 X-4 主类图：MCP 与 JADX 会话模型
- 图 X-5 补充类图：AI Provider 与流式控制
- 图 X-6 补充类图：WAF 与辅助数据模型

