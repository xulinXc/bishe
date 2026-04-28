# JADX MCP 连接方案完整文档

> **文档目的**：记录完整的 JADX MCP 连接方案、代码实现和配置方法，确保可以根据此文档重新完成连接。

## 📋 目录

1. [架构概述](#架构概述)
2. [系统组件说明](#系统组件说明)
3. [连接方案详解](#连接方案详解)
4. [代码实现细节](#代码实现细节)
5. [配置步骤](#配置步骤)
6. [启动流程](#启动流程)
7. [关键代码片段](#关键代码片段)
8. [故障排查指南](#故障排查指南)
9. [端口说明](#端口说明)

---

## 架构概述

### 系统架构图

```
┌─────────────────┐         ┌──────────────────────┐         ┌──────────────┐
│   Web 前端       │         │   Go 后端服务器        │         │  JADX MCP    │
│  (jadx_mcp.html) │ ─────► │   (main.go)          │ ─────► │  Server      │
│                 │  HTTP  │   Port: 8080          │  HTTP  │  Port: 8651/ │
│                 │         │                      │  JSON  │  9999        │
└─────────────────┘         └──────────────────────┘         └──────────────┘
                                      │                                  │
                                      │                                  │
                                      │                                  ▼
                                      │                         ┌──────────────┐
                                      │                         │  JADX Plugin │
                                      │                         │  Port: 8650  │
                                      │                         └──────────────┘
                                      │                                  │
                                      └──────────────────────────────────┘
                                             (通过 MCP 协议调用工具)
```

### 数据流

1. **用户在前端配置 MCP 服务器地址和 API Key**
2. **前端发送配置请求到 Go 后端** (`POST /mcp/jadx/configure`)
3. **Go 后端连接 JADX MCP 服务器** (通过 HTTP + JSON-RPC)
4. **MCP 服务器通过端口 8650 与 JADX Plugin 通信**
5. **返回结果给前端显示**

---

## 系统组件说明

### 1. JADX MCP Server (Python FastMCP)

- **位置**：`G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server`
- **类型**：Python FastMCP 服务器
- **协议**：MCP (Model Context Protocol) over HTTP
- **传输方式**：`streamable-http` (支持 Server-Sent Events)
- **端口**：默认 8651，可自定义（如 9999）
- **端点**：`/mcp` (固定端点)

**关键特性**：
- 使用 FastMCP 框架
- 支持 SSE (Server-Sent Events) 响应格式
- Session ID 通过 HTTP 响应头或 Cookie 传递
- `initialize` 请求不需要 session ID（服务器生成）

### 2. Go 后端服务器

- **位置**：项目根目录 `main.go`
- **框架**：标准库 `net/http`
- **端口**：8080
- **路由**：
  - `POST /mcp/jadx/configure` - 配置会话并连接 MCP
  - `POST /mcp/jadx/connect` - 仅连接 MCP
  - `POST /mcp/jadx/chat` - 发送聊天消息
  - `GET /mcp/jadx/messages?sessionId=xxx` - 获取消息历史
  - `GET /mcp/jadx/status?sessionId=xxx` - 获取会话状态

### 3. Web 前端

- **位置**：`web/jadx_mcp.html`
- **功能**：
  - MCP 服务器地址配置
  - API Key 配置（支持 OpenAI/Anthropic/Ollama）
  - 聊天界面
  - 消息历史显示

---

## 连接方案详解

### FastMCP streamable-http 协议

JADX MCP Server 使用 FastMCP 的 `streamable-http` 传输方式，具有以下特点：

1. **Session ID 管理**：
   - `initialize` 请求：**不发送** session ID，服务器生成并返回
   - 后续请求：**必须发送**从服务器获取的 session ID
   - Session ID 通过 HTTP 响应头 `Mcp-Session-Id` 返回
   - 也可以通过 Cookie 传递

2. **响应格式**：
   - 使用 SSE (Server-Sent Events) 格式
   - 格式：`event: message\ndata: {...JSON...}\n\n`
   - 需要从 `data:` 行提取 JSON

3. **HTTP 请求头**：
   - `Content-Type: application/json`
   - `Accept: application/json, text/event-stream`（必需，支持 SSE）
   - `Mcp-Session-Id: <session-id>`（非 initialize 请求）

4. **端点**：
   - 固定端点：`/mcp`
   - 完整 URL：`http://127.0.0.1:8651/mcp` 或 `http://127.0.0.1:9999/mcp`

### 连接流程

```
1. 客户端发送 initialize 请求（无 session ID）
   ↓
2. 服务器生成 session ID，返回在响应头 Mcp-Session-Id
   ↓
3. 客户端提取 session ID，保存
   ↓
4. 客户端发送 tools/list 请求（带 session ID）
   ↓
5. 服务器验证 session ID，返回工具列表
   ↓
6. 连接建立成功
```

### JSON-RPC 请求格式

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "NeonScan JADX Client",
      "version": "1.0.0"
    }
  }
}
```

### SSE 响应格式

```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{...}}
```

需要提取 `data:` 后的 JSON 部分。

---

## 代码实现细节

### 1. 连接结构体 (`internal/mcp/jadx.go`)

```go
type JADXMCPConnection struct {
    mu            sync.RWMutex
    baseURL       string          // MCP 服务器基础 URL
    client        *http.Client    // HTTP 客户端（带 Cookie 支持）
    connected     bool            // 连接状态
    tools         []MCPTool       // MCP 工具列表
    lastRequestID int64           // 最后一个请求 ID
    sessionID     string          // FastMCP session ID
}
```

**关键点**：
- 使用 `cookiejar` 支持 Cookie（FastMCP 可能通过 Cookie 传递 session ID）
- `sessionID` 初始为空，等待服务器生成
- 使用读写锁保护并发访问

### 2. 连接初始化 (`ConnectJADXMCP`)

```go
func ConnectJADXMCP(baseURL string) (*JADXMCPConnection, error) {
    // 1. 设置默认 URL（如果为空）
    if baseURL == "" {
        baseURL = "http://127.0.0.1:8651" // FastMCP 默认端口
    }
    
    // 2. 创建支持 Cookie 的 HTTP 客户端
    jar, _ := cookiejar.New(nil)
    conn := &JADXMCPConnection{
        baseURL:       baseURL,
        client:        &http.Client{Timeout: 30 * time.Second, Jar: jar},
        sessionID:     "", // 初始为空
    }
    
    // 3. 初始化连接并获取工具列表
    if err := conn.initialize(); err != nil {
        return nil, fmt.Errorf("初始化 MCP 连接失败: %v", err)
    }
    
    return conn, nil
}
```

### 3. 初始化流程 (`initialize`)

```go
func (conn *JADXMCPConnection) initialize() error {
    // 1. 发送 initialize 请求（不包含 session ID）
    initParams := map[string]interface{}{
        "protocolVersion": "2024-11-05",
        "capabilities":    map[string]interface{}{},
        "clientInfo": map[string]interface{}{
            "name":    "NeonScan JADX Client",
            "version": "1.0.0",
        },
    }
    
    initResp, err := conn.SendMCPRequest("initialize", initParams)
    if err != nil {
        log.Printf("[JADX MCP] 初始化请求失败（继续尝试）: %v", err)
    } else {
        // 尝试从响应中提取 session ID（如果服务器在 JSON 中返回）
        if result, ok := initResp["result"].(map[string]interface{}); ok {
            if serverSessionID, ok := result["sessionId"].(string); ok {
                conn.sessionID = serverSessionID
            }
        }
    }
    
    // 2. 发送 tools/list 请求验证连接
    tools, err := conn.listTools()
    if err != nil {
        return fmt.Errorf("检查连接失败: %v", err)
    }
    
    conn.tools = tools
    conn.connected = true
    return nil
}
```

### 4. 发送 MCP 请求 (`SendMCPRequest`)

这是核心函数，处理所有的 MCP 请求：

```go
func (conn *JADXMCPConnection) SendMCPRequest(method string, params map[string]interface{}) (map[string]interface{}, error) {
    // 1. 构建 JSON-RPC 请求
    request := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      requestID,
        "method":  method,
    }
    if len(params) > 0 {
        request["params"] = params
    }
    
    // 2. 创建 HTTP 请求
    fullURL := baseURL + "/mcp"
    req, _ := http.NewRequest("POST", fullURL, bytes.NewBuffer(reqData))
    
    // 3. 设置请求头
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json, text/event-stream") // 关键：支持 SSE
    
    // 4. Session ID 处理（关键逻辑）
    if method != "initialize" && sessionID != "" {
        // 非 initialize 请求：发送 session ID
        req.Header.Set("Mcp-Session-Id", sessionID)
        req.Header.Set("X-Session-ID", sessionID)
        req.Header.Set("Session-ID", sessionID)
        // ... 其他可能的 header 名称
    } else {
        // initialize 请求：不发送 session ID
    }
    
    // 5. 发送请求
    resp, err := client.Do(req)
    
    // 6. 从响应头提取 session ID
    if headerName == "Mcp-Session-Id" {
        conn.sessionID = headerValues[0]
    }
    
    // 7. 从 Cookie 提取 session ID（备用）
    for _, cookie := range resp.Cookies() {
        if cookie.Name == "session" || cookie.Name == "mcp_session" {
            conn.sessionID = cookie.Value
        }
    }
    
    // 8. 解析 SSE 响应
    // 提取 data: 后的 JSON
    
    // 9. 返回解析后的 JSON
}
```

**关键实现细节**：

1. **Session ID 提取优先级**：
   - 响应头 `Mcp-Session-Id`（优先级最高）
   - 其他包含 "session" 的响应头
   - Cookie 中的 session 相关字段

2. **SSE 响应解析**：
   ```go
   if strings.Contains(bodyStr, "data:") {
       lines := strings.Split(bodyStr, "\n")
       for _, line := range lines {
           if strings.HasPrefix(line, "data:") {
               jsonData = strings.TrimPrefix(line, "data:")
               jsonData = strings.TrimSpace(jsonData)
               break
           }
       }
   }
   ```

3. **错误处理**：
   - 400 Bad Request：请求格式错误（通常是缺少 session ID）
   - 404 Not Found：端点不存在
   - 406 Not Acceptable：Accept 头不正确

### 5. HTTP 处理器 (`internal/mcp/jadx_handlers.go`)

所有错误响应必须返回 JSON 格式（前端期望）：

```go
func ConfigureJADXSessionHandler(w http.ResponseWriter, r *http.Request) {
    // ... 解析请求
    
    // 连接 MCP 服务器
    conn, err := ConnectJADXMCP(req.BaseURL)
    if err != nil {
        // 关键：返回 JSON 格式的错误
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "success": false,
            "message": fmt.Sprintf("连接 MCP 失败: %v", err),
        })
        return
    }
    
    // 成功响应
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "sessionId": session.ID,
    })
}
```

**注意**：不要使用 `http.Error()`，因为它返回纯文本，会导致前端 JSON 解析失败。

---

## 配置步骤

### 步骤 1：启动 JADX MCP 服务器

```powershell
# 切换到 MCP 服务器目录
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"

