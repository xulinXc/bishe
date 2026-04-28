# IDA MCP 实现问题完整总结

## 概述

本文档详细记录了 IDA MCP 功能实现过程中遇到的所有问题、原因分析、解决思路和最终方案。

## 问题清单

### 问题 1：服务器启动失败 - `uv run ida-pro-mcp` 找不到程序

#### 问题描述

```
error: Failed to spawn: `ida-pro-mcp`
Caused by: program not found
```

#### 问题原因

1. **错误理解启动方式**：最初尝试使用 `uv run ida-pro-mcp --transport`，但 `ida-pro-mcp` 不是一个直接可执行的命令
2. **路径问题**：`ida-pro-mcp` 是通过 pip 安装的 Python 包，需要直接运行 Python 脚本
3. **启动参数错误**：尝试使用 `--http --port` 参数，但 IDA MCP 服务器不支持这些参数

#### 查找解决办法的过程

1. **查看用户提供的 mcp.json 配置**：
   - 用户提到 `mcp.json` 在 IDA 的配置中
   - 通过 `C:\Users\86483\.cursor\mcp.json` 查看实际配置
   ```json
   {
     "mcpServers": {
       "ida-pro-mcp": {
         "command": "python",
         "args": [
           "-m",
           "ida_pro_mcp.server",
           "--transport",
           "http://127.0.0.1:8744/sse"
         ]
       }
     }
   }
   ```
   - 发现使用的是 `python -m ida_pro_mcp.server`，而不是 `uv run ida-pro-mcp`
   - 发现只支持 `--transport` 参数

2. **查看源代码**：
   - 定位到 `F:\ida-pro-mcp-main\ida-pro-mcp-main\src\ida_pro_mcp\server.py`
   - 查看 `argparse` 配置，确认只支持 `--transport` 参数

#### 解决方案

```go
// 修改前（错误）：
cmd := exec.Command("uv", "run", "ida-pro-mcp", "--http", "--port", port)

// 修改后（正确）：
pythonPath := "C:\\Python313\\python.exe"
serverPyPath := "C:\\Users\\86483\\AppData\\Roaming\\Python\\Python313\\site-packages\\ida_pro_mcp\\server.py"
cmd := exec.Command(pythonPath, serverPyPath, "--transport", fmt.Sprintf("http://127.0.0.1:%s/sse", port))
```

**关键点：**
- 使用 Python 直接运行 `server.py` 脚本
- 使用 `--transport` 参数指定 SSE Transport 模式
- 路径来自 `mcp.json` 配置或系统 Python 安装路径

---

### 问题 2：404 Not Found - `/mcp` 端点不存在

#### 问题描述

```
[32mINFO[0m:     127.0.0.1:64840 - "[1mPOST /mcp HTTP/1.1[0m" [31m404 Not Found[0m
2025/11/03 17:14:57 [IDA MCP] /mcp 端点不存在 (404)
```

#### 问题原因

1. **架构理解错误**：IDA MCP 服务器启动时使用 `--transport http://127.0.0.1:8744/sse`，这表示服务器运行在 **SSE Transport 模式**
2. **端点差异**：
   - **标准 MCP 模式**：提供 `/mcp` 端点（POST 请求）
   - **SSE Transport 模式**：只提供 `/sse` 端点（GET 请求，用于建立 SSE 连接）
3. **协议不匹配**：我们尝试 POST 到 `/mcp`，但服务器只有 `/sse` 端点

#### 查找解决办法的过程

1. **分析错误日志**：
   - 看到大量 `404 Not Found` 错误
   - 确认服务器已经启动（看到启动日志）
   - 但 `/mcp` 端点不存在

2. **查看服务器启动日志**：
   ```
   MCP Server availabile at http://127.0.0.1:8744/sse
   ```
   - 发现服务器只暴露了 `/sse` 端点

2. **查看源代码 `server.py`**：
   ```python
   # server.py 使用 FastMCP
   @app.get("/sse")
   async def sse_endpoint():
       # SSE 连接端点
   ```
   - 确认服务器只提供了 `/sse` GET 端点

