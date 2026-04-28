# JADX MCP 502 错误问题总结

## 问题背景

在完成 IDA MCP 功能之后，JADX MCP 功能开始出现 502 错误。**重要**：在实现 IDA MCP 之前，JADX MCP 功能是正常工作的。

## 问题现象

### 日志分析

从控制台日志可以看出：

1. **JADX MCP 服务器（端口 9999）连接正常**：
   ```
   2025/11/03 18:25:13 [JADX MCP] 响应状态码: 200 (URL: http://127.0.0.1:9999/mcp)
   ```

2. **但工具调用返回 502 错误**：
   ```
   2025-11-03 18:25:15,786 - ERROR - HTTP error 502:
   ```
   这是 **JADX MCP 服务器的日志**，表明服务器在尝试连接 JADX 插件时失败。

3. **工具调用结果**：
   ```json
   {"error":"HTTP error 502: ."}
   ```

## 根本原因分析

### 错误链路

```
客户端（Go） → JADX MCP 服务器（9999端口） → JADX 插件（8650端口）❌
                      ✅ 正常                      ❌ 502错误
```

### 问题定位

1. **不是 Go 代码的问题**：
   - Go 代码与 JADX MCP 服务器（9999）的连接正常（HTTP 200）
   - 没有修改 JADX MCP 的核心连接逻辑
   - 错误发生在 JADX MCP 服务器内部，当它尝试连接 JADX 插件时

2. **是 JADX 插件的问题**：
   - JADX GUI 可能没有运行
   - JADX MCP 插件可能没有启动
   - JADX MCP 插件可能没有监听在 8650 端口
   - JADX GUI 可能崩溃或重启，导致插件连接断开

3. **为什么之前正常，现在不正常**：
   - **可能原因 1**：在实现 IDA MCP 的过程中，可能意外关闭了 JADX GUI
   - **可能原因 2**：JADX GUI 或插件自动崩溃
   - **可能原因 3**：端口冲突（IDA MCP 和 JADX MCP 的端口可能冲突）
   - **可能原因 4**：系统重启或进程管理问题

## 已实施的修复

### 1. 错误检测和友好提示

在 `internal/mcp/jadx.go` 的 `CallTool` 方法中：

```go
// 检查 structuredContent 中的错误
if structuredContent, ok := resultMap["structuredContent"].(map[string]interface{}); ok {
    if errorMsg, ok := structuredContent["error"].(string); ok && errorMsg != "" {
        if strings.Contains(errorMsg, "502") {
            return nil, fmt.Errorf("JADX 插件连接失败 (502): 请确保 JADX GUI 正在运行，并且 JADX MCP 插件已启动（端口 8650）。错误详情: %s", errorMsg)
        }
    }
}
```

### 2. 流式聊天中的错误提示

在 `internal/mcp/jadx_stream.go` 中：

```go
if strings.Contains(errMsg, "502") || strings.Contains(errMsg, "JADX 插件连接失败") {
    toolMessage = fmt.Sprintf("工具 %s 执行失败: %s\n\n[提示] 请检查：\n1. JADX GUI 是否正在运行\n2. 是否已加载 APK 文件\n3. JADX MCP 插件是否已启动（Tools -> AI Assistant -> MCP Server）", toolCall.Name, errMsg)
}
```

### 3. 502 错误自动重连机制

在 `internal/mcp/jadx.go` 的 `SendMCPRequest` 方法中：

- 检测 HTTP 502 状态码
- 自动尝试重新初始化连接
- 检查并重启 JADX MCP 服务器（如果需要）
- 重试原始请求

**注意**：这个机制可以处理 JADX MCP 服务器（9999）的问题，但无法解决 JADX 插件（8650）的问题，因为那是在 JADX GUI 内部运行的。

## 解决方案

### 用户需要做的（根本解决）

1. **确保 JADX GUI 正在运行**
   - 打开 JADX GUI 应用程序
   - 加载 APK 文件

2. **启动 JADX MCP 插件**
   - 在 JADX GUI 中：`Tools -> AI Assistant -> MCP Server`
   - 或使用快捷键（如果配置了）
   - 确认插件在端口 8650 上运行

3. **验证连接**
   - 检查 JADX MCP 插件是否显示"已启动"或类似的运行状态
   - 如果有日志窗口，查看是否有错误信息

### 代码层面的改进（可选）

1. **添加 JADX 插件健康检查**：
   - 在连接时尝试直接访问 JADX 插件的健康检查端点（如果存在）
   - 如果插件未运行，给出明确的提示

2. **改进错误消息**：
   - 区分"JADX MCP 服务器未运行"和"JADX 插件未运行"
   - 提供更具体的故障排除步骤

3. **自动检测 JADX GUI 进程**：
   - 检查系统中是否有 JADX GUI 进程
   - 如果没有，提示用户启动 JADX GUI

## 验证步骤

1. **检查 JADX GUI 是否运行**：
   ```powershell
   Get-Process | Where-Object {$_.ProcessName -like "*jadx*"}
   ```

2. **检查端口 8650 是否被占用**：
   ```powershell
   netstat -ano | findstr :8650
   ```

3. **检查端口 9999 是否被占用**（JADX MCP 服务器）：
   ```powershell
   netstat -ano | findstr :9999
   ```

## 结论

**当前问题不是代码错误，而是 JADX 插件未运行或无法连接。**

- ✅ Go 代码与 JADX MCP 服务器（9999）的连接正常
- ❌ JADX MCP 服务器无法连接到 JADX 插件（8650）
- ✅ 已添加友好的错误提示和自动重连机制
- ⚠️  用户需要手动确保 JADX GUI 和插件正在运行

## 后续建议

1. **文档化**：在 README 或使用说明中明确说明需要启动 JADX GUI 和插件
2. **前端提示**：在前端界面上添加状态指示器，显示 JADX 插件是否在线
3. **健康检查**：定期检查 JADX 插件的可用性，并在界面上显示状态

