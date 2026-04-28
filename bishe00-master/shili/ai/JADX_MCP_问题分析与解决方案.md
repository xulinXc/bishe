# JADX MCP 功能问题分析与解决方案文档

## 概述

本文档详细记录了 JADX MCP（Model Context Protocol）功能开发过程中遇到的主要问题、问题原因分析、解决方案以及关键实现细节。包括完整的消息流程、端口配置说明、所有兜底和健壮性设计、以及调试输出信息等。

## 项目架构

JADX MCP 功能实现了以下架构：
- **前端**：HTML/JavaScript (SSE 流式响应)
- **后端**：Go (HTTP 服务器，端口 8080)
- **MCP 服务器**：Python FastMCP (JSON-RPC over HTTP，默认端口 8651，可自定义如 9999)
- **JADX Plugin**：运行在 JADX 中，端口 8650（MCP 服务器内部连接）
- **AI 提供商**：OpenAI、DeepSeek、Anthropic、Ollama（本地或云端）

## 系统架构图

```
┌─────────────┐         ┌──────────────┐         ┌──────────────────┐         ┌──────────────┐
│   Web前端    │ ──────► │  Go后端服务器   │ ──────► │  MCP服务器        │ ──────► │ JADX Plugin  │
│ (浏览器)     │  HTTP  │  (端口 8080)   │  HTTP  │  (端口 9999)     │  HTTP  │ (端口 8650)  │
│             │  SSE   │              │  JSON  │                  │  JSON  │              │
└─────────────┘         └──────────────┘         └──────────────────┘         └──────────────┘
                               │                           │
                               │                           │
                               ▼                           ▼
                        ┌──────────────┐         ┌──────────────┐
                        │  AI提供商     │         │  本地Ollama   │
                        │ (DeepSeek等) │         │ (可选,端口11434)│
                        └──────────────┘         └──────────────┘
```

---

## 问题分类

### 1. 流式响应中断导致数据丢失问题

#### 问题描述
当用户在 AI 回答过程中发送新消息时，旧的流式响应会被中止（`AbortController.abort()`），导致已收到的部分响应内容丢失，显示为 "错误: BodyStreamBuffer was aborted"。

**典型场景**：
1. 用户发送问题 A
2. AI 开始流式回答
3. 用户在 AI 回答过程中发送问题 B
4. 问题 A 的回答被替换为错误信息
5. 即使 AI 已经回答了问题 A，内容也会丢失

#### 问题原因

**前端问题**：
- `isLoading` 状态管理不当：即使流被中止，`isLoading` 可能在 `finally` 块执行前仍为 `true`
- 没有保留部分响应：当流被中止时，`currentText`（已累积的文本）被丢弃
- 状态检查时序问题：新请求可能在 `finally` 块执行前就开始，导致状态检查失败

**后端问题**：
- 流被中止时，已接收的部分内容没有保存到会话
- `processChatStream` 在检测到中止后直接返回错误，不保存已收到的内容

#### 解决方案

**前端修复**（`web/jadx_mcp.html`）：

1. **改进 `isLoading` 状态管理**：
   ```javascript
   // 在 AbortController 被中止时设置标志
   currentAbortController.signal.addEventListener('abort', () => {
     aborted = true;
   });
   
   // 在 finally 块中立即重置状态
   finally {
     isLoading = false; // 立即重置，确保后续请求不被误判
     currentAbortController = null;
   }
   ```

2. **保留部分响应内容**：
   ```javascript
   // 如果流被中止，保留已收到的内容
   if (aborted) {
     if (currentText.trim()) {
       updateMessage(loadingId, 'assistant', currentText); // 保留内容
       console.log('流被中止，但保留了已收到的内容');
     }
   }
   ```

3. **添加延迟和防御性检查**：
   ```javascript
   if (isLoading) {
     currentAbortController.abort();
     // 等待 finally 块执行完成
     await new Promise(resolve => setTimeout(resolve, 150));
     // 防御性检查
     if (isLoading) {
       isLoading = false; // 强制重置
     }
   }
   ```

**后端修复**（`internal/mcp/jadx_stream.go`）：

1. **保存部分响应到会话**：
   ```go
   // 即使被中止，如果已经收到部分内容，也要保存
   if fullResponse != "" || len(toolCalls) > 0 || (isAborted && lastSavedContent.Len() > 0) {
     contentToSave := fullResponse
     if contentToSave == "" && isAborted && lastSavedContent.Len() > 0 {
       contentToSave = lastSavedContent.String()
     }
     
     // 保存到会话（即使内容为空，如果有 tool_calls 也要保存）
     if contentToSave != "" || len(toolCalls) > 0 {
       session.AddMessage(ChatMessage{
         Role:      "assistant",
         Content:   contentToSave,
         Time:      time.Now().Format(time.RFC3339),
         ToolCalls: toolCalls,
       })
       
       // 如果被中止，记录日志但不返回错误（保留已收到的内容）
       if isAborted {
         log.Printf("流被中止，但已保存部分响应（长度: %d）", len(contentToSave))
         return nil // 正常返回，不返回错误
       }
     }
   }
   ```

2. **AI Provider 层修复**（`internal/mcp/ai_stream.go`）：
   ```go
   // 检查是否被中止
   if controller != nil && controller.IsAborted() {
     // 返回已累积的内容和 tool_calls，不返回错误
     return fullContent.String(), toolCalls, nil
   }
   ```

#### 关键要点
- **前端**：保留已接收的文本，不因中止而丢弃
- **后端**：流被中止时保存已收到的内容，返回 `nil` 错误表示正常终止
- **状态同步**：确保 `isLoading` 在 `finally` 中立即重置，避免竞态条件

---

### 2. FastMCP Session ID 管理问题

#### 问题描述
FastMCP 使用 HTTP 传输时，需要通过 Session ID 来维护会话状态。但 Session ID 的提取和传递机制不明确，导致连接失败或请求无法正确路由。

#### 问题原因

1. **Session ID 提取位置不确定**：
   - 可能在响应头 `Mcp-Session-Id` 中
   - 可能在 Cookie 中
   - 可能在响应体的 JSON 中

