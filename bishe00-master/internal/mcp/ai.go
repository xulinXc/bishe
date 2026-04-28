// Package mcp AI Provider实现
// 该文件实现了多种AI提供商的接口，包括OpenAI、DeepSeek、Anthropic、Ollama
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// LogAIInteraction 记录 AI 交互日志
// previousMsgCount 参数用于标识之前已打印的消息数量，避免重复打印
// 注意：如果不传递 previousMsgCount，则默认只打印消息摘要（不打印完整内容）
func LogAIInteraction(provider, model string, messages []ChatMessage, response string, toolCalls []AIToolCall, err error, previousMsgCount ...int) {
	showFull := len(previousMsgCount) > 0 && previousMsgCount[0] >= 0

	log.Println("========== AI Interaction Log ==========")
	log.Printf("Time: %s\n", time.Now().Format(time.RFC3339))
	log.Printf("Provider: %s | Model: %s | Messages: %d\n", provider, model, len(messages))

	if !showFull {
		log.Println("--- Messages (abbreviated) ---")
		for i, msg := range messages {
			roleStr := msg.Role
			if len(msg.Content) > 100 {
				log.Printf("[%d] %s (len:%d)", i, roleStr, len(msg.Content))
			} else {
				log.Printf("[%d] %s: %s", i, roleStr, msg.Content)
			}
		}
	} else {
		prevCount := previousMsgCount[0]
		log.Println("---------- Request Messages ----------")

		// 只打印新增的消息
		for i := prevCount; i < len(messages); i++ {
			msg := messages[i]
			content := msg.Content

			// 对 system prompt 进行特殊处理
			if msg.Role == "system" {
				if len(content) > 200 {
					content = content[:200] + "...[已截断]"
				}
				log.Printf("[%d] Role: system (len:%d)\nContent: %s\n", i, len(messages[i].Content), content)
				continue
			}

			// 对 tool 消息进行压缩显示
			if msg.Role == "tool" {
				if len(content) > 300 {
					content = content[:300] + "...[已截断]"
				}
				log.Printf("[%d] Role: tool\nContent: %s\n", i, content)
				continue
			}

			// 其他消息
			if len(content) > 500 {
				content = content[:500] + "...[已截断]"
			}
			log.Printf("[%d] Role: %s\nContent: %s\n", i, msg.Role, content)

			if len(msg.ToolCalls) > 0 {
				log.Printf("ToolCalls: %+v\n", msg.ToolCalls)
			}
		}

		if prevCount > 0 {
			log.Printf("[... 省略了前 %d 条消息 ...]\n", prevCount)
		}
	}

	log.Println("---------- Response ----------")
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		if showFull {
			content := response
			if len(content) > 1000 {
				content = content[:1000] + "...(truncated)"
			}
			log.Printf("Content: %s\n", content)
			if len(toolCalls) > 0 {
				log.Printf("ToolCalls: %+v\n", toolCalls)
			}
		} else {
			if len(response) > 200 {
				log.Printf("Response (len:%d): %s...(truncated)", len(response), response[:200])
			} else {
				log.Printf("Response (len:%d): %s", len(response), response)
			}
			if len(toolCalls) > 0 {
				log.Printf("ToolCalls: %d", len(toolCalls))
			}
		}
	}
	log.Println("========================================")
}

// ChatMessage 聊天消息结构体
// 用于表示AI聊天中的单条消息
type ChatMessage struct {
	Role       string       `json:"role"`                   // 消息角色："user"（用户）、"assistant"（AI助手）、"tool"（工具调用结果）
	Content    string       `json:"content"`                // 消息内容
	Time       string       `json:"time"`                   // RFC3339格式的时间戳
	ToolCallID string       `json:"tool_call_id,omitempty"` // tool消息必须的字段（DeepSeek/OpenAI要求，用于关联工具调用）
	ToolCalls  []AIToolCall `json:"tool_calls,omitempty"`   // assistant消息的tool_calls（DeepSeek/OpenAI要求，当AI决定调用工具时）
}

