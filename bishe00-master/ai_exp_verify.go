package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bishe/internal/mcp"
)

type VulnCategory int

const (
	CatCommandInjection VulnCategory = iota
	CatCodeExecution
	CatInfoDisclosure
	CatFileUpload
	CatUnauthorizedAccess
	CatUnknown
)

func (c VulnCategory) String() string {
	switch c {
	case CatCommandInjection:
		return "命令执行"
	case CatCodeExecution:
		return "代码执行"
	case CatInfoDisclosure:
		return "信息泄露"
	case CatFileUpload:
		return "文件上传"
	case CatUnauthorizedAccess:
		return "未授权访问"
	default:
		return "未知类型"
	}
}

func DetectVulnCategory(expSpec ExpSpec) VulnCategory {
	specStr := strings.ToLower(expSpec.Name + " " + expSpec.ExploitSuggestion)

	patterns := map[VulnCategory][]string{
		CatCommandInjection:   {"command injection", "rce", "remote code exec", "os command", "shell exec", "命令注入", "命令执行", "远程代码执行"},
		CatCodeExecution:      {"code exec", "eval", "code injection", "代码执行", "代码注入"},
		CatInfoDisclosure:     {"info disclosure", "information leak", "path traversal", "lfi", "ssrf", "info disclosure", "信息泄露", "敏感信息"},
		CatFileUpload:         {"file upload", "upload", "webshell", "文件上传", "getshell"},
		CatUnauthorizedAccess: {"unauthorized", "auth bypass", "idor", "access control", "未授权", "越权"},
	}

	for cat, keywords := range patterns {
		for _, kw := range keywords {
			if strings.Contains(specStr, kw) {
				return cat
			}
		}
	}

	for _, step := range expSpec.Steps {
		stepStr := strings.ToLower(step.Path + " " + step.Body)
		if strings.Contains(stepStr, "{{cmd") || strings.Contains(stepStr, "{{command") {
			return CatCommandInjection
		}
		if strings.Contains(stepStr, "{{code") || strings.Contains(stepStr, "{{php") {
			return CatCodeExecution
		}
		if strings.Contains(stepStr, "{{file") || strings.Contains(stepStr, "{{upload") {
			return CatFileUpload
		}
	}

	return CatUnknown
}

type ExpVerifyResult struct {
	Success     bool
	Output      string
	Error       string
	Matched     bool
	CanExtract  bool
	ExtractDemo string
}

type ExpVerifyConfig struct {
	TargetURL  string
	ExpSpec    ExpSpec
	PythonCode string
	Category   VulnCategory
	MaxRetries int
	TimeoutSec int
}

// extractVulnMarker 从POC规范中提取漏洞验证标记
func extractVulnMarker(expSpec ExpSpec) string {
	// 遍历所有步骤，查找bodyContains中的验证标记
	for _, step := range expSpec.Steps {
		if step.Validate.BodyContains != nil && len(step.Validate.BodyContains) > 0 {
			// 查找最可能的验证标记（排除常见的HTML标签）
			for _, contain := range step.Validate.BodyContains {
				contain = strings.TrimSpace(contain)
				// 跳过HTML标签和常见文本
				if !strings.Contains(contain, "<") &&
					!strings.Contains(contain, ">") &&
					!strings.Contains(strings.ToLower(contain), "html") &&
					!strings.Contains(strings.ToLower(contain), "doctype") &&
					len(contain) > 3 {
					return contain
				}
			}
		}
	}
	// 默认返回VULNERABLE
	return "VULNERABLE"
}