# 检查依赖（如果需要）
pip install fastmcp

# 启动 HTTP 模式（默认端口 8651）
python jadx_mcp_server.py --http

# 或指定端口（如 9999）
python jadx_mcp_server.py --http --port 9999
```

**验证服务器启动**：
```powershell
# 检查端口是否监听
netstat -ano | findstr :8651
# 或
netstat -ano | findstr :9999
```

### 步骤 2：启动 Go 后端服务器

```powershell
# 在项目根目录
cd "C:\Users\86483\Desktop\桌面\bishe"

# 运行服务器
go run .
```

服务器会在 `http://localhost:8080` 启动。

### 步骤 3：在前端配置

1. 打开 `http://localhost:8080/jadx_mcp.html`
2. 填写配置：
   - **MCP 服务器地址**：`http://127.0.0.1:8651`（或您指定的端口）
   - **API Key**：您的 OpenAI/Anthropic API Key
   - **API 类型**：选择对应的类型
3. 点击 "保存配置并连接"

---

## 启动流程

### 完整启动流程

```bash
# 终端 1：启动 JADX MCP 服务器
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"
python jadx_mcp_server.py --http --port 9999

# 终端 2：启动 Go 后端
cd "C:\Users\86483\Desktop\桌面\bishe"
go run .
```