// AIProvider AI提供商接口
// 定义了AI服务的基本交互方法
type AIProvider interface {
	// Chat 发送聊天消息并获取AI响应
	// @param messages 消息列表
	// @param tools 可用的工具列表（用于函数调用）
	// @return 响应内容、工具调用列表（如果有）、错误信息
	Chat(messages []ChatMessage, tools []AITool) (string, []AIToolCall, error)
}

// AITool AI工具定义（用于函数调用）
// 定义AI可以调用的工具/函数
type AITool struct {
	Type        string                 `json:"type"`        // 工具类型，通常为"function"
	Name        string                 `json:"name"`        // 工具名称
	Description string                 `json:"description"` // 工具描述（AI用此描述决定是否调用）
	Parameters  map[string]interface{} `json:"parameters"`  // 工具参数的JSON Schema
}

// AIToolCall AI工具调用
// 表示AI决定调用某个工具时的调用信息
type AIToolCall struct {
	ID        string                 `json:"id"`        // 工具调用ID（用于关联工具返回结果）
	Name      string                 `json:"name"`      // 要调用的工具名称
	Arguments map[string]interface{} `json:"arguments"` // 工具调用的参数
}

// OpenAIProvider OpenAI API实现
// 实现了OpenAI的Chat Completions API
type OpenAIProvider struct {
	APIKey    string        // OpenAI API密钥
	BaseURL   string        // API基础URL（默认为 https://api.openai.com/v1，可用于代理）
	Model     string        // 使用的模型（默认为 gpt-4o）
	MaxTokens int           // 最大生成token数（默认为 4096）
	Timeout   time.Duration // HTTP客户端超时时间（默认120秒）
}

func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:    apiKey,
		BaseURL:   "https://api.openai.com/v1",
		Model:     "gpt-4o",
		MaxTokens: 4096,
		Timeout:   120 * time.Second,
	}
}

// SetTimeout 设置HTTP客户端超时时间
func (p *OpenAIProvider) SetTimeout(timeout time.Duration) {
	p.Timeout = timeout
}

func (p *OpenAIProvider) Chat(messages []ChatMessage, tools []AITool) (string, []AIToolCall, error) {
	content, toolCalls, err := func() (string, []AIToolCall, error) {
		// 转换消息格式
		// 注意：tool 消息必须包含 tool_call_id 字段
		// assistant 消息如果有 tool_calls，也必须包含 tool_calls 字段
		msgs := make([]map[string]interface{}, len(messages))
		for i, msg := range messages {
			msgMap := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
			// 如果是 tool 消息，必须添加 tool_call_id（OpenAI API 要求）
			if msg.Role == "tool" && msg.ToolCallID != "" {
				msgMap["tool_call_id"] = msg.ToolCallID
			}
			// 如果是 assistant 消息且有 tool_calls，必须添加 tool_calls 字段
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				toolCallsArray := make([]map[string]interface{}, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					// 将 Arguments map 序列化为 JSON 字符串
					argsJSON, err := json.Marshal(tc.Arguments)
					if err != nil {
						argsJSON = []byte("{}")
					}
					toolCallsArray[j] = map[string]interface{}{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      tc.Name,
							"arguments": string(argsJSON),
						},
					}
				}
				msgMap["tool_calls"] = toolCallsArray
			}
			msgs[i] = msgMap
		}

		// 构建请求体
		reqBody := map[string]interface{}{
			"model":      p.Model,
			"messages":   msgs,
			"max_tokens": p.MaxTokens,
		}

		// 如果有工具，添加 tools 参数
		if len(tools) > 0 {
			toolsArray := make([]map[string]interface{}, len(tools))
			for i, tool := range tools {
				toolsArray[i] = map[string]interface{}{
					"type": tool.Type,
					"function": map[string]interface{}{
						"name":        tool.Name,
						"description": tool.Description,
						"parameters":  tool.Parameters,
					},
				}
			}
			reqBody["tools"] = toolsArray
			reqBody["tool_choice"] = "auto"
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", nil, fmt.Errorf("序列化请求失败: %v", err)
		}

		// 发送请求
		url := fmt.Sprintf("%s/chat/completions", p.BaseURL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.APIKey))

		timeout := p.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("请求失败: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
		}

		// 解析响应
		var result struct {
			Choices []struct {
				Message struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"` // JSON 字符串
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", nil, fmt.Errorf("解析响应失败: %v", err)
		}

		if len(result.Choices) == 0 {
			return "", nil, fmt.Errorf("响应中没有 choices")
		}

		msg := result.Choices[0].Message
		content := msg.Content

		// 解析工具调用
		toolCalls := make([]AIToolCall, 0)
		for _, tc := range msg.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				// 如果解析失败，创建一个空的 map
				args = make(map[string]interface{})
			}

			toolCalls = append(toolCalls, AIToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}

		return content, toolCalls, nil
	}()

	LogAIInteraction("OpenAI", p.Model, messages, content, toolCalls, err)
	return content, toolCalls, err
}