2. **Session ID 传递方式不统一**：
   - FastMCP 可能支持多种请求头名称
   - Cookie 和请求头的优先级不明确

3. **初始化请求的特殊性**：
   - `initialize` 请求不应该发送 Session ID
   - 首次请求后需要从响应中提取 Session ID

#### 解决方案

**实现多层级 Session ID 提取**（`internal/mcp/jadx.go`）：

```go
// 1. 优先检查响应头中的 Mcp-Session-Id
for headerName, headerValues := range resp.Header {
    if headerName == "Mcp-Session-Id" || headerName == "MCP-Session-Id" {
        if len(headerValues) > 0 && headerValues[0] != "" {
            conn.sessionID = headerValues[0]
            sessionIDUpdated = true
        }
    }
}

// 2. 检查 Cookie 中的 Session ID
if !sessionIDUpdated {
    cookies := resp.Cookies()
    for _, cookie := range cookies {
        if cookie.Name == "session" || cookie.Name == "mcp_session" ||
           cookie.Name == "fastmcp_session" {
            conn.sessionID = cookie.Value
            sessionIDUpdated = true
        }
    }
}

// 3. 检查其他包含 "session" 的响应头（备用）
if !sessionIDUpdated {
    for headerName, headerValues := range resp.Header {
        lowerName := strings.ToLower(headerName)
        if strings.Contains(lowerName, "session") {
            conn.sessionID = headerValues[0]
            break
        }
    }
}
```

**多种请求头传递方式**：

```go
// 在发送请求时，尝试多种可能的请求头名称
if method != "initialize" && sessionID != "" {
    req.Header.Set("Mcp-Session-Id", sessionID)
    req.Header.Set("X-Session-ID", sessionID)
    req.Header.Set("Session-ID", sessionID)
    req.Header.Set("X-MCP-Session-ID", sessionID)
    req.Header.Set("X-FastMCP-Session-ID", sessionID)
}
```

**关键要点**：
- **初始化请求**：不发送 Session ID，等待服务器生成
- **后续请求**：使用提取到的 Session ID，通过多个请求头名称发送
- **优先级**：响应头 `Mcp-Session-Id` > Cookie > 其他包含 "session" 的响应头

---

### 3. Tool Calls 消息格式问题

#### 问题描述
OpenAI 和 DeepSeek API 对包含 `tool_calls` 的 `assistant` 消息格式要求严格：
- 如果 `assistant` 消息包含 `tool_calls`，后面必须紧跟着对应数量的 `tool` 消息
- 每个 `tool` 消息必须包含 `tool_call_id`，且必须与 `tool_calls` 中的 `id` 匹配
- 如果 `tool` 消息数量不足，API 会返回错误

#### 问题原因

1. **消息历史过滤不当**：
   - 过滤掉了一些必要的 `tool` 消息
   - 保留了孤立的 `tool` 消息（不在 `assistant` 消息之后）

2. **Tool Call ID 匹配问题**：
   - 没有正确匹配 `tool_call_id` 和 `tool_calls[].id`
   - 没有处理 `tool` 消息缺失的情况

#### 解决方案

**实现智能消息过滤**（`internal/mcp/chat.go` 和 `internal/mcp/jadx_stream.go`）：

```go
// 保留所有非 tool 消息
for i := 0; i < len(allMessages); i++ {
    msg := allMessages[i]
    
    if msg.Role != "tool" {
        messages = append(messages, msg)
        
        // 如果这是一个包含 tool_calls 的 assistant 消息
        if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
            // 收集所有需要的 tool_call_id
            requiredToolCallIDs := make(map[string]bool)
            for _, tc := range msg.ToolCalls {
                requiredToolCallIDs[tc.ID] = true
            }
            
            // 检查并添加紧跟在后的匹配 tool 消息
            toolMessagesAdded := 0
            for j := i + 1; j < len(allMessages) && toolMessagesAdded < len(msg.ToolCalls); j++ {
                if allMessages[j].Role == "tool" {
                    if requiredToolCallIDs[allMessages[j].ToolCallID] {
                        messages = append(messages, allMessages[j])
                        toolMessagesAdded++
                        delete(requiredToolCallIDs, allMessages[j].ToolCallID)
                    }
                } else {
                    break // 遇到非 tool 消息，停止添加
                }
            }
            
            // 如果 tool 消息数量不足，移除 tool_calls（避免 API 错误）
            if toolMessagesAdded < len(msg.ToolCalls) {
                log.Printf("警告：assistant 消息有 %d 个 tool_calls，但只找到 %d 个匹配的 tool 消息，将移除 tool_calls",
                    len(msg.ToolCalls), toolMessagesAdded)
                // 创建不包含 tool_calls 的 assistant 消息副本
                messages[len(messages)-1] = ChatMessage{
                    Role:    msg.Role,
                    Content: msg.Content,
                    Time:    msg.Time,
                    // 不包含 ToolCalls
                }
            }
            
            // 跳过已经添加的 tool 消息
            i += toolMessagesAdded
        }
    }
    // 跳过孤立的 tool 消息（不在 assistant 之后的）
}
```

**为每个工具调用创建单独的消息**：

```go
// 将工具调用结果添加到消息历史
for _, toolCall := range toolCalls {
    result, err := mcpConn.CallTool(toolCall.Name, toolCall.Arguments)
    
    // 为每个工具调用创建单独的消息，包含对应的 tool_call_id
    messages = append(messages, ChatMessage{
        Role:       "tool",
        Content:    toolMessage,
        Time:       time.Now().Format(time.RFC3339),
        ToolCallID: toolCall.ID, // 每个工具调用使用对应的 ID
    })
}
```

**关键要点**：
- **匹配规则**：`tool` 消息必须紧跟在包含 `tool_calls` 的 `assistant` 消息之后
- **ID 匹配**：每个 `tool` 消息的 `tool_call_id` 必须匹配 `tool_calls[].id`
- **容错处理**：如果 `tool` 消息缺失，移除 `tool_calls` 以避免 API 错误

---