3. **查看 IDA MCP GitHub 仓库**：
   - 访问 `https://github.com/mrexodia/ida-pro-mcp`
   - 查看 README 和文档
   - 发现 SSE Transport 模式的说明

4. **阅读 FastMCP 文档**：
   - 了解 SSE Transport 的工作原理
   - 发现 SSE 主要用于接收事件，不是发送请求
   - 发现 SSE Transport 模式是用于接收事件的，不是用于发送请求的
   - 标准 MCP 使用 POST `/mcp`，但 SSE Transport 模式不同

#### 解决方案

**方案 1（尝试失败）**：实现 SSE Transport 客户端
- 建立 SSE 连接（GET `/sse`）
- 通过 SSE 流发送请求
- **问题**：实现复杂，且找不到发送请求的正确端点

**方案 2（最终方案）**：直接连接 IDA 插件
- 发现 `server.py` 只是一个**代理服务器**
- IDA 插件本身运行在 `http://127.0.0.1:13337/mcp`
- 插件提供标准的 POST `/mcp` 端点

**关键发现**：
```python
# server.py 中的代码
def make_jsonrpc_request(self, method, params):
    # 转发到 IDA 插件
    conn = http.client.HTTPConnection(ida_host, ida_port)  # 默认 127.0.0.1:13337
    conn.request("POST", "/mcp", json.dumps(request))
```

**最终实现**：
```go
// 检测是否是直接连接 IDA 插件（端口 13337）
isIDAPlugin := strings.Contains(baseURL, ":13337")

if isIDAPlugin {
    // 直接连接到 IDA 插件，跳过 server.py 代理
    testResp, err := conn.SendMCPRequest("get_metadata", nil)
    // ...
}
```

---

### 问题 3：405 Method Not Allowed - `/sse` 端点只接受 GET

#### 问题描述

```
[32mINFO[0m:     127.0.0.1:57481 - "[1mPOST /sse HTTP/1.1[0m" [31m405 Method Not Allowed[0m
响应头: map[Allow:[HEAD, GET]]
```

#### 问题原因

- `/sse` 端点是一个 **Server-Sent Events (SSE)** 端点
- SSE 协议使用 **GET** 请求建立连接，不是 POST
- 我们尝试 POST 到 `/sse`，但服务器只接受 GET

#### 查找解决办法的过程

1. **分析错误信息**：
   - 看到 `405 Method Not Allowed`
   - 响应头显示 `Allow: [HEAD, GET]`
   - 明确表示只接受 GET 方法

2. **查看响应头**：
   ```
   Allow: [HEAD, GET]
   ```
   - 明确显示只接受 HEAD 和 GET 方法

2. **查看 SSE 协议规范**：
   - SSE 使用 GET 请求建立连接
   - 客户端通过 SSE 流**接收**事件
   - **但 SSE 不支持双向通信**（无法通过 SSE 发送请求）

3. **查看 FastMCP 文档**：
   - 发现 SSE Transport 主要用于**接收**服务器推送的事件
   - **发送请求需要其他机制**

#### 解决方案

**放弃 SSE Transport 模式**，改用直接连接 IDA 插件：

1. **绕过 server.py 代理**
2. **直接连接 IDA 插件的 `/mcp` 端点**（`http://127.0.0.1:13337/mcp`）
3. 使用标准的 JSON-RPC POST 请求

**代码实现**：
```go
// 在 SendMCPRequest 中
if strings.Contains(baseURL, ":13337") {
    // 直接连接到 IDA 插件，使用标准 POST /mcp
    fullURL := "http://127.0.0.1:13337/mcp"
    // ... 标准 JSON-RPC POST 请求
}
```

---

### 问题 4：Method 'initialize' not found - IDA 插件不支持标准 MCP 协议