func GenerateAndVerifyExp(config ExpVerifyConfig, provider mcp.AIProvider, logChan chan<- string) (string, error) {
	log := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		logChan <- msg
		fmt.Println(msg)
	}

	category := DetectVulnCategory(config.ExpSpec)
	log("[分类] 漏洞类型: %s", category)

	// 从POC中提取验证标记
	vulnMarker := extractVulnMarker(config.ExpSpec)
	if vulnMarker != "VULNERABLE" {
		log("[标记] 检测到POC中的验证标记: %s", vulnMarker)
		log("[标记] 注意：系统标准标记为 VULNERABLE，AI生成的EXP应使用标准标记")
	}

	testCmd := getTestCommand(category)
	log("[测试命令] 使用测试命令: %s", testCmd)

	expDir := filepath.Join(os.TempDir(), "neonscan_exp")
	os.MkdirAll(expDir, 0755)
	log("[目录] EXP保存目录: %s", expDir)

	maxRetries := config.MaxRetries
	if maxRetries < 3 {
		maxRetries = 5
	}

	currentCode := config.PythonCode

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log("")
		log("========== 验证尝试 #%d/%d ==========", attempt, maxRetries)

		if attempt > 1 {
			log("[修正] 等待AI分析失败原因...")
			time.Sleep(2 * time.Second)
		}

		expFile := filepath.Join(expDir, fmt.Sprintf("exp_attempt_%d.py", attempt))
		if err := os.WriteFile(expFile, []byte(currentCode), 0644); err != nil {
			return "", fmt.Errorf("保存EXP文件失败: %v", err)
		}
		log("[保存] EXP已保存到: %s", expFile)
		log("[代码] EXP代码长度: %d 字符", len(currentCode))

		result := verifyExp(expFile, config.TargetURL, testCmd, config.TimeoutSec, category, vulnMarker, log)

		if result.Success && result.Matched && result.CanExtract {
			log("")
			log("========== ✓ 验证成功! ==========")
			log("[成功] EXP验证通过!")
			log("[文件] 最终EXP: %s", expFile)
			if result.ExtractDemo != "" {
				lines := strings.Split(result.ExtractDemo, "\n")
				log("[输出示例] %s", lines[0])
				if len(lines) > 1 {
					for i := 1; i < len(lines) && i < 3; i++ {
						log("             %s", lines[i])
					}
				}
			}
			return currentCode, nil
		}

		if attempt >= maxRetries {
			log("")
			log("[失败] 已达到最大重试次数 (%d/%d)", attempt, maxRetries)
			log("[最终文件] %s", expFile)
			break
		}

		log("")
		log("[失败] EXP验证未通过，开始分析原因...")

		failureReason := analyzeFailure(result, category, log)

		log("[分析] 开始构建修正提示词...")
		correctionPrompt := buildCorrectionPrompt(config.ExpSpec, currentCode, failureReason, category, testCmd, result, log)

		log("[AI] 请求AI生成修正版本...")
		correctedCode := requestExpCorrection(provider, correctionPrompt, log)

		if correctedCode != "" {
			currentCode = correctedCode
			log("[修正] ✓ 已生成修正版本的EXP")
			log("[修正] 新代码长度: %d 字符", len(currentCode))
		} else {
			log("[警告] AI未能生成修正版本，使用备用修复方法...")
			currentCode = tryAlternativeFix(currentCode, result, category, testCmd, log)
			log("[备用] 备用修复完成，代码长度: %d 字符", len(currentCode))
		}
	}

	return "", fmt.Errorf("EXP验证失败，已达到最大重试次数 (%d)", maxRetries)
}

func getTestCommand(category VulnCategory) string {
	switch category {
	case CatCommandInjection, CatCodeExecution:
		return "echo NEONSCAN_TEST_$(whoami)_$(pwd)"
	case CatInfoDisclosure:
		return "cat /etc/passwd"
	case CatFileUpload:
		return "echo test"
	case CatUnauthorizedAccess:
		return "id"
	default:
		return "echo test"
	}
}