### 启动顺序

1. ✅ **先启动 JADX MCP 服务器**（必须在 Go 后端之前）
2. ✅ **再启动 Go 后端服务器**
3. ✅ **打开前端页面进行配置**

### 启动检查清单

- [ ] JADX MCP 服务器已启动（端口 8651/9999 监听中）
- [ ] Go 后端服务器已启动（端口 8080 监听中）
- [ ] JADX GUI 已打开（如果需要在 JADX 中查看）
- [ ] 前端页面可以访问

---

## 关键代码片段

### 1. Session ID 管理

```go
// 从响应头提取 session ID（优先级最高）
sessionIDUpdated := false
for headerName, headerValues := range resp.Header {
    if headerName == "Mcp-Session-Id" || headerName == "MCP-Session-Id" {
        if len(headerValues) > 0 && headerValues[0] != "" {
            conn.mu.Lock()
            conn.sessionID = headerValues[0]
            conn.mu.Unlock()
            sessionIDUpdated = true
        }
    }
}

// 如果没有找到，尝试 Cookie
if !sessionIDUpdated {
    for _, cookie := range resp.Cookies() {
        if cookie.Name == "session" || cookie.Name == "mcp_session" {
            conn.mu.Lock()
            conn.sessionID = cookie.Value
            conn.mu.Unlock()
        }
    }
}
```

