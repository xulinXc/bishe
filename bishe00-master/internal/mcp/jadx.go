package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// JADXMCPConnection JADX MCP 连接
type JADXMCPConnection struct {
	mu            sync.RWMutex
	baseURL       string // MCP 服务器基础 URL，如 http://127.0.0.1:9999
	client        *http.Client
	connected     bool
	tools         []MCPTool // MCP 工具列表
	lastRequestID int64     // 最后一个请求 ID
	sessionID     string    // FastMCP streamable-http 需要的会话 ID
}

// MCPTool MCP 工具定义
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"inputSchema"`
}

var (
	jadxSessions      = make(map[string]*JADXChatSession)
	jadxSessionsMu    sync.RWMutex
	jadxmcpConn       *JADXMCPConnection
	jadxmcpConnMu     sync.RWMutex
	jadxServerProcess *exec.Cmd  // JADX MCP 服务器进程
	jadxServerMu      sync.Mutex // 保护服务器进程的互斥锁
)

// checkJADXServerRunning 检查 JADX MCP 服务器是否正在运行
func checkJADXServerRunning(baseURL string) bool {
	testClient := &http.Client{Timeout: 2 * time.Second}

	testURL := strings.TrimSuffix(baseURL, "/") + "/mcp"
	testReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      999999,
		"method":  "tools/list",
	}

	reqData, _ := json.Marshal(testReq)
	req, err := http.NewRequest("POST", testURL, bytes.NewBuffer(reqData))
	if err != nil {
		return false
	}

	// FastMCP 需要 Accept 头，否则返回 406 Not Acceptable
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := testClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 406 Not Acceptable 通常表示服务器在运行但请求头不正确
	// 200/400/500 表示服务器在运行并能处理请求
	return resp.StatusCode == 200 || resp.StatusCode == 400 || resp.StatusCode == 500 || resp.StatusCode == 406
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// commandExists 检查命令是否存在
func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// startJADXServer 启动 JADX MCP HTTP 服务器
func startJADXServer(baseURL string) error {
	jadxServerMu.Lock()
	defer jadxServerMu.Unlock()

	if jadxServerProcess != nil && jadxServerProcess.Process != nil {
		if jadxServerProcess.ProcessState == nil {
			if checkJADXServerRunning(baseURL) {
				log.Printf("[JADX MCP] 服务器进程已在运行")
				return nil
			}
			log.Printf("[JADX MCP] 服务器进程存在但无响应，将重新启动")
		}
	}

	port := "8651"
	if strings.HasPrefix(baseURL, "http://") {
		parts := strings.Split(baseURL, ":")
		if len(parts) == 3 {
			port = parts[2]
		}
	}

	log.Printf("[JADX MCP] 正在启动服务器，端口: %s", port)

	serverDir := "G:\\jadx-gui-1.5.3-with-jre-win\\jadx-mcp-server-v3.3.5\\jadx-mcp-server"
	serverScript := "jadx_mcp_server.py"

	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		log.Printf("[JADX MCP] 默认服务器目录不存在: %s", serverDir)
		return fmt.Errorf("JADX MCP 服务器目录不存在: %s", serverDir)
	}

	// 优先尝试使用 Python 直接启动（推荐方式）
	pythonPath := "python"
	if python3 := os.Getenv("PYTHON3_PATH"); python3 != "" {
		pythonPath = python3
	} else if path := "C:\\Python313\\python.exe"; fileExists(path) {
		pythonPath = path
	} else if commandExists("python3") {
		pythonPath = "python3"
	}

	// 检查 Python 是否可用
	if !commandExists(pythonPath) {
		// 如果 Python 不可用，尝试使用 uv
		if commandExists("uv") {
			log.Printf("[JADX MCP] Python 不可用，尝试使用 uv 启动服务器...")
			cmd := exec.Command("uv", "--directory", serverDir, "run", serverScript, "--http", "--port", port)
			cmd.Dir = serverDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Start(); err != nil {
				log.Printf("[JADX MCP] 使用 uv 启动失败: %v", err)
				return fmt.Errorf("无法启动 JADX MCP 服务器: %v", err)
			}

			jadxServerProcess = cmd
			log.Printf("[JADX MCP] 使用 uv 启动服务器成功 (PID: %d)，等待就绪...", cmd.Process.Pid)

			maxWait := 10 * time.Second
			checkInterval := 500 * time.Millisecond
			startTime := time.Now()

			for time.Since(startTime) < maxWait {
				if checkJADXServerRunning(baseURL) {
					log.Printf("[JADX MCP] 服务器已就绪")
					return nil
				}
				time.Sleep(checkInterval)
			}

			log.Printf("[JADX MCP] 服务器进程已启动，但检测响应超时（服务器可能仍在启动中）")
			return nil
		} else {
			return fmt.Errorf("未找到 Python 或 uv 命令，无法启动 JADX MCP 服务器")
		}
	}

	// 使用 Python 启动服务器（推荐方式）
	log.Printf("[JADX MCP] 尝试使用 Python (%s) 启动服务器...", pythonPath)
	scriptPath := filepath.Join(serverDir, serverScript)

	// 命令：python jadx_mcp_server.py --http --port PORT
	cmd := exec.Command(pythonPath, scriptPath, "--http", "--port", port)
	cmd.Dir = serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[JADX MCP] 使用 Python 启动失败: %v", err)
		return fmt.Errorf("无法启动 JADX MCP 服务器: %v", err)
	}

	jadxServerProcess = cmd
	log.Printf("[JADX MCP] 使用 Python 启动服务器成功 (PID: %d)，等待就绪...", cmd.Process.Pid)

	// 等待服务器启动（最多等待 10 秒）
	maxWait := 10 * time.Second
	checkInterval := 500 * time.Millisecond
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		if checkJADXServerRunning(baseURL) {
			log.Printf("[JADX MCP] 服务器已就绪")
			return nil
		}
		time.Sleep(checkInterval)
	}

	log.Printf("[JADX MCP] 服务器进程已启动，但检测响应超时（服务器可能仍在启动中）")
	return nil
}

// StopJADXServer 停止 JADX MCP 服务器
func StopJADXServer() {
	jadxServerMu.Lock()
	defer jadxServerMu.Unlock()

	if jadxServerProcess != nil && jadxServerProcess.Process != nil {
		log.Printf("[JADX MCP] 正在停止服务器进程 (PID: %d)...", jadxServerProcess.Process.Pid)
		if err := jadxServerProcess.Process.Kill(); err != nil {
			log.Printf("[JADX MCP] 停止服务器进程失败: %v", err)
		} else {
			log.Printf("[JADX MCP] 服务器进程已停止")
		}
		jadxServerProcess = nil
	}
}

// ConnectJADXMCP 连接到 JADX MCP 服务器（通过 HTTP）
func ConnectJADXMCP(baseURL string) (*JADXMCPConnection, error) {
	jadxmcpConnMu.Lock()
	defer jadxmcpConnMu.Unlock()

	if baseURL == "" {
		// 默认端口：FastMCP 服务器默认是 8651，但如果手动启动可以指定其他端口（如 9999）
		// JADX AI MCP Plugin 在 8650，但那是插件端口，不是 MCP 服务器端口
		baseURL = "http://127.0.0.1:8651" // FastMCP 默认端口
	}

	// 如果已经连接且 URL 相同，验证连接是否仍然有效
	if jadxmcpConn != nil && jadxmcpConn.connected && jadxmcpConn.baseURL == baseURL {
		// 验证连接是否仍然有效
		if checkJADXServerRunning(baseURL) {
			return jadxmcpConn, nil
		} else {
			log.Printf("[JADX MCP] 之前的连接已失效，重新初始化...")
			jadxmcpConn.mu.Lock()
			jadxmcpConn.connected = false
			jadxmcpConn.mu.Unlock()
		}
	}

	// 检查服务器是否运行，如果没有则启动
	if !checkJADXServerRunning(baseURL) {
		log.Printf("[JADX MCP] 服务器未运行，正在自动启动...")
		if err := startJADXServer(baseURL); err != nil {
			return nil, fmt.Errorf("启动 JADX MCP 服务器失败: %v", err)
		}

		// 再次等待服务器完全启动（最多 10 秒）
		// 使用渐进式检查间隔：开始时检查频繁，后面逐渐减少
		maxWait := 10 * time.Second
		startTime := time.Now()
		checkCount := 0
		for time.Since(startTime) < maxWait {
			if checkJADXServerRunning(baseURL) {
				log.Printf("[JADX MCP] 服务器启动成功并已就绪（经过 %d 次检查，耗时 %v）", checkCount, time.Since(startTime))
				break
			}
			checkCount++
			// 前3次检查间隔200ms，然后500ms，最后1秒
			var checkInterval time.Duration
			if checkCount <= 3 {
				checkInterval = 200 * time.Millisecond
			} else if checkCount <= 10 {
				checkInterval = 500 * time.Millisecond
			} else {
				checkInterval = 1 * time.Second
			}
			time.Sleep(checkInterval)
		}
	} else {
		log.Printf("[JADX MCP] 服务器已在运行")
	}

	// FastMCP streamable-http：session ID 由服务器生成，不在客户端生成
	// 初始时 session ID 为空，等待 initialize 响应后从服务器获取

	// 创建支持 Cookie 的 HTTP 客户端（FastMCP 可能使用 Cookie 传递 session ID）
	jar, _ := cookiejar.New(nil)
	conn := &JADXMCPConnection{
		baseURL:       baseURL,
		client:        &http.Client{Timeout: 30 * time.Second, Jar: jar},
		connected:     false,
		tools:         []MCPTool{},
		lastRequestID: 0,
		sessionID:     "", // 初始为空，等待服务器生成
	}

	// 测试连接并获取工具列表
	if err := conn.initialize(); err != nil {
		return nil, fmt.Errorf("初始化 MCP 连接失败: %v", err)
	}

	jadxmcpConn = conn
	return conn, nil
}

// initialize 初始化连接并获取工具列表
func (conn *JADXMCPConnection) initialize() error {
	// 先检查服务器是否运行
	conn.mu.RLock()
	baseURL := conn.baseURL
	conn.mu.RUnlock()

	// 如果服务器已在运行（已在 ConnectJADXMCP 中检查过），这里不再重复检查
	// 只有在连接过程中发现服务器不可用时才重新检查和启动
	if !checkJADXServerRunning(baseURL) {
		log.Printf("[JADX MCP] 服务器未运行，尝试启动...")
		if err := startJADXServer(baseURL); err != nil {
			return fmt.Errorf("启动服务器失败: %v", err)
		}
		// 等待服务器启动（快速检查，最多等待5秒）
		// 使用渐进式检查间隔，减少不必要的请求
		maxWait := 5 * time.Second
		startTime := time.Now()
		checkCount := 0
		for time.Since(startTime) < maxWait {
			if checkJADXServerRunning(baseURL) {
				log.Printf("[JADX MCP] 服务器启动成功并已就绪（经过 %d 次检查，耗时 %v）", checkCount, time.Since(startTime))
				break
			}
			checkCount++
			// 前3次检查间隔200ms，然后500ms
			var checkInterval time.Duration
			if checkCount <= 3 {
				checkInterval = 200 * time.Millisecond
			} else {
				checkInterval = 500 * time.Millisecond
			}
			time.Sleep(checkInterval)
		}
	} else {
		// 服务器已在运行，无需等待
		log.Printf("[JADX MCP] 服务器已在运行，直接初始化连接")
	}

	// 重置 session ID，重新获取
	conn.mu.Lock()
	conn.sessionID = ""
	conn.mu.Unlock()

	// FastMCP streamable-http 可能需要先发送 initialize 请求
	// 尝试发送 initialize 请求
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
		// 如果初始化失败，继续尝试列出工具
	} else {
		log.Printf("[JADX MCP] 初始化成功: %v", initResp)
		// 尝试从初始化响应中获取或更新 session ID（如果有）
		// FastMCP 可能在响应中返回 session ID
		if result, ok := initResp["result"].(map[string]interface{}); ok {
			if serverSessionID, ok := result["sessionId"].(string); ok && serverSessionID != "" {
				conn.mu.Lock()
				conn.sessionID = serverSessionID
				conn.mu.Unlock()
				log.Printf("[JADX MCP] 从服务器获取到 Session ID: %s", serverSessionID)
			}
		}
	}

	// 然后检查连接（通过列出工具来测试连接）
	tools, err := conn.listTools()
	if err != nil {
		return fmt.Errorf("检查连接失败: %v", err)
	}

	conn.mu.Lock()
	conn.tools = tools
	conn.connected = true
	conn.mu.Unlock()

	log.Printf("[JADX MCP] 连接成功，获取到 %d 个工具", len(tools))
	return nil
}

// listTools 列出所有可用工具
func (conn *JADXMCPConnection) listTools() ([]MCPTool, error) {
	// JADX MCP Server 使用 MCP 协议，通过 JSON-RPC 调用 tools/list
	resp, err := conn.SendMCPRequest("tools/list", nil)
	if err != nil {
		return nil, err
	}

	// 解析响应
	result, ok := resp["result"]
	if !ok {
		return nil, fmt.Errorf("响应中没有 result 字段")
	}

	// 提取工具列表
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("result 格式错误")
	}

	toolsData, ok := resultMap["tools"]
	if !ok {
		return []MCPTool{}, nil // 没有工具，返回空列表
	}

	toolsArray, ok := toolsData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("tools 不是数组")
	}

	tools := make([]MCPTool, 0, len(toolsArray))
	for _, t := range toolsArray {
		toolMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		tool := MCPTool{}
		if name, ok := toolMap["name"].(string); ok {
			tool.Name = name
		}
		if desc, ok := toolMap["description"].(string); ok {
			tool.Description = desc
		}
		if params, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
			tool.Parameters = params
		} else if params, ok := toolMap["parameters"].(map[string]interface{}); ok {
			tool.Parameters = params
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// SendMCPRequest 向 MCP 服务器发送 JSON-RPC 请求
func (conn *JADXMCPConnection) SendMCPRequest(method string, params map[string]interface{}) (map[string]interface{}, error) {
	conn.mu.Lock()
	conn.lastRequestID++
	requestID := conn.lastRequestID
	baseURL := conn.baseURL
	client := conn.client
	conn.mu.Unlock()

	// 构建 JSON-RPC 请求
	conn.mu.RLock()
	sessionID := conn.sessionID
	conn.mu.RUnlock()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
	}

	// 合并参数
	finalParams := map[string]interface{}{}
	for k, v := range params {
		finalParams[k] = v
	}
	// 注意：FastMCP streamable-http 通过 Cookie 或请求头传递 session ID，不需要在 params 中
	// 如果需要，可以在非 initialize 方法中添加，但通常不需要

	if len(finalParams) > 0 {
		request["params"] = finalParams
	}

	reqData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	// 发送 HTTP POST 请求到 MCP 端点
	// FastMCP streamable-http 使用固定的 /mcp 端点
	url := strings.TrimSuffix(baseURL, "/")
	fullURL := url + "/mcp"

	log.Printf("[JADX MCP] 尝试连接: %s", fullURL)
	log.Printf("[JADX MCP] 请求方法: %s, 请求ID: %d, SessionID: %s", method, requestID, sessionID)
	log.Printf("[JADX MCP] 请求体: %s", string(reqData))

	req, err := http.NewRequest("POST", fullURL, bytes.NewBuffer(reqData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败 (%s): %v", fullURL, err)
	}

	req.Header.Set("Content-Type", "application/json")
	// FastMCP HTTP 传输需要同时接受 application/json 和 text/event-stream
	req.Header.Set("Accept", "application/json, text/event-stream")

	// FastMCP streamable-http 的 session ID 处理：
	// 1. initialize 请求：不发送 session ID，让服务器生成
	// 2. 后续请求：使用服务器返回的 session ID
	if method != "initialize" && sessionID != "" {
		// 只在非 initialize 请求中发送 session ID
		// 从响应头看，服务器使用 "Mcp-Session-Id"，所以在请求中也使用相同的名称
		req.Header.Set("Mcp-Session-Id", sessionID)
		// 同时尝试其他可能的名称以确保兼容性
		req.Header.Set("X-Session-ID", sessionID)
		req.Header.Set("Session-ID", sessionID)
		req.Header.Set("X-MCP-Session-ID", sessionID)
		req.Header.Set("X-FastMCP-Session-ID", sessionID)
		log.Printf("[JADX MCP] 发送 Session ID: %s (方法: %s)", sessionID, method)
	} else {
		log.Printf("[JADX MCP] initialize 请求，不发送 Session ID")
	}

	// 记录所有请求头用于调试（特别是 session 相关的）
	var reqHeaders []string
	for k, v := range req.Header {
		lowerK := strings.ToLower(k)
		if strings.Contains(lowerK, "session") || strings.Contains(lowerK, "cookie") {
			reqHeaders = append(reqHeaders, fmt.Sprintf("%s: %v", k, v))
		}
	}
	if len(reqHeaders) > 0 {
		log.Printf("[JADX MCP] Session/Cookie 相关请求头: %v", reqHeaders)
	}

	// 记录完整的请求头用于调试
	log.Printf("[JADX MCP] 完整请求头: %v", req.Header)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败 (%s): %v", fullURL, err)
	}
	defer resp.Body.Close()

	// 记录所有响应头用于调试
	log.Printf("[JADX MCP] 响应头: %v", resp.Header)
	log.Printf("[JADX MCP] 响应状态码: %d (URL: %s)", resp.StatusCode, fullURL)

	// 处理 502 Bad Gateway 错误（服务器可能崩溃或不可用）
	if resp.StatusCode == http.StatusBadGateway {
		log.Printf("[JADX MCP] 检测到 502 Bad Gateway 错误，尝试重新连接...")

		// 标记连接为断开
		conn.mu.Lock()
		conn.connected = false
		conn.mu.Unlock()

		// 检查服务器是否还在运行
		if !checkJADXServerRunning(baseURL) {
			log.Printf("[JADX MCP] 服务器未运行，尝试重新启动...")
			if err := startJADXServer(baseURL); err != nil {
				log.Printf("[JADX MCP] 重启服务器失败: %v", err)
			} else {
				// 等待服务器启动
				time.Sleep(2 * time.Second)
				if checkJADXServerRunning(baseURL) {
					log.Printf("[JADX MCP] 服务器重启成功，重新初始化连接...")
					// 重新初始化连接
					if initErr := conn.initialize(); initErr != nil {
						return nil, fmt.Errorf("服务器重启后重新初始化失败: %v", initErr)
					}
					// 重试原始请求
					log.Printf("[JADX MCP] 重试原始请求: %s", method)
					return conn.SendMCPRequest(method, params)
				}
			}
		} else {
			// 服务器还在运行，可能是连接问题，尝试重新初始化
			log.Printf("[JADX MCP] 服务器仍在运行，重新初始化连接...")
			if initErr := conn.initialize(); initErr != nil {
				return nil, fmt.Errorf("重新初始化连接失败: %v", initErr)
			}
			// 重试原始请求
			log.Printf("[JADX MCP] 重试原始请求: %s", method)
			return conn.SendMCPRequest(method, params)
		}

		// 如果重连失败，返回错误
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		return nil, fmt.Errorf("服务器返回 502 Bad Gateway，尝试重连后仍失败 (URL: %s): %s", fullURL, bodyStr)
	}

	// 检查响应头中的 Set-Cookie（FastMCP 可能通过 Cookie 设置 session ID）
	cookies := resp.Cookies()
	if len(cookies) > 0 {
		log.Printf("[JADX MCP] 收到 Cookies: %v", cookies)
		// 尝试从 Cookie 中提取 session ID
		for _, cookie := range cookies {
			log.Printf("[JADX MCP] Cookie: %s = %s", cookie.Name, cookie.Value)
			if cookie.Name == "session" || cookie.Name == "sessionId" ||
				cookie.Name == "mcp_session" || cookie.Name == "fastmcp_session" ||
				cookie.Name == "session_id" || cookie.Name == "mcp-session-id" {
				conn.mu.Lock()
				conn.sessionID = cookie.Value
				conn.mu.Unlock()
				log.Printf("[JADX MCP] 从 Cookie 获取到 Session ID: %s", cookie.Value)
			}
		}
	}

	// 检查响应头中是否有 session ID（优先级：Mcp-Session-Id > 其他）
	// 注意：必须在读取响应体之前提取，因为响应体读取后响应头可能失效
	sessionIDUpdated := false
	for headerName, headerValues := range resp.Header {
		// 优先检查 Mcp-Session-Id（服务器使用的标准名称）
		if headerName == "Mcp-Session-Id" || headerName == "MCP-Session-Id" || headerName == "mcp-session-id" {
			if len(headerValues) > 0 && headerValues[0] != "" {
				newSessionID := headerValues[0]
				conn.mu.Lock()
				oldSessionID := conn.sessionID
				conn.sessionID = newSessionID
				conn.mu.Unlock()
				log.Printf("[JADX MCP] 更新 Session ID: %s -> %s (从响应头 %s)", oldSessionID, newSessionID, headerName)
				sessionIDUpdated = true
			}
		}
	}

	// 如果没有找到 Mcp-Session-Id，尝试其他可能的名称
	if !sessionIDUpdated {
		for headerName, headerValues := range resp.Header {
			lowerName := strings.ToLower(headerName)
			if strings.Contains(lowerName, "session") && !sessionIDUpdated {
				log.Printf("[JADX MCP] 发现 Session 相关响应头: %s = %v", headerName, headerValues)
				if len(headerValues) > 0 && headerValues[0] != "" {
					conn.mu.Lock()
					conn.sessionID = headerValues[0]
					conn.mu.Unlock()
					log.Printf("[JADX MCP] 从响应头获取到 Session ID: %s", headerValues[0])
					sessionIDUpdated = true
				}
			}
		}
	}

	// 读取响应体
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	log.Printf("[JADX MCP] 响应状态码: %d (URL: %s)", resp.StatusCode, fullURL)
	log.Printf("[JADX MCP] 响应体长度: %d 字节", len(bodyStr))
	if len(bodyStr) > 500 {
		log.Printf("[JADX MCP] 响应体（前500字符）: %s...", bodyStr[:500])
	} else {
		log.Printf("[JADX MCP] 响应体: %s", bodyStr)
	}

	if resp.StatusCode == http.StatusOK {
		// FastMCP streamable-http 使用 SSE 格式（Server-Sent Events）
		// 格式: event: message\n\ndata: {...JSON...}\n\n
		// 需要提取 data: 后面的 JSON
		jsonData := bodyStr

		// 如果是 SSE 格式，提取 data 部分
		if strings.HasPrefix(bodyStr, "event:") || strings.Contains(bodyStr, "data:") {
			// 提取 data: 后面的 JSON
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

		if jsonData == "" {
			return nil, fmt.Errorf("响应体为空 (%s)", fullURL)
		}

		// 解析响应
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
			return nil, fmt.Errorf("解析响应失败 (%s): %v, 原始响应: %s, 提取的JSON: %s", fullURL, err, bodyStr, jsonData)
		}

		// 检查是否有错误
		if errVal, ok := response["error"]; ok && errVal != nil {
			errMap, _ := errVal.(map[string]interface{})
			errMsg := "未知错误"
			if msg, ok := errMap["message"].(string); ok {
				errMsg = msg
			}
			if code, ok := errMap["code"].(float64); ok {
				errMsg = fmt.Sprintf("错误代码 %.0f: %s", code, errMsg)
			}
			return nil, fmt.Errorf("MCP 服务器返回错误 (%s): %s", fullURL, errMsg)
		}

		log.Printf("[JADX MCP] 请求成功: %s", fullURL)
		return response, nil
	}

	// 处理错误响应
	if bodyStr == "" {
		bodyStr = "(响应体为空)"
	}

	if resp.StatusCode == http.StatusNotAcceptable {
		return nil, fmt.Errorf("请求格式不被接受 (406, URL: %s): %s。请检查 Accept 头设置", fullURL, bodyStr)
	} else if resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("请求格式错误 (400, URL: %s): %s", fullURL, bodyStr)
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("端点不存在 (404, URL: %s)", fullURL)
	} else if resp.StatusCode == http.StatusBadGateway {
		// 502 错误应该在上面已经处理过，这里作为兜底
		return nil, fmt.Errorf("服务器返回 502 Bad Gateway (URL: %s): %s", fullURL, bodyStr)
	} else {
		return nil, fmt.Errorf("服务器返回错误 (状态码 %d, URL: %s): %s", resp.StatusCode, fullURL, bodyStr)
	}
}

// CallTool 调用 MCP 工具
func (conn *JADXMCPConnection) CallTool(toolName string, arguments map[string]interface{}) (interface{}, error) {
	// 检查连接状态
	conn.mu.RLock()
	connected := conn.connected
	conn.mu.RUnlock()

	if !connected {
		log.Printf("[JADX MCP] 连接已断开，尝试重新初始化...")
		if err := conn.initialize(); err != nil {
			return nil, fmt.Errorf("重新初始化连接失败: %v", err)
		}
	}

	params := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	resp, err := conn.SendMCPRequest("tools/call", params)
	if err != nil {
		// 如果是 502 或其他连接相关错误，SendMCPRequest 已经尝试重连，这里只返回错误
		return nil, err
	}

	result, ok := resp["result"]
	if !ok {
		return nil, fmt.Errorf("响应中没有 result 字段")
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return result, nil
	}

	// 检查 structuredContent 中的错误（JADX MCP 服务器返回的错误格式）
	if structuredContent, ok := resultMap["structuredContent"].(map[string]interface{}); ok {
		if errorMsg, ok := structuredContent["error"].(string); ok && errorMsg != "" {
			// 检查是否是 JADX 插件连接错误（502）
			if strings.Contains(errorMsg, "502") {
				return nil, fmt.Errorf("JADX 插件连接失败 (502): 请确保 JADX GUI 正在运行，并且 JADX MCP 插件已启动（端口 8650）。错误详情: %s", errorMsg)
			}
			return nil, fmt.Errorf("工具执行错误: %s", errorMsg)
		}
	}

	// 提取内容或文本字段
	if content, ok := resultMap["content"].([]interface{}); ok && len(content) > 0 {
		if firstContent, ok := content[0].(map[string]interface{}); ok {
			if text, ok := firstContent["text"].(string); ok {
				// 检查文本内容中是否包含错误（JSON 格式的错误信息）
				if strings.Contains(text, "\"error\"") {
					var errorResult map[string]interface{}
					if err := json.Unmarshal([]byte(text), &errorResult); err == nil {
						if errorMsg, ok := errorResult["error"].(string); ok && errorMsg != "" {
							if strings.Contains(errorMsg, "502") {
								return nil, fmt.Errorf("JADX 插件连接失败 (502): 请确保 JADX GUI 正在运行，并且 JADX MCP 插件已启动（端口 8650）。错误详情: %s", errorMsg)
							}
							return nil, fmt.Errorf("工具执行错误: %s", errorMsg)
						}
					}
				}
				return text, nil
			}
		}
	}

	if text, ok := resultMap["text"].(string); ok {
		// 检查文本内容中是否包含错误
		if strings.Contains(text, "\"error\"") {
			var errorResult map[string]interface{}
			if err := json.Unmarshal([]byte(text), &errorResult); err == nil {
				if errorMsg, ok := errorResult["error"].(string); ok && errorMsg != "" {
					if strings.Contains(errorMsg, "502") {
						return nil, fmt.Errorf("JADX 插件连接失败 (502): 请确保 JADX GUI 正在运行，并且 JADX MCP 插件已启动（端口 8650）。错误详情: %s", errorMsg)
					}
					return nil, fmt.Errorf("工具执行错误: %s", errorMsg)
				}
			}
		}
		return text, nil
	}

	if isError, ok := resultMap["isError"].(bool); ok && isError {
		if errorMsg, ok := resultMap["text"].(string); ok {
			return nil, fmt.Errorf("工具执行错误: %s", errorMsg)
		}
		return nil, fmt.Errorf("工具执行错误")
	}

	return result, nil
}

// GetTools 获取工具列表（转换为 AI 工具格式）
func (conn *JADXMCPConnection) GetTools() []AITool {
	conn.mu.RLock()
	defer conn.mu.RUnlock()

	aiTools := make([]AITool, len(conn.tools))
	for i, tool := range conn.tools {
		aiTools[i] = AITool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
	}

	return aiTools
}

// IsConnected 检查连接状态
func (conn *JADXMCPConnection) IsConnected() bool {
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return conn.connected
}

// Close 关闭 MCP 连接
func (conn *JADXMCPConnection) Close() error {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	conn.connected = false
	conn.tools = []MCPTool{}
	return nil
}

// JADXChatSession JADX 聊天会话
type JADXChatSession struct {
	BaseChatSession
}

// GetOrCreateJADXSession 获取或创建 JADX 会话
func GetOrCreateJADXSession(sessionID string) *JADXChatSession {
	jadxSessionsMu.Lock()
	defer jadxSessionsMu.Unlock()

	if sessionID == "" {
		sessionID = fmt.Sprintf("jadx-%d", time.Now().UnixNano())
	}

	session, exists := jadxSessions[sessionID]
	if !exists {
		session = &JADXChatSession{
			BaseChatSession: BaseChatSession{
				ID:           sessionID,
				Messages:     []ChatMessage{},
				CreatedAt:    time.Now(),
				LastActivity: time.Now(),
			},
		}
		jadxSessions[sessionID] = session
	}

	session.UpdateLastActivity()
	return session
}

// CleanupOldJADXSessions 清理旧的会话
func CleanupOldJADXSessions(maxAge time.Duration) {
	jadxSessionsMu.Lock()
	defer jadxSessionsMu.Unlock()

	now := time.Now()
	for id, session := range jadxSessions {
		if now.Sub(session.GetLastActivity()) > maxAge {
			delete(jadxSessions, id)
		}
	}
}
