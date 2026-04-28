# IDA MCP 使用说明

## 📋 两种运行模式的区别

### 模式 1：stdio 模式（Cursor 内置，当前您使用的）

**当前状态**：
- ✅ IDA MCP 已在 Cursor 中通过 `mcp.json` 配置启用
- ✅ Cursor 自动启动 Python 脚本：`C:\Users\86483\AppData\Roaming\Python\Python313\site-packages\ida_pro_mcp\server.py`
- ✅ 通过标准输入输出（stdio）与 Cursor 通信

**限制**：
- ❌ **不能**直接通过 HTTP 访问（我们的 Web 代码需要 HTTP 访问）
- ❌ 只能被 Cursor 的内部 MCP 客户端使用

### 模式 2：HTTP/SSE 模式（我们的 Web 代码需要）

**需要的状态**：
- ✅ IDA MCP 运行在独立的 HTTP 服务器上
- ✅ 可以通过 `http://127.0.0.1:8744/mcp` 访问
- ✅ 支持 JSON-RPC over HTTP 协议

---

## 🚀 解决方案：启动 HTTP 模式的 IDA MCP 服务器

### 方法 1：使用 IDA Pro 插件（推荐，如果插件支持）

如果 IDA Pro 插件支持 HTTP 模式，在 IDA Pro 中：
1. 打开插件菜单
2. 找到 IDA Pro MCP 插件
3. 选择"启动 HTTP 服务器"或类似选项
4. 设置端口为 `8744`

### 方法 2：手动启动独立的 HTTP 服务器

如果 IDA Pro MCP 支持命令行启动 HTTP 模式，在命令行运行：

```bash
# 方式 A：使用 uv（如果已安装）
uv run ida-pro-mcp --transport http://127.0.0.1:8744/sse

# 方式 B：直接使用 Python（可能需要先进入正确的目录）
cd "C:\Users\86483\AppData\Roaming\Python\Python313\site-packages"
python -m ida_pro_mcp.server --transport http://127.0.0.1:8744/sse

# 方式 C：查看 ida-pro-mcp 的帮助信息
ida-pro-mcp --help
# 或
python -m ida_pro_mcp --help
```

### 方法 3：检查 IDA Pro MCP 是否已经在运行 HTTP 模式

尝试访问：
```
http://127.0.0.1:8744/mcp
```

如果可以访问，说明已经运行了 HTTP 模式，您**不需要**再做任何操作！

---

## 🔍 如何确认是否已启动 HTTP 模式

### 步骤 1：检查端口是否被占用

在 PowerShell 中运行：
```powershell
netstat -ano | findstr "8744"
```

如果看到 `LISTENING`，说明有程序在监听 8744 端口。

### 步骤 2：测试 HTTP 连接

在浏览器访问：
```
http://127.0.0.1:8744/mcp
```

或者使用 PowerShell：
```powershell
$body = '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
$response = Invoke-WebRequest -Uri "http://127.0.0.1:8744/mcp" -Method POST -Body $body -ContentType "application/json"
$response.Content
```

如果返回 JSON 响应（不一定是工具列表，可能是错误），说明 HTTP 服务器在运行。

---

## 💡 推荐做法

**如果您已经在 IDA Pro 中开启了 MCP**：

1. **首先检查** IDA Pro 插件是否有"HTTP 服务器"选项
   - 查看插件菜单
   - 查看插件设置/配置
   - 查看插件文档

2. **如果没有找到 HTTP 选项**，查看 IDA Pro MCP 的 GitHub 文档：
   - https://github.com/mrexodia/ida-pro-mcp
   - 查找 "HTTP"、"SSE"、"transport" 相关文档

3. **如果插件不支持 HTTP 模式**：
   - 可以考虑修改我们的代码来支持 stdio 模式（需要较大改动）
   - 或者等待插件更新支持 HTTP 模式

---

## 📝 当前状态说明

根据您的 `mcp.json` 配置：
- ✅ IDA MCP 已安装并配置在 Cursor 中
- ❓ **不确定**是否同时运行了 HTTP 模式
- ❓ **需要确认**是否可以通过 `http://127.0.0.1:8744` 访问

**下一步**：
1. 尝试访问 `http://127.0.0.1:8744/mcp` 看是否有响应
2. 检查 IDA Pro 插件是否有 HTTP 服务器选项
3. 如果都没有，查看 IDA Pro MCP 的官方文档