func verifyExp(expFile, targetURL, testCmd string, timeoutSec int, category VulnCategory, vulnMarker string, log func(string, ...interface{})) ExpVerifyResult {
	result := ExpVerifyResult{Success: false}

	log("[运行] 开始执行EXP...")
	log("[运行] 文件: %s", expFile)
	log("[运行] 目标: %s", targetURL)
	log("[运行] 命令: %s", testCmd)
	log("[运行] 超时: %d 秒", timeoutSec)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Windows系统直接使用python命令（最常见且可靠）
	// Linux/Mac系统会尝试python3
	pythonCmd := "python"
	pythonPath := "python"

	// 测试python命令是否可用
	testPythonCmd := exec.Command("python", "--version")
	if err := testPythonCmd.Run(); err != nil {
		log("[运行] python命令不可用，尝试python3...")
		// 如果python不可用，尝试python3
		testPython3Cmd := exec.Command("python3", "--version")
		if err := testPython3Cmd.Run(); err != nil {
			log("[运行] python3命令也不可用，尝试py...")
			// 最后尝试py命令（Windows Python Launcher）
			testPyCmd := exec.Command("py", "--version")
			if err := testPyCmd.Run(); err != nil {
				result.Error = "Python未安装或未在PATH中。请安装Python 3.6+并确保添加到PATH。\n" +
					"安装步骤：\n" +
					"1. 下载Python: https://www.python.org/downloads/\n" +
					"2. 安装时勾选 'Add Python to PATH'\n" +
					"3. 或手动添加到PATH环境变量"
				log("[运行] ✗ 无法找到任何Python命令（python, python3, py都不可用）")
				return result
			} else {
				pythonCmd = "py"
				pythonPath = "py (Python Launcher)"
				log("[运行] 使用Python Launcher: py")
			}
		} else {
			pythonCmd = "python3"
			pythonPath = "python3"
			log("[运行] 使用python3命令")
		}
	} else {
		log("[运行] 使用python命令")
	}

	// 确保targetURL包含scheme
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
		log("[运行] 自动添加http://前缀: %s", targetURL)
	}

	log("[运行] 执行命令: %s %s --target %s --cmd \"%s\"", pythonCmd, expFile, targetURL, testCmd)

	cmd := exec.CommandContext(ctx, pythonCmd, expFile, "--target", targetURL, "--cmd", testCmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result.Output = stdout.String()
	result.Error = stderr.String()

	log("[运行] 执行耗时: %.2f 秒", duration.Seconds())

	if ctx.Err() == context.DeadlineExceeded {
		result.Error = "执行超时"
		log("[运行] ✗ EXP执行超时")
		return result
	}

	// 处理执行错误
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// exit status 9009 在Windows上表示找不到命令
		// 但我们已经测试过Python可用，所以这可能是PATH问题
		if exitCode == 9009 || strings.Contains(err.Error(), "executable file not found") {
			result.Error = fmt.Sprintf("Python命令执行失败: %v\nPython路径: %s\n\n可能的原因：\n1. Go进程没有继承正确的PATH环境变量\n2. 需要重启IDE或终端\n3. Python安装路径未正确添加到系统PATH\n\n建议：\n- 重启IDE/终端后重试\n- 或在系统环境变量中添加Python路径", err, pythonPath)
			log("[运行] ✗ Python命令执行失败（exit %d）", exitCode)
			log("[运行] 这通常是PATH环境变量问题")
			return result
		}

		// 其他错误（如参数错误、连接错误、脚本逻辑错误等）
		// 这些错误说明Python可以运行，只是脚本有问题，可以通过AI修复
		result.Success = true // 标记为"可以运行"，虽然有错误

		// 合并stdout和stderr作为完整的错误信息
		fullError := stderr.String()
		if stdout.String() != "" {
			fullError = stdout.String() + "\n" + fullError
		}
		result.Error = fullError

		log("[运行] ⚠ 脚本执行返回错误码 %d，但Python可以运行", exitCode)
		log("[运行] 这可能是参数错误、连接错误或脚本逻辑问题")

		// 打印错误信息供AI分析
		if fullError != "" {
			errorLines := strings.Split(fullError, "\n")
			log("[运行] 错误输出（共 %d 行）：", len(errorLines))
			for i := 0; i < len(errorLines) && i < 10; i++ {
				if strings.TrimSpace(errorLines[i]) != "" {
					log("[错误] %s", strings.TrimSpace(errorLines[i]))
				}
			}
		}
	} else {
		result.Success = true
		log("[运行] ✓ EXP执行成功（返回码 0）")
	}

	// 打印输出的前几行
	if result.Output != "" {
		outputLines := strings.Split(result.Output, "\n")
		log("[输出] 共 %d 行输出", len(outputLines))
		for i := 0; i < len(outputLines) && i < 5; i++ {
			if strings.TrimSpace(outputLines[i]) != "" {
				log("[输出] %s", strings.TrimSpace(outputLines[i]))
			}
		}
	}

	// 打印错误输出的前几行（如果还没打印过）
	if result.Error != "" && !result.Success {
		errorLines := strings.Split(result.Error, "\n")
		log("[错误输出] 共 %d 行错误", len(errorLines))
		for i := 0; i < len(errorLines) && i < 5; i++ {
			if strings.TrimSpace(errorLines[i]) != "" {
				log("[错误输出] %s", strings.TrimSpace(errorLines[i]))
			}
		}
	}

	output := result.Output
	outputLower := strings.ToLower(output)

	// 检查是否包含验证标记（支持POC中的标记和标准VULNERABLE标记）
	matched := false
	if strings.Contains(outputLower, "vulnerable") {
		matched = true
		log("[验证] ✓ 检测到漏洞特征 (VULNERABLE)")
	} else if vulnMarker != "VULNERABLE" && strings.Contains(output, vulnMarker) {
		matched = true
		log("[验证] ✓ 检测到漏洞特征 (%s)", vulnMarker)
		log("[验证] 注意：建议使用标准标记 VULNERABLE")
	} else {
		log("[验证] ✗ 未检测到漏洞特征")
		if vulnMarker != "VULNERABLE" {
			log("[验证] 期望标记: VULNERABLE 或 %s", vulnMarker)
		} else {
			log("[验证] 期望标记: VULNERABLE")
		}
	}

	result.Matched = matched

	// 尝试提取命令输出
	extractDemo := extractOutput(result.Output, log)
	if extractDemo != "" {
		result.CanExtract = true
		result.ExtractDemo = extractDemo
		log("[提取] ✓ 成功提取命令输出")

		// 显示提取的内容
		extractLines := strings.Split(extractDemo, "\n")
		for i, line := range extractLines {
			if i < 3 && strings.TrimSpace(line) != "" {
				log("[提取] 输出内容: %s", strings.TrimSpace(line))
			}
		}

		// 验证提取的内容是否是有效的命令输出
		// 对于RCE类型，应该包含类似用户名、路径等信息
		if category == CatCommandInjection || category == CatCodeExecution {
			// 检查是否包含典型的命令输出特征
			hasValidOutput := false

			// 检查是否包含NEONSCAN_TEST标记（说明命令执行成功）
			if strings.Contains(extractDemo, "NEONSCAN_TEST") {
				hasValidOutput = true
				log("[验证] ✓ 检测到NEONSCAN_TEST标记，命令执行成功")
			}

			// 检查是否包含典型的Linux/Unix输出（用户名、路径等）
			if strings.Contains(extractDemo, "root") ||
				strings.Contains(extractDemo, "www-data") ||
				strings.Contains(extractDemo, "/") ||
				strings.Contains(extractDemo, "uid=") ||
				strings.Contains(extractDemo, "gid=") {
				hasValidOutput = true
				log("[验证] ✓ 检测到典型的命令输出特征")
			}

			// 如果没有有效输出，标记为无法提取
			if !hasValidOutput {
				result.CanExtract = false
				log("[验证] ✗ 提取的内容不是有效的命令输出")
			}
		}
	} else {
		log("[提取] ✗ 无法提取命令输出")
	}

	return result
}