// DeepSeekProvider DeepSeek API 实现
type DeepSeekProvider struct {
	APIKey    string
	BaseURL   string        // 默认为 https://api.deepseek.com
	Model     string        // 默认为 deepseek-chat（对应 DeepSeek-V3.2-Exp 非思考模式）
	MaxTokens int           // 默认为 4096
	Timeout   time.Duration // HTTP客户端超时时间（默认120秒）
}

func NewDeepSeekProvider(apiKey string) *DeepSeekProvider {
	return &DeepSeekProvider{
		APIKey:    apiKey,
		BaseURL:   "https://api.deepseek.com",
		Model:     "deepseek-chat", // 使用正确的模型名称（对应 DeepSeek-V3.2-Exp 非思考模式）
		MaxTokens: 4096,
		Timeout:   120 * time.Second,
	}
}

// SetTimeout 设置HTTP客户端超时时间
func (p *DeepSeekProvider) SetTimeout(timeout time.Duration) {
	p.Timeout = timeout
}

func (p *DeepSeekProvider) Chat(messages []ChatMessage, tools []AITool) (string, []AIToolCall, error) {
	content, toolCalls, err := func() (string, []AIToolCall, error) {
		// 转换消息格式（DeepSeek 使用类似 OpenAI 的格式）
		// 注意：tool 消息必须包含 tool_call_id 字段
		// assistant 消息如果有 tool_calls，也必须包含 tool_calls 字段
		msgs := make([]map[string]interface{}, len(messages))
		for i, msg := range messages {
			msgMap := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
			// 如果是 tool 消息，必须添加 tool_call_id（DeepSeek API 要求）
			if msg.Role == "tool" && msg.ToolCallID != "" {
				msgMap["tool_call_id"] = msg.ToolCallID
			}
			// 如果是 assistant 消息且有 tool_calls，必须添加 tool_calls 字段
			// 这样 tool 消息才能正确关联到前面的 assistant 消息
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				toolCallsArray := make([]map[string]interface{}, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					// 将 Arguments map 序列化为 JSON 字符串
					argsJSON, err := json.Marshal(tc.Arguments)
					if err != nil {
						argsJSON = []byte("{}")
					}
					toolCallsArray[j] = map[string]interface{}{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      tc.Name,
							"arguments": string(argsJSON),
						},
					}
				}
				msgMap["tool_calls"] = toolCallsArray
			}
			msgs[i] = msgMap
		}

		// 构建请求体（DeepSeek 完全兼容 OpenAI 格式）
		// 参考文档：https://api-docs.deepseek.com/zh-cn/
		reqBody := map[string]interface{}{
			"model":       p.Model,
			"messages":    msgs,
			"max_tokens":  p.MaxTokens,
			"temperature": 0.7,   // DeepSeek 支持此参数
			"stream":      false, // 明确设置为非流式（文档要求）
		}

		// 如果有工具，添加 tools 参数
		if len(tools) > 0 {
			toolsArray := make([]map[string]interface{}, len(tools))
			for i, tool := range tools {
				toolsArray[i] = map[string]interface{}{
					"type": tool.Type,
					"function": map[string]interface{}{
						"name":        tool.Name,
						"description": tool.Description,
						"parameters":  tool.Parameters,
					},
				}
			}
			reqBody["tools"] = toolsArray
			reqBody["tool_choice"] = "auto"
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", nil, fmt.Errorf("序列化请求失败: %v", err)
		}

		// 发送请求（DeepSeek API 端点为 /chat/completions，不是 /v1/chat/completions）
		url := fmt.Sprintf("%s/chat/completions", p.BaseURL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.APIKey))

		timeout := p.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("请求失败: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", nil, fmt.Errorf("DeepSeek API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
		}

		// 解析响应（DeepSeek 响应格式与 OpenAI 完全相同）
		var result struct {
			Choices []struct {
				Message struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"` // JSON 字符串
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", nil, fmt.Errorf("DeepSeek API 解析响应失败: %v", err)
		}

		// 检查 API 返回的错误
		if result.Error != nil {
			return "", nil, fmt.Errorf("DeepSeek API 错误: %s (类型: %s)", result.Error.Message, result.Error.Type)
		}

		if len(result.Choices) == 0 {
			return "", nil, fmt.Errorf("DeepSeek API 响应中没有 choices")
		}

		msg := result.Choices[0].Message
		content := msg.Content

		// 解析工具调用
		toolCalls := make([]AIToolCall, 0)
		for _, tc := range msg.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				// 如果解析失败，创建一个空的 map
				args = make(map[string]interface{})
			}

			toolCalls = append(toolCalls, AIToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}

		return content, toolCalls, nil
	}()

	LogAIInteraction("DeepSeek", p.Model, messages, content, toolCalls, err)
	return content, toolCalls, err
}