### 4. SSE 流式响应解析问题

#### 问题描述
FastMCP 和 AI API（OpenAI、DeepSeek）都使用 Server-Sent Events (SSE) 格式返回流式响应，但格式略有不同，需要统一解析。

#### 问题原因

1. **FastMCP SSE 格式**：
   ```
   event: message
   
   data: {"jsonrpc": "2.0", ...}
   ```

2. **OpenAI/DeepSeek SSE 格式**：
   ```
   data: {"choices": [{"delta": {...}}]}
   
   data: {"choices": [{"delta": {...}}]}
   
   data: [DONE]
   ```

3. **缓冲问题**：
   - 响应可能分多个 chunk 到达
   - 需要正确处理不完整的 JSON

#### 解决方案

**FastMCP 响应解析**（`internal/mcp/jadx.go`）：

```go
// 如果是 SSE 格式，提取 data 部分
if strings.HasPrefix(bodyStr, "event:") || strings.Contains(bodyStr, "data:") {
    lines := strings.Split(bodyStr, "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "data:") {
            jsonData = strings.TrimPrefix(line, "data:")
            jsonData = strings.TrimSpace(jsonData)
            break
        }
    }
}
```

**前端 SSE 解析**（`web/jadx_mcp.html`）：

```javascript
const reader = response.body.getReader();
const decoder = new TextDecoder();
let buffer = '';

while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    
    const chunk = decoder.decode(value, { stream: true });
    buffer += chunk;
    
    // 按行处理（SSE格式: data: ...\n\n）
    const lines = buffer.split('\n');
    buffer = lines.pop() || ''; // 保留最后一个不完整的行
    
    for (const line of lines) {
        if (line.trim().startsWith('data: ')) {
            const jsonStr = line.substring(6).trim();
            if (jsonStr && jsonStr !== 'null' && jsonStr !== '""') {
                const data = JSON.parse(jsonStr);
                // 处理数据...
            }
        }
    }
}
```

**AI Provider SSE 解析**（`internal/mcp/ai_stream.go`）：

```go
scanner := bufio.NewScanner(resp.Body)
buf := make([]byte, 0, 64*1024)
scanner.Buffer(buf, 1024*1024) // 增加缓冲区大小

for scanner.Scan() {
    line := strings.TrimSpace(scanner.Text())
    if line == "" || !strings.HasPrefix(line, "data: ") {
        continue
    }
    
    dataStr := strings.TrimPrefix(line, "data: ")
    if dataStr == "[DONE]" {
        break
    }
    
    // 解析 JSON...
}
```

**关键要点**：
- **缓冲区管理**：正确处理分块到达的数据
- **格式兼容**：支持多种 SSE 格式变体
- **错误处理**：对无效 JSON 进行容错处理

---

### 5. 前端状态同步问题

#### 问题描述
前端 `isLoading` 状态与实际的流式请求状态不同步，导致：
- 用户无法发送新消息（按钮被禁用）
- 新请求被误判为"正在加载中"
- 状态恢复不及时

#### 问题原因

1. **异步操作的时序问题**：
   - `finally` 块中的异步操作可能在新请求之后执行
   - `isLoading` 的重置时机不当

2. **多个状态标志冲突**：
   - `isLoading`、`aborted`、`currentAbortController` 之间的状态不一致

3. **DOM 更新延迟**：
   - 按钮状态的更新可能滞后于实际状态

#### 解决方案

**统一状态管理**：

```javascript
let isLoading = false;
let currentAbortController = null;
let currentEventSource = null;

async function sendMessage() {
    // 如果正在加载，先中止并等待状态恢复
    if (isLoading) {
        if (currentAbortController) {
            currentAbortController.abort();
        }
        // 等待 finally 块执行完成
        await new Promise(resolve => setTimeout(resolve, 150));
        // 防御性检查
        if (isLoading) {
            isLoading = false; // 强制重置
        }
    }
    
    isLoading = true;
    // 不禁用发送按钮，允许用户随时发送新消息
    
    try {
        await sendMessageStream(loadingId, message);
    } finally {
        // 立即重置状态
        isLoading = false;
        currentAbortController = null;
        currentEventSource = null;
    }
}
```

**监听中止事件**：

```javascript
currentAbortController = new AbortController();

currentAbortController.signal.addEventListener('abort', () => {
    aborted = true;
    console.log('AbortController 被中止');
});
```

**关键要点**：
- **立即重置**：在 `finally` 块中立即重置 `isLoading`
- **防御性编程**：在发送新请求前检查并强制重置状态
- **延迟等待**：给前一个请求的 `finally` 块足够的执行时间

---

### 6. 消息历史管理问题

#### 问题描述
在多轮对话中，消息历史的管理变得复杂：
- Tool calls 和 tool 消息的关联关系需要维护
- 流式响应中断后，消息可能不完整
- 会话重启后需要恢复消息历史

#### 解决方案

**消息结构设计**：

```go
type ChatMessage struct {
    Role       string       `json:"role"`                   // "user", "assistant", "tool"
    Content    string       `json:"content"`                // 消息内容
    Time       string       `json:"time"`                   // RFC3339 格式时间戳
    ToolCallID string       `json:"tool_call_id,omitempty"` // tool 消息必须的字段
    ToolCalls  []AIToolCall `json:"tool_calls,omitempty"`   // assistant 消息的 tool_calls
}
```

**会话管理**：

```go
type BaseChatSession struct {
    mu            sync.RWMutex
    ID            string
    APIKey        string
    APIType       string
    Messages      []ChatMessage
    MCPConnection MCPConnection
    CreatedAt     time.Time
    LastActivity  time.Time
}

// 线程安全的消息添加
func (s *BaseChatSession) AddMessage(msg ChatMessage) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.Messages = append(s.Messages, msg)
    s.LastActivity = time.Now()
}
```

**消息过滤和重建**：

- 在发送给 AI 之前，根据 API 要求过滤和重建消息列表
- 确保 `tool` 消息与对应的 `assistant` 消息正确关联
- 移除孤立的或无效的 `tool` 消息

---