func extractOutput(output string, log func(string, ...interface{})) string {
	// 首先尝试使用NEONSCAN标记提取
	re := regexp.MustCompile(`NEONSCAN_BEGIN(.*?)NEONSCAN_END`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		extracted := strings.TrimSpace(matches[1])
		// 确保提取的内容不为空，且不包含HTML标签
		if extracted != "" && !strings.Contains(extracted, "<") && !strings.Contains(extracted, ">") {
			// 验证提取的内容是有效的命令输出
			// 不能只是"VULNERABLE"或空白
			if extracted != "VULNERABLE" && extracted != "NOT VULNERABLE" && len(extracted) > 3 {
				log("[提取] 使用NEONSCAN标记提取到: %s", extracted)
				return extracted
			}
		}
	}

	// 如果NEONSCAN标记提取失败，尝试从输出中提取有用的行
	lines := strings.Split(output, "\n")
	var usefulLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 跳过日志行、状态行和无用的行
		if strings.Contains(strings.ToLower(line), "vulnerable") ||
			strings.Contains(strings.ToLower(line), "not vulnerable") ||
			strings.HasPrefix(line, "[") || // 跳过日志行 [INFO], [ERROR]等
			strings.HasPrefix(line, "-") ||
			strings.HasPrefix(line, "+") ||
			strings.HasPrefix(line, "*") ||
			strings.Contains(line, "request failed") || // 跳过错误信息
			strings.Contains(line, "No connection") {
			continue
		}
		// 只保留看起来像命令输出的行
		if !strings.Contains(line, "<") && !strings.Contains(line, ">") && len(line) < 200 {
			usefulLines = append(usefulLines, line)
		}
	}

	if len(usefulLines) > 0 {
		extracted := strings.Join(usefulLines[:min(len(usefulLines), 3)], "\n")
		log("[提取] 从输出中提取到: %s", extracted)
		return extracted
	}

	log("[提取] 未能提取到有效的命令输出")
	return ""
}

func analyzeFailure(result ExpVerifyResult, category VulnCategory, log func(string, ...interface{})) string {
	var reasons []string

	// 如果Python本身无法运行，这是致命错误
	if !result.Success && (strings.Contains(result.Error, "Python命令执行失败") ||
		strings.Contains(result.Error, "executable file not found")) {
		reasons = append(reasons, "Python命令执行失败，这是环境问题，不是代码问题")
		return strings.Join(reasons, "; ")
	}

	// Python可以运行，但脚本有错误 - 这些都可以通过AI修复

	// 1. 检查参数相关错误
	if strings.Contains(result.Error, "the following arguments are required") {
		// 提取缺少的参数
		re := regexp.MustCompile(`required: (.+)`)
		if matches := re.FindStringSubmatch(result.Error); len(matches) > 1 {
			reasons = append(reasons, fmt.Sprintf("缺少必需的命令行参数: %s", matches[1]))
		} else {
			reasons = append(reasons, "缺少必需的命令行参数")
		}
	}

	if strings.Contains(result.Error, "unrecognized arguments") {
		reasons = append(reasons, "命令行参数解析错误，可能参数名称不正确")
	}

	// 2. 检查URL格式错误
	if strings.Contains(result.Error, "No connection adapters were found") {
		reasons = append(reasons, "URL格式错误：目标URL缺少http://或https://前缀，需要在代码中自动添加")
	}

	if strings.Contains(result.Error, "Invalid URL") || strings.Contains(result.Error, "No scheme supplied") {
		reasons = append(reasons, "URL格式无效，需要确保URL包含完整的scheme（http://或https://）")
	}

	// 3. 检查网络连接错误
	if strings.Contains(result.Error, "Connection refused") || strings.Contains(result.Error, "connection refused") {
		reasons = append(reasons, "目标服务器拒绝连接，可能端口不正确或服务未运行")
	}

	if strings.Contains(result.Error, "timeout") || strings.Contains(result.Error, "timed out") {
		reasons = append(reasons, "连接超时，目标服务器可能无响应或网络不通")
	}

	if strings.Contains(result.Error, "Name or service not known") || strings.Contains(result.Error, "nodename nor servname provided") {
		reasons = append(reasons, "无法解析目标主机名，请检查URL是否正确")
	}

	// 4. 检查Python语法错误
	if strings.Contains(result.Error, "SyntaxError") {
		reasons = append(reasons, "Python语法错误，需要修复代码语法")
	}

	if strings.Contains(result.Error, "IndentationError") {
		reasons = append(reasons, "Python缩进错误，需要修复代码缩进")
	}

	if strings.Contains(result.Error, "NameError") {
		re := regexp.MustCompile(`name '(.+?)' is not defined`)
		if matches := re.FindStringSubmatch(result.Error); len(matches) > 1 {
			reasons = append(reasons, fmt.Sprintf("变量未定义: %s", matches[1]))
		} else {
			reasons = append(reasons, "变量或函数未定义")
		}
	}

	// 5. 检查模块导入错误
	if strings.Contains(result.Error, "No module named") {
		re := regexp.MustCompile(`No module named '(.+?)'`)
		if matches := re.FindStringSubmatch(result.Error); len(matches) > 1 {
			moduleName := matches[1]
			// 特别处理 readline 模块（Windows 不支持）
			if moduleName == "readline" {
				reasons = append(reasons, "【致命错误】代码导入了 readline 模块，但该模块在 Windows 上不可用。必须移除所有 readline 相关的导入和功能。")
			} else {
				reasons = append(reasons, fmt.Sprintf("缺少Python模块: %s，需要添加导入或安装", moduleName))
			}
		} else {
			reasons = append(reasons, "缺少必需的Python模块")
		}
	}

	// 6. 检查HTTP请求错误
	if strings.Contains(result.Error, "404") || strings.Contains(result.Error, "Not Found") {
		reasons = append(reasons, "HTTP 404错误，请求的路径不存在，需要检查URL路径")
	}

	if strings.Contains(result.Error, "500") || strings.Contains(result.Error, "Internal Server Error") {
		reasons = append(reasons, "HTTP 500错误，服务器内部错误，可能payload格式不正确")
	}

	// 7. 检查漏洞检测结果
	if result.Success && strings.Contains(strings.ToLower(result.Output), "not vulnerable") {
		reasons = append(reasons, "目标不存在该漏洞，或payload不正确")
	}

	if result.Success && !result.Matched && !strings.Contains(result.Output, "vulnerable") {
		reasons = append(reasons, "脚本执行成功但未输出VULNERABLE标记，需要添加漏洞检测逻辑和输出")
	}

	// 8. 检查输出提取问题
	if result.Matched && !result.CanExtract {
		// 检查是否是因为输出无效
		if strings.Contains(result.Output, "VULNERABLE") && !strings.Contains(result.Output, "NEONSCAN_TEST") {
			reasons = append(reasons, "检测到VULNERABLE标记但命令未实际执行，可能是URL格式错误或连接失败，需要检查URL处理逻辑")
		} else {
			reasons = append(reasons, "检测到漏洞但无法提取命令输出，需要改进输出提取逻辑（使用NEONSCAN_BEGIN/END标记）")
		}
	}

	// 如果没有具体错误但执行失败，添加通用错误信息
	if len(reasons) == 0 && !result.Success {
		if result.Error != "" {
			// 提取错误的关键部分
			errorLines := strings.Split(result.Error, "\n")
			var keyErrors []string
			for _, line := range errorLines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "Warning") && !strings.Contains(line, "Traceback") {
					keyErrors = append(keyErrors, line)
					if len(keyErrors) >= 3 {
						break
					}
				}
			}
			if len(keyErrors) > 0 {
				reasons = append(reasons, fmt.Sprintf("执行错误: %s", strings.Join(keyErrors, "; ")))
			} else {
				reasons = append(reasons, "执行失败，原因未知")
			}
		} else {
			reasons = append(reasons, "执行失败，无错误信息")
		}
	}

	// 如果一切正常，返回空字符串
	if result.Success && result.Matched && result.CanExtract {
		return ""
	}

	reasonStr := strings.Join(reasons, "\n")
	log("[分析] 失败原因:")
	for _, reason := range reasons {
		log("[分析]   - %s", reason)
	}
	return reasonStr
}

