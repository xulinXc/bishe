// Package mcp 提供MCP（Model Context Protocol）相关功能的通用接口和基础实现
// 当前主要用于与JADX及AI服务进行交互
package mcp

import (
	"sync"
	"time"
)

// MCPConnection MCP连接接口（通用接口）
// 定义了与MCP服务器进行交互的基本方法
type MCPConnection interface {
	IsConnected() bool                                                           // 检查连接状态
	GetTools() []AITool                                                          // 获取可用的工具列表
	CallTool(name string, arguments map[string]interface{}) (interface{}, error) // 调用指定的工具
}

// ChatSession 聊天会话接口（通用接口）
// 定义了AI聊天会话的基本操作
type ChatSession interface {
	GetID() string                      // 获取会话ID
	GetAPIKey() string                  // 获取API密钥
	GetAPIType() string                 // 获取API类型（如"openai"、"anthropic"等）
	GetMessages() []ChatMessage         // 获取消息列表
	GetMCPConnection() MCPConnection    // 获取MCP连接对象
	SetMessages(messages []ChatMessage) // 设置消息列表
	AddMessage(msg ChatMessage)         // 添加消息
	GetLastActivity() time.Time         // 获取最后活动时间
	UpdateLastActivity()                // 更新最后活动时间
}

// BaseChatSession 基础聊天会话实现
// 提供了ChatSession接口的基础实现，包含线程安全的消息管理
type BaseChatSession struct {
	mu            sync.RWMutex  // 读写互斥锁，用于保证并发安全
	ID            string        // 会话唯一标识符
	APIKey        string        // API密钥
	APIType       string        // API类型："openai"、"anthropic"、"deepseek"、"ollama"等
	Messages      []ChatMessage // 消息列表
	MCPConnection MCPConnection // MCP连接对象
	CreatedAt     time.Time     // 会话创建时间
	LastActivity  time.Time     // 最后活动时间（用于清理过期会话）
}

// GetID 获取会话ID（线程安全）
// @return 会话唯一标识符
func (s *BaseChatSession) GetID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ID
}

// GetAPIKey 获取API密钥（线程安全）
// @return API密钥
func (s *BaseChatSession) GetAPIKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.APIKey
}

// GetAPIType 获取API类型（线程安全）
// @return API类型字符串
func (s *BaseChatSession) GetAPIType() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.APIType
}

// GetMessages 获取消息列表的副本（线程安全）
// 返回消息列表的深拷贝，避免外部修改影响内部状态
// @return 消息列表副本
func (s *BaseChatSession) GetMessages() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make([]ChatMessage, len(s.Messages))
	copy(messages, s.Messages)
	return messages
}

// GetMCPConnection 获取MCP连接对象（线程安全）
// @return MCP连接对象
func (s *BaseChatSession) GetMCPConnection() MCPConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MCPConnection
}

// SetMessages 设置消息列表（线程安全）
// 同时更新最后活动时间
// @param messages 新的消息列表
func (s *BaseChatSession) SetMessages(messages []ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = messages
	s.LastActivity = time.Now()
}

// AddMessage 添加消息到会话（线程安全）
// 将新消息追加到消息列表末尾，并更新最后活动时间
// @param msg 要添加的消息
func (s *BaseChatSession) AddMessage(msg ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.LastActivity = time.Now()
}

// GetLastActivity 获取最后活动时间（线程安全）
// @return 最后活动时间
func (s *BaseChatSession) GetLastActivity() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastActivity
}

// UpdateLastActivity 更新最后活动时间（线程安全）
// 将最后活动时间设置为当前时间
func (s *BaseChatSession) UpdateLastActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivity = time.Now()
}
