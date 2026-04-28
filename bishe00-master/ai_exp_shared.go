package main

import (
	"strings"
	"time"

	"bishe/internal/mcp"
)

func loadRequestedExps(inlineExps []ExpSpec, expPaths []string, expDir string) ([]ExpSpec, error) {
	if len(inlineExps) > 0 {
		return inlineExps, nil
	}
	if len(expPaths) > 0 {
		return loadExpsFromFiles(expPaths)
	}
	return loadExps(expDir)
}

func generatePythonForSpec(targetBaseURL string, spec ExpSpec, provider mcp.AIProvider, verify *ExpVerifyInfo) (string, string, VulnCategory) {
	targetBaseURL = strings.TrimSpace(targetBaseURL)
	keyInfo := buildExpKeyInfo(targetBaseURL, spec)
	fallback := generatePythonFromExpSpec(targetBaseURL, spec)
	category := DetectVulnCategory(spec)

	if provider == nil {
		return fallback, keyInfo, category
	}

	systemPrompt := getSystemPromptForCategory(category)
	userPrompt := buildUserPromptForCategory(category, targetBaseURL, spec, keyInfo, verify)
	messages := []mcp.ChatMessage{
		{Role: "system", Content: systemPrompt, Time: time.Now().Format(time.RFC3339)},
		{Role: "user", Content: userPrompt, Time: time.Now().Format(time.RFC3339)},
	}
	content, _, err := provider.Chat(messages, nil)
	if err != nil {
		return fallback, keyInfo, category
	}

	clean := stripCodeFence(content)
	if strings.TrimSpace(clean) == "" {
		return fallback, keyInfo, category
	}
	return clean, keyInfo, category
}