func buildCorrectionPrompt(expSpec ExpSpec, currentCode, failureReason string, category VulnCategory, testCmd string, result ExpVerifyResult, log func(string, ...interface{})) string {
	var prompt strings.Builder

	prompt.WriteString("请修正以下Python EXP代码中的问题。\n\n")
	prompt.WriteString(fmt.Sprintf("漏洞类型: %s\n", category))
	prompt.WriteString(fmt.Sprintf("测试命令: %s\n\n", testCmd))

	prompt.WriteString("原始EXP规范:\n")
	prompt.WriteString(fmt.Sprintf("- 名称: %s\n", expSpec.Name))
	prompt.WriteString(fmt.Sprintf("- 利用建议: %s\n", expSpec.ExploitSuggestion))
	if len(expSpec.Steps) > 0 {
		prompt.WriteString(fmt.Sprintf("- 步骤数: %d\n", len(expSpec.Steps)))
		for i, step := range expSpec.Steps {
			prompt.WriteString(fmt.Sprintf("  步骤%d: %s %s\n", i+1, step.Method, step.Path))
			if step.Body != "" {
				prompt.WriteString(fmt.Sprintf("    Body: %s\n", step.Body))
			}
		}
	}

	prompt.WriteString("\n=== 失败原因分析 ===\n")
	prompt.WriteString(failureReason)
	prompt.WriteString("\n\n")

	// 如果有实际输出，显示给AI
	if result.Output != "" {
		prompt.WriteString("=== 脚本标准输出 ===\n")
		prompt.WriteString(result.Output)
		prompt.WriteString("\n\n")
	}

	// 如果有错误输出，显示给AI
	if result.Error != "" {
		prompt.WriteString("=== 脚本错误输出 ===\n")
		prompt.WriteString(result.Error)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("=== 修正要求 ===\n")
	prompt.WriteString("1. 只输出修正后的完整Python代码，不要任何解释或markdown标记\n")
	prompt.WriteString("2. 代码必须是可以直接运行的完整脚本\n\n")

	// 根据具体错误类型给出针对性的修正建议
	if strings.Contains(failureReason, "缺少必需的命令行参数") ||
		strings.Contains(result.Error, "the following arguments are required") {
		prompt.WriteString("【关键修复】命令行参数问题：\n")
		prompt.WriteString("- 确保使用argparse正确定义所有参数\n")
		prompt.WriteString("- --target 参数必须是required=True\n")
		prompt.WriteString("- --cmd 参数应该有默认值或设为可选\n")
		prompt.WriteString("- 示例代码：\n")
		prompt.WriteString("  parser.add_argument('--target', required=True, help='Target URL')\n")
		prompt.WriteString("  parser.add_argument('--cmd', default='whoami', help='Command to execute')\n\n")
	}

	if strings.Contains(failureReason, "URL格式错误") ||
		strings.Contains(result.Error, "No connection adapters") ||
		strings.Contains(result.Error, "Invalid URL") {
		prompt.WriteString("【关键修复】URL格式问题：\n")
		prompt.WriteString("- 在使用target之前，检查并添加http://前缀\n")
		prompt.WriteString("- 使用urllib.parse.urljoin正确拼接URL路径\n")
		prompt.WriteString("- 示例代码：\n")
		prompt.WriteString("  from urllib.parse import urljoin\n")
		prompt.WriteString("  if not target.startswith('http://') and not target.startswith('https://'):\n")
		prompt.WriteString("      target = 'http://' + target\n")
		prompt.WriteString("  url = urljoin(target, '/path/to/endpoint')\n\n")
	}

	if strings.Contains(failureReason, "未输出VULNERABLE标记") || !result.Matched {
		prompt.WriteString("【关键修复】漏洞检测输出：\n")
		prompt.WriteString("- 成功利用后必须输出 'VULNERABLE' 标记\n")
		prompt.WriteString("- 失败时输出 'NOT VULNERABLE'\n")
		prompt.WriteString("- 示例代码：\n")
		prompt.WriteString("  if success:\n")
		prompt.WriteString("      print('VULNERABLE')\n")
		prompt.WriteString("  else:\n")
		prompt.WriteString("      print('NOT VULNERABLE')\n\n")
	}

	if strings.Contains(failureReason, "无法提取命令输出") || (result.Matched && !result.CanExtract) {
		prompt.WriteString("【关键修复】命令输出提取：\n")

		// 检查是否是URL格式导致命令未执行
		if strings.Contains(result.Output, "VULNERABLE") && !strings.Contains(result.Output, "NEONSCAN_TEST") {
			prompt.WriteString("- **重要**：虽然输出了VULNERABLE，但命令并未实际执行\n")
			prompt.WriteString("- 这通常是因为URL格式错误导致请求失败\n")
			prompt.WriteString("- 必须确保URL包含http://或https://前缀\n")
			prompt.WriteString("- 必须正确拼接URL路径\n")
			prompt.WriteString("- 示例代码：\n")
			prompt.WriteString("  # 1. 确保URL有scheme\n")
			prompt.WriteString("  if not target.startswith('http://') and not target.startswith('https://'):\n")
			prompt.WriteString("      target = 'http://' + target\n")
			prompt.WriteString("  \n")
			prompt.WriteString("  # 2. 正确拼接URL\n")
			prompt.WriteString("  from urllib.parse import urljoin\n")
			prompt.WriteString("  url = urljoin(target, '/index.php?s=captcha')\n")
			prompt.WriteString("  \n")
			prompt.WriteString("  # 3. 发送请求并检查响应\n")
			prompt.WriteString("  response = requests.post(url, data=payload, timeout=10)\n")
			prompt.WriteString("  print(f'[INFO] Response status: {response.status_code}')\n")
			prompt.WriteString("  print(f'[INFO] Response length: {len(response.text)}')\n\n")
		} else {
			prompt.WriteString("- 使用 NEONSCAN_BEGIN 和 NEONSCAN_END 标记包裹命令输出\n")
			prompt.WriteString("- 在payload中添加：echo NEONSCAN_BEGIN; <命令>; echo NEONSCAN_END\n")
			prompt.WriteString("- 使用正则提取：re.search(r'NEONSCAN_BEGIN(.*?)NEONSCAN_END', text, re.DOTALL)\n")
			prompt.WriteString("- 示例代码：\n")
			prompt.WriteString("  cmd_with_markers = f'echo NEONSCAN_BEGIN; {cmd}; echo NEONSCAN_END'\n")
			prompt.WriteString("  # 发送请求...\n")
			prompt.WriteString("  match = re.search(r'NEONSCAN_BEGIN(.*?)NEONSCAN_END', response.text, re.DOTALL)\n")
			prompt.WriteString("  if match:\n")
			prompt.WriteString("      output = match.group(1).strip()\n")
			prompt.WriteString("      print(f'Command output: {output}')\n\n")
		}
	}

	if strings.Contains(result.Error, "SyntaxError") || strings.Contains(result.Error, "IndentationError") {
		prompt.WriteString("【关键修复】Python语法错误：\n")
		prompt.WriteString("- 检查所有括号、引号是否配对\n")
		prompt.WriteString("- 确保缩进一致（使用4个空格）\n")
		prompt.WriteString("- 检查函数定义和调用是否正确\n\n")
	}

	if strings.Contains(result.Error, "NameError") || strings.Contains(result.Error, "not defined") {
		prompt.WriteString("【关键修复】变量未定义：\n")
		prompt.WriteString("- 确保所有变量在使用前已定义\n")
		prompt.WriteString("- 检查变量名拼写是否正确\n")
		prompt.WriteString("- 确保导入了所有需要的模块\n\n")
	}

	// 特别处理 readline 模块错误（Windows 不支持）
	if strings.Contains(result.Error, "No module named 'readline'") || strings.Contains(failureReason, "readline") {
		prompt.WriteString("【致命错误修复】readline 模块问题（Windows 不支持）：\n")
		prompt.WriteString("- 立即删除所有 `import readline` 语句\n")
		prompt.WriteString("- 如果代码中有 readline 相关的功能（如 shell 模式的输入），必须完全移除\n")
		prompt.WriteString("- 示例修复：\n")
		prompt.WriteString("  # 删除这一行：\n")
		prompt.WriteString("  # import readline\n\n")
		prompt.WriteString("  # 如果 --shell 模式使用了 readline，直接删除该功能，保留 --cmd 单命令模式\n\n")
	}

	// 通用要求
	prompt.WriteString("【通用要求】\n")
	prompt.WriteString("3. 添加必要的导入语句（requests, re, sys, argparse等）\n")
	prompt.WriteString("4. 添加[INFO]级别的日志输出，显示执行进度\n")
	prompt.WriteString("5. 使用try-except处理异常，避免脚本崩溃\n")
	prompt.WriteString("6. 禁用urllib3警告：\n")
	prompt.WriteString("   import urllib3\n")
	prompt.WriteString("   urllib3.disable_warnings()\n")
	prompt.WriteString("7. 设置合理的超时时间（timeout=10）\n")
	prompt.WriteString("8. 不要打印整页HTML，只打印关键信息\n")

	prompt.WriteString("\n=== 当前代码 ===\n")
	prompt.WriteString("```python\n")
	prompt.WriteString(currentCode)
	prompt.WriteString("\n```\n")

	return prompt.String()
}

func tryAlternativeFix(currentCode string, result ExpVerifyResult, category VulnCategory, testCmd string, log func(string, ...interface{})) string {
	log("[备用修复] 尝试自动修复...")

	// 修复 readline 模块错误（Windows 不支持）
	if strings.Contains(result.Error, "No module named 'readline'") {
		log("[备用修复] 检测到 readline 模块错误，尝试自动移除...")
		fixed := removeReadlineImport(currentCode, log)
		if fixed != currentCode {
			return fixed
		}
	}

	// 修复URL格式问题
	if strings.Contains(result.Error, "No connection adapters") ||
		strings.Contains(result.Error, "Invalid URL") ||
		strings.Contains(result.Error, "No scheme supplied") {
		log("[备用修复] 检测到URL格式错误，添加URL前缀检查...")
		return fixURLScheme(currentCode, log)
	}

	if strings.Contains(result.Error, "SyntaxError") || strings.Contains(result.Error, "IndentationError") {
		log("[备用修复] 检测到语法错误，尝试修复...")
		return fixSyntaxError(currentCode, log)
	}

	if !result.Success || !result.Matched {
		log("[备用修复] 尝试添加更好的输出提取逻辑...")
		return improveOutputExtraction(currentCode, log)
	}

	return currentCode
}

// fixURLScheme 修复URL格式问题
func fixURLScheme(code string, log func(string, ...interface{})) string {
	lines := strings.Split(code, "\n")
	var fixedLines []string
	inMainFunction := false
	targetAssigned := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检测main函数
		if strings.Contains(trimmed, "def main(") {
			inMainFunction = true
		}

		// 在main函数中，找到target赋值后添加URL前缀检查
		if inMainFunction && !targetAssigned {
			if strings.Contains(trimmed, "target = args.target") ||
				strings.Contains(trimmed, "base = args.target") {
				fixedLines = append(fixedLines, line)

				// 添加URL前缀检查
				indent := ""
				for j := 0; j < len(line); j++ {
					if line[j] == ' ' || line[j] == '\t' {
						indent += string(line[j])
					} else {
						break
					}
				}

				fixedLines = append(fixedLines, indent+"# 确保URL包含scheme")
				fixedLines = append(fixedLines, indent+"if not target.startswith('http://') and not target.startswith('https://'):")
				fixedLines = append(fixedLines, indent+"    target = 'http://' + target")

				targetAssigned = true
				log("[备用修复] 已添加URL前缀检查")
				continue
			}
		}

		fixedLines = append(fixedLines, line)
	}

	if !targetAssigned {
		log("[备用修复] 未找到target赋值语句，尝试在argparse后添加...")
		// 如果没有找到target赋值，在args解析后添加
		fixedLines = []string{}
		for _, line := range lines {
			fixedLines = append(fixedLines, line)

			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "args = parser.parse_args()") {
				// 获取缩进
				indent := ""
				for j := 0; j < len(line); j++ {
					if line[j] == ' ' || line[j] == '\t' {
						indent += string(line[j])
					} else {
						break
					}
				}

				fixedLines = append(fixedLines, "")
				fixedLines = append(fixedLines, indent+"# 确保URL包含scheme")
				fixedLines = append(fixedLines, indent+"target = args.target")
				fixedLines = append(fixedLines, indent+"if not target.startswith('http://') and not target.startswith('https://'):")
				fixedLines = append(fixedLines, indent+"    target = 'http://' + target")
				fixedLines = append(fixedLines, indent+"args.target = target")

				log("[备用修复] 已在args解析后添加URL前缀检查")
			}
		}
	}

	fixed := strings.Join(fixedLines, "\n")
	return fixed
}