### 2. SSE 响应解析

```go
// FastMCP streamable-http 使用 SSE 格式
jsonData := bodyStr

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

// 解析 JSON
var response map[string]interface{}
json.Unmarshal([]byte(jsonData), &response)
```

### 3. 请求头设置

```go
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Accept", "application/json, text/event-stream") // 关键

// 非 initialize 请求添加 session ID
if method != "initialize" && sessionID != "" {
    req.Header.Set("Mcp-Session-Id", sessionID)
    req.Header.Set("X-Session-ID", sessionID)
    req.Header.Set("Session-ID", sessionID)
    // ... 其他可能的 header
}
```

### 4. 错误响应格式

```go
// 正确：返回 JSON
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusInternalServerError)
json.NewEncoder(w).Encode(map[string]interface{}{
    "success": false,
    "message": fmt.Sprintf("连接 MCP 失败: %v", err),
})

// 错误：不要使用 http.Error()（返回纯文本）
// http.Error(w, fmt.Sprintf("连接 MCP 失败: %v", err), http.StatusInternalServerError)
```

---

## 故障排查指南

### 问题 1：连接失败 - 无法连接到 MCP 服务器

**错误信息**：
```
连接 MCP 失败: 初始化 MCP 连接失败: 检查连接失败: 无法连接到 MCP 服务器
```

**原因**：
- MCP 服务器未启动
- 端口号不匹配
- 防火墙阻止连接

**解决方法**：
1. 检查 MCP 服务器是否运行：
   ```powershell
   netstat -ano | findstr :8651
   # 或
   netstat -ano | findstr :9999
   ```
2. 确认前端配置的端口与服务器启动的端口一致
3. 重启 MCP 服务器：
   ```powershell
   python jadx_mcp_server.py --http --port 9999
   ```

### 问题 2：406 Not Acceptable

**错误信息**：
```
请求格式不被接受 (406): 请检查 Accept 头设置
```

**原因**：`Accept` 请求头不正确

**解决方法**：确保代码中设置了：
```go
req.Header.Set("Accept", "application/json, text/event-stream")
```

### 问题 3：400 Bad Request - Missing session ID

**错误信息**：
```
Bad Request: Missing session ID
或
Bad Request: No valid session ID provided
```

**原因**：
- `initialize` 请求不应该发送 session ID
- 后续请求必须发送 session ID

**解决方法**：
1. 确保 `initialize` 请求不发送 session ID：
   ```go
   if method != "initialize" && sessionID != "" {
       // 只在这里发送 session ID
   }
   ```
2. 确保从响应头正确提取 session ID
3. 检查日志，确认 session ID 已从服务器获取

### 问题 4：JSON 解析错误 - Unexpected token

**错误信息**：
```
配置失败: Unexpected token '连', "连接 MCP 失败:"... is not valid JSON
```

**原因**：后端返回了纯文本错误，而不是 JSON

**解决方法**：确保所有错误响应都返回 JSON 格式：
```go
// 正确
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
    "success": false,
    "message": "错误消息",
})

// 错误
http.Error(w, "错误消息", http.StatusInternalServerError)
```

### 问题 5：端点不存在 (404)

**错误信息**：
```
端点不存在 (404, URL: http://127.0.0.1:9999/)
```

**原因**：使用了错误的端点路径

**解决方法**：确保使用 `/mcp` 端点：
```go
fullURL := baseURL + "/mcp"  // 正确
// 不是 baseURL + "/"        // 错误
```

### 问题 6：Session ID 未正确提取

**调试方法**：
1. 查看后端日志，确认响应头内容：
   ```
   [JADX MCP] 响应头: map[Mcp-Session-Id:[xxx] ...]
   ```
2. 确认从响应头提取了 session ID：
   ```
   [JADX MCP] 更新 Session ID:  -> xxx (从响应头 Mcp-Session-Id)
   ```
3. 确认后续请求发送了 session ID：
   ```
   [JADX MCP] 发送 Session ID: xxx (方法: tools/list)
   ```

### 问题 7：SSE 响应解析失败

**错误信息**：
```
解析响应失败: invalid character 'e' looking for beginning of value
```

**原因**：响应是 SSE 格式，但没有正确提取 `data:` 后的 JSON

**解决方法**：确保代码正确处理 SSE 格式：
```go
if strings.Contains(bodyStr, "data:") {
    lines := strings.Split(bodyStr, "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "data:") {
            jsonData = strings.TrimPrefix(line, "data:")
            jsonData = strings.TrimSpace(jsonData)
            break
        }
    }
}
```

