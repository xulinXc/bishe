# IDA MCP 实现思路

## 一、架构对比

### JADX MCP（已完成）
- **协议**：FastMCP streamable-http
- **连接方式**：HTTP/JSON-RPC，直接连接到独立运行的 MCP 服务器
- **服务器端口**：默认 8651 或 9999
- **连接示例**：`http://127.0.0.1:9999/mcp`

### IDA MCP（待实现）
- **协议**：标准 MCP 协议（通过 Cursor mcp.json 配置）
- **连接方式**：需要桥接，因为 IDA MCP 服务器运行在 Cursor 的 Python 进程中
- **配置位置**：`C:\Users\86483\.cursor\mcp.json`
- **服务器路径**：`C:\Users\86483\AppData\Roaming\Python\Python313\site-packages\ida_pro_mcp\server.py`

## 二、实现方案

### 方案 A：通过本地 HTTP 代理桥接（推荐）

IDA Pro MCP 支持 SSE Transport，可以启动独立的 HTTP 服务器：

```bash
uv run ida-pro-mcp --transport http://127.0.0.1:8744/sse
```

或者使用 idalib（headless）：

```bash
uv run idalib-mcp --host 127.0.0.1 --port 8745 path/to/executable
```

**优点**：
- 与 JADX MCP 实现方式一致
- 直接通过 HTTP 连接，复用现有代码结构
- 支持 SSE 流式传输

**实现步骤**：
1. 用户在前端配置 IDA MCP 服务器地址（如 `http://127.0.0.1:8744`）
2. 后端通过 HTTP 连接到 IDA MCP 服务器
3. 使用标准 MCP 协议（JSON-RPC over HTTP）通信
4. 复用 JADX MCP 的连接和工具调用逻辑

## 三、文件结构

参考 JADX MCP 的实现，创建以下文件：

```
internal/mcp/
├── ida.go              # IDA MCP 连接和工具调用（类似 jadx.go）
├── ida_handlers.go     # HTTP 路由处理器（类似 jadx_handlers.go）
├── ida_stream.go       # 流式聊天处理器（复用 jadx_stream.go 逻辑）
└── common.go           # 通用接口（已存在，可复用）

web/
├── ida_mcp.html        # IDA MCP 前端界面（参考 jadx_mcp.html）

main.go                 # 注册 IDA MCP 路由
```

## 四、核心实现细节

### 1. `internal/mcp/ida.go` 结构

```go
// IDAMCPConnection IDA MCP 连接
type IDAMCPConnection struct {
    mu            sync.RWMutex
    baseURL       string // MCP 服务器基础 URL，如 http://127.0.0.1:8744
    client        *http.Client
    connected     bool
    tools         []MCPTool
    lastRequestID int64
}

// ConnectIDAMCP 连接到 IDA MCP 服务器
func ConnectIDAMCP(baseURL string) (*IDAMCPConnection, error) {
    // 1. 创建 HTTP 客户端
    // 2. 发送 initialize 请求
    // 3. 获取工具列表
    // 4. 返回连接对象
}

// SendMCPRequest 向 MCP 服务器发送 JSON-RPC 请求
func (conn *IDAMCPConnection) SendMCPRequest(method string, params map[string]interface{}) (map[string]interface{}, error) {
    // 标准 MCP 协议，JSON-RPC 2.0
    // 不需要 session ID（与 FastMCP 不同）
}

// CallTool 调用 MCP 工具
func (conn *IDAMCPConnection) CallTool(toolName string, arguments map[string]interface{}) (interface{}, error) {
    // 调用 tools/call 方法
}

// GetTools 获取工具列表
func (conn *IDAMCPConnection) GetTools() []AITool {
    // 转换为 AI 工具格式
}
```

**与 JADX MCP 的区别**：
- **不需要 session ID**：标准 MCP 协议不依赖 session
- **响应格式不同**：不需要处理 SSE 格式的响应体，直接是 JSON
- **初始化流程**：可能需要不同的初始化参数

### 2. `internal/mcp/ida_handlers.go` 结构

完全复用 JADX 的实现模式：

