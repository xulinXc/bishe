// Package shouji 提供解包与JS信息收集功能
// 该包实现了与FFUF、URLFinder、Packer-Fuzzer等工具的集成
// 支持目录爆破、JS/URL收集、代码解包等功能
package shouji

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// jsonMap JSON映射类型别名，用于简化代码
type jsonMap = map[string]any

// normalizeURL 规范化URL，确保包含协议（http:// 或 https://）
// 如果URL没有协议前缀，默认添加 https://
// @param url 原始URL字符串
// @return 规范化后的URL
func normalizeURL(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return url
	}
	if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
		url = "https://" + url
	}
	return url
}

// tryGetURL 尝试获取URL内容，优先使用HTTPS，失败则尝试HTTP
// @param rawURL 原始URL
// @param timeout 请求超时时间
// @return 响应体内容、最终使用的URL、协议信息、错误信息
func tryGetURL(rawURL string, timeout time.Duration) ([]byte, string, string, error) {
	url := normalizeURL(rawURL)
	client := &http.Client{Timeout: timeout}
	originalURL := url
	resp, httpsErr := client.Get(url)
	if httpsErr == nil && resp != nil {
		if resp.StatusCode < 500 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
				return b, url, fmt.Sprintf("使用HTTPS协议访问: %s", url), nil
			}
			return b, url, fmt.Sprintf("使用HTTPS协议: %s", url), nil
		}
		resp.Body.Close()
	}
	if strings.HasPrefix(strings.ToLower(url), "https://") {
		httpURL := strings.Replace(url, "https://", "http://", 1)
		resp, httpErr := client.Get(httpURL)
		if httpErr == nil && resp != nil {
			if resp.StatusCode < 500 {
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				msg := fmt.Sprintf("HTTPS访问失败，已切换到HTTP: %s -> %s", originalURL, httpURL)
				return b, httpURL, msg, nil
			}
			resp.Body.Close()
		}
		httpsErrMsg := "无错误"
		if httpsErr != nil {
			httpsErrMsg = httpsErr.Error()
		}
		httpErrMsg := "无错误"
		if httpErr != nil {
			httpErrMsg = httpErr.Error()
		}
		return nil, "", "", fmt.Errorf("无法访问URL（已尝试HTTPS和HTTP）。HTTPS错误: %s；HTTP错误: %s", httpsErrMsg, httpErrMsg)
	}
	if httpsErr != nil {
		return nil, "", "", fmt.Errorf("无法访问URL（HTTP）：%v", httpsErr)
	}
	return nil, "", "", fmt.Errorf("无法访问URL：服务器返回错误状态码")
}

// AnalyzePackable 分析目标URL是否可能使用了打包/混淆工具
// 检测常见的打包模式（如webpack、vite、rollup等现代打包工具，以及传统打包器）
// @param w HTTP响应写入器
// @param r HTTP请求对象
func AnalyzePackable(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		URL string `json:"url"`
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "请求格式错误：缺少URL参数", http.StatusBadRequest)
		return
	}
	b, finalURL, protocolMsg, err := tryGetURL(req.URL, 8*time.Second)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	body := string(b)
	score := 0

	// 检查HTML页面中的常见打包模式（传统打包器）
	patterns := []string{"eval(function(p,a,c,k,e,d)", "Function(\"return this\")()", "atob(", "fromCharCode", "unescape(\"%"}
	for _, p := range patterns {
		if strings.Contains(body, p) {
			score++
		}
	}
	reHex := regexp.MustCompile(`['\"](?:[0-9a-f]{200,})['\"]`)
	reB64 := regexp.MustCompile(`[A-Za-z0-9+/]{200,}={0,2}`)
	if reHex.FindStringIndex(body) != nil {
		score++
	}
	if reB64.FindStringIndex(body) != nil {
		score++
	}

	// 检查现代打包工具标识（webpack, vite, rollup等）
	modernPackerPatterns := []string{
		"webpack", "webpackChunkName", "__webpack_require__", "webpack://",
		"__webpack_modules__", "webpackJsonp", "webpack_require",
		"__WEBPACK__", "chunkLoadingGlobal", "webpackBootstrap",
		"vite/", "__vite__", "import.meta.env", "vite/client",
		"__ROLLUP__", "__rollup", "rollup-plugin",
		"parcel", "__parcel__", "parcelRequire",
		"__PACKAGE__", "bundle.js", "chunk.", "main.", "app.",
	}
	for _, p := range modernPackerPatterns {
		if strings.Contains(strings.ToLower(body), strings.ToLower(p)) {
			score += 2 // 现代打包工具权重更高
			break
		}
	}

	// 检查HTML中引用的JS文件数量和模式（webpack通常生成多个chunk文件）
	jsFilePattern := regexp.MustCompile(`<script[^>]*src=["']([^"']+\.js[^"']*)["']`)
	jsFiles := jsFilePattern.FindAllStringSubmatch(body, -1)

	// 检查是否有多个JS文件引用（通常表示打包工具）
	if len(jsFiles) >= 3 {
		score++
	}

	// 检查JS文件名模式（webpack/vite等现代工具的特征）
	for _, match := range jsFiles {
		if len(match) > 1 {
			jsPath := strings.ToLower(match[1])
			// webpack chunk文件通常包含hash或数字（例如：main-0190b75afa062468.js 或 4183-fd69e8b1c922647a.js）
			if regexp.MustCompile(`[\w-]+-[\da-f]{8,}\.js`).MatchString(jsPath) ||
				regexp.MustCompile(`[\w-]+\.[\da-f]{8,}\.js`).MatchString(jsPath) ||
				regexp.MustCompile(`[\w-]+\.[\d]+\.[\w-]+\.js`).MatchString(jsPath) {
				score += 2
				break
			}
			// 检查常见的打包文件名（main, app, vendor, chunk, bundle, runtime, polyfills等）
			if regexp.MustCompile(`(main|app|vendor|chunk|bundle|runtime|polyfills|framework|webpack)[\.-][\w-]*\.js`).MatchString(jsPath) {
				score++
				break
			}
		}
	}

	// 尝试检查JS文件内容（如果有明显的JS引用，下载并检查前几个）
	if len(jsFiles) > 0 && score < 2 {
		client := &http.Client{Timeout: 5 * time.Second}
		checked := 0
		maxCheck := 3 // 最多检查3个JS文件
		for _, match := range jsFiles {
			if checked >= maxCheck {
				break
			}
			if len(match) > 1 {
				jsURL := match[1]
				// 处理相对路径
				if strings.HasPrefix(jsURL, "//") {
					jsURL = "https:" + jsURL
				} else if strings.HasPrefix(jsURL, "/") {
					u, err := url.Parse(finalURL)
					if err == nil {
						jsURL = u.Scheme + "://" + u.Host + jsURL
					}
				} else if !strings.HasPrefix(jsURL, "http://") && !strings.HasPrefix(jsURL, "https://") {
					u, err := url.Parse(finalURL)
					if err == nil {
						base := u.Scheme + "://" + u.Host
						if u.Path != "" {
							base += filepath.Dir(u.Path)
						}
						if !strings.HasSuffix(base, "/") {
							base += "/"
						}
						jsURL = base + jsURL
					}
				}

				// 下载并检查JS文件
				resp, err := client.Get(jsURL)
				if err == nil && resp != nil && resp.StatusCode == 200 {
					jsContent, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					jsBody := string(jsContent)

					// 检查JS文件中的打包标识
					for _, p := range modernPackerPatterns {
						if strings.Contains(strings.ToLower(jsBody), strings.ToLower(p)) {
							score += 3         // JS文件中发现打包标识，权重很高
							checked = maxCheck // 找到后不再检查其他文件
							break
						}
					}
					// 检查JS文件中是否有大量压缩代码（单行很长，没有换行）
					if len(jsBody) > 50000 {
						lines := strings.Split(jsBody, "\n")
						if len(lines) < 10 && len(jsBody) > 50000 {
							score++ // 大文件且行数少，可能是打包后的代码
						}
					}
					checked++
				}
			}
		}
	}

	res := jsonMap{"packable": score >= 2, "score": score, "finalURL": finalURL, "protocolMsg": protocolMsg}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// shoujiBaseDir 解析shouji工具的基础目录
