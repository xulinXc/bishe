package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bishe/internal/mcp"
)

type ExpVerifyInfo struct {
	MatchedSteps int    `json:"matchedSteps"`
	LastStatus   int    `json:"lastStatus"`
	Usage        string `json:"usage"`
	Suggestion   string `json:"suggestion"`
}

type AIGenPythonFromExpReq struct {
	Provider      string         `json:"provider"`
	APIKey        string         `json:"apiKey"`
	BaseURL       string         `json:"baseUrl"`
	Model         string         `json:"model"`
	TargetBaseURL string         `json:"targetBaseUrl"`
	TimeoutMs     int            `json:"timeoutMs"`
	Exp           ExpSpec        `json:"exp"`
	Verify        *ExpVerifyInfo `json:"verify,omitempty"`
	AutoVerify    bool           `json:"autoVerify"` // 是否启用自动验证
	TargetURL     string         `json:"targetURL"`  // 用于自动验证的目标URL
	MaxRetries    int            `json:"maxRetries"` // 最大重试次数（默认5）
}

type AIGenPythonFromExpResp struct {
	Name           string   `json:"name"`
	KeyInfo        string   `json:"keyInfo"`
	Python         string   `json:"python"`
	Verified       bool     `json:"verified"`       // 是否验证成功
	VerifyAttempts int      `json:"verifyAttempts"` // 验证尝试次数
	VerifyLogs     []string `json:"verifyLogs"`     // 验证日志
	Category       string   `json:"category"`       // 漏洞类型
}

func newAIProvider(providerName, apiKey, baseURL, model string) (mcp.AIProvider, error) {
	trimSpace := func(s string) string { return strings.TrimSpace(s) }

	var p mcp.AIProvider
	var err error

	switch providerName {
	case "deepseek":
		if trimSpace(apiKey) == "" {
			return nil, fmt.Errorf("API Key 不能为空")
		}
		p = mcp.NewDeepSeekProvider(apiKey)
	case "openai":
		if trimSpace(apiKey) == "" {
			return nil, fmt.Errorf("API Key 不能为空")
		}
		p = mcp.NewOpenAIProvider(apiKey)
	case "anthropic":
		if trimSpace(apiKey) == "" {
			return nil, fmt.Errorf("API Key 不能为空")
		}
		p = mcp.NewAnthropicProvider(apiKey)
	case "ollama":
		p = mcp.NewOllamaProvider(baseURL, model)
	default:
		return nil, fmt.Errorf("不支持的AI提供商")
	}

	// 通用设置
	if baseURLProvider, ok := p.(*mcp.DeepSeekProvider); ok && trimSpace(baseURL) != "" {
		baseURLProvider.BaseURL = trimSpace(baseURL)
	}
	if baseURLProvider, ok := p.(*mcp.OpenAIProvider); ok && trimSpace(baseURL) != "" {
		baseURLProvider.BaseURL = trimSpace(baseURL)
	}
	if baseURLProvider, ok := p.(*mcp.AnthropicProvider); ok && trimSpace(baseURL) != "" {
		baseURLProvider.BaseURL = trimSpace(baseURL)
	}
	if trimSpace(model) != "" {
		switch provider := p.(type) {
		case *mcp.DeepSeekProvider:
			provider.Model = trimSpace(model)
		case *mcp.OpenAIProvider:
			provider.Model = trimSpace(model)
		case *mcp.AnthropicProvider:
			provider.Model = trimSpace(model)
		case *mcp.OllamaProvider:
			provider.Model = trimSpace(model)
		}
	}
	return p, err
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			return strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	return s
}

