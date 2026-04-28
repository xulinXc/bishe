# 启动 JADX MCP 服务器（HTTP 模式）

## 问题说明

`uv` 命令不可用，需要找到替代方式启动 MCP 服务器。

## 解决方案

### 方案 1：使用 Python 直接运行（推荐）✅

`jadx_mcp_server.py` 支持 HTTP 模式，可以直接使用 Python 运行：

```powershell
# 切换到 MCP 服务器目录
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"

# 启动 HTTP 模式（指定端口）
python jadx_mcp_server.py --http --port 9999

# 注意：默认端口是 8651，如果不指定 --port，使用默认端口
# python jadx_mcp_server.py --http
```

**服务器参数说明：**
- `--http`: 启用 HTTP 模式
- `--port PORT`: 指定 HTTP 端口（默认：8651）
- `--jadx-port JADX_PORT`: 指定 JADX AI MCP Plugin 端口（默认：8650）

**重要：** 服务器启动后会持续运行，保持终端窗口打开。在前端配置时使用启动的端口号。

### 方案 2：查看服务器脚本的参数

首先查看脚本的帮助信息：

```powershell
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"
python jadx_mcp_server.py --help
```

查看脚本中是否有 HTTP 模式的相关代码。

### 方案 3：安装 uv（如果需要）

如果 MCP 服务器依赖 `uv` 环境，可以安装它：

```powershell
# 使用 pip 安装 uv
pip install uv

# 或者使用官方安装脚本（Windows）
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
```

安装后，重新尝试：

```powershell
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"
uv run jadx_mcp_server.py --http --port 9999
```

### 方案 4：检查是否有虚拟环境

检查 MCP 服务器目录下是否有虚拟环境：

```powershell
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"

# 检查是否有 .venv 或 venv 目录
dir .venv
dir venv

# 如果有虚拟环境，激活它然后运行
.venv\Scripts\activate
python jadx_mcp_server.py --http --port 9999
```

### 方案 5：查看 README 或文档

检查 MCP 服务器目录中是否有 README.md 或其他文档文件，查看正确的启动方式：

```powershell
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"
Get-ChildItem *.md
Get-ChildItem *.txt
```

## 验证服务器是否启动成功

### 1. 检查端口是否监听

```powershell
# 检查端口是否被占用
netstat -ano | findstr :9999
```

如果看到端口在监听，说明服务器启动成功。

### 2. 测试 HTTP 连接

在另一个终端窗口测试连接：

```powershell
# 使用 PowerShell 的 Invoke-WebRequest（推荐）
$body = @{
    jsonrpc = "2.0"
    id = 1
    method = "tools/list"
} | ConvertTo-Json

Invoke-WebRequest -Uri "http://127.0.0.1:9999/mcp" `
  -Method POST `
  -ContentType "application/json" `
  -Body $body

# 或者使用 curl（如果已安装）
curl -X POST http://127.0.0.1:9999/mcp `
  -H "Content-Type: application/json" `
  -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}'
```

如果返回 JSON 响应（包含工具列表），说明连接成功。

## 检查端口是否被占用

如果端口 9999 已被占用，可以：

1. 检查端口占用：
```powershell
netstat -ano | findstr :9999
```

2. 更换端口（如果 MCP 服务器支持）：
```powershell
python jadx_mcp_server.py --http --port 9998
```

然后在前端界面中修改 MCP 服务器地址为 `http://127.0.0.1:9998`

## 常见问题

### 问题 1：MCP 服务器可能只支持 stdio 模式

如果 `jadx_mcp_server.py` 不支持 HTTP 模式，可能需要：
- 查找是否有其他 HTTP 模式的启动脚本
- 或者修改服务器代码以支持 HTTP 模式
- 或者使用其他支持 HTTP 模式的 MCP 服务器

### 问题 2：缺少依赖包

如果运行时提示缺少模块，需要安装依赖：

```powershell
cd "G:\jadx-gui-1.5.3-with-jre-win\jadx-mcp-server-v3.3.5\jadx-mcp-server"
pip install -r requirements.txt
```

### 问题 3：需要配置 JADX 路径

某些 MCP 服务器可能需要配置 JADX 的安装路径：

```powershell
python jadx_mcp_server.py --jadx-path "G:\jadx-gui-1.5.3-with-jre-win"
```

## 在 Web 界面中配置

服务器启动成功后：

1. 打开 Web 界面（JADX MCP 页面）
2. 在 "JADX MCP 服务器地址" 字段中输入：
   - 如果使用默认端口：`http://127.0.0.1:8651`
   - 如果使用 `--port 9999`：`http://127.0.0.1:9999`
3. 配置 API Key 和其他设置
4. 点击 "保存配置并连接"

## 下一步

1. ✅ 依赖已安装完成
2. ✅ 服务器已支持 HTTP 模式启动
3. 在 Web 界面配置 MCP 服务器地址（注意端口号）
4. 测试连接功能

**提示：** 如果前端配置的端口与服务器启动的端口不一致，连接会失败。确保端口号匹配。

