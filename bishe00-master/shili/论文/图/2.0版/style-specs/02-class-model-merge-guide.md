# 02 类图融合方案

## 结论

现在的类图已经被拆成：

- 4 张主类图
- 2 张补充类图

如果被要求“最终必须融合成一张图”，正确做法不是把 6 个 `puml` 文件简单拼接，而是：

1. 以 `02-class-model-detailed.json` 作为唯一真值源
2. 重新做一张“总类图”的布局
3. 按包区和跨包关系重新排布
4. 保留所有关系，但允许把次要字段和部分方法做摘要化显示

也就是说：

- **结构要全量合并**
- **布局要重新设计**
- **显示密度必须压缩**

## 真值源

融合时以这个文件为准：

- `02-class-model-detailed.json`

原因：

- 这是最完整的结构化类模型
- 每个类的属性、方法、可见性、关系类型、方向、多重度和证据都在这里
- 拆分的 `puml` 只是为了易读，不是最终结构真值

## 6 张图如何映射到 1 张总类图

### A. 顶部横带：扫描请求

来源：

- `02-class-model-main-task-requests.puml`

放置：

- 页面最上方横向展开

包含：

- `PortScanReq`
- `DirScanReq`
- `WebProbeReq`
- `PocScanReq`
- `ExpExecReq`
- `AIAnalyzeReq`
- `AIGenPythonFromExpReq`
- `AIGenPythonFromExpBatchReq`

作用：

- 作为所有业务模型的入口层

### B. 左中：任务管理

来源：

- `02-class-model-main-task-requests.puml`

放置：

- 页面左中

包含：

- `Task`
- `SSEMessage`

作用：

- 形成全图左侧的稳定锚点
- 与请求层、POC 层保持连接

### C. 中左：POC 规则族

来源：

- `02-class-model-main-poc.puml`

放置：

- 页面中左偏中部

包含：

- `POC`
- `XRPOC`
- `XRInfo`
- `XRRule`
- `XRRequest`
- `NucleiPOC`
- `NucleiInfo`
- `NucleiRequest`
- `NucleiMatcher`
- `NucleiHeaders`

作用：

- 这是内容最多、内部关系最密的一组
- 必须放在总图中部，不能放边角

### D. 中右：EXP 与 AI-EXP

来源：

- `02-class-model-main-exp.puml`

放置：

- 页面中右

包含：

- `ExpVerifyInfo`
- `ExpSpec`
- `ExpStep`
- `Validation`
- `StatusList`
- `ExtractRule`
- `ExpVerifyConfig`
- `ExpVerifyResult`

作用：

- 作为 2.0 改动最大的核心区
- 要与请求层和 AI 层都保持连接

### E. 右上：MCP / JADX

来源：

- `02-class-model-main-mcp.puml`

放置：

- 页面右上

包含：

- `MCPConnection`
- `ChatSession`
- `BaseChatSession`
- `JADXMCPConnection`
- `JADXChatSession`
- `MCPTool`

作用：

- 与 AI 层关系密切
- 但内部结构相对独立，适合单独成区

### F. 右下：AI 提供者

来源：

- `02-class-model-supplement-ai-core.puml`
- `02-class-model-supplement-ai-providers.puml`

放置：

- 页面右下

包含：

- `ChatMessage`
- `AIProvider`
- `ChatStreamProvider`
- `AITool`
- `AIToolCall`
- `StreamController`
- `OpenAIProvider`
- `DeepSeekProvider`
- `AnthropicProvider`
- `OllamaProvider`

作用：

- 作为 `BaseChatSession` 和 `AI生成EXP请求` 的外延能力区

### G. 下方边带：WAF 与辅助数据

来源：

- `02-class-model-supplement-waf-other.puml`

放置：

- 页面底部偏左或底部横带

包含：

- `WAFBypassReq`
- `PayloadVariant`
- `FingerprintRule`
- `FingerprintData`
- `DictInfo`

作用：

- 不是主逻辑中心
- 放底部可以减少其与 MCP/AI/POC 主走廊的干扰