// getSystemPromptForCategory 根据漏洞类型返回专用的系统提示词
func getSystemPromptForCategory(category VulnCategory) string {
	basePrompt := "你是一位专业的渗透测试开发者。你将根据给定的 EXP 规范生成可运行的 Python 利用脚本。"

	switch category {
	case CatCommandInjection, CatCodeExecution:
		return basePrompt + "\n\n你正在处理一个命令执行/代码执行类型的漏洞。重点关注：\n" +
			"1. 使用 echo NEONSCAN_BEGIN; <命令>; echo NEONSCAN_END 包裹命令\n" +
			"2. 准确提取 NEONSCAN_BEGIN 和 NEONSCAN_END 之间的输出\n" +
			"3. 支持交互式Shell模式\n" +
			"4. 处理各种命令输出格式（纯文本、HTML包裹等）\n" +
			"5. 【重要】验证成功时必须输出 'VULNERABLE'（大写），不要使用其他标记"
	case CatFileUpload:
		return basePrompt + "\n\n你正在处理一个文件上传类型的漏洞。重点关注：\n" +
			"1. 正确构造multipart/form-data请求\n" +
			"2. 验证文件上传成功（检查响应状态和文件路径）\n" +
			"3. 如果可能，尝试访问上传的文件以确认\n" +
			"4. 处理各种上传限制绕过（文件类型、大小等）"
	case CatUnauthorizedAccess:
		return basePrompt + "\n\n你正在处理一个未授权访问/越权类型的漏洞。重点关注：\n" +
			"1. 测试不同的身份验证绕过方法\n" +
			"2. 验证是否成功访问受保护资源\n" +
			"3. 提取敏感信息（用户数据、配置等）\n" +
			"4. 清晰展示越权访问的证据"
	case CatInfoDisclosure:
		return basePrompt + "\n\n你正在处理一个信息泄露类型的漏洞。重点关注：\n" +
			"1. 准确提取泄露的敏感信息\n" +
			"2. 过滤无关的HTML/JSON结构\n" +
			"3. 格式化输出关键信息（路径、配置、凭证等）\n" +
			"4. 验证信息的有效性"
	default:
		return basePrompt + "\n\n请根据实际的漏洞特征生成通用的EXP，不要假设目标一定是特定框架。"
	}
}