// 查找项目中的shouji目录路径，用于定位工具可执行文件
// @return shouji目录的绝对路径
func shoujiBaseDir() string {
	// assume project structure keeps original shouji folder at repo root
	cwd, _ := os.Getwd()
	// prefer repo/shouji/tools
	p := filepath.Join(cwd, "shouji")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return cwd
}

// findBin 查找可执行文件
// 优先在shouji/tools/bin目录下查找，如果找不到则在系统PATH中查找
// @param bin 可执行文件名（如"ffuf"、"urlfinder"）
// @return 可执行文件的完整路径
func findBin(bin string) string {
	ext := ".exe"
	binName := bin
	if !strings.HasSuffix(strings.ToLower(bin), ext) {
		binName = bin + ext
	}
	localPath := filepath.Join(shoujiBaseDir(), "tools", "bin", binName)
	if _, err := os.Stat(localPath); err == nil {
		abs, _ := filepath.Abs(localPath)
		return abs
	}
	return bin
}

// findPythonScript 查找Python脚本文件
// 在shouji/tools目录下查找指定的Python脚本
// @param scriptPath 脚本相对路径（如"Packer-Fuzzer/PackerFuzzer.py"）
// @return 脚本的绝对路径和是否找到的标志
func findPythonScript(scriptPath string) (string, bool) {
	localPath := filepath.Join(shoujiBaseDir(), "tools", scriptPath)
	if _, err := os.Stat(localPath); err == nil {
		abs, _ := filepath.Abs(localPath)
		return abs, true
	}
	return scriptPath, false
}

// runCmd 执行外部命令
// 执行指定的可执行文件，并捕获标准输出和标准错误
// @param bin 可执行文件名
// @param args 命令参数
// @return 标准输出、标准错误、错误信息
func runCmd(bin string, args ...string) (string, string, error) {
	binPath := findBin(bin)
	var stdout, stderr bytes.Buffer
	// 设置超时时间（URLFinder 可能需要较长时间，设置10分钟超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	// 设置工作目录为项目根目录，确保相对路径能正确解析
	wd, err := os.Getwd()
	if err == nil {
		cmd.Dir = wd
		fmt.Printf("[DEBUG] runCmd 工作目录: %s\n", wd)
	}
	fmt.Printf("[DEBUG] runCmd 执行: %s %v\n", binPath, args)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	fmt.Printf("[DEBUG] runCmd 返回: stdout_len=%d, stderr_len=%d, err=%v\n", len(stdoutStr), len(stderrStr), err)
	return stdoutStr, stderrStr, err
}