## 跨区连接怎么恢复

融合时必须恢复这些跨图连接：

### 1. 请求层 -> 任务层

- 各扫描请求结构与 `Task`
- `Task` -> `SSEMessage`

这条线束建议走页面左侧主走廊。

### 2. 请求层 -> POC层

- `PocScanReq` -> `POC/XRPOC/NucleiPOC`

这条线束建议从顶部下沉到中左。

### 3. 请求层 -> EXP层

- `ExpExecReq` -> `ExpSpec`
- `AIGenPythonFromExpReq` -> `ExpSpec`
- `AIGenPythonFromExpReq` -> `ExpVerifyInfo`
- `AIGenPythonFromExpBatchReq` -> `ExpSpec`

这几条必须集中汇入 `ExpSpec`，不要分别四处穿行。

### 4. EXP层内部

- `ExpSpec` *-- `ExpStep`
- `ExpStep` *-- `Validation`
- `ExpStep` *-- `ExtractRule`
- `Validation` *-- `StatusList`
- `ExpVerifyConfig` -> `ExpSpec`
- `ExpVerifyConfig` ..> `ExpVerifyResult`

这一组必须保持局部紧凑。

### 5. MCP层 -> AI层

- `BaseChatSession` ..> `AIProvider`
- `BaseChatSession` *-- `ChatMessage`
- `MCPConnection` ..> `AITool`

这条线束建议走页面右侧主走廊。

### 6. AI层内部

- `ChatMessage` *-- `AIToolCall`
- `OpenAIProvider` <|.. `AIProvider`
- `DeepSeekProvider` <|.. `AIProvider`
- `AnthropicProvider` <|.. `AIProvider`
- `OllamaProvider` <|.. `AIProvider`
- `OpenAIProvider` <|.. `ChatStreamProvider`
- `DeepSeekProvider` <|.. `ChatStreamProvider`
- `OllamaProvider` <|.. `ChatStreamProvider`
- `ChatStreamProvider` ..> `StreamController`

AI 抽象接口必须在 provider 之上，不要反过来。

### 7. 辅助数据连接

- `DirScanReq` ..> `DictInfo`
- `WebProbeReq` ..> `FingerprintData`
- `FingerprintData` *-- `FingerprintRule`
- `WAFBypassReq` ..> `PayloadVariant`

这一组应该放底部或侧边，不抢中部资源。

## 融合后如何避免“过宽”

必须做这几件事：

1. 请求层只保留关键字段，不保留所有字段说明文字
2. `POC规则` 与 `Nuclei规则` 内部字段可以只保留 4 到 6 个代表字段
3. `AIProvider` 四个实现类保留关键属性和 2 到 3 个核心方法
4. 不在总图里写完整返回类型，比如：
   - 把 `tuple<string, List<AIToolCall>, error>` 压成 `result`
5. 包区标题保留，字段压缩
6. 关系标签只保留关键的：
   - `implements`
   - `steps`
   - `validate`
   - `rules`
   - `messages`

## 推荐总图布局

推荐采用 3 层 2 列主结构：

第一层：

- 左：任务管理
- 中到右：扫描请求横带

第二层：

- 左：POC规则
- 右：EXP与AI-EXP

第三层：

- 左：WAF与辅助数据
- 中：AI核心
- 右：MCP / JADX

如果页面更宽，可以改成：

- 顶部：扫描请求
- 左列：任务管理 + POC规则
- 中列：EXP与AI-EXP
- 右列：MCP / JADX + AI Provider
- 底部：WAF与辅助数据

## 论文里怎么说

如果老师要求“必须一张图”，建议你这样解释：

> 由于系统类数量较多、关系较复杂，为保证总类图完整性，图中保留了所有核心类及其关键关系，同时对部分字段和方法进行了摘要化处理；详细的分模块类图作为补充材料单独给出。

这样你就能：

- 交 1 张总图满足要求
- 仍保留拆分图做补充
- 避免总图细节爆炸