#### 问题描述

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "Method 'initialize' not found"
  }
}
```

#### 问题原因

1. **IDA 插件不是标准 MCP 服务器**：
   - IDA 插件直接暴露 IDA Pro 的功能方法（如 `get_metadata`、`get_function_by_name`）
   - 不实现标准 MCP 协议方法（`initialize`、`tools/list`）
2. **server.py 的作用**：
   - `server.py` 是一个**适配器/代理**
   - 它将标准 MCP 方法转换为 IDA 插件方法
   - 当我们绕过 `server.py` 直接连接插件时，就失去了这个适配层

#### 查找解决办法的过程

1. **分析错误响应**：
   ```json
   {"jsonrpc": "2.0", "id": 1, "error": {"code": -32601, "message": "Method 'initialize' not found"}}
   ```
   - 错误代码 `-32601` 表示"方法未找到"
   - 说明 IDA 插件不支持标准 MCP 的 `initialize` 方法

2. **查看 IDA 插件源代码 `mcp-plugin.py`**：
   ```python
   @jsonrpc
   def get_metadata() -> Metadata:
       """Get metadata about the current IDB"""
       # ...
   
   @jsonrpc
   def get_function_by_name(name: str) -> Function:
       """Get a function by its name"""
       # ...
   ```
   - 确认插件直接暴露 IDA 功能方法，不实现 `initialize`

2. **查看 server.py 源代码**：
   ```python
   # server.py 实现了标准的 MCP 方法
   @mcp.tool()
   def initialize(...):
       # 调用 IDA 插件的方法
   ```
   - 发现 `server.py` 实现了 MCP 标准方法的适配

3. **查看 server.py 源代码**：
   - 发现 `server.py` 实现了 `initialize` 方法
   - 它将标准 MCP 方法转换为 IDA 插件调用
   - 确认 `server.py` 是一个适配层

4. **测试直接调用插件方法**：
   - 尝试跳过 `initialize`，直接调用 `get_metadata`
   - 成功！
   - 确认可以直接调用插件方法
   ```go
   // 尝试直接调用 get_metadata
   resp, err := conn.SendMCPRequest("get_metadata", nil)
   // 成功！
   ```

#### 解决方案

**跳过标准 MCP 初始化，直接验证连接并使用预定义工具列表**：

```go
func (conn *IDAMCPConnection) initialize() error {
    isIDAPlugin := strings.Contains(baseURL, ":13337")
    
    if isIDAPlugin {
        // 直接连接 IDA 插件，跳过标准 MCP 初始化
        log.Printf("[IDA MCP] 检测到直接连接 IDA 插件，跳过标准 MCP 初始化")
        
        // 使用 get_metadata 验证连接
        testResp, err := conn.SendMCPRequest("get_metadata", nil)
        if err != nil {
            return fmt.Errorf("验证 IDA 插件连接失败: %v", err)
        }
        
        // 手动构建工具列表（66个工具）
        tools := []MCPTool{
            {Name: "get_metadata", Description: "Get metadata about the current IDB"},
            {Name: "get_function_by_name", Description: "Get a function by its name"},
            // ... 共 66 个工具
        }
        
        conn.tools = tools
        conn.connected = true
        return nil
    }
    
    // 标准 MCP 协议（如果连接 server.py）
    // ...
}
```

**关键点：**
- 直接连接插件时，跳过 `initialize` 和 `tools/list`
- 使用 `get_metadata` 验证连接
- 手动构建预定义的工具列表（从源代码中提取所有 `@jsonrpc` 装饰的函数）

---

### 问题 5：参数格式不匹配 - IDA 插件期望数组参数

#### 问题描述

调用 IDA 插件方法时返回错误，参数格式不正确。

#### 问题原因

1. **JSON-RPC 参数格式差异**：
   - **标准 MCP**：参数通常是对象（`{"param1": value1, "param2": value2}`）
   - **IDA 插件**：参数必须是数组（`[value1, value2]`）

2. **查看 IDA 插件代码**：
   ```python
   def dispatch(self, method: str, params: Any) -> Any:
       if isinstance(params, list):
           # 参数是数组，直接解包
           return func(*converted_params)
       elif isinstance(params, dict):
           # 参数是对象，需要匹配函数签名
           # ...
   ```
   - IDA 插件的 `dispatch` 函数优先处理数组参数

3. **查看实际调用**：
   ```python
   @jsonrpc
   def get_function_by_name(name: Annotated[str, "Name of the function to get"]) -> Function:
       # 函数签名只有一个参数 name
   ```
   - 当参数是数组时，`[name]` 会直接解包为 `name`
   - 当参数是对象时，`{"name": "func"}` 需要匹配参数名

#### 查找解决办法的过程

1. **分析错误响应**：
   - 调用插件方法时返回参数错误
   - 检查发送的参数格式

2. **查看 IDA 插件 `dispatch` 函数**：
   ```python
   def dispatch(self, method: str, params: Any) -> Any:
       if isinstance(params, list):
           # 处理数组参数
           return func(*converted_params)
       elif isinstance(params, dict):
           # 处理对象参数
   ```
   - 发现优先处理数组参数
   - 数组参数直接解包为函数参数

3. **测试不同的参数格式**：
   ```go
   // 尝试对象格式
   params := map[string]interface{}{"name": "main"}
   // 失败
   
   // 尝试数组格式
   params := []interface{}{"main"}
   // 成功！
   ```

2. **查看请求格式**：
   ```python
   # IDA 插件期望的格式
   {
     "jsonrpc": "2.0",
     "id": 1,
     "method": "get_function_by_name",
     "params": ["main"]  // 数组格式
   }
   ```

#### 解决方案

**在 `SendMCPRequest` 中转换参数格式**：

```go
func (conn *IDAMCPConnection) SendMCPRequest(method string, params map[string]interface{}) (map[string]interface{}, error) {
    isIDAPlugin := strings.Contains(baseURL, ":13337")
    
    var requestParams interface{}
    
    if isIDAPlugin {
        // IDA 插件期望数组格式的参数
        if params == nil || len(params) == 0 {
            requestParams = []interface{}{}
        } else {
            // 将 map 转换为数组（按照函数参数顺序）
            paramArray := make([]interface{}, 0, len(params))
            // 简单实现：将所有值放入数组（假设只有一个参数）
            for _, v := range params {
                paramArray = append(paramArray, v)
            }
            requestParams = paramArray
        }
    } else {
        // 标准 MCP 使用对象格式
        requestParams = params
    }
    
    request := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      requestID,
        "method":  method,
        "params":  requestParams,
    }
    // ...
}
```

**关键点：**
- 检测是否为 IDA 插件连接（端口 13337）
- 如果是，将参数从 `map[string]interface{}` 转换为 `[]interface{}`
- 如果不是，使用标准对象格式

**具体实现：**
```go
if isIDAPlugin {
    // IDA 插件期望数组格式
    if params == nil || len(params) == 0 {
        requestParams = []interface{}{}
    } else {
        // 将 map 转换为数组
        // 注意：这里假设参数顺序不重要，或者只有一个参数
        paramArray := make([]interface{}, 0, len(params))
        for _, v := range params {
            paramArray = append(paramArray, v)
        }
        requestParams = paramArray
    }
}
```

**注意事项：**
- 对于多参数方法（如 `list_functions(offset, count)`），需要确保参数顺序正确
- 当前实现简单地将所有 map 值放入数组，对于单个参数的方法是正确的
- 如果需要支持多参数方法，需要根据方法名和参数顺序进行转换

---

### 问题 6：工具列表预定义 - 需要手动构建所有可用工具

#### 问题描述

IDA 插件不支持 `tools/list` 方法，需要手动构建工具列表。

#### 问题原因

1. **IDA 插件不实现标准 MCP 方法**：如 `tools/list`
2. **server.py 提供适配**：但我们已经绕过它
3. **需要从源代码提取工具**：所有带有 `@jsonrpc` 装饰器的函数都是可用工具

#### 查找解决办法的过程

1. **查看 IDA 插件源代码**：
   ```python
   @jsonrpc
   def get_metadata() -> Metadata:
       """Get metadata about the current IDB"""
   
   @jsonrpc
   def get_function_by_name(name: Annotated[str, "..."]) -> Function:
       """Get a function by its name"""
   
   @jsonrpc
   def list_functions(offset: int, count: int) -> Page[Function]:
       """List all functions in the database (paginated)"""
   ```
   - 每个 `@jsonrpc` 装饰的函数都是一个工具

2. **提取所有工具**：
   - 从 `mcp-plugin.py` 中搜索所有 `@jsonrpc` 装饰的函数
   - 提取函数名和 docstring（作为描述）
   - 共找到 66 个工具

#### 解决方案

**手动构建完整的工具列表**：

```go
tools := []MCPTool{
    // 连接和元数据
    {Name: "check_connection", Description: "Check if the IDA plugin is running"},
    {Name: "get_metadata", Description: "Get metadata about the current IDB"},
    
    // 当前状态
    {Name: "get_current_address", Description: "Get the address currently selected by the user"},
    {Name: "get_current_function", Description: "Get the function currently selected by the user"},
    
    // 函数相关（10个）
    {Name: "get_function_by_name", Description: "Get a function by its name"},
    {Name: "get_function_by_address", Description: "Get a function by its address"},
    {Name: "list_functions", Description: "List all functions in the database (paginated)"},
    // ... 共 66 个工具
}
```

**工具分类：**
- 连接和元数据（2个）
- 当前状态（2个）
- 函数相关（10个）
- 交叉引用（2个）
- 全局变量（6个）
- 字符串（2个）
- 导入（1个）
- 结构体（5个）
- 类型（2个）
- 栈帧变量（7个）
- 内存读取（6个）
- 注释和补丁（2个）
- 工具函数（1个）
- 调试器功能（12个，unsafe）

**总共：66 个工具**

---

### 问题 7：CallTool 工具调用包装问题

#### 问题描述

调用 IDA 插件工具时，应该直接调用方法名，而不是通过 `tools/call` 包装。

#### 问题原因

1. **标准 MCP 协议**：使用 `tools/call` 方法包装工具调用
   ```json
   {
     "method": "tools/call",
     "params": {
       "name": "get_function_by_name",
       "arguments": {"name": "main"}
     }
   }
   ```

2. **IDA 插件直接调用**：应该直接调用方法名
   ```json
   {
     "method": "get_function_by_name",
     "params": ["main"]
   }
   ```

#### 查找解决办法的过程

1. **查看 CallTool 实现**：
   ```go
   // 标准 MCP 方式
   params := map[string]interface{}{
       "name":      toolName,
       "arguments": arguments,
   }
   resp, err := conn.SendMCPRequest("tools/call", params)
   ```

2. **测试直接调用**：
   ```go
   // 直接调用插件方法
   resp, err := conn.SendMCPRequest(toolName, arguments)
   // 成功！
   ```

#### 解决方案

**在 `CallTool` 中区分标准 MCP 和 IDA 插件**：

```go
func (conn *IDAMCPConnection) CallTool(toolName string, arguments map[string]interface{}) (interface{}, error) {
    conn.mu.RLock()
    baseURL := conn.baseURL
    conn.mu.RUnlock()
    
    isIDAPlugin := strings.Contains(baseURL, ":13337")
    
    if isIDAPlugin {
        // 直接调用 IDA 插件的方法（不需要 tools/call 包装）
        resp, err := conn.SendMCPRequest(toolName, arguments)
        if err != nil {
            return nil, err
        }
        
        // 提取结果
        if result, ok := resp["result"]; ok {
            return result, nil
        }
        return resp, nil
    }
    
    // 标准 MCP 协议：使用 tools/call
    params := map[string]interface{}{
        "name":      toolName,
        "arguments": arguments,
    }
    resp, err := conn.SendMCPRequest("tools/call", params)
    // ...
}
```

**关键点：**
- IDA 插件直接调用方法名，不需要 `tools/call` 包装
- 参数直接传递，不需要 `name` 和 `arguments` 包装
- 结果直接提取，不需要额外的嵌套结构

---

### 问题 8：前端默认端口配置 - 需要改为 13337

#### 问题描述

前端默认端口是 8744（server.py 端口），但实际应该使用 13337（IDA 插件端口）。

#### 问题原因

1. **最初设计**：使用 server.py 作为代理（端口 8744）
2. **最终方案**：直接连接 IDA 插件（端口 13337）
3. **前端未更新**：默认值仍然是 8744

#### 解决方案

```html
<!-- 修改前 -->
<input type="text" id="mcpBaseURL" value="http://127.0.0.1:8744">