```go
// ConnectIDAHandler 连接 IDA MCP 服务器
func ConnectIDAHandler(w http.ResponseWriter, r *http.Request)

// ConfigureIDASessionHandler 配置 IDA 会话（设置 API key）
func ConfigureIDASessionHandler(w http.ResponseWriter, r *http.Request)

// GetIDAMessagesHandler 获取 IDA 会话消息历史
func GetIDAMessagesHandler(w http.ResponseWriter, r *http.Request)

// GetIDASessionStatusHandler 获取 IDA 会话状态
func GetIDASessionStatusHandler(w http.ResponseWriter, r *http.Request)
```

### 3. `internal/mcp/ida_stream.go` 结构

可以直接复用 `jadx_stream.go` 的 `JADXChatStreamHandler` 和 `processChatStream` 逻辑，只需要：

```go
// IDAChatStreamHandler 流式聊天处理器（SSE）
func IDAChatStreamHandler(w http.ResponseWriter, r *http.Request) {
    // 复制 jadx_stream.go 的实现
    // 只需要将 GetOrCreateJADXSession 改为 GetOrCreateIDASession
    // 将 session.GetMCPConnection() 返回的接口类型保持不变（MCPConnection 接口）
}

// IDAChatSession IDA 聊天会话
type IDAChatSession struct {
    BaseChatSession
}

// GetOrCreateIDASession 获取或创建 IDA 会话
func GetOrCreateIDASession(sessionID string) *IDAChatSession
```

### 4. 会话管理

在 `ida.go` 中添加：

```go
var (
    idaSessions   = make(map[string]*IDAChatSession)
    idaSessionsMu sync.RWMutex
    idamcpConn    *IDAMCPConnection
    idamcpConnMu  sync.RWMutex
)

// CleanupOldIDASessions 清理旧的会话
func CleanupOldIDASessions(maxAge time.Duration)
```

### 5. `main.go` 路由注册

```go
// IDA MCP routes
mux.HandleFunc("/mcp/ida/connect", ida.ConnectIDAHandler)
mux.HandleFunc("/mcp/ida/configure", ida.ConfigureIDASessionHandler)
mux.HandleFunc("/mcp/ida/chat/stream", ida.IDAChatStreamHandler)
mux.HandleFunc("/mcp/ida/messages", ida.GetIDAMessagesHandler)
mux.HandleFunc("/mcp/ida/status", ida.GetIDASessionStatusHandler)

// 启动 IDA 会话清理器
go func() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for range ticker.C {
        ida.CleanupOldIDASessions(24 * time.Hour)
        log.Printf("[清理] IDA 会话清理完成")
    }
}()
```

### 6. `web/ida_mcp.html` 前端

完全参考 `jadx_mcp.html`，只需要：
- 修改标题和提示文本（"IDA MCP" 替代 "JADX MCP"）
- 修改 API 端点（`/mcp/ida/*` 替代 `/mcp/jadx/*`）
- 修改默认 MCP 服务器地址（如 `http://127.0.0.1:8744`）
- 修改欢迎消息（"分析二进制文件" 替代 "分析 Android APK 文件"）

## 五、MCP 协议差异处理

### JADX MCP（FastMCP streamable-http）
```json
// 请求头可能需要
Mcp-Session-Id: xxx

// 响应可能是 SSE 格式
event: message
data: {...JSON...}
```

### IDA MCP（标准 MCP）
```json
// 直接 JSON-RPC 2.0
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {...}
}

// 响应直接是 JSON
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {...}
}
```

**实现时需要注意**：
1. `SendMCPRequest` 不需要处理 SSE 格式
2. 不需要 session ID 管理
3. 响应解析更简单（直接 `json.Unmarshal`）

## 六、实施步骤

### 阶段 1：基础连接（1-2 小时）
1. ✅ 创建 `internal/mcp/ida.go`，实现 `ConnectIDAMCP` 和基础连接
2. ✅ 测试连接到 IDA MCP 服务器（需要先启动 SSE 服务器）
3. ✅ 实现 `listTools` 和 `GetTools`

