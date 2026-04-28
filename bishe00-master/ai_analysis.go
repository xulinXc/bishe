package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"bishe/internal/mcp"
)

// AIAnalyzeReq AI分析请求结构体
// 用于接收前端发送的AI分析请求参数
type AIAnalyzeReq struct {
	Provider   string                 `json:"provider"`
	APIKey     string                 `json:"apiKey"`
	BaseURL    string                 `json:"baseURL"`
	Model      string                 `json:"model"`
	ReportData map[string]interface{} `json:"reportData"`
}

// runAIAnalysisInternal 内部AI分析函数（流式输出版本，保留用于兼容性）
func runAIAnalysisInternal(provider, apiKey, baseURL, model string, reportData map[string]interface{}, stepName string, t *Task, sendChunk func(string)) string {
	reportText := buildReportPrompt(reportData)

	var aiProvider mcp.AIProvider
	var streamProvider mcp.ChatStreamProvider
	controller := mcp.NewStreamController()

	switch provider {
	case "deepseek":
		p := mcp.NewDeepSeekProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		aiProvider = p
		streamProvider = p
	case "openai":
		p := mcp.NewOpenAIProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		aiProvider = p
		streamProvider = p
	case "anthropic":
		p := mcp.NewAnthropicProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		aiProvider = p
		streamProvider = nil
	case "ollama":
		p := mcp.NewOllamaProvider(baseURL, model)
		if model != "" {
			p.Model = model
		}
		aiProvider = p
		streamProvider = p
	default:
		return ""
	}

	var systemPrompt string
	if stepName == "最终报告" || stepName == "综合扫描" {
		systemPrompt = `你是一位专业的安全分析专家。请对扫描结果进行全面分析，输出纯文本分析报告。

要求：
1. 输出纯文本，不要使用Markdown格式
2. 包含以下部分：
   - 执行摘要（简要概述）
   - 关键发现（列出重要发现）
   - 风险评估（按严重程度分类）
   - 修复建议（针对性的修复措施）
3. 使用中文输出
4. 只输出用户需要了解的信息，不要输出"根据提供的扫描报告"等冗余说明
5. 换行使用实际换行，不要使用\n转义字符

请直接输出分析结果，不要包含任何解释性文字或元数据。`
	} else {
		systemPrompt = fmt.Sprintf(`你是一位专业的安全分析专家。请对%s的扫描结果进行简要分析。

要求：
1. 输出纯文本，不要使用Markdown格式
2. 包含：关键发现、风险评估、下一步建议
3. 控制在200字以内
4. 使用中文输出
5. 只输出用户需要了解的信息，不要输出"根据提供的扫描报告"等冗余说明
6. 换行使用实际换行，不要使用\n转义字符

请直接输出分析结果，不要包含任何解释性文字。`, stepName)
	}

	messages := []mcp.ChatMessage{
		{Role: "system", Content: systemPrompt, Time: time.Now().Format(time.RFC3339)},
		{Role: "user", Content: reportText, Time: time.Now().Format(time.RFC3339)},
	}

	var fullContent strings.Builder
	if streamProvider != nil {
		done := make(chan bool)
		go func() {
			defer close(done)
			_, _, err := streamProvider.ChatStream(messages, nil, controller)
			if err != nil {
				log.Printf("[AI自动扫描] AI流式分析失败: %v", err)
			}
		}()

		for {
			select {
			case <-t.stop:
				controller.Abort()
				return fullContent.String()
			case msg := <-controller.GetMessageChan():
				if msg != "" {
					cleanMsg := strings.ReplaceAll(msg, "\\n", "\n")
					cleanMsg = strings.ReplaceAll(cleanMsg, "\\\"", "\"")
					cleanMsg = strings.ReplaceAll(cleanMsg, "\"\"", "\"")
					fullContent.WriteString(cleanMsg)
					if sendChunk != nil {
						sendChunk(cleanMsg)
					}
				}
			case <-controller.GetAbortChan():
				return fullContent.String()
			case <-done:
				for {
					select {
					case msg := <-controller.GetMessageChan():
						if msg != "" {
							cleanMsg := strings.ReplaceAll(msg, "\\n", "\n")
							cleanMsg = strings.ReplaceAll(cleanMsg, "\\\"", "\"")
							cleanMsg = strings.ReplaceAll(cleanMsg, "\"\"", "\"")
							fullContent.WriteString(cleanMsg)
							if sendChunk != nil {
								sendChunk(cleanMsg)
							}
						}
					default:
						return fullContent.String()
					}
				}
			}
		}
	}

	content, _, err := aiProvider.Chat(messages, nil)
	if err != nil {
		log.Printf("[AI自动扫描] AI分析失败: %v", err)
		return ""
	}
	if sendChunk != nil {
		cleanContent := strings.ReplaceAll(content, "\\n", "\n")
		cleanContent = strings.ReplaceAll(cleanContent, "\\\"", "\"")
		cleanContent = strings.ReplaceAll(cleanContent, "\"\"", "\"")
		chunkSize := 20
		for i := 0; i < len(cleanContent); i += chunkSize {
			end := i + chunkSize
			if end > len(cleanContent) {
				end = len(cleanContent)
			}
			sendChunk(cleanContent[i:end])
			time.Sleep(50 * time.Millisecond)
		}
	}
	content = strings.ReplaceAll(content, "\\n", "\n")
	content = strings.ReplaceAll(content, "\\\"", "\"")
	content = strings.ReplaceAll(content, "\"\"", "\"")
	return content
}