func fixSyntaxError(code string, log func(string, ...interface{})) string {
	lines := strings.Split(code, "\n")
	var fixedLines []string
	inFunction := false
	indentLevel := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "def ") && !strings.Contains(trimmed, "#") {
			inFunction = true
		}

		if inFunction && indentLevel == 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			ws := ""
			for i := 0; i < len(line); i++ {
				if line[i] == ' ' || line[i] == '\t' {
					ws += string(line[i])
				} else {
					break
				}
			}
			indentLevel = len(ws)
		}

		if trimmed == "" {
			fixedLines = append(fixedLines, line)
			continue
		}

		if strings.Contains(trimmed, "print(") && !strings.Contains(trimmed, "print(f") && !strings.Contains(trimmed, "print(") {
			if !strings.HasPrefix(trimmed, "#") {
				fixedLines = append(fixedLines, line)
			} else {
				fixedLines = append(fixedLines, line)
			}
			continue
		}

		fixedLines = append(fixedLines, line)
	}

	fixed := strings.Join(fixedLines, "\n")
	if fixed != code {
		log("[备用修复] 已修复语法问题")
	}
	return fixed
}

// removeReadlineImport 移除 readline 模块导入（Windows 不支持）
func removeReadlineImport(code string, log func(string, ...interface{})) string {
	lines := strings.Split(code, "\n")
	var fixedLines []string
	removed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 检测并移除 readline 导入
		if strings.Contains(trimmed, "import readline") || strings.HasPrefix(trimmed, "import readline;") {
			if !strings.HasPrefix(trimmed, "#") { // 跳过注释行
				removed = true
				log("[备用修复] 已移除: %s", strings.TrimSpace(line))
				continue
			}
		}
		// 同时移除使用 readline 的代码块（如 if readline: 等）
		if strings.Contains(trimmed, "readline.") && !strings.HasPrefix(trimmed, "#") {
			// 检查是否是简单的 readline 调用，如果是则移除
			if strings.Contains(trimmed, "readline.get") || strings.Contains(trimmed, "readline.set") {
				removed = true
				log("[备用修复] 已移除 readline 调用: %s", strings.TrimSpace(line))
				continue
			}
		}
		fixedLines = append(fixedLines, line)
	}

	fixed := strings.Join(fixedLines, "\n")
	if removed {
		log("[备用修复] 已移除 readline 模块导入和相关调用")
	}
	return fixed
}