// RunFfuf FFUF目录爆破处理器
// 调用ffuf工具进行目录/文件爆破扫描
// 请求参数：baseUrl（目标URL）、wordlist（字典文件路径）、threads（并发数）、
//
//	mc（匹配状态码）、maxSeconds（最大执行时间）、rate（请求速率）、reqTimeout（请求超时）
//
// @param w HTTP响应写入器
// @param r HTTP请求对象
func RunFfuf(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("[DEBUG FFUF] ========== 开始 FFUF 请求 ==========\n")
	startTime := time.Now()

	type Req struct {
		BaseURL, Wordlist, MatchCodes string
		Threads                       int
		MaxSeconds                    int
		Rate                          int
		ReqTimeout                    int
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("[DEBUG FFUF] ❌ JSON解析失败: %v\n", err)
		http.Error(w, "请求格式错误：无法解析JSON数据", 400)
		return
	}
	fmt.Printf("[DEBUG FFUF] 接收参数: baseURL=%s, wordlist长度=%d, threads=%d, maxSeconds=%d\n",
		req.BaseURL, len(req.Wordlist), req.Threads, req.MaxSeconds)

	if req.BaseURL == "" || req.Wordlist == "" {
		fmt.Printf("[DEBUG FFUF] ❌ 参数验证失败: baseURL或wordlist为空\n")
		http.Error(w, "请求参数错误：缺少baseUrl或wordlist", 400)
		return
	}
	if req.Threads <= 0 {
		req.Threads = 100
	}
	if strings.TrimSpace(req.MatchCodes) == "" {
		req.MatchCodes = "200,301,302,401,403"
	}
	if req.MaxSeconds <= 0 {
		req.MaxSeconds = 60
	} // 默认快速模式，避免长时间卡顿
	if req.ReqTimeout <= 0 {
		req.ReqTimeout = 5
	}
	fmt.Printf("[DEBUG FFUF] 参数处理完成: threads=%d, mc=%s, maxSeconds=%d, reqTimeout=%d\n",
		req.Threads, req.MatchCodes, req.MaxSeconds, req.ReqTimeout)

	// 先探测协议可达性：优先 https，失败自动降级 http（与页面判定一致）
	fmt.Printf("[DEBUG FFUF] 开始URL规范化...\n")
	finalURL := normalizeURL(req.BaseURL)
	if b, fu, _, e := tryGetURL(req.BaseURL, 3*time.Second); e == nil && fu != "" {
		_ = b // 不使用内容，仅借助协议判定
		finalURL = fu
		fmt.Printf("[DEBUG FFUF] URL协议探测完成: %s\n", finalURL)
	} else {
		fmt.Printf("[DEBUG FFUF] URL协议探测失败或跳过，使用规范化URL: %s\n", finalURL)
	}
	url := strings.TrimRight(finalURL, "/") + "/FUZZ"
	fmt.Printf("[DEBUG FFUF] 最终扫描URL: %s\n", url)

	// 支持多个字典，以分号/逗号/空白分隔
	fmt.Printf("[DEBUG FFUF] 处理字典列表...\n")
	wlParts := strings.FieldsFunc(req.Wordlist, func(r rune) bool { return r == ';' || r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if len(wlParts) == 0 {
		fmt.Printf("[DEBUG FFUF] ❌ 字典列表为空\n")
		http.Error(w, "请求参数错误：wordlist为空", 400)
		return
	}
	fmt.Printf("[DEBUG FFUF] 字典文件数量: %d\n", len(wlParts))
	for i, wl := range wlParts {
		fmt.Printf("[DEBUG FFUF] 字典 [%d]: %s\n", i+1, strings.TrimSpace(wl))
	}

	args := []string{"-u", url, "-t", fmt.Sprint(req.Threads), "-mc", req.MatchCodes, "-of", "json", "-timeout", fmt.Sprint(req.ReqTimeout), "-maxtime", fmt.Sprint(req.MaxSeconds)}
	for _, wl := range wlParts {
		if strings.TrimSpace(wl) != "" {
			args = append(args, "-w", wl)
		}
	}
	if req.Rate > 0 {
		args = append(args, "-rate", fmt.Sprint(req.Rate))
	}
	fmt.Printf("[DEBUG FFUF] 构建命令参数完成: %v\n", args)

	fmt.Printf("[DEBUG FFUF] 查找 ffuf 可执行文件...\n")
	binPath := findBin("ffuf")
	fmt.Printf("[DEBUG FFUF] ffuf 路径: %s\n", binPath)

	fmt.Printf("[DEBUG FFUF] 开始执行 ffuf 命令...\n")
	fmt.Printf("[DEBUG FFUF] 提示: 路径爆破可能需要较长时间（最多 %d 秒），请耐心等待...\n", req.MaxSeconds)
	out, errOut, err := runCmd("ffuf", args...)

	elapsed := time.Since(startTime)
	fmt.Printf("[DEBUG FFUF] 命令执行完成，耗时: %v\n", elapsed)
	fmt.Printf("[DEBUG FFUF] 返回码: err=%v\n", err)
	fmt.Printf("[DEBUG FFUF] stdout长度: %d字节\n", len(out))
	fmt.Printf("[DEBUG FFUF] stderr长度: %d字节\n", len(errOut))

	if errOut != "" {
		previewLen := 500
		if len(errOut) < previewLen {
			previewLen = len(errOut)
		}
		fmt.Printf("[DEBUG FFUF] stderr内容预览（前%d字符）: %s\n", previewLen, errOut[:previewLen])
	}
	if out != "" {
		previewLen := 500
		if len(out) < previewLen {
			previewLen = len(out)
		}
		fmt.Printf("[DEBUG FFUF] stdout内容预览（前%d字符）: %s\n", previewLen, out[:previewLen])
		// 尝试统计 JSON 行数（ffuf JSON 模式是逐行输出）
		lines := strings.Split(strings.TrimSpace(out), "\n")
		jsonLineCount := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && (strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[")) {
				jsonLineCount++
			}
		}
		fmt.Printf("[DEBUG FFUF] 检测到 %d 行可能的 JSON 结果\n", jsonLineCount)
	}

	if err != nil && out == "" {
		fmt.Printf("[DEBUG FFUF] ❌ ffuf执行失败\n")
		errMsg := "ffuf执行失败: " + err.Error()
		if errOut != "" {
			errMsg += "。错误输出: " + errOut
			fmt.Printf("[DEBUG FFUF] 错误输出: %s\n", errOut)
		}
		http.Error(w, errMsg, 502)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if strings.TrimSpace(out) == "" {
		fmt.Printf("[DEBUG FFUF] ⚠️ 输出为空，返回诊断信息\n")
		cwd, _ := os.Getwd()
		stderrPreview := errOut
		if len(stderrPreview) > 2000 {
			stderrPreview = stderrPreview[:2000] + "... (截断)"
		}
		_ = json.NewEncoder(w).Encode(jsonMap{
			"note":    "empty output",
			"stderr":  stderrPreview,
			"cmd":     binPath,
			"args":    strings.Join(args, " "),
			"cwd":     cwd,
			"usedUrl": url,
			"err":     err != nil,
		})
		return
	}

	fmt.Printf("[DEBUG FFUF] ✅ 返回结果: %d字节\n", len(out))
	fmt.Printf("[DEBUG FFUF] ========== FFUF 请求完成 ==========\n")
	_, _ = w.Write([]byte(out))
}

// validateURLForSecurity 验证URL是否有安全风险（防止RCE等攻击）
// 检查URL格式、协议、路径中的危险字符和可执行文件扩展名
// @param u 要验证的URL
// @return 错误信息（如果URL不安全）
func validateURLForSecurity(u string) error {
	// 先验证URL格式
	parsedURL, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("URL格式无效: %v", err)
	}

	// 检查URL Scheme（只允许http和https）
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("不支持的URL协议: %s (仅支持 http/https)", scheme)
	}

	// 检查是否包含明显的命令注入尝试（在URL路径中）
	path := parsedURL.Path
	dangerousInPath := []string{"|", "&&", "`", "$(", "${", "<", ">", ";"}
	for _, d := range dangerousInPath {
		if strings.Contains(path, d) {
			return fmt.Errorf("URL路径包含潜在危险字符: %s", d)
		}
	}

	// 检查是否包含可执行文件名扩展
	lowerPath := strings.ToLower(path)
	dangerousExts := []string{".exe", ".bat", ".cmd", ".sh", ".ps1", ".py", ".php"}
	for _, ext := range dangerousExts {
		if strings.HasSuffix(lowerPath, ext) {
			return fmt.Errorf("URL路径包含可执行文件扩展名: %s", ext)
		}
	}

	return nil
}

// RunURLFinder URLFinder JS/URL收集处理器
// 调用urlfinder工具进行JS文件和URL的收集
// 请求参数：target（目标URL）、depth（爬取深度）、outDir（输出目录）
// @param w HTTP响应写入器
// @param r HTTP请求对象
func RunURLFinder(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Target string
		Depth  int
		OutDir string
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误：无法解析JSON数据", 400)
		return
	}
	if req.Target == "" {
		http.Error(w, "请求参数错误：缺少target", 400)
		return
	}
	if req.Depth <= 0 {
		req.Depth = 1
	}
	// 注意：不要使用normalizeURL，因为它会默认添加https://
	// URLFinder需要保持用户提供的原始URL格式（http或https）
	target := strings.TrimSpace(req.Target)
	// 如果用户没有提供协议，才添加默认协议（优先尝试http，因为很多内网服务不支持https）
	if !strings.HasPrefix(strings.ToLower(target), "http://") && !strings.HasPrefix(strings.ToLower(target), "https://") {
		target = "http://" + target
	}

	// 安全检查：验证URL是否有RCE等安全风险
	if err := validateURLForSecurity(target); err != nil {
		http.Error(w, "安全验证失败："+err.Error(), 400)
		return
	}

	// 默认使用模式3（安全深入抓取），与命令行一致，获得完整数据
	mode := 3 // 使用安全深入抓取模式，过滤敏感路由，与命令行一致
	// 设置输出目录
	if strings.TrimSpace(req.OutDir) == "" {
		req.OutDir = "uploads/urlfinder"
	}
	// 统一使用绝对路径，避免工作目录差异导致写入失败
	absDir, _ := filepath.Abs(req.OutDir)
	_ = os.MkdirAll(absDir, 0755)

	// 创建临时URL文件（用于 -f 参数）
	// 临时文件在函数结束时清理（延迟删除，确保URLFinder执行完成后再删除）
	tmpFile := filepath.Join(absDir, fmt.Sprintf("urls_%d.txt", time.Now().Unix()))

	// 将URL写入临时文件（确保使用正确的换行符和编码）
	// URLFinder期望的是简单的文本文件，每行一个URL
	urlContent := target + "\r\n" // 使用Windows换行符，与命令行环境一致
	if err := os.WriteFile(tmpFile, []byte(urlContent), 0644); err != nil {
		http.Error(w, "创建临时URL文件失败："+err.Error(), 500)
		return
	}

	// 验证临时文件是否创建成功
	if st, err := os.Stat(tmpFile); err != nil {
		http.Error(w, "临时文件验证失败："+err.Error(), 500)
		return
	} else {
		fmt.Printf("[DEBUG] 临时文件创建成功: %s, 大小=%d字节\n", tmpFile, st.Size())
		// 读取并验证文件内容
		if content, err := os.ReadFile(tmpFile); err == nil {
			fmt.Printf("[DEBUG] 临时文件内容: %q\n", string(content))
			fmt.Printf("[DEBUG] 临时文件URL: %s\n", target)
		}
	}

	// 延迟清理临时文件（在函数返回前删除）
	defer func() {
		// 等待一小段时间确保URLFinder已读取文件
		time.Sleep(500 * time.Millisecond)
		if err := os.Remove(tmpFile); err != nil {
			// 忽略删除失败的错误（文件可能已被删除或正在使用）
		}
	}()

	// 文件名包含 host 以便区分目标
	hostForName := func(u string) string {
		if uu, e := url.Parse(u); e == nil {
			return strings.ReplaceAll(uu.Host, ":", "_")
		}
		return "target"
	}
	reportPath := filepath.Join(absDir, fmt.Sprintf("urlfinder_%s_%d.html", hostForName(target), time.Now().Unix()))
	// 确保报告路径是绝对路径（URLFinder 需要绝对路径才能正确生成报告）
	absReportPath, _ := filepath.Abs(reportPath)
	absTmpFile, _ := filepath.Abs(tmpFile)

	// 使用 -f 参数（批量模式），与用户提供的命令行一致：-s all -m 3 -f urls.txt -o report.html
	outArgs := []string{"-s", "all", "-m", fmt.Sprint(mode), "-f", absTmpFile, "-o", absReportPath}

	// 调试：记录执行的命令和参数
	fmt.Printf("[DEBUG] URLFinder命令: urlfinder %s\n", strings.Join(outArgs, " "))
	fmt.Printf("[DEBUG] 临时文件路径: %s\n", absTmpFile)
	fmt.Printf("[DEBUG] 报告输出路径: %s\n", absReportPath)

	out, errOut, err := runCmd("urlfinder", outArgs...)

	// 调试：记录执行结果
	fmt.Printf("[DEBUG] URLFinder stdout: %s\n", out)
	fmt.Printf("[DEBUG] URLFinder stderr: %s\n", errOut)
	fmt.Printf("[DEBUG] URLFinder error: %v\n", err)

	// 注意：URLFinder 即使成功也可能返回 err（例如网络超时），需要检查 stdout 输出
	// 如果输出中包含 "未获取到数据"，说明执行完成但没有找到数据
	hasNoData := strings.Contains(out, "未获取到数据") || strings.Contains(errOut, "未获取到数据")

	// 检查URLFinder是否真的执行成功（即使有err，如果输出了内容也认为可能成功）
	// 只有当完全没有输出且不是"未获取到数据"时才认为失败
	if err != nil && out == "" && errOut == "" && !hasNoData {
		errMsg := "urlfinder执行失败: " + err.Error()
		if errOut != "" {
			errMsg += "。错误输出: " + errOut
		}
		http.Error(w, errMsg, 502)
		return
	}
	var reportNote string
	// 等待报告文件生成（URLFinder 可能需要一点时间写入文件）
	// URLFinder 生成报告可能需要较长时间，尤其是大目标
	// 增加等待时间，并多次检查文件大小是否稳定
	initialWait := 2 * time.Second
	time.Sleep(initialWait)

	// 辅助函数：从 HTML 报告中提取 JS 和 URL 数量
	extractCountsFromReport := func(data string) (jsCount, urlCount int) {
		reJsCount := regexp.MustCompile(`(\d+)\s*JS\s+to`)
		reUrlCount := regexp.MustCompile(`(\d+)\s*URL\s+to`)
		jsMatches := reJsCount.FindAllStringSubmatch(data, -1)
		urlMatches := reUrlCount.FindAllStringSubmatch(data, -1)
		for _, m := range jsMatches {
			if v, err := strconv.Atoi(m[1]); err == nil {
				jsCount += v
			}
		}
		for _, m := range urlMatches {
			if v, err := strconv.Atoi(m[1]); err == nil {
				urlCount += v
			}
		}
		return jsCount, urlCount
	}

	// 首先检查 stdout 是否已经有统计信息（URLFinder 会输出类似 "74JS + 2208URL --> path"）
	reSummaryStdout := regexp.MustCompile(`(\d+)\s*JS\s*\+\s*(\d+)\s*URL\s*-->\s*(.+)`)
	hasSummaryInStdout := reSummaryStdout.MatchString(out)

	// 验证报告是否实际生成且非空（等待直到文件大小稳定或超时）
	// 使用绝对路径检查报告文件
	checkReportPath := absReportPath
	reportData := ""
	maxWait := 30 // 增加等待时间到30秒（URLFinder可能需要更长时间）
	lastSize := int64(-1)
	stableCount := 0

	fmt.Printf("[DEBUG] 开始等待报告文件生成: %s\n", checkReportPath)

	for i := 0; i < maxWait; i++ {
		if st, statErr := os.Stat(checkReportPath); statErr == nil {
			currentSize := st.Size()
			fmt.Printf("[DEBUG] 报告文件检查 [%d/%d]: 大小=%d, 上次=%d\n", i+1, maxWait, currentSize, lastSize)

			// 如果文件大小稳定（连续3次相同），说明写入完成
			if currentSize > 0 {
				if currentSize == lastSize {
					stableCount++
					fmt.Printf("[DEBUG] 文件大小稳定计数: %d/2\n", stableCount)
					if stableCount >= 2 { // 连续2次相同，认为写入完成
						if b, err := os.ReadFile(checkReportPath); err == nil {
							reportData = string(b)
							fmt.Printf("[DEBUG] 读取报告文件成功，大小=%d字节\n", len(reportData))
							// 检查是否是真正的URLFinder报告（不是fallback报告）
							if !strings.Contains(reportData, "URLFinder Report (fallback)") {
								fmt.Printf("[DEBUG] 检测到真正的URLFinder报告\n")
								break // 成功读取真正的报告，退出循环
							} else {
								fmt.Printf("[DEBUG] 警告：检测到fallback报告，继续等待...\n")
							}
						}
					}
				} else {
					stableCount = 0
				}
				lastSize = currentSize
			} else if currentSize == 0 {
				fmt.Printf("[DEBUG] 报告文件存在但为空，继续等待...\n")
			}
		} else {
			fmt.Printf("[DEBUG] 报告文件不存在，继续等待... (%v)\n", statErr)
		}
		// 等待一下再检查（可能还在写入中）
		if i < maxWait-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// 最终尝试读取报告（即使大小未稳定）
	if reportData == "" {
		fmt.Printf("[DEBUG] 最终尝试读取报告文件\n")
		if st, statErr := os.Stat(checkReportPath); statErr == nil && st.Size() > 0 {
			if b, err := os.ReadFile(checkReportPath); err == nil {
				reportData = string(b)
				fmt.Printf("[DEBUG] 最终读取报告文件，大小=%d字节\n", len(reportData))
				// 如果是fallback报告，清空它（说明URLFinder没有生成真正的报告）
				if strings.Contains(reportData, "URLFinder Report (fallback)") {
					fmt.Printf("[DEBUG] 警告：最终读取的仍是fallback报告\n")
					reportData = ""
				}
			} else {
				fmt.Printf("[DEBUG] 读取报告文件失败: %v\n", err)
			}
		} else {
			if statErr != nil {
				fmt.Printf("[DEBUG] 报告文件不存在: %v\n", statErr)
			} else {
				fmt.Printf("[DEBUG] 报告文件存在但为空\n")
			}
		}
	}

	// 如果 stdout 没有统计信息，但报告文件已读取，从报告中提取统计信息
	if !hasSummaryInStdout && reportData != "" {
		jsCount, urlCount := extractCountsFromReport(reportData)
		if jsCount > 0 || urlCount > 0 {
			out += fmt.Sprintf("\n%d JS + %d URL --> %s\n", jsCount, urlCount, reportPath)
		}
	}

	// 如果报告文件不存在或为空，生成说明报告
	// URLFinder在"未获取到数据"时不会生成报告文件，这是正常行为
	// 但我们仍然生成一个说明性报告，包含执行日志和状态信息
	if reportData == "" {
		// 提取stdout中的URL（如果有）
		reURL := regexp.MustCompile(`(?i)https?://[^\s"'<>\)]+`)
		urls := reURL.FindAllString(out, -1)
		uniq := func(in []string) []string {
			m := map[string]struct{}{}
			var out []string
			for _, v := range in {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				if _, ok := m[v]; ok {
					continue
				}
				m[v] = struct{}{}
				out = append(out, v)
			}
			return out
		}
		urls = uniq(urls)

		// 生成说明性报告（包含URLFinder的执行日志）
		reportHTML := `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>URLFinder Report</title>
<style>
body{font-family:Segoe UI,Arial;line-height:1.6;padding:16px;background:#f5f5f5}
.container{background:white;padding:20px;border-radius:8px;box-shadow:0 2px 4px rgba(0,0,0,0.1);max-width:1200px;margin:0 auto}
h1{color:#333;border-bottom:2px solid #007acc;padding-bottom:10px}
h2{color:#555;margin-top:24px}
.info{background:#e3f2fd;padding:12px;border-left:4px solid #2196F3;margin:12px 0}
.warning{background:#fff3cd;padding:12px;border-left:4px solid #ffc107;margin:12px 0}
pre{background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:6px;overflow-x:auto;font-size:13px;line-height:1.5}
.url-list{background:#f8f9fa;padding:12px;border-radius:4px;margin:8px 0}
.url-item{padding:4px 0;font-family:monospace;color:#0066cc}
</style>
</head>
<body>
<div class="container">
<h1>URLFinder Report</h1>
<div class="info">
<strong>Target URL:</strong> ` + html.EscapeString(target) + `<br>
<strong>Generated at:</strong> ` + time.Now().Format(time.RFC3339) + `<br>
<strong>Mode:</strong> Security Thorough Crawl (Mode 3)<br>
<strong>Status:</strong> ` + func() string {
			if hasNoData {
				return `<span style="color:#ff9800;">未获取到数据 (No data obtained)</span>`
			}
			return "执行完成"
		}() + `
</div>`

		if hasNoData {
			reportHTML += `
<div class="warning">
<strong>注意：</strong>URLFinder 执行完成但未获取到数据。这可能是因为：
<ul>
<li>目标URL无法访问或已失效</li>
<li>目标网站没有可提取的JS或URL资源</li>
<li>目标网站返回了空页面</li>
<li>网络连接问题或超时</li>
</ul>
</div>`
		}

		if len(urls) > 0 {
			reportHTML += `
<h2>Extracted URLs</h2>
<div class="url-list">`
			for _, u := range urls {
				reportHTML += `<div class="url-item">` + html.EscapeString(u) + `</div>`
			}
			reportHTML += `</div>`
		}

		reportHTML += `
<h2>Execution Log</h2>
<pre>` + html.EscapeString(out) + `</pre>
</div>
</body>
</html>`

		// 写入报告文件
		if err := os.WriteFile(absReportPath, []byte(reportHTML), 0644); err != nil {
			fmt.Printf("[DEBUG] 写入报告文件失败: %v\n", err)
			reportNote = " (write-error)"
		} else {
			// 验证文件是否写入成功
			if st, statErr := os.Stat(absReportPath); statErr == nil && st.Size() > 0 {
				reportNote = ""
				if hasNoData {
					reportNote = " (no-data)"
				}
				fmt.Printf("[DEBUG] 说明性报告写入成功: %s, 大小=%d字节\n", absReportPath, st.Size())
			} else {
				reportNote = " (not-found)"
			}
		}
	}

	// 在文本输出末尾追加报告路径提示行（不会被URL解析器当作URL）
	// 使用绝对路径，确保前端能找到报告文件
	finalReportPath := absReportPath
	if reportData == "" {
		// 如果URLFinder没有生成报告，使用相对路径（fallback报告路径）
		finalReportPath = reportPath
	}
	if strings.TrimSpace(finalReportPath) != "" {
		out = strings.TrimRight(out, "\n") + "\nREPORT_FILE: " + finalReportPath + reportNote + "\n"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if strings.TrimSpace(out) == "" {
		_, _ = w.Write([]byte(errOut))
		return
	}
	_, _ = w.Write([]byte(out))
}

// RunPacker Packer-Fuzzer解包处理器
// 调用Packer-Fuzzer工具对目标网站进行代码解包
// 请求参数：target（目标URL）、outDir（输出目录）
// @param w HTTP响应写入器
// @param r HTTP请求对象
func RunPacker(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Target, OutDir string
		FastMode       bool `json:"fastMode"`
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误：无法解析JSON数据", 400)
		return
	}
	if strings.TrimSpace(req.Target) == "" {
		http.Error(w, "请求参数错误：缺少target", 400)
		return
	}
	if strings.TrimSpace(req.OutDir) == "" {
		req.OutDir = "uploads/shouji"
	}

	fmt.Printf("[DEBUG Packer] 开始执行解包任务\n")
	fmt.Printf("[DEBUG Packer] 目标URL: %s\n", req.Target)
	fmt.Printf("[DEBUG Packer] 输出目录: %s\n", req.OutDir)

	_ = os.MkdirAll(req.OutDir, 0755)
	target := normalizeURL(req.Target)
	// 额外清理：去除可能存在的引号、反引号，避免参数传递错误
	target = strings.Trim(target, "\"`'")

	scriptPath := filepath.Join("Packer-Fuzzer", "PackerFuzzer.py")
	script, found := findPythonScript(scriptPath)

	fmt.Printf("[DEBUG Packer] 查找脚本: %s, 找到: %v\n", scriptPath, found)
	if found {
		fmt.Printf("[DEBUG Packer] 脚本路径: %s\n", script)
	}

	type Try struct {
		Bin  string
		Args []string
	}
	var tries []Try
	// -s 参数用于静默模式（自动回答YES），参数值是报告名称，不能包含路径分隔符
	// 使用简单的时间戳作为报告名称，避免路径错误（如 reports\uploads/shouji.docx）
	// Packer-Fuzzer 会将 -s 参数值直接拼接到 reports/ 目录下作为文件名
	reportName := fmt.Sprintf("unpack_%d", time.Now().Unix())
	fmt.Printf("[DEBUG Packer] 使用报告名称（静默模式）: %s\n", reportName)
	if found {
		// -s 参数用于静默模式，避免交互式提示
		// 报告会生成在 Packer-Fuzzer/reports/ 目录下，文件名：unpack_<timestamp>.html/.docx
		baseArgs := []string{script, "-u", target, "-s", reportName}
		if req.FastMode {
			baseArgs = append(baseArgs, "--collect-only")
			fmt.Printf("[DEBUG Packer] 启用快速模式（仅收集）\n")
		}
		tries = []Try{
			{"python", baseArgs},
			{"python3", baseArgs},
		}
	} else {
		baseArgs := []string{"-u", target, "-s", reportName}
		baseArgsModule := []string{"-m", "packer_fuzzer", "-u", target, "-s", reportName}
		if req.FastMode {
			baseArgs = append(baseArgs, "--collect-only")
			baseArgsModule = append(baseArgsModule, "--collect-only")
			fmt.Printf("[DEBUG Packer] 启用快速模式（仅收集）\n")
		}
		tries = []Try{
			{"packer-fuzzer", baseArgs},
			{"python", baseArgsModule},
			{"python3", baseArgsModule},
		}
	}

	fmt.Printf("[DEBUG Packer] 将尝试 %d 种执行方式\n", len(tries))

	var used Try
	var out, errOut string
	var err error
	for idx, t := range tries {
		fmt.Printf("[DEBUG Packer] 尝试方式 [%d/%d]: %s %v\n", idx+1, len(tries), t.Bin, t.Args)

		// 设置超时（解包可能需要较长时间，设置15分钟超时）
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

		cmd := exec.CommandContext(ctx, t.Bin, t.Args...)
		if found && (t.Bin == "python" || t.Bin == "python3") {
			cmd.Dir = filepath.Join(shoujiBaseDir(), "tools", "Packer-Fuzzer")
			fmt.Printf("[DEBUG Packer] 设置工作目录: %s\n", cmd.Dir)
		}

		fmt.Printf("[DEBUG Packer] 执行命令: %s %v\n", t.Bin, t.Args)
		fmt.Printf("[DEBUG Packer] 工作目录: %s\n", func() string {
			if cmd.Dir != "" {
				return cmd.Dir
			}
			wd, _ := os.Getwd()
			return wd
		}())

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		startTime := time.Now()
		fmt.Printf("[DEBUG Packer] 开始执行命令，时间: %s\n", startTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("[DEBUG Packer] 提示: 解包过程可能需要较长时间，请耐心等待...\n")

		err = cmd.Run()

		// 取消上下文（释放资源）
		cancel()

		elapsed := time.Since(startTime)
		out = stdout.String()
		errOut = stderr.String()

		fmt.Printf("[DEBUG Packer] 命令执行完成，耗时: %v\n", elapsed)
		fmt.Printf("[DEBUG Packer] 返回码: err=%v\n", err)
		fmt.Printf("[DEBUG Packer] stdout长度: %d字节\n", len(out))
		fmt.Printf("[DEBUG Packer] stderr长度: %d字节\n", len(errOut))

		if len(out) > 0 {
			// 只显示前500个字符，避免日志过长
			preview := out
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			fmt.Printf("[DEBUG Packer] stdout预览: %s\n", preview)
		}
		if len(errOut) > 0 {
			preview := errOut
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			fmt.Printf("[DEBUG Packer] stderr预览: %s\n", preview)
		}

		if err == nil || (out != "" && !strings.Contains(strings.ToLower(errOut), "not found") && !strings.Contains(strings.ToLower(errOut), "no module")) {
			used = t
			fmt.Printf("[DEBUG Packer] ✓ 成功找到可用的执行方式: %s\n", t.Bin)
			break
		} else {
			fmt.Printf("[DEBUG Packer] ✗ 执行方式失败，继续尝试下一个\n")
			if err != nil {
				fmt.Printf("[DEBUG Packer] 错误详情: %v\n", err)
			}
		}
	}

	if used.Bin == "" {
		errMsg := "packer-fuzzer未找到或执行失败。请确认已安装Python及依赖（pip install -r shouji/tools/Packer-Fuzzer/requirements.txt）"
		if err != nil {
			errMsg += fmt.Sprintf("。错误: %v", err)
			// 如果是超时错误，给予明确提示
			if strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "killed") {
				errMsg += " (任务超时，请检查网络连接或目标站点响应速度)"
			}
		}
		if errOut != "" {
			errMsg += "。错误输出: " + errOut
		}
		fmt.Printf("[DEBUG Packer] ❌ 所有执行方式都失败\n")
		http.Error(w, errMsg, 502)
		return
	}

	fmt.Printf("[DEBUG Packer] 开始搜索生成的文件...\n")

	// 获取项目根目录作为基准路径
	wd, _ := os.Getwd()
	fmt.Printf("[DEBUG Packer] 当前工作目录: %s\n", wd)
	fmt.Printf("[DEBUG Packer] shoujiBaseDir: %s\n", shoujiBaseDir())

	var files []string
	searchDirs := []string{
		filepath.Join(shoujiBaseDir(), "tools", "Packer-Fuzzer", "tmp"),
		filepath.Join(shoujiBaseDir(), "tools", "Packer-Fuzzer", "reports"),
		req.OutDir,
	}

	// 也尝试搜索绝对路径的req.OutDir
	if absOutDir, err := filepath.Abs(req.OutDir); err == nil {
		searchDirs = append(searchDirs, absOutDir)
	}

	for idx, dir := range searchDirs {
		// 确保目录路径是绝对路径
		if !filepath.IsAbs(dir) {
			absDir, err := filepath.Abs(dir)
			if err == nil {
				dir = absDir
			}
		}

		fmt.Printf("[DEBUG Packer] 搜索目录 [%d/%d]: %s\n", idx+1, len(searchDirs), dir)
		if _, err := os.Stat(dir); err == nil {
			count := 0
			fileCount := 0
			_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					fmt.Printf("[DEBUG Packer] WalkDir错误: %v\n", e)
					return nil
				}
				if d.IsDir() {
					return nil
				}
				count++

				// 只处理JS文件
				if !strings.HasSuffix(strings.ToLower(path), ".js") {
					return nil
				}
				fileCount++

				// 计算相对于当前工作目录的相对路径
				rel, relErr := filepath.Rel(wd, path)
				if relErr != nil {
					// 如果相对路径计算失败，使用绝对路径
					fmt.Printf("[DEBUG Packer] 计算相对路径失败，使用绝对路径: %s (错误: %v)\n", path, relErr)
					rel = path
				} else {
					// 确保相对路径是正常的（不是 ".." 开头的奇怪路径）
					if strings.HasPrefix(rel, "..") {
						// 如果相对路径需要向上太多层级，使用绝对路径
						fmt.Printf("[DEBUG Packer] 相对路径异常，使用绝对路径: %s -> %s\n", path, rel)
						rel = path
					}
				}

				// 标准化路径分隔符
				rel = strings.ReplaceAll(rel, "\\", "/")

				// 确保路径不为空
				if rel != "" && rel != "." {
					files = append(files, rel)
					if fileCount <= 10 {
						fmt.Printf("[DEBUG Packer]   找到文件 [%d]: %s (原始: %s)\n", fileCount, rel, path)
					}
				} else {
					fmt.Printf("[DEBUG Packer]   警告：文件路径为空或无效: %s\n", path)
				}
				return nil
			})
			fmt.Printf("[DEBUG Packer] 在 %s 中遍历了 %d 个文件，其中 %d 个是JS文件\n", dir, count, fileCount)
		} else {
			fmt.Printf("[DEBUG Packer] 目录不存在或无法访问: %s (%v)\n", dir, err)
		}
	}

	// 去重
	seen := make(map[string]struct{})
	uniqueFiles := []string{}
	for _, f := range files {
		if _, exists := seen[f]; !exists {
			seen[f] = struct{}{}
			uniqueFiles = append(uniqueFiles, f)
		}
	}
	files = uniqueFiles

	fmt.Printf("[DEBUG Packer] 总共找到 %d 个唯一JS文件\n", len(files))
	if len(files) > 0 {
		fmt.Printf("[DEBUG Packer] 文件列表（前20个）:\n")
		for i, f := range files {
			if i >= 20 {
				break
			}
			fmt.Printf("[DEBUG Packer]   %d. %s\n", i+1, f)
		}
		if len(files) > 20 {
			fmt.Printf("[DEBUG Packer]   ... 还有 %d 个文件\n", len(files)-20)
		}
	} else {
		fmt.Printf("[DEBUG Packer] ⚠️ 警告：未找到任何JS文件！\n")
	}

	fmt.Printf("[DEBUG Packer] 准备返回结果\n")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jsonMap{"used": fmt.Sprintf("%s %s", used.Bin, strings.Join(used.Args, " ")), "stdout": out, "stderr": errOut, "outDir": req.OutDir, "files": files})
	fmt.Printf("[DEBUG Packer] 响应已发送\n")
}