---

## 端口说明

### 端口 8650 - JADX AI MCP Plugin

- **用途**：JADX 插件端口
- **访问者**：MCP 服务器（内部使用）
- **默认值**：8650
- **说明**：Web 应用**不需要**配置此端口

### 端口 8651 - FastMCP HTTP 服务器（默认）

- **用途**：MCP 服务器对外 HTTP 接口
- **访问者**：Go 后端
- **默认值**：8651
- **说明**：如果使用默认端口启动，前端配置 `http://127.0.0.1:8651`

### 端口 9999 - FastMCP HTTP 服务器（自定义）

- **用途**：MCP 服务器对外 HTTP 接口
- **访问者**：Go 后端
- **自定义**：通过 `--port 9999` 指定
- **说明**：如果使用自定义端口，前端配置 `http://127.0.0.1:9999`

### 端口 8080 - Go 后端服务器

- **用途**：Web 应用后端 API
- **访问者**：Web 前端
- **默认值**：8080

---

## 文件位置索引

### 核心代码文件

1. **`internal/mcp/jadx.go`**
   - `JADXMCPConnection` 结构体
   - `ConnectJADXMCP` 函数
   - `SendMCPRequest` 函数（核心）
   - `initialize` 函数
   - `listTools` 函数
   - `CallTool` 函数

2. **`internal/mcp/jadx_handlers.go`**
   - `ConnectJADXHandler` - 连接处理器
   - `ConfigureJADXSessionHandler` - 配置处理器
   - `JADXChatHandler` - 聊天处理器
   - `GetJADXMessagesHandler` - 消息获取处理器
   - `GetJADXSessionStatusHandler` - 状态获取处理器

3. **`web/jadx_mcp.html`**
   - 前端配置界面
   - 聊天界面
   - JavaScript 客户端代码

4. **`main.go`**
   - HTTP 路由注册
   - 服务器启动

### 配置文件

1. **`启动JADX_MCP服务器.md`** - 服务器启动指南
2. **`端口说明.md`** - 端口说明文档
3. **`MCP_CONFIG_README.md`** - MCP 配置说明（Cursor stdio 模式）

---

## 关键配置参数

### Go 后端默认配置

```go
// 默认 MCP 服务器地址
baseURL := "http://127.0.0.1:8651"

// HTTP 客户端超时
timeout := 30 * time.Second

// 支持 Cookie
jar, _ := cookiejar.New(nil)
```

### MCP 请求头

```go
Content-Type: application/json
Accept: application/json, text/event-stream
Mcp-Session-Id: <session-id> (非 initialize 请求)
```

### JSON-RPC 请求格式

```json
{
  "jsonrpc": "2.0",
  "id": <递增ID>,
  "method": "<method>",
  "params": { ... }
}
```

---

## 测试连接

### 使用 curl 测试

```powershell
# 测试 initialize 请求
$body = @{
    jsonrpc = "2.0"
    id = 1
    method = "initialize"
    params = @{
        protocolVersion = "2024-11-05"
        capabilities = @{}
        clientInfo = @{
            name = "Test Client"
            version = "1.0.0"
        }
    }
} | ConvertTo-Json -Depth 10

Invoke-WebRequest -Uri "http://127.0.0.1:9999/mcp" `
  -Method POST `
  -ContentType "application/json" `
  -Headers @{"Accept"="application/json, text/event-stream"} `
  -Body $body
```

### 使用 Go 代码测试

```go
conn, err := ConnectJADXMCP("http://127.0.0.1:9999")
if err != nil {
    log.Fatal(err)
}
log.Printf("连接成功，工具数量: %d", len(conn.GetTools()))
```

---

## 总结

### 连接成功的关键要素

1. ✅ **MCP 服务器已启动**（端口 8651 或 9999）
2. ✅ **正确的 Accept 头**：`application/json, text/event-stream`
3. ✅ **initialize 请求不发送 session ID**
4. ✅ **从响应头正确提取 session ID**
5. ✅ **后续请求发送 session ID**
6. ✅ **正确解析 SSE 响应格式**
7. ✅ **所有错误响应返回 JSON 格式**
8. ✅ **使用固定的 `/mcp` 端点**

### 快速检查清单

- [ ] MCP 服务器运行中（`netstat` 确认端口）
- [ ] Go 后端运行中（`localhost:8080` 可访问）
- [ ] 前端配置的端口与 MCP 服务器端口一致
- [ ] 后端日志显示 session ID 已提取
- [ ] 前端可以成功配置并连接

---

**文档版本**：1.0  
**最后更新**：2025-11-02  
**维护者**：根据实际连接方案整理