## 潜在问题和注意事项

### 1. 并发请求处理
**问题**：如果用户在短时间内发送多个请求，可能会导致会话状态混乱。

**建议**：
- 前端限制：使用防抖（debounce）机制
- 后端限制：使用请求队列或锁机制
- 会话隔离：确保每个请求使用正确的会话 ID

### 2. 内存管理
**问题**：长时间运行的会话可能积累大量消息，导致内存占用过高。

**建议**：
- 实现消息历史限制（如最近 100 条消息）
- 定期清理旧会话
- 实现会话持久化（数据库存储）

### 3. 错误恢复
**问题**：网络错误、API 错误等可能导致会话状态不一致。

**建议**：
- 实现重试机制
- 记录错误日志
- 提供错误恢复接口

### 4. 安全性
**问题**：API Key、Session ID 等敏感信息可能泄露。

**建议**：
- 不在日志中输出完整的 API Key
- 使用 HTTPS 传输
- 实现会话超时机制

### 5. 性能优化
**问题**：大量并发请求可能影响服务器性能。

**建议**：
- 实现连接池
- 使用异步处理
- 优化消息序列化/反序列化

---

## 关键代码片段总结

### 1. 流式响应中断处理（前端）
```javascript
// 保留已接收的内容
if (aborted) {
    if (currentText.trim()) {
        updateMessage(loadingId, 'assistant', currentText);
    }
}

// 立即重置状态
finally {
    isLoading = false;
    currentAbortController = null;
}
```

### 2. 流式响应中断处理（后端）
```go
// 保存部分响应
if fullResponse != "" || len(toolCalls) > 0 {
    session.AddMessage(ChatMessage{
        Role:      "assistant",
        Content:   fullResponse,
        ToolCalls: toolCalls,
    })
}

if isAborted {
    return nil // 正常返回，不返回错误
}
```

### 3. Session ID 提取
```go
// 优先级：响应头 > Cookie > 其他
if headerName == "Mcp-Session-Id" {
    conn.sessionID = headerValues[0]
} else {
    // 检查 Cookie...
}
```

### 4. Tool Calls 消息过滤
```go
// 保留包含 tool_calls 的 assistant 消息及其后的匹配 tool 消息
if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
    // 添加匹配的 tool 消息...
    if toolMessagesAdded < len(msg.ToolCalls) {
        // 移除 tool_calls 以避免 API 错误
    }
}
```

---

## 测试建议

### 1. 流式响应中断测试
- 在 AI 回答过程中发送新消息
- 验证旧回答是否保留
- 验证新请求是否正常处理

### 2. Session ID 管理测试
- 测试连接建立和 Session ID 提取
- 测试多个请求的 Session ID 传递
- 测试连接断开重连

### 3. Tool Calls 测试
- 测试单个工具调用
- 测试多个工具调用
- 测试工具调用失败的情况

### 4. 并发测试
- 测试多个用户同时使用
- 测试同一用户快速发送多个请求

---

## 总结

JADX MCP 功能的开发过程中，主要解决了以下核心问题：

1. **流式响应中断导致数据丢失** - 通过前端保留内容和后端保存部分响应解决
2. **Session ID 管理** - 通过多层级提取和多种传递方式解决
3. **Tool Calls 消息格式** - 通过智能消息过滤和 ID 匹配解决
4. **SSE 解析** - 通过缓冲区管理和格式兼容解决
5. **前端状态同步** - 通过统一状态管理和防御性编程解决
6. **消息历史管理** - 通过合理的消息结构和过滤逻辑解决

这些问题大多源于异步操作、状态管理和协议实现的复杂性。通过仔细的状态管理、错误处理和容错机制，确保了功能的稳定性和可靠性。

---

## 端口配置问题详解

### 问题：为什么 JADX Plugin 端口是 8650，但网页端却使用 9999？

这是开发过程中最容易混淆的问题之一，涉及两个不同的端口角色：

#### 端口角色说明

1. **端口 8650 - JADX AI MCP Plugin（内部端口）**
   - **作用**：JADX 插件在 JADX GUI 中运行时监听的端口
   - **访问者**：只有 MCP 服务器（Python FastMCP）会连接这个端口
   - **用户可见性**：用户在网页端**不需要**知道或配置这个端口
   - **配置位置**：MCP 服务器启动时通过 `--jadx-port 8650` 参数指定（默认值）
   - **问题场景**：最初尝试在网页端直接连接 8650 端口，导致连接失败

2. **端口 8651/9999 - FastMCP HTTP 服务器（外部端口）**
   - **作用**：FastMCP 服务器对外提供的 HTTP API 端口
   - **访问者**：Go 后端服务器（网页端通过 Go 后端间接访问）
   - **用户可见性**：用户在网页端**必须**配置这个端口
   - **配置位置**：网页端的 "MCP 服务器地址" 输入框
   - **默认值**：8651（FastMCP 默认），但可以自定义为 9999 或其他端口

#### 为什么需要两个端口？

这是**代理架构**的设计：
- **MCP 服务器作为代理**：它连接到 JADX Plugin（8650），同时对外提供 HTTP API（9999）
- **职责分离**：
  - JADX Plugin（8650）：处理 JADX 相关的操作（获取类代码、分析 APK 等）
  - MCP 服务器（9999）：实现 MCP 协议，将 JADX Plugin 的功能暴露为标准化工具

#### 解决过程

**问题 1：初始连接失败**
- **现象**：在网页端配置 `http://127.0.0.1:8650`，连接失败
- **原因**：JADX Plugin 端口不支持标准的 HTTP MCP 协议，它有自己的协议格式
- **解决**：查阅文档后发现需要连接 FastMCP 服务器的 HTTP 端口（9999），而不是插件端口

**问题 2：端口配置混乱**
- **现象**：代码注释中提到多个端口，用户不知道配置哪个
- **解决**：在代码中添加详细注释（`internal/mcp/jadx.go:47-49`）：
  ```go
  // 默认端口：FastMCP 服务器默认是 8651，但如果手动启动可以指定其他端口（如 9999）
  // JADX AI MCP Plugin 在 8650，但那是插件端口，不是 MCP 服务器端口
  baseURL = "http://127.0.0.1:8651" // FastMCP 默认端口
  ```