// MergeResults 合并结果处理器
// 将FFUF、URLFinder、Packer-Fuzzer的结果合并为一个统一的URL列表
// 请求参数：ffufJson（FFUF的JSON输出）、urlList（URLFinder的URL列表）、packerFiles（解包文件列表）
// @param w HTTP响应写入器
// @param r HTTP请求对象
func MergeResults(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	fmt.Printf("[DEBUG Merge] ========== 开始合并请求 ==========\n")

	type Req struct {
		FfufJson    string `json:"ffufJson"`
		UrlList     string `json:"urlList"`
		PackerFiles string `json:"packerFiles"`
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("[DEBUG Merge] ❌ JSON解析失败: %v\n", err)
		http.Error(w, "请求格式错误：无法解析JSON数据", 400)
		return
	}
	fmt.Printf("[DEBUG Merge] 接收数据: ffufJson=%d字节, urlList=%d字节, packerFiles=%d字节\n",
		len(req.FfufJson), len(req.UrlList), len(req.PackerFiles))

	set := map[string]struct{}{}

	// 处理 ffuf 结果（JSON格式，包含URL）
	ffufCount := 0
	if strings.TrimSpace(req.FfufJson) != "" {
		scanner := bufio.NewScanner(strings.NewReader(req.FfufJson))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(strings.TrimSpace(line), "{") {
				continue
			}
			m := regexp.MustCompile(`"url"\s*:\s*"([^"]+)"`).FindStringSubmatch(line)
			if len(m) == 2 {
				set[m[1]] = struct{}{}
				ffufCount++
			}
		}
		fmt.Printf("[DEBUG Merge] ffuf 处理完成: %d 个URL\n", ffufCount)
	}

	// 处理 URLFinder 结果（URL列表，每行一个）
	urlfinderCount := 0
	reURL := regexp.MustCompile(`^(?i)https?://[^\s]+$`)
	for _, ln := range strings.Split(req.UrlList, "\n") {
		u := strings.TrimSpace(ln)
		if u == "" {
			continue
		}
		if !reURL.MatchString(u) {
			continue
		}
		set[u] = struct{}{}
		urlfinderCount++
	}
	fmt.Printf("[DEBUG Merge] URLFinder 处理完成: %d 个URL\n", urlfinderCount)

	// 处理解包文件列表（文件路径，转换为可访问的URL）
	// 解包文件可能是本地路径，需要转换为相对URL或保留为文件路径
	fmt.Printf("[DEBUG Merge] 开始处理解包文件，输入长度: %d\n", len(req.PackerFiles))
	packerFileCount := 0
	addedCount := 0
	for _, ln := range strings.Split(req.PackerFiles, "\n") {
		filePath := strings.TrimSpace(ln)
		if filePath == "" {
			continue
		}
		packerFileCount++

		// 每100个文件输出一次进度，避免日志过多
		if packerFileCount%100 == 0 {
			fmt.Printf("[DEBUG Merge] 已处理 %d 个文件，已添加 %d 个到集合...\n", packerFileCount, addedCount)
		}

		// 将本地文件路径转换为相对URL（如果文件在uploads目录下）
		// 或者保留为文件路径格式，标记为本地文件
		normalizedPath := strings.ReplaceAll(filePath, "\\", "/")

		// 如果路径包含 uploads/，转换为相对URL
		if strings.Contains(normalizedPath, "uploads/") {
			// 提取 uploads/ 之后的部分作为URL路径
			parts := strings.Split(normalizedPath, "uploads/")
			if len(parts) > 1 {
				urlPath := "/uploads/" + strings.TrimPrefix(parts[1], "/")
				set[urlPath] = struct{}{}
				addedCount++
				continue
			}
		}

		// 检查是否是 Packer-Fuzzer 的 tmp 或 reports 目录下的文件
		// 匹配路径如：shouji/tools/Packer-Fuzzer/tmp/9Y8biz_doctors-staging.letsgethappi.com/tJjuHo.webpack-d508853eee0c4a23.js
		if strings.Contains(normalizedPath, "Packer-Fuzzer/tmp/") || strings.Contains(normalizedPath, "Packer-Fuzzer/reports/") {
			// 提取文件名，构建为可访问的路径
			fileName := filepath.Base(normalizedPath)
			// 如果文件以 .js 结尾，添加到集合中，使用相对路径格式
			if strings.HasSuffix(strings.ToLower(fileName), ".js") {
				urlPath := "/uploads/shouji/" + fileName
				set[urlPath] = struct{}{}
				addedCount++
				// 实时输出：每转换好一个就输出
				fmt.Printf("[DEBUG Merge] ✓ 转换解包文件: %s -> %s\n", fileName, urlPath)
				continue
			}
		}

		// 如果是相对路径且以 .js 结尾，直接作为路径添加（去掉路径前缀，只保留文件名）
		if strings.HasSuffix(strings.ToLower(normalizedPath), ".js") && !strings.HasPrefix(normalizedPath, "http://") && !strings.HasPrefix(normalizedPath, "https://") {
			// 提取文件名
			fileName := filepath.Base(normalizedPath)
			urlPath := "/uploads/shouji/" + fileName
			set[urlPath] = struct{}{}
			addedCount++
			// 实时输出：每转换好一个就输出
			fmt.Printf("[DEBUG Merge] ✓ 转换解包文件: %s -> %s\n", fileName, urlPath)
		}
	}
	fmt.Printf("[DEBUG Merge] 解包文件处理完成: 共处理 %d 个文件，添加到集合 %d 个\n", packerFileCount, addedCount)

	type Item struct {
		URL  string `json:"url"`
		Kind string `json:"kind"`
	}
	var items []Item
	jsCount := 0
	apiCount := 0
	otherCount := 0
	for u := range set {
		kind := "other"
		low := strings.ToLower(u)
		switch {
		case strings.HasSuffix(low, ".js"):
			kind = "js"
			jsCount++
		case strings.Contains(low, "/api/") || strings.Contains(low, "?"):
			kind = "api"
			apiCount++
		default:
			otherCount++
		}
		items = append(items, Item{URL: u, Kind: kind})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].URL < items[j].URL })

	fmt.Printf("[DEBUG Merge] 合并完成: 总计 %d 个唯一URL (JS: %d, API: %d, 其他: %d)\n",
		len(items), jsCount, apiCount, otherCount)

	elapsed := time.Since(startTime)
	fmt.Printf("[DEBUG Merge] 合并耗时: %v\n", elapsed)
	fmt.Printf("[DEBUG Merge] ========== 合并请求完成 ==========\n")

	w.Header().Set("Content-Type", "application/json")
	result := jsonMap{"total": len(items), "items": items}
	fmt.Printf("[DEBUG Merge] 返回结果: total=%d, items数量=%d\n", len(items), len(items))
	if err := json.NewEncoder(w).Encode(result); err != nil {
		fmt.Printf("[DEBUG Merge] ❌ JSON编码失败: %v\n", err)
		http.Error(w, "内部错误：无法编码响应", 500)
		return
	}
	fmt.Printf("[DEBUG Merge] 响应已发送\n")
}

