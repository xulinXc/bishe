# JADX MCP Server 配置说明

## 配置 MCP.json

要在 Cursor（或其他支持 MCP 的编辑器）中使用 JADX MCP Server，需要在 `MCP.json` 配置文件中添加以下配置。

### 配置文件位置

MCP.json 通常位于：
- **Windows**: `%APPDATA%\Cursor\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json` 或类似位置
- **macOS**: `~/Library/Application Support/Cursor/User/globalStorage/.../cline_mcp_settings.json`
- **Linux**: `~/.config/Cursor/User/globalStorage/.../cline_mcp_settings.json`

### 配置步骤

1. 打开或创建 `MCP.json` 文件
2. 在文件中找到 `mcpServers` 部分（如果不存在则创建一个）
3. 添加以下配置：

```json
{
  "mcpServers": {
    "jadx-mcp-server": {
      "command": "uv",
      "args": [
        "--directory",
        "/PATH/TO/jadx-mcp-server/",
        "run",
        "jadx_mcp_server.py"
      ]
    }
  }
}
```

### 重要说明

1. **路径替换**: 将 `/PATH/TO/jadx-mcp-server/` 替换为您实际的 jadx-mcp-server 文件夹的**绝对路径**
   - 示例（Windows）: `C:\\Users\\YourName\\jadx-mcp-server\\`
   - 示例（macOS/Linux）: `/Users/YourName/jadx-mcp-server/`

2. **路径要求**:
   - ✅ 必须使用绝对路径
   - ✅ 路径中**不能包含中文字符**
   - ✅ Windows 路径中的反斜杠需要转义（使用 `\\`）或使用正斜杠（`/`）

3. **依赖要求**:
   - 确保已安装 `uv`（Python 包管理器）
   - 确保 jadx-mcp-server 文件夹中存在 `jadx_mcp_server.py` 文件

### 示例配置（完整）

```json
{
  "mcpServers": {
    "jadx-mcp-server": {
      "command": "uv",
      "args": [
        "--directory",
        "C:\\Users\\YourUsername\\jadx-mcp-server",
        "run",
        "jadx_mcp_server.py"
      ]
    }
  }
}
```

### 验证配置

保存配置文件后，重启 Cursor 编辑器，MCP 服务器应该会自动启动。您可以在编辑器的 MCP 面板中查看连接状态。

### 故障排查

如果配置不生效：

1. 检查路径是否正确（使用绝对路径）
2. 检查路径中是否包含中文字符（必须移除）
3. 确认 `uv` 命令是否在系统 PATH 中
4. 确认 `jadx_mcp_server.py` 文件存在于指定目录
5. 查看编辑器日志或终端输出中的错误信息

## 重要说明：HTTP 模式 vs stdio 模式

### 问题说明

JADX MCP 服务器可以通过两种模式运行：

1. **stdio 模式**（默认）：通过 `mcp.json` 配置，由 Cursor 直接管理，通过标准输入/输出通信
   - 这种模式下，MCP 服务器不能通过 HTTP 直接访问
   - 只能通过 Cursor 的 MCP 接口访问

2. **HTTP 模式**：作为独立的 HTTP 服务器运行，监听特定端口
   - 可以通过 HTTP 接口访问（如 `http://127.0.0.1:9999`）
   - 适用于 Web 应用集成

### 当前架构限制

**重要**：本项目的 Web 界面需要通过 **HTTP 模式** 连接到 MCP 服务器，而不是 stdio 模式。

### 解决方案

#### 方案 1：启动独立的 HTTP 模式 MCP 服务器

如果 JADX MCP Server 支持 HTTP 模式，需要：

1. 手动启动 MCP 服务器为 HTTP 模式
2. 确认服务器监听地址和端口（如 `http://127.0.0.1:9999`）
3. 在 Web 界面中配置该 HTTP 地址

#### 方案 2：检查 JADX MCP Server 是否支持 HTTP 模式

请查看 JADX MCP Server 的文档，确认是否支持 HTTP 模式。如果不支持，可能需要：
- 修改 MCP 服务器代码以支持 HTTP 模式
- 或使用其他支持 HTTP 模式的 MCP 服务器实现

### 连接错误排查

如果出现 "无法连接到 MCP 服务器" 错误：

1. **确认 MCP 服务器是否在运行**
   - 检查是否有进程监听指定端口
   - Windows: `netstat -ano | findstr :9999`
   - Linux/macOS: `lsof -i :9999`

2. **检查服务器地址和端口**
   - 确认在 Web 界面中配置的地址正确
   - 默认地址通常是 `http://127.0.0.1:9999`

3. **查看后端日志**
   - 检查 Go 后端日志中的详细错误信息
   - 查看尝试连接的 URL 列表

4. **确认防火墙设置**
   - 确保本地端口没有被防火墙阻止

5. **测试连接**
   - 可以使用 `curl` 或 Postman 测试 MCP 服务器的 HTTP 端点：
     ```bash
     curl -X POST http://127.0.0.1:9999/mcp \
       -H "Content-Type: application/json" \
       -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
     ```