// AnthropicProvider Anthropic (Claude) API 实现
type AnthropicProvider struct {
	APIKey    string
	BaseURL   string        // 默认为 https://api.anthropic.com
	Model     string        // 默认为 claude-3-5-sonnet-20241022
	MaxTokens int           // 默认为 4096
	Timeout   time.Duration // HTTP客户端超时时间（默认120秒）
}

func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey:    apiKey,
		BaseURL:   "https://api.anthropic.com",
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 4096,
		Timeout:   120 * time.Second,
	}
}

// SetTimeout 设置HTTP客户端超时时间
func (p *AnthropicProvider) SetTimeout(timeout time.Duration) {
	p.Timeout = timeout
}

func (p *AnthropicProvider) Chat(messages []ChatMessage, tools []AITool) (string, []AIToolCall, error) {
	content, toolCalls, err := func() (string, []AIToolCall, error) {
		// 转换消息格式（Anthropic 使用不同的格式）
		msgs := make([]map[string]interface{}, 0)
		for _, msg := range messages {
			msgs = append(msgs, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}

		// 构建请求体
		reqBody := map[string]interface{}{
			"model":      p.Model,
			"max_tokens": p.MaxTokens,
			"messages":   msgs,
		}

		// 如果有工具，添加 tools 参数
		if len(tools) > 0 {
			toolsArray := make([]map[string]interface{}, len(tools))
			for i, tool := range tools {
				toolsArray[i] = map[string]interface{}{
					"name":         tool.Name,
					"description":  tool.Description,
					"input_schema": tool.Parameters,
				}
			}
			reqBody["tools"] = toolsArray
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", nil, fmt.Errorf("序列化请求失败: %v", err)
		}

		// 发送请求
		url := fmt.Sprintf("%s/v1/messages", p.BaseURL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", p.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		timeout := p.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("请求失败: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
		}

		// 解析响应
		var result struct {
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text,omitempty"`
				ToolUse *struct {
					ID    string                 `json:"id"`
					Name  string                 `json:"name"`
					Input map[string]interface{} `json:"input"`
				} `json:"tool_use,omitempty"`
			} `json:"content"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", nil, fmt.Errorf("解析响应失败: %v", err)
		}

		// 提取文本内容和工具调用
		var contentBuilder strings.Builder
		toolCalls := make([]AIToolCall, 0)

		for _, item := range result.Content {
			if item.Type == "text" {
				if contentBuilder.Len() > 0 {
					contentBuilder.WriteString("\n")
				}
				contentBuilder.WriteString(item.Text)
			} else if item.Type == "tool_use" && item.ToolUse != nil {
				toolCalls = append(toolCalls, AIToolCall{
					ID:        item.ToolUse.ID,
					Name:      item.ToolUse.Name,
					Arguments: item.ToolUse.Input,
				})
			}
		}

		return contentBuilder.String(), toolCalls, nil
	}()

	LogAIInteraction("Anthropic", p.Model, messages, content, toolCalls, err)
	return content, toolCalls, err
}

// OllamaProvider Ollama 本地 AI API 实现
type OllamaProvider struct {
	APIKey    string        // Ollama 通常不需要 API Key，但保留字段以便扩展
	BaseURL   string        // 默认为 http://localhost:11434
	Model     string        // 模型名称，如 "llama2", "mistral", "codellama" 等
	MaxTokens int           // 默认为 4096
	Timeout   time.Duration // HTTP客户端超时时间（默认300秒）
}

func NewOllamaProvider(baseURL string, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434" // Ollama 默认端口
	}
	if model == "" {
		model = "gpt-oss:20b" // 默认模型
	}
	return &OllamaProvider{
		APIKey:    "",
		BaseURL:   baseURL,
		Model:     model,
		MaxTokens: 4096,
		Timeout:   300 * time.Second,
	}
}

// SetTimeout 设置HTTP客户端超时时间
func (p *OllamaProvider) SetTimeout(timeout time.Duration) {
	p.Timeout = timeout
}

func (p *OllamaProvider) Chat(messages []ChatMessage, tools []AITool) (string, []AIToolCall, error) {
	content, toolCalls, err := func() (string, []AIToolCall, error) {
		// 转换消息格式（Ollama 使用类似 OpenAI 的格式）
		// 注意：Ollama 可能也支持 tool_call_id 和 tool_calls，但通常不强制要求
		msgs := make([]map[string]interface{}, len(messages))
		for i, msg := range messages {
			msgMap := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
			// 如果 Ollama 支持，添加 tool_call_id
			if msg.Role == "tool" && msg.ToolCallID != "" {
				msgMap["tool_call_id"] = msg.ToolCallID
			}
			// 如果是 assistant 消息且有 tool_calls，添加 tool_calls 字段（兼容 OpenAI 格式）
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				toolCallsArray := make([]map[string]interface{}, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					// 将 Arguments map 序列化为 JSON 字符串
					argsJSON, err := json.Marshal(tc.Arguments)
					if err != nil {
						argsJSON = []byte("{}")
					}
					toolCallsArray[j] = map[string]interface{}{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      tc.Name,
							"arguments": string(argsJSON),
						},
					}
				}
				msgMap["tool_calls"] = toolCallsArray
			}
			msgs[i] = msgMap
		}

		// 构建请求体
		reqBody := map[string]interface{}{
			"model":    p.Model,
			"messages": msgs,
			"stream":   false, // 关闭流式输出，一次性返回
		}

		// Ollama 的 tools 支持（如果模型支持）
		// 注意：不是所有 Ollama 模型都支持 function calling
		if len(tools) > 0 {
			toolsArray := make([]map[string]interface{}, len(tools))
			for i, tool := range tools {
				toolsArray[i] = map[string]interface{}{
					"type": tool.Type,
					"function": map[string]interface{}{
						"name":        tool.Name,
						"description": tool.Description,
						"parameters":  tool.Parameters,
					},
				}
			}
			reqBody["tools"] = toolsArray
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", nil, fmt.Errorf("序列化请求失败: %v", err)
		}

		// 发送请求到 Ollama
		url := fmt.Sprintf("%s/api/chat", p.BaseURL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		// Ollama 不需要 Authorization header（除非配置了认证）

		timeout := p.Timeout
		if timeout == 0 {
			timeout = 300 * time.Second
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("请求失败: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
		}

		// 解析响应
		var result struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			} `json:"message"`
			Done bool `json:"done"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", nil, fmt.Errorf("解析响应失败: %v", err)
		}

		content := result.Message.Content

		// 解析工具调用（如果 Ollama 模型支持）
		toolCalls := make([]AIToolCall, 0)
		for _, tc := range result.Message.ToolCalls {
			toolCalls = append(toolCalls, AIToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		return content, toolCalls, nil
	}()

	LogAIInteraction("Ollama", p.Model, messages, content, toolCalls, err)
	return content, toolCalls, err
}
