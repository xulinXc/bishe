package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StreamController 流式输出控制器
type StreamController struct {
	mu          sync.RWMutex
	aborted     bool
	abortChan   chan struct{}
	messageChan chan string
}

func NewStreamController() *StreamController {
	return &StreamController{
		aborted:     false,
		abortChan:   make(chan struct{}),
		messageChan: make(chan string, 100),
	}
}

func (sc *StreamController) Abort() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.aborted {
		sc.aborted = true
		close(sc.abortChan)
	}
}

func (sc *StreamController) IsAborted() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.aborted
}

func (sc *StreamController) Send(message string) bool {
	if sc.IsAborted() {
		return false
	}
	select {
	case sc.messageChan <- message:
		return true
	case <-sc.abortChan:
		return false
	default:
		return false
	}
}

// GetMessageChan 获取消息通道
// 返回一个只读的消息通道，用于从StreamController接收AI返回的消息片段
// 供外部调用者（如main.go中的aiAnalyzeHandler）使用
func (sc *StreamController) GetMessageChan() <-chan string {
	return sc.messageChan
}

// GetAbortChan 获取中止通道
// 返回一个只读的中止通道，用于接收AI处理完成或中止的信号
// 当通道关闭或收到信号时，表示AI处理已完成
func (sc *StreamController) GetAbortChan() <-chan struct{} {
	return sc.abortChan
}

// JADXChatStreamHandler 流式聊天处理器（SSE）
func JADXChatStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Method not allowed",
		})
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("解析请求失败: %v", err),
		})
		return
	}

	if req.Message == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "消息不能为空",
		})
		return
	}

	// 获取会话
	session := GetOrCreateJADXSession(req.SessionID)

	// 检查会话是否已配置
	session.mu.RLock()
	apiType := session.APIType
	apiKey := session.APIKey
	session.mu.RUnlock()

	if apiType == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "会话未配置，请先配置 API Key 和 API 类型",
		})
		return
	}

	if apiKey == "" && apiType != "ollama" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("API Key 未设置（API 类型: %s）", apiType),
		})
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 创建流式控制器
	controller := NewStreamController()
	defer controller.Abort()

	// 添加用户消息到会话
	session.AddMessage(ChatMessage{
		Role:    "user",
		Content: req.Message,
		Time:    time.Now().Format(time.RFC3339),
	})
	session.UpdateLastActivity()

	// 所有 AI 提供商都使用流式输出
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[JADX Chat Stream] Panic: %v", r)
				controller.Send(jsonEscape(fmt.Sprintf("\n\n[错误] %v", r)))
			}
		}()

		if err := processChatStream(session, req.Message, controller); err != nil {
			log.Printf("[JADX Chat Stream] 流式处理失败: %v", err)
			controller.Send(jsonEscape(fmt.Sprintf("\n\n[错误] %v", err)))
		}
	}()

	// 发送流式数据
	for {
		select {
		case msg := <-controller.messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-controller.abortChan:
			return
		case <-r.Context().Done():
			controller.Abort()
			return
		}
	}
}