### 阶段 2：HTTP 路由（30 分钟）
1. ✅ 创建 `internal/mcp/ida_handlers.go`，复制 JADX 的处理逻辑
2. ✅ 在 `main.go` 注册路由
3. ✅ 测试 `/mcp/ida/connect` 和 `/mcp/ida/configure`

### 阶段 3：聊天功能（1 小时）
1. ✅ 创建 `internal/mcp/ida_stream.go`，复用 `jadx_stream.go` 逻辑
2. ✅ 实现会话管理（`GetOrCreateIDASession`）
3. ✅ 测试流式聊天功能

### 阶段 4：前端界面（1 小时）
1. ✅ 创建 `web/ida_mcp.html`，复制并修改 `jadx_mcp.html`
2. ✅ 修改所有 API 端点和文本提示
3. ✅ 测试完整流程

### 阶段 5：测试与优化（1 小时）
1. ✅ 测试各种 IDA MCP 工具调用
2. ✅ 处理错误情况（连接失败、工具执行失败等）
3. ✅ 优化日志和错误提示

## 七、启动 IDA MCP 服务器

在开始实现之前，需要确保 IDA MCP 服务器可以独立运行：

### 方法 1：使用 SSE Transport（推荐）
```bash
# 在 IDA Pro 中，通过插件启动 MCP 服务器后，可以指定 SSE 模式
# 或者在命令行启动独立的 SSE 服务器
uv run ida-pro-mcp --transport http://127.0.0.1:8744/sse
```

### 方法 2：使用 idalib（headless）
```bash
# 如果不需要 GUI，可以使用 idalib-mcp
uv run idalib-mcp --host 127.0.0.1 --port 8745 path/to/executable
```

### 验证连接
```bash
# 测试连接
curl http://127.0.0.1:8744/mcp -X POST -H "Content-Type: application/json" -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "test",
      "version": "1.0.0"
    }
  }
}'
```

## 八、与 JADX MCP 的主要差异总结

| 特性 | JADX MCP | IDA MCP |
|------|----------|---------|
| 协议 | FastMCP streamable-http | 标准 MCP (JSON-RPC 2.0) |
| Session ID | 需要（通过 Cookie 或 Header） | 不需要 |
| 响应格式 | SSE 格式（`data: {...}`） | 直接 JSON |
| 连接方式 | HTTP POST to `/mcp` | HTTP POST to `/mcp` 或 stdio |
| 初始化 | 需要 initialize + session | 可能需要不同的初始化 |
| 工具调用 | `tools/call` | `tools/call`（相同） |

## 九、注意事项

1. **IDA Pro 插件必须运行**：IDA MCP 服务器需要 IDA Pro 插件支持，确保插件已安装并运行
2. **端口配置**：默认端口可能不同，需要在前端配置界面说明
3. **二进制文件路径**：IDA MCP 需要加载二进制文件，可能需要文件上传或路径配置功能
4. **工具列表**：IDA MCP 的工具列表与 JADX 不同，需要测试各个工具的参数格式
5. **错误处理**：IDA Pro 特定错误（如未加载文件、插件未启动等）需要友好提示

## 十、代码复用策略

- **完全复用**：`ai.go`（AI 提供商）、`common.go`（基础接口）
- **修改复用**：`jadx_stream.go` → `ida_stream.go`（只需改会话类型）
- **参考实现**：`jadx_handlers.go` → `ida_handlers.go`（逻辑相同，改函数名）
- **新建实现**：`ida.go`（连接逻辑需要适配标准 MCP）

## 十一、测试清单

- [ ] IDA MCP 服务器可以独立启动（SSE 模式）
- [ ] 后端可以连接到 IDA MCP 服务器
- [ ] 可以获取工具列表
- [ ] 可以调用基础工具（如 `check_connection`、`get_metadata`）
- [ ] 可以调用分析工具（如 `decompile_function`、`get_function_by_name`）
- [ ] 流式聊天功能正常
- [ ] 前端界面可以正常显示和交互
- [ ] 错误处理友好（连接失败、工具执行失败等）

