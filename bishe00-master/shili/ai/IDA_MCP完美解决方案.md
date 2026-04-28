# IDA MCP 完美解决方案

## 核心发现

通过查看 IDA MCP 源代码 (`server.py`)，发现了关键架构：

### IDA MCP 架构

1. **IDA MCP Server (`server.py`)**
   - 使用 FastMCP 库，启动 SSE Transport 模式
   - 提供 `/sse` 端点（GET 请求，用于 SSE 连接）
   - **不提供** `/mcp` 或 `/messages` POST 端点

2. **IDA 插件 (`mcp-plugin.py`)**
   - 运行在 `http://127.0.0.1:13337/mcp`
   - 提供标准的 JSON-RPC POST 端点
   - `server.py` 通过 `make_jsonrpc_request` 函数将请求转发到这里

3. **工作流程**
   ```
   客户端 → FastMCP SSE Transport (/sse) → server.py → IDA 插件 (/mcp)
   ```

## 解决方案

### 方案 1：直接连接 IDA 插件（已实现 ✅）

**优点**：
- 简单直接，无需实现 SSE Transport 客户端
- 可以绕过 FastMCP，直接使用标准 JSON-RPC
- 与 JADX MCP 的实现方式一致

**实现**：
当 `server.py` 的 `/mcp` 端点返回 404 时，直接连接到 IDA 插件：
```go
idaPluginURL := "http://127.0.0.1:13337/mcp"
POST http://127.0.0.1:13337/mcp
```

**要求**：
- IDA Pro 必须运行
- IDA 插件必须已启动（`Edit -> Plugins -> MCP`）

### 方案 2：实现完整 SSE Transport 客户端（可选）

如果需要使用 FastMCP 的完整功能，可以实现 SSE Transport 客户端：

1. **建立 SSE 连接**：`GET http://127.0.0.1:8744/sse`
2. **读取 SSE 响应**：从 SSE 流中解析 `data: {...json...}` 格式的响应
3. **发送请求**：通过 FastMCP 的机制发送请求（可能需要通过 SSE 连接发送）

## 当前实现

已实现**方案 1**，作为主要解决方案：

```go
// sendMCPRequestViaSSE
// 1. 先尝试标准的 POST /messages 端点
// 2. 如果失败，直接连接到 IDA 插件的 /mcp 端点
idaPluginURL := "http://127.0.0.1:13337/mcp"
```

## 使用说明

### 前提条件

1. **启动 IDA Pro** 并加载一个二进制文件
2. **启动 IDA MCP 插件**：在 IDA 中点击 `Edit -> Plugins -> MCP`（或按 `Ctrl+Alt+M`）
3. **验证插件运行**：检查 `http://127.0.0.1:13337/mcp` 是否可访问

### 配置

在前端配置页面设置：
- **MCP 服务器地址**：`http://127.0.0.1:8744`（可选，如果使用 server.py）
- **或者直接使用**：`http://127.0.0.1:13337`（直接连接 IDA 插件）

### 工作流程

1. 用户在前端配置页面点击"保存配置并连接"
2. 后端尝试连接 `http://127.0.0.1:8744/mcp`
3. 如果返回 404，自动尝试 `http://127.0.0.1:13337/mcp`
4. 连接成功后，可以进行 MCP 工具调用

## 优势

✅ **简单可靠**：直接使用标准 JSON-RPC，无需实现 SSE Transport  
✅ **与 JADX MCP 一致**：相同的实现方式，代码复用性好  
✅ **性能更好**：绕过 FastMCP 代理层，直接通信  
✅ **易于调试**：标准的 HTTP POST 请求，易于测试和调试

## 注意事项

⚠️ **需要 IDA Pro 运行**：IDA 插件必须在运行状态  
⚠️ **端口固定**：IDA 插件默认使用 13337 端口  
⚠️ **依赖 IDA 插件**：必须先启动 IDA MCP 插件

## 测试

1. 启动 IDA Pro 并加载二进制文件
2. 在 IDA 中启动 MCP 插件
3. 在前端配置页面设置 MCP 服务器地址为 `http://127.0.0.1:13337`
4. 点击"保存配置并连接"
5. 验证连接成功，可以调用工具（如 `check_connection`）

## 相关文件

- `internal/mcp/ida.go`：IDA MCP 连接实现
- `internal/mcp/ida_handlers.go`：HTTP 路由处理器
- `internal/mcp/ida_stream.go`：流式聊天处理器
- `web/ida_mcp.html`：前端界面