func improveOutputExtraction(code string, log func(string, ...interface{})) string {
	improvedCode := code

	improvePattern := []string{
		`re.search\(r'NEONSCAN_BEGIN(.*?)NEONSCAN_END', resp\.text`,
		`re.search(r'NEONSCAN_BEGIN(.*?)NEONSCAN_END', text`,
	}

	for _, pattern := range improvePattern {
		if strings.Contains(improvedCode, pattern) && !strings.Contains(improvedCode, "re.DOTALL") {
			improvedCode = strings.Replace(improvedCode, pattern, pattern+", re.DOTALL", 1)
			log("[备用修复] 已添加 re.DOTALL 标志")
			break
		}
	}

	if !strings.Contains(improvedCode, "NEONSCAN_BEGIN") || !strings.Contains(improvedCode, "NEONSCAN_END") {
		if strings.Contains(improvedCode, "{{cmd") || strings.Contains(improvedCode, "{{command") {
			log("[备用修复] 添加命令输出标记...")
			improvedCode = addEchoMarkers(improvedCode, log)
		}
	}

	return improvedCode
}

func addEchoMarkers(code string, log func(string, ...interface{})) string {
	lines := strings.Split(code, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "{{cmd}}") || strings.Contains(trimmed, "{{cmd_urlenc}}") {
			result = append(result, `        echo NEONSCAN_BEGIN; `+trimmed+`; echo NEONSCAN_END`)
		} else {
			result = append(result, line)
		}
	}

	fixed := strings.Join(result, "\n")
	log("[备用修复] 已添加输出标记")
	return fixed
}

func requestExpCorrection(provider mcp.AIProvider, prompt string, log func(string, ...interface{})) string {
	if provider == nil {
		log("[修正] 没有可用的AI Provider，无法生成修正版本")
		return ""
	}

	log("[修正] 请求AI生成修正版本...")

	messages := []mcp.ChatMessage{
		{Role: "system", Content: "你是一位专业的渗透测试开发者，擅长修正EXP代码。请直接输出修正后的Python代码，不要任何解释。", Time: time.Now().Format(time.RFC3339)},
		{Role: "user", Content: prompt, Time: time.Now().Format(time.RFC3339)},
	}

	content, _, err := provider.Chat(messages, nil)
	if err != nil {
		log("[修正] AI调用失败: %v", err)
		return ""
	}

	code := stripCodeFence(content)
	if strings.TrimSpace(code) == "" {
		log("[修正] AI返回为空")
		return ""
	}

	log("[修正] 收到修正版本，长度: %d 字符", len(code))
	return code
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