// extractTitle 从HTML中提取标题
func extractTitle(body []byte) string {
	lb := strings.ToLower(string(body))
	start := strings.Index(lb, "<title>")
	if start >= 0 {
		end := strings.Index(lb[start+7:], "</title>")
		if end >= 0 {
			return strings.TrimSpace(string(body[start+7 : start+7+end]))
		}
	}
	return ""
}

// aiAnalyzeHandler AI安全分析处理器
func aiAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AIAnalyzeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("解析请求失败: %v", err), http.StatusBadRequest)
		return
	}
	if req.APIKey == "" {
		http.Error(w, "API Key 不能为空", http.StatusBadRequest)
		return
	}

	reportText := buildReportPrompt(req.ReportData)
	systemPrompt := `你是一位专业的安全分析专家。请对提供的扫描报告数据进行深入的安全分析。

## 输出格式要求
请严格按照以下格式输出分析结果，使用Markdown格式：

### 1. 扫描概览
简要描述扫描的目标、范围和时间。

### 2. 风险等级评估
- 【高危】列出所有高危漏洞（可直接利用的漏洞，如RCE、SQL注入等）
- 【中危】列出所有中危漏洞（需要特定条件才能利用的漏洞）
- 【低危】列出所有低危漏洞（信息收集类风险）

### 3. 漏洞详情
对每个发现的漏洞，按以下格式详细说明：
**漏洞名称**: 
**风险等级**: 
**目标URL**: 
**漏洞描述**: 
**利用条件**: 
**修复建议**: 

### 4. 潜在攻击路径
描述攻击者可能的入侵路径和利用链。

### 5. 修复建议
按优先级列出需要修复的问题。

### 6. 总结
简要总结扫描结果和建议。

请确保：
1. 每个漏洞都有详细的信息说明
2. 修复建议具体可操作
3. 使用中文输出
4. 结构清晰，层次分明`

	var provider mcp.AIProvider
	var streamProvider mcp.ChatStreamProvider
	controller := mcp.NewStreamController()

	switch req.Provider {
	case "deepseek":
		p := mcp.NewDeepSeekProvider(req.APIKey)
		if req.BaseURL != "" {
			p.BaseURL = req.BaseURL
		}
		if req.Model != "" {
			p.Model = req.Model
		}
		provider = p
		streamProvider = p
	case "openai":
		p := mcp.NewOpenAIProvider(req.APIKey)
		if req.BaseURL != "" {
			p.BaseURL = req.BaseURL
		}
		if req.Model != "" {
			p.Model = req.Model
		}
		provider = p
		streamProvider = p
	case "anthropic":
		p := mcp.NewAnthropicProvider(req.APIKey)
		if req.BaseURL != "" {
			p.BaseURL = req.BaseURL
		}
		if req.Model != "" {
			p.Model = req.Model
		}
		provider = p
		streamProvider = nil
	case "ollama":
		p := mcp.NewOllamaProvider(req.BaseURL, req.Model)
		if req.Model != "" {
			p.Model = req.Model
		}
		provider = p
		streamProvider = p
	default:
		http.Error(w, "不支持的AI提供商", http.StatusBadRequest)
		return
	}

	messages := []mcp.ChatMessage{
		{Role: "system", Content: systemPrompt, Time: time.Now().Format(time.RFC3339)},
		{Role: "user", Content: reportText, Time: time.Now().Format(time.RFC3339)},
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ctx := r.Context()

	if streamProvider != nil {
		go func() {
			defer controller.Abort()
			_, _, err := streamProvider.ChatStream(messages, nil, controller)
			if err != nil {
				log.Printf("[AI分析] 流式处理失败: %v", err)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				controller.Abort()
				return
			case msg := <-controller.GetMessageChan():
				data := map[string]interface{}{"content": msg}
				jsonData, _ := json.Marshal(data)
				fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
				if fl, ok := w.(http.Flusher); ok {
					fl.Flush()
				}
			case <-controller.GetAbortChan():
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if fl, ok := w.(http.Flusher); ok {
					fl.Flush()
				}
				return
			}
		}
	}

	go func() {
		content, _, err := provider.Chat(messages, nil)
		if err != nil {
			log.Printf("[AI分析] 处理失败: %v", err)
			controller.Abort()
			return
		}
		chunkSize := 50
		for i := 0; i < len(content); i += chunkSize {
			end := i + chunkSize
			if end > len(content) {
				end = len(content)
			}
			data := map[string]interface{}{"content": content[i:end]}
			jsonData, _ := json.Marshal(data)
			controller.Send(string(jsonData))
		}
		controller.Abort()
	}()

	for {
		select {
		case <-ctx.Done():
			controller.Abort()
			return
		case msg := <-controller.GetMessageChan():
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		case <-controller.GetAbortChan():
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			return
		}
	}
}

