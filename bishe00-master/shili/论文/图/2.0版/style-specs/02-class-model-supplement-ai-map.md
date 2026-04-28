# AI补充类图拆分说明

原文件：
- `02-class-model-supplement-ai.puml`

现拆成两张：

1. `02-class-model-supplement-ai-core.puml`
   - `ChatMessage`
   - `AIProvider`
   - `ChatStreamProvider`
   - `AITool`
   - `AIToolCall`
   - `StreamController`
   - 与主类图中的 `BaseChatSession`、`MCPConnection` 的连接

2. `02-class-model-supplement-ai-providers.puml`
   - `OpenAIProvider`
   - `DeepSeekProvider`
   - `AnthropicProvider`
   - `OllamaProvider`
   - 它们与 `AIProvider`、`ChatStreamProvider` 的实现关系

## 两张图之间如何连接

- `AI核心交互` 图定义抽象接口和消息/工具模型
- `AI Provider实现` 图只负责展示四个 Provider 如何实现这些接口

关键连接点：

- `OpenAIProvider` 实现 `AIProvider`
- `DeepSeekProvider` 实现 `AIProvider`
- `AnthropicProvider` 实现 `AIProvider`
- `OllamaProvider` 实现 `AIProvider`
- `OpenAIProvider` / `DeepSeekProvider` / `OllamaProvider` 实现 `ChatStreamProvider`

推荐阅读顺序：

1. 先看 `02-class-model-supplement-ai-core.puml`
2. 再看 `02-class-model-supplement-ai-providers.puml`

