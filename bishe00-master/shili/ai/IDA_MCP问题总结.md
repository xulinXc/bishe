# IDA MCP 连接问题总结

## 核心问题

**IDA MCP 服务器在 SSE Transport 模式下只提供 `/sse` 端点（仅支持 GET），没有提供可用于发送 JSON-RPC 请求的 POST 端点。**

## 当前状态

### ✅ 服务器启动正常
- 服务器地址：`http://127.0.0.1:8744`
- 启动命令：`python server.py --transport http://127.0.0.1:8744/sse`
- 服务器日志：`MCP Server availabile at http://127.0.0.1:8744/sse`

### ✅ GET /sse 端点可用
- `GET http://127.0.0.1:8744/sse` → 200 OK
- 可以建立 SSE 连接（Server-Sent Events）

### ❌ 所有 POST 端点均不可用
- `POST /mcp` → 404 Not Found
- `POST /sse` → 405 Method Not Allowed（只允许 GET）
- `POST /messages` → 404 Not Found
- `POST /sse/messages` → 404 Not Found
- `POST /mcp/messages` → 404 Not Found
- 其他尝试的端点 → 均返回 404

## 问题原因

### 标准 MCP SSE Transport 规范
根据 MCP 规范，SSE Transport 模式应该提供：
1. **GET /sse** - 建立 SSE 连接（接收服务器推送）
2. **POST /messages** - 发送 JSON-RPC 请求

### IDA MCP 的实际实现
IDA MCP 的实现**不符合标准规范**：
- ✅ 提供了 `GET /sse` 端点
- ❌ **没有提供** `POST /messages` 或任何 POST 端点

## 可能的解决方案

### 方案 1：通过 GET /sse 查询参数发送请求
IDA MCP 可能支持通过查询参数发送 JSON-RPC 请求，例如：
```
GET /sse?request={"jsonrpc":"2.0","id":1,"method":"initialize",...}
```

### 方案 2：通过 SSE 连接发送请求
建立 SSE 连接后，可能需要在连接建立时通过特定方式发送请求，或者服务器在连接建立后主动发送某些信息。

### 方案 3：查看 IDA MCP 实际源代码
需要查看 `ida-pro-mcp` 的 GitHub 仓库源代码，了解：
- `/sse` 端点的实际实现
- 如何通过 `/sse` 端点发送请求
- 是否有其他隐藏的端点或协议

### 方案 4：使用不同的启动模式
可能 `--transport` 参数不是用于独立服务器，而是用于连接到 IDA Pro 插件。
需要检查是否有其他启动模式，或是否必须先启动 IDA Pro 插件。

## 下一步行动

1. **查看 IDA MCP 源代码**
   - 检查 `server.py` 的实现
   - 查看 `/sse` 端点的路由和处理逻辑
   - 了解实际的通信协议

2. **测试不同的请求方式**
   - 尝试通过 GET 请求的查询参数发送 JSON-RPC
   - 尝试建立 SSE 连接后，通过连接发送请求
   - 查看服务器启动后的实际行为

3. **参考 JADX MCP 的实现**
   - JADX MCP 成功使用了 `/mcp` POST 端点
   - 对比两种实现的差异
   - 寻找可能的通用解决方案

## 相关日志

```
2025/11/03 17:45:10 [IDA MCP] 尝试连接: http://127.0.0.1:8744/mcp
2025/11/03 17:45:10 [IDA MCP] 请求方法: initialize, 请求ID: 1
[32mINFO[0m:     127.0.0.1:64844 - "[1mPOST /mcp HTTP/1.1[0m" [31m404 Not Found[0m
2025/11/03 17:45:10 [IDA MCP] /mcp 端点不存在 (404)，尝试使用 SSE Transport 模式
[32mINFO[0m:     127.0.0.1:15104 - "[1mPOST /sse/messages HTTP/1.1[0m" [31m404 Not Found[0m
```

