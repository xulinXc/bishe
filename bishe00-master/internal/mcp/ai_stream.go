package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Package mcp 流式AI聊天功能
// 该文件实现了支持流式输出的AI聊天功能，可以实时返回AI的响应内容

// ChatStreamProvider 流式聊天接口
// 定义了支持流式输出的AI聊天方法
type ChatStreamProvider interface {
	// ChatStream 流式聊天
	// @param messages 消息列表
	// @param tools 可用的工具列表
	// @param controller 流控制器（用于控制流式输出的中止和消息传递）
	// @return 完整响应内容、工具调用列表（如果有）、错误信息
	ChatStream(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error)
}

// ChatStream Ollama流式聊天实现
// 实现Ollama的流式聊天API，实时返回AI响应
func (p *OllamaProvider) ChatStream(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error) {
	// 转换消息格式
	msgs := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		msgs[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// 构建请求体（启用流式）
	reqBody := map[string]interface{}{
		"model":    p.Model,
		"messages": msgs,
		"stream":   true, // 启用流式输出
	}

	// 添加 tools 支持
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
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{} // 流式请求不使用超时
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	// 流式解析响应
	var fullContent strings.Builder
	var toolCalls []AIToolCall

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 增加缓冲区大小

	lineCount := 0
	for scanner.Scan() {
		// 检查是否已中止
		if controller != nil && controller.IsAborted() {
			return fullContent.String(), toolCalls, nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		lineCount++

		// 解析 JSON 行
		var chunk struct {
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

		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			// 记录解析错误但不中断
			// fmt.Printf("[Ollama Stream] 解析错误: %v, 行: %s\n", err, line)
			continue
		}

		// 添加内容片段（包括增量内容）
		if chunk.Message.Content != "" {
			content := chunk.Message.Content
			fullContent.WriteString(content)
			// 立即发送到控制器，让前端实时显示
			if controller != nil && !controller.IsAborted() {
				// 转义 JSON（只转义内容字符串）
				escaped, err := json.Marshal(content)
				if err == nil {
					// 发送 JSON 字符串（已经是转义的）
					success := controller.Send(string(escaped))
					if !success {
						// 如果发送失败（可能因为已中止），停止处理
						break
					}
				}
			}
		}

		// 解析工具调用
		for _, tc := range chunk.Message.ToolCalls {
			toolCalls = append(toolCalls, AIToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		// 如果完成，退出
		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("读取流失败: %v", err)
	}

	return fullContent.String(), toolCalls, nil
}

// DeepSeek ChatStream 流式聊天
func (p *DeepSeekProvider) ChatStream(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error) {
	return p.streamChat(messages, tools, controller)
}

// OpenAI ChatStream 流式聊天
func (p *OpenAIProvider) ChatStream(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error) {
	return p.streamChat(messages, tools, controller)
}

// streamChat 通用流式聊天实现（用于 DeepSeek 和 OpenAI）
func streamChatCommon(messages []ChatMessage, tools []AITool, controller *StreamController, baseURL, apiKey, model string, isDeepSeek bool) (string, []AIToolCall, error) {
	// 转换消息格式（与 Chat 方法一致）
	msgs := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		msgMap := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		// 如果是 tool 消息，必须添加 tool_call_id
		if msg.Role == "tool" && msg.ToolCallID != "" {
			msgMap["tool_call_id"] = msg.ToolCallID
		}
		// 如果是 assistant 消息且有 tool_calls，必须添加 tool_calls 字段
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCallsArray := make([]map[string]interface{}, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
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

	// 构建请求体（启用流式）
	reqBody := map[string]interface{}{
		"model":    model,
		"messages": msgs,
		"stream":   true, // 启用流式输出
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

	// 构建 URL
	var url string
	if isDeepSeek {
		url = fmt.Sprintf("%s/chat/completions", baseURL)
	} else {
		url = fmt.Sprintf("%s/v1/chat/completions", baseURL)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{} // 流式请求不使用超时
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("API 返回错误 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	// 流式解析响应（OpenAI/DeepSeek 使用 SSE 格式）
	var fullContent strings.Builder
	var toolCalls []AIToolCall
	toolCallsMap := make(map[string]AIToolCall) // 用于去重

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		// 检查是否已中止
		if controller != nil && controller.IsAborted() {
			return fullContent.String(), toolCalls, nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		// 移除 "data: " 前缀
		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		// 解析 JSON
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
						Index *int `json:"index"`
					} `json:"tool_calls,omitempty"`
				} `json:"delta"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// 处理内容增量
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			// 立即发送到控制器
			if controller != nil && !controller.IsAborted() {
				escaped, err := json.Marshal(delta.Content)
				if err == nil {
					if !controller.Send(string(escaped)) {
						break
					}
				}
			}
		}

		// 处理工具调用增量
		for _, tc := range delta.ToolCalls {
			if tc.ID == "" {
				continue
			}

			// 检查是否已存在
			if existing, ok := toolCallsMap[tc.ID]; ok {
				// 更新现有工具调用（合并 arguments）
				if tc.Function.Arguments != "" {
					var newArgs map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &newArgs); err == nil {
						// 合并参数（新参数覆盖旧参数）
						for k, v := range newArgs {
							existing.Arguments[k] = v
						}
					}
				}
				if tc.Function.Name != "" {
					existing.Name = tc.Function.Name
				}
				toolCallsMap[tc.ID] = existing
			} else {
				// 创建新的工具调用
				var args map[string]interface{}
				if tc.Function.Arguments != "" {
					json.Unmarshal([]byte(tc.Function.Arguments), &args)
				} else {
					args = make(map[string]interface{})
				}
				toolCallsMap[tc.ID] = AIToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("读取流失败: %v", err)
	}

	// 转换 map 为 slice
	toolCalls = make([]AIToolCall, 0, len(toolCallsMap))
	for _, tc := range toolCallsMap {
		toolCalls = append(toolCalls, tc)
	}

	return fullContent.String(), toolCalls, nil
}

func (p *DeepSeekProvider) streamChat(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error) {
	return streamChatCommon(messages, tools, controller, p.BaseURL, p.APIKey, p.Model, true)
}

func (p *OpenAIProvider) streamChat(messages []ChatMessage, tools []AITool, controller *StreamController) (string, []AIToolCall, error) {
	return streamChatCommon(messages, tools, controller, p.BaseURL, p.APIKey, p.Model, false)
}