**问题 3：前端默认值**
- **现象**：前端输入框默认值为 9999，但用户可能使用默认端口 8651
- **解决**：在输入框 placeholder 中提示两种可能的端口：
  ```html
  <input placeholder="http://127.0.0.1:9999 (默认) 或 http://127.0.0.1:8651">
  ```

#### 代码中的兜底机制

1. **空值处理**（`internal/mcp/jadx.go:46-50`）：
   ```go
   if baseURL == "" {
       // 提供默认值，避免连接失败
       baseURL = "http://127.0.0.1:8651"
   }
   ```

2. **连接复用检查**（`internal/mcp/jadx.go:53-55`）：
   ```go
   // 如果已经连接且 URL 相同，直接返回，避免重复连接
   if jadxmcpConn != nil && jadxmcpConn.connected && jadxmcpConn.baseURL == baseURL {
       return jadxmcpConn, nil
   }
   ```

3. **URL 格式化**（`internal/mcp/jadx.go:221-222`）：
   ```go
   // 确保 URL 格式正确，移除尾部斜杠
   url := strings.TrimSuffix(baseURL, "/")
   fullURL := url + "/mcp"
   ```

---

## 完整消息流程分析

### 场景 1：使用 DeepSeek（云端 AI）

#### 流程步骤

```
1. 用户在前端输入问题
   ↓
2. 前端发送 POST 请求到 Go 后端: /mcp/jadx/chat/stream
   ├─ 请求体: { sessionId, message }
   ├─ 使用 AbortController 支持取消
   └─ 期望响应: SSE 流式数据
   ↓
3. Go 后端 (jadx_stream.go: JADXChatStreamHandler)
   ├─ 验证会话配置（API Key、API 类型）
   ├─ 创建 StreamController
   ├─ 添加用户消息到会话
   └─ 启动 goroutine 调用 processChatStream
   ↓
4. processChatStream (jadx_stream.go:179)
   ├─ 获取消息历史（包含 tool_calls 和 tool 消息）
   ├─ 获取 MCP 工具列表（如果 MCP 已连接）
   ├─ 根据 API 类型创建 AI Provider（DeepSeekProvider）
   └─ 进入迭代循环
   ↓
5. DeepSeekProvider.ChatStream (ai_stream.go:164)
   ├─ 构建请求体（包含消息历史和工具列表）
   ├─ 发送 HTTP POST 到 DeepSeek API
   ├─ 接收 SSE 流式响应
   ├─ 解析每个 data: 块
   ├─ 累积文本内容（fullContent）
   ├─ 累积工具调用（toolCalls）
   ├─ 实时发送到 StreamController
   └─ 返回完整内容和工具调用列表
   ↓
6. 如果 AI 返回工具调用（tool_calls）
   ├─ 遍历每个工具调用
   ├─ 调用 MCP 连接: CallTool(toolName, arguments)
   ├─ MCP 连接发送 JSON-RPC 请求到 MCP 服务器（端口 9999）
   │  ├─ 方法: tools/call
   │  ├─ 参数: { name, arguments }
   │  └─ Session ID 通过请求头传递
   ├─ MCP 服务器转发请求到 JADX Plugin（端口 8650）
   ├─ JADX Plugin 执行操作（如获取类代码）
   ├─ 结果返回到 MCP 服务器
   ├─ MCP 服务器返回 JSON-RPC 响应
   ├─ Go 后端解析响应，提取结果
   ├─ 将结果作为 tool 消息添加到会话
   └─ 继续迭代，将工具结果发送给 AI
   ↓
7. AI 生成最终回答（基于工具调用结果）
   ├─ 继续流式返回
   └─ 完整内容保存到会话
   ↓
8. 前端接收 SSE 数据
   ├─ 解析 data: 后的 JSON 字符串
   ├─ 实时更新消息显示
   └─ 流结束时添加时间戳
   ↓
9. 后端保存完整消息到会话
   └─ 包含 AI 回答和所有工具调用结果
```

#### 关键代码路径

**前端**（`web/jadx_mcp.html`）：
- `sendMessage()` → `sendMessageStream()` → `fetch('/mcp/jadx/chat/stream')`
- SSE 解析：`reader.read()` → `decoder.decode()` → 提取 `data:` 后的 JSON

**后端**（`internal/mcp/`）：
- `jadx_stream.go: JADXChatStreamHandler` → `processChatStream` → `ai_stream.go: DeepSeekProvider.ChatStream`
- 工具调用：`jadx.go: CallTool` → `SendMCPRequest("tools/call")`

**MCP 服务器**（Python FastMCP）：
- 接收 JSON-RPC 请求 → 解析 → 调用 JADX Plugin → 返回结果

### 场景 2：使用本地 Ollama

#### 流程差异

主要差异在于 AI 提供商的不同：

```
1-3. 同场景 1（用户输入 → Go 后端接收）

4. processChatStream
   ├─ 解析 Ollama 配置（APIKey 格式: "http://localhost:11434|model_name"）
   ├─ 创建 OllamaProvider
   └─ 其他流程相同

5. OllamaProvider.ChatStream (ai_stream.go:18)
   ├─ 构建请求体
   ├─ 发送到本地 Ollama: http://localhost:11434/api/chat
   ├─ 接收 SSE 流式响应（格式与 DeepSeek 不同）
   ├─ 解析 JSON 行（每行一个 JSON 对象）
   ├─ 提取 content 字段
   ├─ 处理工具调用（如果支持）
   └─ 实时发送到 StreamController

6-9. 同场景 1（工具调用 → MCP 服务器 → JADX Plugin → 返回结果）
```

#### Ollama 的特殊处理

1. **配置解析**（`internal/mcp/jadx_handlers.go:99-108`）：
   ```go
   if session.APIType == "ollama" {
       ollamaURL := req.OllamaBaseURL
       if ollamaURL == "" {
           ollamaURL = "http://localhost:11434" // 默认值
       }
       model := req.Model
       if model == "" {
           model = "gpt-oss:20b" // 默认模型
       }
       session.APIKey = fmt.Sprintf("%s|%s", ollamaURL, model) // 组合存储
   }
   ```