// ParseURLFinderReport 解析URLFinder报告文件处理器
// 从URLFinder生成的HTML或文本报告中提取URL列表和详细信息
// 请求参数：path（报告文件路径）
// @param w HTTP响应写入器
// @param r HTTP请求对象
func ParseURLFinderReport(w http.ResponseWriter, r *http.Request) {
	type Req struct{ Path string }
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		http.Error(w, "请求格式错误：缺少path", 400)
		return
	}
	b, err := os.ReadFile(req.Path)
	if err != nil {
		http.Error(w, "读取报告失败："+err.Error(), 404)
		return
	}
	text := string(b)
	// 去除ANSI颜色码
	reAnsi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	clean := reAnsi.ReplaceAllString(text, "")
	lines := strings.Split(clean, "\n")
	getHost := func(u string) string {
		uu, err := url.Parse(u)
		if err == nil {
			return uu.Host
		}
		return ""
	}
	// 从报告中尝试找 Target URL
	reTarget := regexp.MustCompile(`(?i)Target URL:\s*(\S+)`)
	selfHost := ""
	if m := reTarget.FindStringSubmatch(clean); len(m) == 2 {
		selfHost = getHost(m[1])
	}
	// 定义URL项目结构（包含所有列信息）
	type URLItem struct {
		URL      string
		Status   string
		Size     string
		Title    string
		Redirect string
		Source   string
	}

	sec := map[string][]string{"jsSelf": []string{}, "jsOther": []string{}, "urlSelf": []string{}, "urlOther": []string{}, "fuzz": []string{}, "domains": []string{}, "info": []string{}}
	var allItems []URLItem // 存储所有提取的项目（包含完整信息）

	// 若输入的是 HTML 报告文件，解析表格中的所有列
	if strings.HasSuffix(strings.ToLower(req.Path), ".html") {
		// URLFinder HTML 报告格式：使用Ant Design表格
		// <tr class="ant-table-row">包含数据行
		// 第一个td中有<a href="URL">URL</a>
		// 第二个td中有Status文本
		// 第三个td中有Size文本
		// 第四个td中有Title文本
		// 第五个td中有<a href="Redirect">Redirect</a>
		// 第六个td中有<a href="Source">Source</a>

		// 匹配包含ant-table-row类的行（数据行，排除表头）
		// 使用更灵活的正则，匹配class中包含ant-table-row的行
		reTableRow := regexp.MustCompile(`(?is)<tr[^>]*class\s*=\s*["'][^"']*ant-table-row[^"']*["'][^>]*>(.*?)</tr>`)
		rows := reTableRow.FindAllStringSubmatch(clean, -1)

		fmt.Printf("[DEBUG ParseReport] 找到 %d 个表格行\n", len(rows))

		for i, rowMatch := range rows {
			if len(rowMatch) < 2 {
				continue
			}
			row := rowMatch[1] // 行的内容部分

			// 提取URL（从第一个<a href>标签，它在第一个td中）
			// 使用更灵活的匹配，允许换行和空格
			reFirstLink := regexp.MustCompile(`(?is)<td[^>]*>.*?<a[^>]*href\s*=\s*["']([^"']+)["'][^>]*>`)
			urlMatch := reFirstLink.FindStringSubmatch(row)
			if len(urlMatch) < 2 {
				fmt.Printf("[DEBUG ParseReport] 行 %d: 未找到URL，行内容前100字符: %s\n", i+1, func() string {
					if len(row) > 100 {
						return row[:100]
					}
					return row
				}())
				continue
			}
			itemURL := strings.TrimSpace(urlMatch[1])

			// 只处理完整的HTTP/HTTPS URL
			if !strings.HasPrefix(strings.ToLower(itemURL), "http://") && !strings.HasPrefix(strings.ToLower(itemURL), "https://") {
				continue
			}

			// 跳过无效URL
			if itemURL == "" || itemURL == "#" || itemURL == "javascript:" || strings.HasPrefix(strings.ToLower(itemURL), "javascript:") {
				continue
			}

			// 提取所有<td>标签中的内容（使用非贪婪匹配，允许跨行）
			reTdContent := regexp.MustCompile(`(?is)<td[^>]*>\s*(.*?)\s*</td>`)
			tdMatches := reTdContent.FindAllStringSubmatch(row, -1)

			fmt.Printf("[DEBUG ParseReport] 行 %d: 找到 %d 个td, URL=%s\n", i+1, len(tdMatches), itemURL)

			// 提取各列数据
			itemStatus := ""
			itemSize := ""
			itemTitle := ""
			itemRedirect := ""
			itemSource := ""

			// URL是第一个td中的第一个<a>（已提取）
			// Status是第二个td（索引1）
			if len(tdMatches) > 1 {
				statusText := tdMatches[1][1]
				// 移除所有HTML标签，只保留纯文本
				statusText = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(statusText, "")
				statusText = strings.ReplaceAll(statusText, "\n", "")
				statusText = strings.ReplaceAll(statusText, "\r", "")
				statusText = strings.ReplaceAll(statusText, "\t", "")
				itemStatus = strings.TrimSpace(statusText)
				fmt.Printf("[DEBUG ParseReport] 行 %d: Status='%s'\n", i+1, itemStatus)
			}

			// 过滤404状态码（以及4xx系列的其他错误状态）
			if itemStatus == "404" || strings.HasPrefix(itemStatus, "404") {
				fmt.Printf("[DEBUG ParseReport] 行 %d: 跳过404状态码\n", i+1)
				continue
			}

			// Size是第三个td（索引2）
			if len(tdMatches) > 2 {
				sizeText := tdMatches[2][1]
				sizeText = regexp.MustCompile(`(?i)<[^>]+>`).ReplaceAllString(sizeText, "")
				itemSize = strings.TrimSpace(sizeText)
			}

			// Title是第四个td（索引3）
			if len(tdMatches) > 3 {
				titleText := tdMatches[3][1]
				titleText = regexp.MustCompile(`(?i)<[^>]+>`).ReplaceAllString(titleText, "")
				itemTitle = strings.TrimSpace(titleText)
			}

			// Redirect是第五个td（索引4）中的<a href>
			if len(tdMatches) > 4 {
				redirectTd := tdMatches[4][1]
				reRedirectLink := regexp.MustCompile(`(?i)<a[^>]*href\s*=\s*["']([^"']+)["'][^>]*>`)
				redirectLinkMatch := reRedirectLink.FindStringSubmatch(redirectTd)
				if len(redirectLinkMatch) >= 2 {
					itemRedirect = strings.TrimSpace(redirectLinkMatch[1])
				}
			}

			// Source是第六个td（索引5）中的<a href>
			if len(tdMatches) > 5 {
				sourceTd := tdMatches[5][1]
				reSourceLink := regexp.MustCompile(`(?i)<a[^>]*href\s*=\s*["']([^"']+)["'][^>]*>`)
				sourceLinkMatch := reSourceLink.FindStringSubmatch(sourceTd)
				if len(sourceLinkMatch) >= 2 {
					itemSource = strings.TrimSpace(sourceLinkMatch[1])
				}
			} else {
				// 如果td数量不足，尝试从所有<a>标签中提取（第三个应该是Source）
				allLinks := regexp.MustCompile(`(?i)<a[^>]*href\s*=\s*["']([^"']+)["'][^>]*>`).FindAllStringSubmatch(row, -1)
				if len(allLinks) >= 3 {
					itemSource = strings.TrimSpace(allLinks[2][1])
				}
			}

			// 创建URLItem
			item := URLItem{
				URL:      itemURL,
				Status:   itemStatus,
				Size:     itemSize,
				Title:    itemTitle,
				Redirect: itemRedirect,
				Source:   itemSource,
			}
			allItems = append(allItems, item)
			fmt.Printf("[DEBUG ParseReport] 行 %d: 成功提取项目 URL=%s, Status=%s, Size=%s\n", i+1, itemURL, itemStatus, itemSize)

			// 同时更新分区信息（用于向后兼容）
			h := getHost(itemURL)
			if strings.HasSuffix(strings.ToLower(itemURL), ".js") || strings.HasSuffix(strings.ToLower(itemURL), ".js?") {
				if h == selfHost {
					sec["jsSelf"] = append(sec["jsSelf"], itemURL)
				} else {
					sec["jsOther"] = append(sec["jsOther"], itemURL)
				}
			} else {
				if h == selfHost {
					sec["urlSelf"] = append(sec["urlSelf"], itemURL)
				} else {
					sec["urlOther"] = append(sec["urlOther"], itemURL)
				}
			}
		}

		fmt.Printf("[DEBUG ParseReport] 总共提取 %d 个项目\n", len(allItems))
		uniq := func(in []string) []string {
			m := map[string]struct{}{}
			var out []string
			for _, v := range in {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				if _, ok := m[v]; ok {
					continue
				}
				m[v] = struct{}{}
				out = append(out, v)
			}
			return out
		}
		for k, v := range sec {
			sec[k] = uniq(v)
		}
		urlsAll := uniq(append(append(append(append(sec["jsSelf"], sec["jsOther"]...), sec["urlSelf"]...), sec["urlOther"]...), sec["fuzz"]...))

		// 将allItems转换为JSON格式
		itemsJSON := make([]map[string]string, 0, len(allItems))
		for _, item := range allItems {
			itemsJSON = append(itemsJSON, map[string]string{
				"url":      item.URL,
				"status":   item.Status,
				"size":     item.Size,
				"title":    item.Title,
				"redirect": item.Redirect,
				"source":   item.Source,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jsonMap{
			"urls":     urlsAll,   // 保持向后兼容
			"items":    itemsJSON, // 新增：包含所有列的详细信息
			"sections": sec,
			"host":     selfHost,
		})
		return
	}
	cur := ""
	isHeader := func(s string) (string, bool) {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(strings.ToLower(s), "js to ") {
			return "js", true
		}
		if strings.HasPrefix(strings.ToLower(s), "url to ") {
			return "url", true
		}
		if strings.EqualFold(s, "Fuzz") {
			return "fuzz", true
		}
		if strings.EqualFold(s, "Domains") {
			return "domains", true
		}
		if strings.EqualFold(s, "Info") {
			return "info", true
		}
		return "", false
	}
	reURL := regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if kind, ok := isHeader(line); ok {
			cur = kind
			continue
		}
		// 抓取URL，或按 domains/info 收集原文
		if cur == "domains" || cur == "info" {
			sec[cur] = append(sec[cur], line)
			continue
		}
		u := reURL.FindString(line)
		if u == "" { // 不是URL，则可能是附加信息
			if strings.HasPrefix(strings.ToLower(line), "target url:") || strings.HasPrefix(strings.ToLower(line), "update:") || strings.Contains(strings.ToLower(line), "spider") {
				sec["info"] = append(sec["info"], line)
			}
			continue
		}
		host := getHost(u)
		switch cur {
		case "js":
			if host == selfHost {
				sec["jsSelf"] = append(sec["jsSelf"], u)
			} else {
				sec["jsOther"] = append(sec["jsOther"], u)
			}
		case "url":
			if host == selfHost {
				sec["urlSelf"] = append(sec["urlSelf"], u)
			} else {
				sec["urlOther"] = append(sec["urlOther"], u)
			}
		case "fuzz":
			sec["fuzz"] = append(sec["fuzz"], u)
		default:
			// 未命中任何分区的URL，按urlOther处理
			sec["urlOther"] = append(sec["urlOther"], u)
		}
	}
	uniq := func(in []string) []string {
		m := map[string]struct{}{}
		var out []string
		for _, v := range in {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := m[v]; ok {
				continue
			}
			m[v] = struct{}{}
			out = append(out, v)
		}
		return out
	}
	for k, v := range sec {
		sec[k] = uniq(v)
	}
	urls := uniq(append(append(append(append(sec["jsSelf"], sec["jsOther"]...), sec["urlSelf"]...), sec["urlOther"]...), sec["fuzz"]...))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jsonMap{"urls": urls, "sections": sec, "host": selfHost})
}

// RegisterRoutes 注册HTTP路由处理器
// 将shouji相关的所有HTTP处理器注册到指定的ServeMux
// @param mux HTTP ServeMux实例
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/scan/shouji/packable", AnalyzePackable)
	mux.HandleFunc("/scan/shouji/ffuf", RunFfuf)
	mux.HandleFunc("/scan/shouji/urlfinder", RunURLFinder)
	mux.HandleFunc("/scan/shouji/packer", RunPacker)
	mux.HandleFunc("/scan/shouji/merge", MergeResults)
	mux.HandleFunc("/scan/shouji/parse_report", ParseURLFinderReport)
	// upload 复用主项目 /upload
}