// buildUserPromptForCategory 根据漏洞类型构建用户提示词
func buildUserPromptForCategory(category VulnCategory, targetBaseURL string, exp ExpSpec, keyInfo string, verify *ExpVerifyInfo) string {
	expJSON, _ := json.MarshalIndent(exp, "", "  ")
	verifyText := ""
	if verify != nil {
		verifyText = fmt.Sprintf("验证结果:\n- matchedSteps: %d\n- lastStatus: %d\n- usage:\n%s\n- suggestion:\n%s\n", verify.MatchedSteps, verify.LastStatus, verify.Usage, verify.Suggestion)
	}

	targetArgDesc := "--target(必填)"
	targetUrlHint := "构建请求URL时，请使用 urllib.parse.urljoin(target, path) 以正确处理相对路径。"
	if targetBaseURL != "" {
		targetArgDesc = fmt.Sprintf("--target(可选, 默认='%s')", targetBaseURL)
		targetUrlHint = fmt.Sprintf("已知目标地址为 '%s'，请在脚本中将其设为 --target 的默认值。构建请求URL时，请务必使用 urllib.parse.urljoin(target, path) 拼接地址，确保 scheme (http/https) 存在。", targetBaseURL)
	}

	// 基础要求
	baseRequirements := fmt.Sprintf(`请基于以下信息生成一个单文件 Python3 利用脚本，要求：
1) 只输出 Python 代码，不要输出解释/Markdown/代码块围栏。
2) 使用 requests.Session()；默认 verify=False，并禁用 urllib3 警告。
3) 命令行参数必须同时支持：%s、--timeout(可选)。
4) 严格按 EXP steps 顺序发包，支持 {{var}} 占位符替换与变量提取（bodyRegex/headerRegex）。
5) 生成的脚本不得直接 print(response.text) 或输出整页 HTML。必须对回显做"去噪"处理。
6) 【重要】实现 validate(status/bodyContains/headerContains)，并在命中时输出 "VULNERABLE"（大写，不要使用其他标记如ThinkPHP_RCE_Vulnerable）。
7) 【Windows兼容要求-必须遵守】禁止导入任何第三方库（除了 requests）。特别禁止：
   - 禁止 import readline（Windows 不支持）
   - 禁止 import readline（Unix only功能）
   - 禁止 import rlcompleter（Windows 不支持）
   - 禁止任何依赖系统C库的模块
8) %s
9) 【关键执行日志】在脚本中增加详细的执行日志，打印关键步骤：
   [INFO] Target: ...
   [INFO] Payload: ...
   [INFO] Sending request...
   [INFO] Response status: ...
   [INFO] Response length: ...
   [INFO] Extracting output...
   [RESULT] ...
`, targetArgDesc, targetUrlHint)

	// 根据漏洞类型添加特定要求
	var specificRequirements string
	switch category {
	case CatCommandInjection, CatCodeExecution:
		specificRequirements = `
10) 【命令执行专用】：
    - 命令行参数额外支持：--cmd(可选 单次命令)、--shell(可选 交互式命令执行)
    - 使用 echo NEONSCAN_BEGIN; <COMMAND>; echo NEONSCAN_END 包裹命令
    - 执行后提取 NEONSCAN_BEGIN 和 NEONSCAN_END 之间的内容
    - 如果提取失败，使用正则 r"^(.*?)(?:<!DOCTYPE|<html)" 截取 HTML 之前的纯文本内容
    - --shell 模式要循环读取命令并执行，exit/quit 退出
    - 确保无论响应是什么格式，都能提取并展示关键输出`
	case CatFileUpload:
		specificRequirements = `
10) 【文件上传专用】：
    - 命令行参数额外支持：--file(要上传的文件路径)
    - 正确构造 multipart/form-data 请求
    - 验证文件上传成功（检查响应中的文件路径或成功标记）
    - 如果响应包含上传文件的URL，尝试访问以确认上传成功
    - 打印上传文件的访问路径`
	case CatUnauthorizedAccess:
		specificRequirements = `
10) 【未授权访问专用】：
    - 清晰展示访问受保护资源的证据
    - 提取并格式化敏感信息（用户数据、权限信息等）
    - 对比正常访问和越权访问的差异
    - 打印关键的身份验证绕过信息`
	case CatInfoDisclosure:
		specificRequirements = `
10) 【信息泄露专用】：
    - 准确提取泄露的敏感信息（路径、配置、凭证等）
    - 过滤无关的HTML/JSON结构，只展示关键信息
    - 使用正则表达式或JSON解析提取结构化数据
    - 格式化输出，使信息易于阅读`
	default:
		specificRequirements = `
10) 【通用响应提取策略】：
    - 优先尝试定位响应中的特殊标记
    - 如果没有标记，使用正则表达式截取 HTML 标签之前的内容
    - 如果响应是纯文本，尝试直接提取
    - 如果响应包含错误信息，提取其中的关键报错内容
    - 确保不打印整个 HTML 页面给用户`
	}

	return fmt.Sprintf(`%s%s

关键信息:
%s

EXP JSON:
%s

%s`, baseRequirements, specificRequirements, keyInfo, string(expJSON), verifyText)
}

func aiGenPythonFromExpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AIGenPythonFromExpReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	req.TargetBaseURL = strings.TrimSpace(req.TargetBaseURL)

	// 检测漏洞类型
	category := DetectVulnCategory(req.Exp)
	fmt.Printf("[分类] 漏洞类型: %s\n", category)

	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		python, keyInfo, category := generatePythonForSpec(req.TargetBaseURL, req.Exp, nil, req.Verify)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AIGenPythonFromExpResp{
			Name:     req.Exp.Name,
			KeyInfo:  keyInfo,
			Python:   python,
			Category: category.String(),
		})
		return
	}

	provider, err := newAIProvider(providerName, req.APIKey, req.BaseURL, req.Model)
	if err != nil {
		python, keyInfo, category := generatePythonForSpec(req.TargetBaseURL, req.Exp, nil, req.Verify)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AIGenPythonFromExpResp{
			Name:     req.Exp.Name,
			KeyInfo:  keyInfo,
			Python:   python,
			Category: category.String(),
		})
		return
	}

	python, keyInfo, category := generatePythonForSpec(req.TargetBaseURL, req.Exp, provider, req.Verify)

	// 如果启用自动验证
	if req.AutoVerify && req.TargetURL != "" {
		fmt.Println("\n========== 启动自动验证流程 ==========")
		logChan := make(chan string, 100)
		var logs []string

		// 启动日志收集协程
		go func() {
			for log := range logChan {
				logs = append(logs, log)
			}
		}()

		config := ExpVerifyConfig{
			TargetURL:  req.TargetURL,
			ExpSpec:    req.Exp,
			PythonCode: python,
			Category:   category,
			MaxRetries: req.MaxRetries,
			TimeoutSec: 10,
		}

		if config.MaxRetries <= 0 {
			config.MaxRetries = 5
		}

		finalCode, verifyErr := GenerateAndVerifyExp(config, provider, logChan)
		close(logChan)

		verified := verifyErr == nil
		attempts := config.MaxRetries
		if verified {
			python = finalCode
			// 计算实际尝试次数
			for i, log := range logs {
				if strings.Contains(log, "验证成功") {
					attempts = i / 5 // 粗略估计
					break
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AIGenPythonFromExpResp{
			Name:           req.Exp.Name,
			KeyInfo:        keyInfo,
			Python:         python,
			Verified:       verified,
			VerifyAttempts: attempts,
			VerifyLogs:     logs,
			Category:       category.String(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AIGenPythonFromExpResp{
		Name:     req.Exp.Name,
		KeyInfo:  keyInfo,
		Python:   python,
		Category: category.String(),
	})
}

func generatePythonFromExpSpec(targetBaseURL string, spec ExpSpec) string {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = "neonscan_exp"
	}
	targetBaseURL = strings.TrimSpace(targetBaseURL)

	expJSON, _ := json.MarshalIndent(spec, "", "  ")
	keyInfo := buildExpKeyInfo(targetBaseURL, spec)
	header := fmt.Sprintf(`# -*- coding: utf-8 -*-
import argparse
import json
import re
import requests
from urllib.parse import urljoin, quote_plus

EXP_NAME = %q
EXP_SPEC = json.loads(%q)
KEY_INFO = %q

def subst_vars(s, vars_):
    if s is None:
        return ""
    out = str(s)
    for k, v in vars_.items():
        out = out.replace("{{" + k + "}}", str(v))
    return out

def validate(resp, body_text, rule):
    ok = True
    status = rule.get("status") or []
    if isinstance(status, int):
        status = [status]
    if status:
        ok = ok and (resp.status_code in status)
    body_contains = rule.get("bodyContains") or []
    if body_contains:
        lb = body_text.lower()
        for sub in body_contains:
            if str(sub).lower() not in lb:
                ok = False
                break
    header_contains = rule.get("headerContains") or {}
    if header_contains:
        for k, sub in header_contains.items():
            hv = resp.headers.get(k, "")
            if str(sub).lower() not in str(hv).lower():
                ok = False
                break
    return ok

def extract_vars(resp, body_text, rules, vars_):
    if not rules:
        return
    for name, rule in rules.items():
        if not isinstance(rule, dict):
            continue
        for rx in (rule.get("bodyRegex") or []):
            try:
                m = re.search(rx, body_text, re.S)
            except re.error:
                m = None
            if m and m.groups():
                vars_[name] = m.group(1)
                break
        header_rx = rule.get("headerRegex") or {}
        for hk, rx in header_rx.items():
            hv = resp.headers.get(hk, "")
            try:
                m = re.search(rx, hv, re.S)
            except re.error:
                m = None
            if m and m.groups():
                vars_[name] = m.group(1)
                break

def spec_has_cmd_placeholders():
    def has(s):
        if not s:
            return False
        s = str(s)
        return ("{{cmd" in s) or ("{{command" in s)
    for st in (EXP_SPEC.get("steps") or []):
        if has(st.get("path")) or has(st.get("body")):
            return True
        for _, v in (st.get("headers") or {}).items():
            if has(v):
                return True
    return False

def wrap_cmd(cmd):
    begin = "NEONSCAN_BEGIN"
    end = "NEONSCAN_END"
    inner = cmd if cmd is not None else ""
    raw = f"echo {begin}; {inner}; echo {end}"
    return begin, end, raw

def extract_between(text, begin, end):
    if not text:
        return ""
    t = str(text)
    i = t.find(begin)
    j = t.find(end)
    if i == -1 or j == -1 or j <= i:
        return ""
    mid = t[i + len(begin):j]
    return mid.strip()

def run_once(s, base, timeout, cmd=None):
    vars_ = {}
    if cmd is not None:
        begin, end, wrapped = wrap_cmd(cmd)
        vars_["cmd_raw"] = cmd
        vars_["command_raw"] = cmd
        vars_["cmd"] = wrapped
        vars_["command"] = wrapped
        vars_["cmd_urlenc"] = quote_plus(wrapped, safe="")
        vars_["command_urlenc"] = quote_plus(wrapped, safe="")
        vars_["_marker_begin"] = begin
        vars_["_marker_end"] = end
    matched = 0
    steps = EXP_SPEC.get("steps") or []
    last_resp = None
    last_text = ""
    for idx, st in enumerate(steps, 1):
        method = (st.get("method") or "GET").upper().strip() or "GET"
        path = subst_vars(st.get("path") or "", vars_)
        body = subst_vars(st.get("body") or "", vars_)
        headers = st.get("headers") or {}
        hdr = {}
        for k, v in headers.items():
            hdr[str(k)] = subst_vars(v, vars_)

        if path.lower().startswith("http://") or path.lower().startswith("https://"):
            url = path
        else:
            url = urljoin(base.rstrip("/") + "/", path.lstrip("/"))

        try:
            resp = s.request(method, url, data=body if body != "" else None, headers=hdr, timeout=timeout, allow_redirects=True)
            text = resp.text or ""
            last_resp = resp
            last_text = text
        except Exception as e:
            print(f"[step {idx}] request failed: {e}")
            continue

        extract_vars(resp, text, st.get("extract") or {}, vars_)
        ok = validate(resp, text, st.get("validate") or {})
        if ok:
            matched += 1

        sleep_ms = int(st.get("sleepMs") or 0)
        if sleep_ms > 0:
            import time
            time.sleep(sleep_ms / 1000.0)

    return steps, matched, vars_, last_resp, last_text

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", required=True)
    ap.add_argument("--timeout", type=float, default=8.0)
    ap.add_argument("--cmd")
    ap.add_argument("--shell", action="store_true")
    args = ap.parse_args()
    base = args.target.rstrip("/")
    timeout = args.timeout
    s = requests.Session()
    s.verify = False
    has_cmd = spec_has_cmd_placeholders()

    if args.shell:
        if not has_cmd:
            print("[!] This EXP does not include {{cmd}}/{{cmd_urlenc}} placeholders; --shell may not work.")
        # Probe once to confirm we can get clean echo output
        steps, _, vars0, _, text0 = run_once(s, base, timeout, cmd="echo NEONSCAN_OK")
        out0 = extract_between(text0, vars0.get("_marker_begin",""), vars0.get("_marker_end",""))
        if out0 and "NEONSCAN_OK" in out0:
            print("VULNERABLE")
        else:
            print("NOT VULNERABLE")
        while True:
            try:
                cmd = input("cmd> ").strip()
            except EOFError:
                break
            if cmd.lower() in ("exit", "quit"):
                break
            if not cmd:
                continue
            steps, matched, vars_, _, text = run_once(s, base, timeout, cmd=cmd)
            out = extract_between(text, vars_.get("_marker_begin",""), vars_.get("_marker_end",""))
            if out:
                print(out)
    else:
        if args.cmd and not has_cmd:
            print("[!] This EXP does not include {{cmd}}/{{cmd_urlenc}} placeholders; --cmd may not work.")
        if args.cmd:
            # Probe once, then execute a single command and only print extracted output.
            steps0, _, vars0, _, text0 = run_once(s, base, timeout, cmd="echo NEONSCAN_OK")
            out0 = extract_between(text0, vars0.get("_marker_begin",""), vars0.get("_marker_end",""))
            if out0 and "NEONSCAN_OK" in out0:
                print("VULNERABLE")
            else:
                print("NOT VULNERABLE")
            steps, matched, vars_, _, text = run_once(s, base, timeout, cmd=args.cmd)
            out = extract_between(text, vars_.get("_marker_begin",""), vars_.get("_marker_end",""))
            if out:
                print(out)
        else:
            steps, matched, vars_, _, text = run_once(s, base, timeout, cmd=None)
            if steps and matched == len(steps):
                print("VULNERABLE")
            else:
                print("NOT VULNERABLE")

if __name__ == "__main__":
    main()
`, name, string(expJSON), keyInfo)
	return header
}