// buildReportPrompt 从报告数据构造提示词
func buildReportPrompt(reportData map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("以下是一份完整的扫描报告数据，请进行安全分析：\n\n")

	if portScan, ok := reportData["portScan"].(map[string]interface{}); ok {
		if enabled, _ := portScan["enabled"].(bool); enabled {
			sb.WriteString("## 端口扫描结果\n")
			if target, ok := portScan["target"].(string); ok && target != "" {
				sb.WriteString(fmt.Sprintf("- 目标: %s\n", target))
			}
			if scanType, ok := portScan["scanType"].(string); ok && scanType != "" {
				sb.WriteString(fmt.Sprintf("- 扫描类型: %s\n", scanType))
			}
			if results, ok := portScan["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 发现开放端口数量: %d\n", len(results)))
				sb.WriteString("- 端口详情:\n")
				for i, r := range results {
					if i >= 20 {
						sb.WriteString(fmt.Sprintf("  ... (还有 %d 个端口)\n", len(results)-20))
						break
					}
					if result, ok := r.(map[string]interface{}); ok {
						port := result["port"]
						proto := result["proto"]
						status := result["status"]
						banner, _ := result["banner"].(string)
						sb.WriteString(fmt.Sprintf("  - 端口 %v (%v): %v", port, proto, status))
						if banner != "" {
							bannerTrim := banner
							if len(bannerTrim) > 50 {
								bannerTrim = bannerTrim[:50] + "..."
							}
							sb.WriteString(fmt.Sprintf(", Banner: %s", bannerTrim))
						}
						sb.WriteString("\n")
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if dirScan, ok := reportData["dirScan"].(map[string]interface{}); ok {
		if enabled, _ := dirScan["enabled"].(bool); enabled {
			sb.WriteString("## 目录扫描结果\n")
			if target, ok := dirScan["target"].(string); ok && target != "" {
				sb.WriteString(fmt.Sprintf("- 目标: %s\n", target))
			}
			if results, ok := dirScan["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 发现目录数量: %d\n", len(results)))
				sb.WriteString("- 目录详情:\n")
				for i, r := range results {
					if i >= 20 {
						sb.WriteString(fmt.Sprintf("  ... (还有 %d 个目录)\n", len(results)-20))
						break
					}
					if result, ok := r.(map[string]interface{}); ok {
						url := result["url"]
						status := result["status"]
						length, _ := result["length"].(float64)
						sb.WriteString(fmt.Sprintf("  - %v (状态码: %v, 长度: %.0f)\n", url, status, length))
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if pocScan, ok := reportData["pocScan"].(map[string]interface{}); ok {
		if enabled, _ := pocScan["enabled"].(bool); enabled {
			sb.WriteString("## POC漏洞扫描结果\n")
			if target, ok := pocScan["target"].(string); ok && target != "" {
				sb.WriteString(fmt.Sprintf("- 目标: %s\n", target))
			}
			if results, ok := pocScan["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 发现漏洞数量: %d\n", len(results)))
				sb.WriteString("- 漏洞详情:\n")
				for _, r := range results {
					if result, ok := r.(map[string]interface{}); ok {
						sb.WriteString(fmt.Sprintf("  - POC: %v, URL: %v, 状态: %v\n", result["poc"], result["url"], result["status"]))
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if expScan, ok := reportData["expScan"].(map[string]interface{}); ok {
		if enabled, _ := expScan["enabled"].(bool); enabled {
			sb.WriteString("## EXP验证结果\n")
			if target, ok := expScan["target"].(string); ok && target != "" {
				sb.WriteString(fmt.Sprintf("- 目标: %s\n", target))
			}
			if results, ok := expScan["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 验证结果数量: %d\n", len(results)))
				for _, r := range results {
					if result, ok := r.(map[string]interface{}); ok {
						sb.WriteString(fmt.Sprintf("  - EXP: %v, 匹配步骤: %v, 最后状态: %v\n", result["name"], result["matchedSteps"], result["lastStatus"]))
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if webProbe, ok := reportData["webProbe"].(map[string]interface{}); ok {
		if enabled, _ := webProbe["enabled"].(bool); enabled {
			sb.WriteString("## Web应用探针结果\n")
			if results, ok := webProbe["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 发现Web应用数量: %d\n", len(results)))
				for _, r := range results {
					if result, ok := r.(map[string]interface{}); ok {
						url := result["url"]
						status := result["status"]
						title, _ := result["title"].(string)
						tech, _ := result["tech"].([]interface{})
						sb.WriteString(fmt.Sprintf("  - URL: %v, 状态: %v", url, status))
						if title != "" {
							sb.WriteString(fmt.Sprintf(", 标题: %s", title))
						}
						if len(tech) > 0 {
							sb.WriteString(fmt.Sprintf(", 技术栈: %v", tech))
						}
						sb.WriteString("\n")
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if wafScan, ok := reportData["wafScan"].(map[string]interface{}); ok {
		if enabled, _ := wafScan["enabled"].(bool); enabled {
			sb.WriteString("## WAF绕过测试结果\n")
			if target, ok := wafScan["target"].(string); ok && target != "" {
				sb.WriteString(fmt.Sprintf("- 目标: %s\n", target))
			}
			if results, ok := wafScan["results"].([]interface{}); ok && len(results) > 0 {
				sb.WriteString(fmt.Sprintf("- 测试结果数量: %d\n", len(results)))
				for _, r := range results {
					if result, ok := r.(map[string]interface{}); ok {
						sb.WriteString(fmt.Sprintf("  - %v %v, 变体: %v, 状态: %v\n", result["method"], result["payload"], result["variant"], result["status"]))
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	if targets, ok := reportData["targets"].([]interface{}); ok && len(targets) > 0 {
		sb.WriteString("## 扫描目标汇总\n")
		sb.WriteString(fmt.Sprintf("- 扫描目标数量: %d\n", len(targets)))
		sb.WriteString("- 目标列表:\n")
		for i, t := range targets {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  ... (还有 %d 个目标)\n", len(targets)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("  - %v\n", t))
		}
		sb.WriteString("\n")
	}

	if scanTime, ok := reportData["scanTime"].(string); ok && scanTime != "" {
		sb.WriteString(fmt.Sprintf("扫描时间: %s\n\n", scanTime))
	}

	sb.WriteString("请基于以上扫描数据，进行全面的安全分析。")
	return sb.String()
}