// processChatStream 处理所有 AI 提供商的流式输出
func processChatStream(session ChatSession, userMessage string, controller *StreamController) error {
	// 获取消息历史（与 chat.go 保持一致，保留包含 tool_calls 的 assistant 消息后的匹配 tool 消息）
	allMessages := session.GetMessages()
	messages := make([]ChatMessage, 0)

	for i := 0; i < len(allMessages); i++ {
		msg := allMessages[i]

		// 总是保留非 tool 消息
		if msg.Role != "tool" {
			messages = append(messages, msg)

			// 如果这是一个包含 tool_calls 的 assistant 消息，保留它后面的匹配 tool 消息
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				// 收集所有需要的 tool_call_id
				requiredToolCallIDs := make(map[string]bool)
				for _, tc := range msg.ToolCalls {
					requiredToolCallIDs[tc.ID] = true
				}

				// 检查并添加紧跟在后的匹配 tool 消息
				toolMessagesAdded := 0
				for j := i + 1; j < len(allMessages) && toolMessagesAdded < len(msg.ToolCalls); j++ {
					if allMessages[j].Role == "tool" {
						// 检查 tool_call_id 是否匹配
						if requiredToolCallIDs[allMessages[j].ToolCallID] {
							messages = append(messages, allMessages[j])
							toolMessagesAdded++
							delete(requiredToolCallIDs, allMessages[j].ToolCallID)
						}
					} else {
						// 遇到非 tool 消息，停止添加
						break
					}
				}

				// 如果 tool 消息数量不足，移除 assistant 消息的 tool_calls（避免 API 错误）
				if toolMessagesAdded < len(msg.ToolCalls) {
					log.Printf("[MCP Chat Stream] 警告：assistant 消息有 %d 个 tool_calls，但只找到 %d 个匹配的 tool 消息，将移除 tool_calls", len(msg.ToolCalls), toolMessagesAdded)
					// 创建不包含 tool_calls 的 assistant 消息副本
					messages[len(messages)-1] = ChatMessage{
						Role:    msg.Role,
						Content: msg.Content,
						Time:    msg.Time,
						// 不包含 ToolCalls
					}
				}

				// 跳过已经添加的 tool 消息
				i += toolMessagesAdded
			}
		}
		// 跳过孤立的 tool 消息（不在 assistant 之后的）
	}

	mcpConn := session.GetMCPConnection()
	apiKey := session.GetAPIKey()
	apiType := session.GetAPIType()

	// 获取 MCP 工具列表
	var tools []AITool
	if mcpConn != nil && mcpConn.IsConnected() {
		tools = mcpConn.GetTools()
		log.Printf("[MCP Chat Stream] 为 AI 提供 %d 个 MCP 工具", len(tools))
	}

	// 根据 API 类型创建对应的 provider
	var aiProvider interface {
		ChatStream(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error)
	}

	switch apiType {
	case "ollama":
		// 解析 Ollama 配置
		baseURL := apiKey
		parts := strings.Split(baseURL, "|")
		ollamaURL := "http://localhost:11434"
		model := "gpt-oss:20b"
		if len(parts) >= 1 && parts[0] != "" {
			ollamaURL = parts[0]
		}
		if len(parts) >= 2 && parts[1] != "" {
			model = parts[1]
		}
		aiProvider = NewOllamaProvider(ollamaURL, model)
	case "deepseek":
		// 解析 DeepSeek 配置
		parts := strings.Split(apiKey, "|")
		var model string
		var actualAPIKey string
		if len(parts) == 2 {
			model = parts[0]
			actualAPIKey = parts[1]
		} else {
			model = "deepseek-chat"
			actualAPIKey = apiKey
		}
		provider := NewDeepSeekProvider(actualAPIKey)
		provider.Model = model
		aiProvider = provider
	case "openai":
		aiProvider = NewOpenAIProvider(apiKey)
	default:
		return fmt.Errorf("不支持的 API 类型: %s", apiType)
	}

	// 限制最大迭代次数，避免无限循环（与 chat.go 保持一致）
	maxIterations := 10
	iteration := 0
	var lastSavedContent strings.Builder // 用于跟踪已保存的内容

	for iteration < maxIterations {
		iteration++

		// 流式调用 AI
		fullResponse, toolCalls, err := aiProvider.ChatStream(messages, tools, controller)

		// 检查是否被中止
		isAborted := controller != nil && controller.IsAborted()

		// 即使被中止或有错误，如果已经收到部分内容，也要保存
		if fullResponse != "" || len(toolCalls) > 0 || (isAborted && lastSavedContent.Len() > 0) {
			contentToSave := fullResponse
			// 如果被中止且当前响应为空，尝试使用上次保存的内容
			if contentToSave == "" && isAborted && lastSavedContent.Len() > 0 {
				contentToSave = lastSavedContent.String()
			}

			// 保存到会话（即使内容为空，如果有 tool_calls 也要保存）
			if contentToSave != "" || len(toolCalls) > 0 {
				// 记录已保存的内容
				if contentToSave != "" {
					lastSavedContent.Reset()
					lastSavedContent.WriteString(contentToSave)
				}

				session.AddMessage(ChatMessage{
					Role:      "assistant",
					Content:   contentToSave,
					Time:      time.Now().Format(time.RFC3339),
					ToolCalls: toolCalls, // 保存 tool_calls，确保在下一轮发送时包含
				})

				// 如果被中止，记录日志但不返回错误（保留已收到的内容）
				if isAborted {
					log.Printf("[MCP Chat Stream] 流被中止，但已保存部分响应（长度: %d）", len(contentToSave))
					return nil // 正常返回，不返回错误
				}
			}
		}

		// 如果有错误且没有被中止，返回错误
		if err != nil {
			return fmt.Errorf("AI 流式调用失败: %v", err)
		}

		// 如果被中止但没有内容，也正常返回（不返回错误）
		if isAborted {
			return nil
		}

		// 如果没有工具调用，返回
		if len(toolCalls) == 0 {
			return nil
		}

		// 处理工具调用
		if mcpConn == nil || !mcpConn.IsConnected() {
			log.Printf("[MCP Chat Stream] MCP 连接不可用，无法执行工具调用")
			controller.Send(jsonEscape("\n\n[错误] MCP 连接不可用，无法执行工具调用"))
			return fmt.Errorf("MCP 连接不可用，无法执行工具调用")
		}

		// 为每个工具调用创建单独的消息，包含对应的 tool_call_id
		for _, toolCall := range toolCalls {
			log.Printf("[MCP Chat Stream] 调用工具: %s", toolCall.Name)

			var toolMessage string
			result, err := mcpConn.CallTool(toolCall.Name, toolCall.Arguments)
			if err != nil {
				// 检查是否是 JADX 插件连接错误（502），给出更友好的提示
				errMsg := err.Error()
				if strings.Contains(errMsg, "502") || strings.Contains(errMsg, "JADX 插件连接失败") {
					toolMessage = fmt.Sprintf("工具 %s 执行失败: %s\n\n[提示] 请检查：\n1. JADX GUI 是否正在运行\n2. 是否已加载 APK 文件\n3. JADX MCP 插件是否已启动（Tools -> AI Assistant -> MCP Server）", toolCall.Name, errMsg)
				} else {
					toolMessage = fmt.Sprintf("工具 %s 执行失败: %v", toolCall.Name, err)
				}
			} else {
				var resultStr string
				switch v := result.(type) {
				case string:
					resultStr = v
				default:
					if jsonBytes, err := json.Marshal(v); err == nil {
						resultStr = string(jsonBytes)
					} else {
						resultStr = fmt.Sprintf("%v", v)
					}
				}
				toolMessage = fmt.Sprintf("工具 %s 返回: %s", toolCall.Name, resultStr)
			}

			// 发送工具调用结果到流
			controller.Send(jsonEscape(fmt.Sprintf("\n\n[工具调用结果]\n%s", toolMessage)))

			// 为每个工具调用创建单独的消息，包含对应的 tool_call_id
			session.AddMessage(ChatMessage{
				Role:       "tool",
				Content:    toolMessage,
				Time:       time.Now().Format(time.RFC3339),
				ToolCallID: toolCall.ID, // 每个工具调用使用对应的 ID
			})
		}

		// 更新消息列表，准备下一轮迭代
		messages = session.GetMessages()
		// 继续循环，让 AI 处理工具结果
	}

	// 如果达到最大迭代次数，提示用户
	log.Printf("[MCP Chat Stream] 达到最大迭代次数 (%d)，可能陷入循环", maxIterations)
	controller.Send(jsonEscape(fmt.Sprintf("\n\n[提示] 已达到最大迭代次数 (%d)，停止处理。", maxIterations)))
	return nil
}

// jsonEscape 转义 JSON 字符串
func jsonEscape(s string) string {
	bytes, _ := json.Marshal(s)
	return string(bytes)
}