2. **响应格式差异**（`internal/mcp/ai_stream.go:99-155`）：
   - DeepSeek: `data: {"choices": [{"delta": {...}}]}`
   - Ollama: `{"message": {"content": "...", ...}}`（每行一个 JSON）

3. **工具调用支持**：
   - 某些 Ollama 模型可能不支持工具调用
   - 代码中已添加工具调用解析，但需要模型支持

---

## 兜底、健壮性和容错机制详解

### 1. 连接层容错

#### Session ID 提取的多层级兜底

**问题**：FastMCP 可能通过多种方式传递 Session ID，需要兼容所有可能。

**解决方案**（`internal/mcp/jadx.go:296-329`）：
```go
// 第一优先级：响应头中的标准名称
if headerName == "Mcp-Session-Id" || headerName == "MCP-Session-Id" {
    conn.sessionID = headerValues[0]
    sessionIDUpdated = true
}

// 第二优先级：Cookie 中的 Session ID
if !sessionIDUpdated {
    for _, cookie := range resp.Cookies() {
        if cookie.Name == "session" || cookie.Name == "mcp_session" ||
           cookie.Name == "fastmcp_session" {
            conn.sessionID = cookie.Value
            sessionIDUpdated = true
        }
    }
}

// 第三优先级：其他包含 "session" 的响应头（兜底）
if !sessionIDUpdated {
    for headerName, headerValues := range resp.Header {
        lowerName := strings.ToLower(headerName)
        if strings.Contains(lowerName, "session") {
            conn.sessionID = headerValues[0]
            break
        }
    }
}
```

**为什么需要**：FastMCP 的不同版本或配置可能使用不同的 Session ID 传递方式，通过多层级检查确保兼容性。

#### 请求头多重发送

**问题**：MCP 服务器可能识别不同的请求头名称。

**解决方案**（`internal/mcp/jadx.go:240-249`）：
```go
if method != "initialize" && sessionID != "" {
    // 发送多个可能的请求头名称，确保兼容性
    req.Header.Set("Mcp-Session-Id", sessionID)
    req.Header.Set("X-Session-ID", sessionID)
    req.Header.Set("Session-ID", sessionID)
    req.Header.Set("X-MCP-Session-ID", sessionID)
    req.Header.Set("X-FastMCP-Session-ID", sessionID)
}
```

**为什么需要**：不同的 MCP 服务器实现可能识别不同的请求头名称，同时发送多个增加成功率。

### 2. 消息格式容错

#### SSE 格式兼容

**问题**：FastMCP 和 AI API 的 SSE 格式可能略有不同。

**解决方案**（`internal/mcp/jadx.go:342-360`）：
```go
// 默认假设是纯 JSON
jsonData := bodyStr

// 如果是 SSE 格式，提取 data 部分
if strings.HasPrefix(bodyStr, "event:") || strings.Contains(bodyStr, "data:") {
    lines := strings.Split(bodyStr, "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "data:") {
            jsonData = strings.TrimPrefix(line, "data:")
            jsonData = strings.TrimSpace(jsonData)
            break
        }
    }
}
```

**为什么需要**：FastMCP 可能返回标准 SSE 格式，也可能返回纯 JSON，需要兼容两种情况。

#### 工具消息格式容错

**问题**：MCP 工具可能使用 `inputSchema` 或 `parameters` 字段。

**解决方案**（`internal/mcp/jadx.go:170-174`）：
```go
// 优先尝试 inputSchema
if params, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
    tool.Parameters = params
} else if params, ok := toolMap["parameters"].(map[string]interface{}); ok {
    // 兜底：尝试 parameters
    tool.Parameters = params
}
```

**为什么需要**：不同的 MCP 服务器版本可能使用不同的字段名称。

### 3. Tool Calls 消息关联容错

#### 工具消息缺失处理

**问题**：如果 assistant 消息包含 tool_calls，但后续没有对应的 tool 消息，API 会报错。

**解决方案**（`internal/mcp/chat.go:59-70`）：
```go
// 如果 tool 消息数量不足，移除 assistant 消息的 tool_calls（避免 API 错误）
if toolMessagesAdded < len(msg.ToolCalls) {
    log.Printf("警告：assistant 消息有 %d 个 tool_calls，但只找到 %d 个匹配的 tool 消息，将移除 tool_calls",
        len(msg.ToolCalls), toolMessagesAdded)
    // 创建不包含 tool_calls 的 assistant 消息副本
    messages[len(messages)-1] = ChatMessage{
        Role:    msg.Role,
        Content: msg.Content,
        Time:    msg.Time,
        // 不包含 ToolCalls（避免 API 错误）
    }
}
```

**为什么需要**：会话历史可能不完整（如会话重启），需要容错处理。

#### 孤立工具消息过滤

**问题**：消息历史中可能存在孤立的 tool 消息（不在 assistant 消息之后）。

**解决方案**（`internal/mcp/chat.go:76`）：
```go
// 跳过孤立的 tool 消息（不在 assistant 之后的）
// 只保留紧跟在包含 tool_calls 的 assistant 消息后的 tool 消息
```

**为什么需要**：OpenAI/DeepSeek API 要求 tool 消息必须紧跟在对应的 assistant 消息之后。

### 4. 流式响应容错

#### 流中断时的内容保存

**问题**：用户取消请求时，已接收的部分内容会丢失。

**解决方案**（`internal/mcp/jadx_stream.go:299-329`）：
```go
// 即使被中止，如果已经收到部分内容，也要保存
if fullResponse != "" || len(toolCalls) > 0 || (isAborted && lastSavedContent.Len() > 0) {
    contentToSave := fullResponse
    // 如果被中止且当前响应为空，尝试使用上次保存的内容
    if contentToSave == "" && isAborted && lastSavedContent.Len() > 0 {
        contentToSave = lastSavedContent.String()
    }
    
    // 保存到会话
    if contentToSave != "" || len(toolCalls) > 0 {
        session.AddMessage(ChatMessage{
            Role:      "assistant",
            Content:   contentToSave,
            ToolCalls: toolCalls,
        })
        
        // 如果被中止，正常返回，不返回错误
        if isAborted {
            log.Printf("流被中止，但已保存部分响应（长度: %d）", len(contentToSave))
            return nil // 正常返回，不返回错误
        }
    }
}
```

