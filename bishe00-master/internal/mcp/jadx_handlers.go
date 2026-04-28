package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ConnectJADXHandler 连接 JADX MCP 服务器
func ConnectJADXHandler(w http.ResponseWriter, r *http.Request) {
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
		BaseURL string `json:"baseURL"`
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

	conn, err := ConnectJADXMCP(req.BaseURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("连接失败: %v", err),
		})
		return
	}

	tools := conn.GetTools()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "连接成功",
		"tools":   len(tools),
	})
}

// ConfigureJADXSessionHandler 配置 JADX 会话（设置 API key）
func ConfigureJADXSessionHandler(w http.ResponseWriter, r *http.Request) {
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
		SessionID     string `json:"sessionId"`
		APIKey        string `json:"apiKey"`        // OpenAI/Anthropic/DeepSeek API Key
		APIType       string `json:"apiType"`       // "openai", "anthropic", "deepseek" 或 "ollama"
		BaseURL       string `json:"baseURL"`       // JADX MCP 服务器 URL
		Model         string `json:"model"`         // 模型名称（ollama 或 deepseek 使用）
		OllamaBaseURL string `json:"ollamaBaseURL"` // Ollama 服务器地址（仅在 apiType=ollama 时使用）
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

	// 获取或创建会话
	session := GetOrCreateJADXSession(req.SessionID)

	// 配置会话
	session.mu.Lock()
	session.APIType = req.APIType
	if session.APIType == "" {
		session.APIType = "openai" // 默认
	}

	// 对于 Ollama，将 baseURL 和 model 组合存储到 APIKey 字段
	if session.APIType == "ollama" {
		ollamaURL := req.OllamaBaseURL
		if ollamaURL == "" {
			ollamaURL = "http://localhost:11434"
		}
		model := req.Model
		if model == "" {
			model = "gpt-oss:20b"
		}
		session.APIKey = fmt.Sprintf("%s|%s", ollamaURL, model)
	} else if session.APIType == "deepseek" {
		// 对于 DeepSeek，将 model 和 apiKey 组合存储（格式：model|apiKey）
		model := req.Model
		if model == "" {
			model = "deepseek-chat" // 默认模型
		}
		session.APIKey = fmt.Sprintf("%s|%s", model, req.APIKey)
	} else {
		session.APIKey = req.APIKey
	}

	session.LastActivity = time.Now()

	// 连接 JADX MCP 服务器（如果提供了 baseURL）
	if req.BaseURL != "" {
		conn, err := ConnectJADXMCP(req.BaseURL)
		if err != nil {
			session.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("连接 MCP 失败: %v", err),
			})
			return
		}
		session.MCPConnection = conn
		log.Printf("[JADX MCP] 会话 %s 已连接 MCP: %s", session.ID, req.BaseURL)
	}
	session.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"sessionId": session.ID,
		"message":   "会话配置成功",
	})
}

// GetJADXMessagesHandler 获取 JADX 会话消息历史
func GetJADXMessagesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "缺少 sessionId 参数",
		})
		return
	}

	session := GetOrCreateJADXSession(sessionID)
	messages := session.GetMessages()
	mcpConnected := session.GetMCPConnection() != nil && session.GetMCPConnection().IsConnected()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"messages":     messages,
		"mcpConnected": mcpConnected,
	})
}

// GetJADXSessionStatusHandler 获取 JADX 会话状态
func GetJADXSessionStatusHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "缺少 sessionId 参数",
		})
		return
	}

	session := GetOrCreateJADXSession(sessionID)
	apiType := session.GetAPIType()
	hasAPIKey := session.GetAPIKey() != ""
	mcpConnected := session.GetMCPConnection() != nil && session.GetMCPConnection().IsConnected()
	messageCount := len(session.GetMessages())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"sessionId":    session.GetID(),
		"apiType":      apiType,
		"hasApiKey":    hasAPIKey,
		"mcpConnected": mcpConnected,
		"messageCount": messageCount,
	})
}