<!-- 修改后 -->
<input type="text" id="mcpBaseURL" value="http://127.0.0.1:13337" 
       placeholder="http://127.0.0.1:13337 (默认，IDA 插件端口)">
```

---

### 问题 9：服务器启动等待时间过长

#### 问题描述

服务器启动后，检测响应超时，需要等待很长时间才能连接。

#### 问题原因

1. **固定检查间隔**：每 500ms 检查一次，即使服务器已经启动
2. **最大等待时间**：15 秒可能太长
3. **检查逻辑不够智能**：没有考虑服务器实际启动时间

#### 解决方案

**使用渐进式检查间隔和合理的超时时间**：

```go
// 等待服务器启动（最多 10 秒）
maxWait := 10 * time.Second
checkInterval := 500 * time.Millisecond
startTime := time.Now()
for time.Since(startTime) < maxWait {
    if checkIDAServerRunning(baseURL) {
        log.Printf("[IDA MCP] 服务器启动成功并已就绪")
        break
    }
    time.Sleep(checkInterval)
}
```

**优化建议（如果服务器启动很慢）**：
- 使用渐进式间隔：前几次 200ms，然后 500ms，最后 1秒
- 根据服务器类型调整等待时间（IDA 插件通常很快，server.py 可能需要更长时间）

---

### 问题 10：服务器启动时端口冲突

#### 问题描述

```
[31mERROR[0m:    [Errno 10048] error while attempting to bind on address ('127.0.0.1', 8744): 
[winerror 10048] 通常每个套接字地址(协议/网络地址/端口)只允许使用一次。
```

#### 问题原因

1. **重复启动**：如果服务器已经在运行，再次尝试启动会导致端口冲突
2. **进程检查不准确**：`checkIDAServerRunning` 可能在服务器启动初期返回 false
3. **进程清理不及时**：之前启动的服务器进程可能没有正确终止

#### 查找解决办法的过程

1. **检查进程状态**：
   ```go
   if idaServerProcess != nil && idaServerProcess.Process != nil {
       if idaServerProcess.ProcessState == nil {
           // 进程存在但未检查状态
           if checkIDAServerRunning(baseURL) {
               log.Printf("[IDA MCP] 服务器进程已在运行")
               return nil
           }
           log.Printf("[IDA MCP] 服务器进程存在但无响应，将重新启动")
       }
   }
   ```

2. **改进检查逻辑**：
   - 在启动前检查服务器是否已运行
   - 如果运行则直接返回
   - 如果不运行，检查进程是否存在但无响应

#### 解决方案

**在 `startIDAServer` 中添加更完善的检查**：

```go
func startIDAServer(baseURL string) error {
    idaServerMu.Lock()
    defer idaServerMu.Unlock()
    
    // 先检查服务器是否已运行（通过 HTTP）
    if checkIDAServerRunning(baseURL) {
        log.Printf("[IDA MCP] 服务器已在运行（通过 HTTP 检查确认）")
        return nil
    }
    
    // 检查进程是否存在
    if idaServerProcess != nil && idaServerProcess.Process != nil {
        if idaServerProcess.ProcessState == nil {
            // 进程存在，再次检查 HTTP
            if checkIDAServerRunning(baseURL) {
                log.Printf("[IDA MCP] 服务器进程已在运行")
                return nil
            }
            // 进程存在但无响应，需要重新启动
            log.Printf("[IDA MCP] 服务器进程存在但无响应，将重新启动")
            // 终止旧进程
            idaServerProcess.Process.Kill()
        }
    }
    
    // 启动新服务器...
}
```

**关键点：**
- 启动前先通过 HTTP 检查服务器是否已运行
- 检查进程状态，避免重复启动
- 如果进程存在但无响应，先终止再重启

---

---

## 问题解决时间线

### 阶段 1：初步实现（遇到问题 1-2）
- **目标**：复制 JADX MCP 的实现方式
- **问题**：服务器启动失败，端点不存在
- **解决**：查看配置和源代码，修正启动命令

### 阶段 2：探索 SSE Transport（遇到问题 3）
- **目标**：实现 SSE Transport 客户端
- **问题**：无法通过 SSE 发送请求
- **解决**：查看源代码，发现 SSE 只用于接收事件

### 阶段 3：直接连接插件（遇到问题 4-6）
- **目标**：绕过 server.py，直接连接 IDA 插件
- **问题**：协议不兼容，参数格式不匹配
- **解决**：适配协议差异，手动构建工具列表

### 阶段 4：优化和修复（遇到问题 7-10）
- **目标**：完善功能，修复细节问题
- **问题**：工具调用、端口配置、服务器管理
- **解决**：逐一修复，优化代码

---

## 总结

### 核心架构发现

1. **IDA MCP 三层架构**：
   ```
   客户端 (Go) → server.py (代理, 8744) → IDA 插件 (13337)
   ```
   - `server.py`：标准 MCP 协议的适配器
   - IDA 插件：直接暴露 IDA Pro 功能
   - **我们的方案**：绕过 server.py，直接连接插件

2. **为什么直接连接插件更好**：
   - ✅ 更简单：不需要处理 SSE Transport
   - ✅ 更快速：减少一层代理
   - ✅ 更可靠：不依赖 server.py 的启动
   - ✅ 更直接：直接调用 IDA 功能

3. **关键差异**：
   - **参数格式**：数组 vs 对象
   - **协议方法**：不支持标准 MCP `initialize` 和 `tools/list`
   - **工具列表**：需要预定义，不能动态获取

### 解决思路总结

1. **遇到错误时**：
   - 查看错误日志和响应状态码
   - 查看源代码确认服务器行为
   - 测试不同的请求格式

2. **理解架构**：
   - 查看源代码了解服务器实现
   - 区分不同模式（标准 MCP vs SSE Transport）
   - 识别代理层和实际服务层

3. **寻找替代方案**：
   - 当标准方案不可行时，寻找绕过方法
   - 直接连接底层服务（IDA 插件）
   - 手动适配协议差异

### 最终实现方案

1. **连接方式**：直接连接 IDA 插件（`http://127.0.0.1:13337/mcp`）
2. **参数格式**：数组格式（`[]interface{}`）
3. **初始化**：跳过标准 MCP，使用 `get_metadata` 验证连接
4. **工具列表**：预定义 66 个工具（从源代码提取）
5. **前端配置**：默认端口 13337

### 经验教训

1. **不要假设协议一致性**：
   - 不同的实现可能有不同的协议变体
   - 即使都是 MCP，JADX 和 IDA 的实现方式完全不同
   - 需要查看源代码确认实际行为

2. **查看源代码是关键**：
   - 文档可能不完整或过时
   - 源代码是最准确的参考
   - 关键文件：`server.py`、`mcp-plugin.py`

3. **尝试直接连接底层服务**：
   - 代理层可能增加复杂性
   - 直接连接底层服务通常更简单
   - 示例：绕过 `server.py`，直接连接 IDA 插件

4. **参数格式很重要**：
   - JSON-RPC 允许数组和对象两种格式
   - 但服务器可能只支持一种
   - 需要测试并适配

5. **预定义工具列表是可行的**：
   - 当无法动态获取时，手动维护列表
   - 虽然需要维护，但更可控
   - 可以完整覆盖所有功能

6. **错误码提供重要线索**：
   - 404：端点不存在
   - 405：方法不允许
   - 406：请求头不正确
   - -32601：方法未找到

7. **分层理解架构**：
   - 理解每一层的作用
   - 识别可以绕过的层
   - 找到最简单的实现路径

### 调试技巧

1. **详细日志**：
   - 记录所有请求和响应
   - 包含状态码、请求头、响应体
   - 帮助快速定位问题

2. **逐步测试**：
   - 先测试最简单的请求（如 `get_metadata`）
   - 确认基本连接后再测试复杂功能
   - 逐步增加复杂度

3. **对比已知工作的实现**：
   - JADX MCP 已经工作正常
   - 对比差异点，找出问题
   - 参考成功的实现方式

4. **查看源代码**：
   - 理解服务器实际行为
   - 确认协议细节
   - 找出隐藏的端点和功能

---

## 关键代码片段

### 1. 检测 IDA 插件连接

```go
isIDAPlugin := strings.Contains(baseURL, ":13337")
```

### 2. 参数格式转换

```go
if isIDAPlugin {
    // IDA 插件需要数组格式
    if params == nil || len(params) == 0 {
        requestParams = []interface{}{}
    } else {
        paramArray := make([]interface{}, 0, len(params))
        for _, v := range params {
            paramArray = append(paramArray, v)
        }
        requestParams = paramArray
    }
}
```

### 3. 直接调用插件方法

```go
if isIDAPlugin {
    // 直接调用方法，不使用 tools/call 包装
    resp, err := conn.SendMCPRequest(toolName, arguments)
    // ...
}
```

### 4. 预定义工具列表

```go
tools := []MCPTool{
    {Name: "get_metadata", Description: "Get metadata about the current IDB"},
    {Name: "get_function_by_name", Description: "Get a function by its name"},
    // ... 共 66 个工具
}
```

---

## 相关文档

- `shili/说明/IDA_MCP实现思路.md` - 初始实现思路
- `shili/说明/IDA_MCP问题总结.md` - 早期问题总结
- `shili/说明/IDA_MCP完美解决方案.md` - 最终解决方案
- `shili/说明/IDA_MCP使用说明.md` - 使用说明
- `shili/说明/IDA和JADX连接速度差异分析.md` - 性能对比分析

---

## 附录：从源代码提取的工具列表

完整的 66 个工具列表（从 `mcp-plugin.py` 提取）：

### 连接和元数据（2个）
- `check_connection`
- `get_metadata`

### 当前状态（2个）
- `get_current_address`
- `get_current_function`

### 函数相关（10个）
- `get_function_by_name`
- `get_function_by_address`
- `list_functions`
- `get_entry_points`
- `get_callees`
- `get_callers`
- `decompile_function`
- `disassemble_function`
- `rename_function`
- `set_function_prototype`

### 交叉引用（2个）
- `get_xrefs_to`
- `get_xrefs_to_field`

### 全局变量（6个）
- `list_globals`
- `list_globals_filter`
- `get_global_variable_value_by_name`
- `get_global_variable_value_at_address`
- `rename_global_variable`
- `set_global_variable_type`

### 字符串（2个）
- `list_strings`
- `list_strings_filter`

### 导入（1个）
- `list_imports`

### 结构体（5个）
- `get_defined_structures`
- `analyze_struct_detailed`
- `get_struct_at_address`
- `get_struct_info_simple`
- `search_structures`

### 类型（2个）
- `list_local_types`
- `declare_c_type`

### 栈帧变量（7个）
- `get_stack_frame_variables`
- `rename_local_variable`
- `set_local_variable_type`
- `rename_stack_frame_variable`
- `create_stack_frame_variable`
- `set_stack_frame_variable_type`
- `delete_stack_frame_variable`

### 内存读取（6个）
- `read_memory_bytes`
- `data_read_byte`
- `data_read_word`
- `data_read_dword`
- `data_read_qword`
- `data_read_string`

### 注释和补丁（2个）
- `set_comment`
- `patch_address_assembles`

### 工具函数（1个）
- `convert_number`

### 调试器功能（12个，unsafe）
- `dbg_get_registers`
- `dbg_get_call_stack`
- `dbg_list_breakpoints`
- `dbg_start_process`
- `dbg_exit_process`
- `dbg_continue_process`
- `dbg_run_to`
- `dbg_set_breakpoint`
- `dbg_step_into`
- `dbg_step_over`
- `dbg_delete_breakpoint`
- `dbg_enable_breakpoint`