**为什么需要**：用户可能因为等待时间过长而取消，但已接收的内容仍然有价值。

#### 工具调用参数解析容错

**问题**：工具调用的 arguments 可能是 JSON 字符串，需要解析。

**解决方案**（`internal/mcp/ai_stream.go:343-349`）：
```go
if tc.Function.Arguments != "" {
    var newArgs map[string]interface{}
    if err := json.Unmarshal([]byte(tc.Function.Arguments), &newArgs); err == nil {
        // 合并参数（新参数覆盖旧参数）
        for k, v := range newArgs {
            existing.Arguments[k] = v
        }
    }
    // 如果解析失败，忽略该参数（容错）
}
```

**为什么需要**：AI 可能在流式输出中分多次发送工具调用参数，需要合并。

### 5. 前端状态管理容错

#### isLoading 状态防御

**问题**：异步操作的时序问题可能导致 `isLoading` 状态不同步。

**解决方案**（`web/jadx_mcp.html:659-680`）：
```javascript
if (isLoading) {
    // 中止当前请求
    currentAbortController.abort();
    
    // 等待 finally 块执行完成（防御性延迟）
    await new Promise(resolve => setTimeout(resolve, 150));
    
    // 防御性检查：如果仍然是 true，强制重置
    if (isLoading) {
        console.warn('警告：等待后 isLoading 仍为 true，强制重置');
        isLoading = false; // 强制重置
    }
}
```

**为什么需要**：JavaScript 的异步特性可能导致状态更新延迟，需要防御性检查。

#### 消息元素查找容错

**问题**：DOM 操作可能失败，消息元素可能不存在。

**解决方案**（`web/jadx_mcp.html:987-1008`）：
```javascript
function updateMessageStreaming(id, content) {
    const messageDiv = document.getElementById(id);
    if (!messageDiv) {
        console.warn('找不到消息元素:', id);
        return; // 容错：找不到就返回，不报错
    }
    
    // 确保这个消息是 assistant 角色
    if (!messageDiv.classList.contains('assistant')) {
        console.error('错误: 尝试更新非 assistant 消息框!');
        // 尝试找到正确的 assistant 消息框
        const assistantMessages = document.querySelectorAll('.message.assistant');
        if (assistantMessages.length > 0) {
            // 使用找到的消息框（兜底）
            const correctMessageDiv = assistantMessages[assistantMessages.length - 1];
            // 继续处理...
        }
        return;
    }
}
```

**为什么需要**：DOM 更新和 JavaScript 执行可能存在时序问题，需要容错处理。

### 6. 错误响应处理

#### HTTP 状态码分类处理

**问题**：不同的 HTTP 状态码需要不同的处理方式。

**解决方案**（`internal/mcp/jadx.go:389-402`）：
```go
// 处理错误响应
if bodyStr == "" {
    bodyStr = "(响应体为空)" // 确保错误信息不为空
}

if resp.StatusCode == http.StatusNotAcceptable {
    return nil, fmt.Errorf("请求格式不被接受 (406): %s。请检查 Accept 头设置", bodyStr)
} else if resp.StatusCode == http.StatusBadRequest {
    return nil, fmt.Errorf("请求格式错误 (400): %s", bodyStr)
} else if resp.StatusCode == http.StatusNotFound {
    return nil, fmt.Errorf("端点不存在 (404)", fullURL)
} else {
    return nil, fmt.Errorf("服务器返回错误 (状态码 %d): %s", resp.StatusCode, bodyStr)
}
```

**为什么需要**：不同错误码表示不同的问题，需要明确的错误信息帮助调试。

#### JSON 解析错误详情

**问题**：JSON 解析失败时，需要提供详细的错误信息。

**解决方案**（`internal/mcp/jadx.go:368-369`）：
```go
if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
    return nil, fmt.Errorf("解析响应失败 (%s): %v, 原始响应: %s, 提取的JSON: %s",
        fullURL, err, bodyStr, jsonData)
}
```

**为什么需要**：提供原始响应和提取的 JSON，便于调试。

### 7. 初始化流程容错

#### Initialize 请求失败容错

**问题**：Initialize 请求可能失败，但不应该阻止连接。

**解决方案**（`internal/mcp/jadx.go:94-109`）：
```go
initResp, err := conn.SendMCPRequest("initialize", initParams)
if err != nil {
    log.Printf("初始化请求失败（继续尝试）: %v", err)
    // 如果初始化失败，继续尝试列出工具（不返回错误）
} else {
    log.Printf("初始化成功: %v", initResp)
    // 尝试从初始化响应中获取 session ID
    // ...
}

// 即使初始化失败，也继续尝试列出工具（兜底）
tools, err := conn.listTools()
```

**为什么需要**：某些 MCP 服务器可能不需要 initialize 请求，直接列出工具即可。

### 8. 迭代限制

#### 最大迭代次数

**问题**：AI 和工具调用可能陷入无限循环。

**解决方案**（`internal/mcp/jadx_stream.go:286`）：
```go
maxIterations := 10
iteration := 0

for iteration < maxIterations {
    iteration++
    // ... 处理逻辑
}

if iteration >= maxIterations {
    log.Printf("达到最大迭代次数 (%d)，可能陷入循环", maxIterations)
    controller.Send("已达到最大迭代次数，停止处理。")
    return nil
}
```

**为什么需要**：防止无限循环导致资源耗尽。

---

## 调试输出信息分析

所有 `log.Printf` 和 `console.log` 都是在遇到问题时添加的，用于定位和解决问题。

### 后端调试输出（Go）

#### 连接相关日志

**位置**：`internal/mcp/jadx.go`

1. **连接尝试日志**（224-226 行）：
   ```go
   log.Printf("[JADX MCP] 尝试连接: %s", fullURL)
   log.Printf("[JADX MCP] 请求方法: %s, 请求ID: %d, SessionID: %s", method, requestID, sessionID)
   log.Printf("[JADX MCP] 请求体: %s", string(reqData))
   ```
   **问题**：连接失败时不知道请求详情
   **解决**：记录完整的请求信息，便于排查

2. **Session ID 相关日志**（249-251, 263, 276, 291, 308, 319, 324 行）：
   ```go
   log.Printf("[JADX MCP] 发送 Session ID: %s (方法: %s)", sessionID, method)
   log.Printf("[JADX MCP] initialize 请求，不发送 Session ID")
   log.Printf("[JADX MCP] Session/Cookie 相关请求头: %v", reqHeaders)
   log.Printf("[JADX MCP] 完整请求头: %v", req.Header)
   log.Printf("[JADX MCP] 响应头: %v", resp.Header)
   log.Printf("[JADX MCP] 收到 Cookies: %v", cookies)
   log.Printf("[JADX MCP] 从 Cookie 获取到 Session ID: %s", cookie.Value)
   log.Printf("[JADX MCP] 更新 Session ID: %s -> %s (从响应头 %s)", oldSessionID, newSessionID, headerName)
   ```
   **问题**：Session ID 提取失败，不知道是请求头还是响应的问题
   **解决**：记录所有 Session ID 相关的操作，便于追踪

3. **响应解析日志**（334-339 行）：
   ```go
   log.Printf("[JADX MCP] 响应状态码: %d (URL: %s)", resp.StatusCode, fullURL)
   log.Printf("[JADX MCP] 响应体长度: %d 字节", len(bodyStr))
   if len(bodyStr) > 500 {
       log.Printf("[JADX MCP] 响应体（前500字符）: %s...", bodyStr[:500])
   } else {
       log.Printf("[JADX MCP] 响应体: %s", bodyStr)
   }
   ```
   **问题**：响应解析失败，不知道响应内容
   **解决**：记录响应详情（截断长响应），便于调试

#### 消息处理相关日志

**位置**：`internal/mcp/jadx_stream.go`, `internal/mcp/chat.go`

1. **工具调用日志**（356 行）：
   ```go
   log.Printf("[MCP Chat Stream] 调用工具: %s", toolCall.Name)
   ```
   **问题**：不知道 AI 调用了哪些工具
   **解决**：记录工具调用，便于追踪执行流程

2. **消息过滤警告**（217 行）：
   ```go
   log.Printf("[MCP Chat Stream] 警告：assistant 消息有 %d 个 tool_calls，但只找到 %d 个匹配的 tool 消息，将移除 tool_calls",
       len(msg.ToolCalls), toolMessagesAdded)
   ```
   **问题**：消息历史不完整时，不知道发生了什么
   **解决**：记录警告信息，便于发现问题

3. **流中断日志**（326 行）：
   ```go
   log.Printf("[MCP Chat Stream] 流被中止，但已保存部分响应（长度: %d）", len(contentToSave))
   ```
   **问题**：流被中断时，不知道是否保存了内容
   **解决**：记录保存情况，便于验证修复是否生效

4. **连接状态日志**（242, 349 行）：
   ```go
   log.Printf("[MCP Chat Stream] 为 AI 提供 %d 个 MCP 工具", len(tools))
   log.Printf("[MCP Chat Stream] MCP 连接不可用，无法执行工具调用")
   ```
   **问题**：工具调用失败时，不知道是连接问题还是其他问题
   **解决**：记录连接状态和工具数量，便于排查

### 前端调试输出（JavaScript）

**位置**：`web/jadx_mcp.html`

1. **状态管理日志**（660, 679, 681 行）：
   ```javascript
   console.log('[sendMessage] 检测到正在加载中，取消当前请求后发送新消息，当前 isLoading:', isLoading);
   console.log('[sendMessage] 等待完成，isLoading 已重置为:', isLoading);
   console.log('[sendMessage] 开始发送消息，isLoading:', isLoading);
   ```
   **问题**：状态不同步，不知道 `isLoading` 的真实值
   **解决**：记录状态变化，便于追踪问题

2. **流处理日志**（734, 738, 749, 760, 818 行）：
   ```javascript
   console.log('[Stream] AbortController 被中止');
   console.log('[Stream] 开始发送流式请求');
   console.log('[Stream] 收到响应，状态:', response.status);
   console.log('[Stream] 开始读取流式数据');
   console.log('[Stream] 收到数据片段，长度:', data.length, '总长度:', currentText.length);
   ```
   **问题**：流式响应中断，不知道是网络问题还是代码问题
   **解决**：记录流的每个阶段，便于定位问题

3. **错误处理日志**（832, 887, 896 行）：
   ```javascript
   console.warn('[Stream] 解析 SSE 数据失败:', e, '原始行:', line);
   console.log('[Stream] 流被中止但有错误，保留了已收到的内容');
   console.log('[Stream] finally 块执行，立即恢复所有状态，aborted:', aborted);
   ```
   **问题**：错误发生时，不知道上下文信息
   **解决**：记录详细的错误信息和状态，便于调试

4. **DOM 操作日志**（988, 994, 1002 行）：
   ```javascript
   console.warn('找不到消息元素:', id);
   console.error('[updateMessageStreaming] 错误: 尝试更新非 assistant 消息框!');
   console.log('[updateMessageStreaming] 已切换到正确的 assistant 消息框:', correctId);
   ```
   **问题**：DOM 更新失败，不知道是元素不存在还是选择器错误
   **解决**：记录 DOM 操作结果，便于排查

---

## 参考资料

- [FastMCP 文档](https://github.com/jlowin/fastmcp)
- [MCP 协议规范](https://modelcontextprotocol.io/)
- [OpenAI API 文档](https://platform.openai.com/docs/api-reference)
- [Server-Sent Events 规范](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)

---

*文档创建时间：2025-01-27*
*最后更新时间：2025-01-27*

