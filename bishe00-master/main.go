// Package main 是NeonScan的主程序包
// NeonScan是一个综合性的网络安全扫描工具，包含端口扫描、目录扫描、POC扫描、EXP验证、Web探针、WAF绕过、JS/URL收集等功能
// 当前保留AI分析、AI生成与自动验证EXP，以及JADX MCP集成能力
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bishe/internal/mcp"           // MCP（Model Context Protocol）相关功能，用于JADX集成和AI分析
	shouji "bishe/internal/shouji" // 解包与JS信息收集功能
)

// SSEMessage Server-Sent Events消息结构体
// 用于向前端实时推送扫描进度和结果
type SSEMessage struct {
	Type     string      `json:"type"`     // 消息类型：start, progress, find, end等
	TaskID   string      `json:"taskId"`   // 任务ID
	Progress string      `json:"progress"` // 进度文本，如 "10/100"
	Percent  int         `json:"percent"`  // 进度百分比，0-100
	Data     interface{} `json:"data"`     // 消息数据，根据type不同而不同
}

// Task 任务结构体
// 用于管理和跟踪异步扫描任务的执行状态和进度
type Task struct {
	m       sync.Mutex      // 互斥锁，用于保证并发安全
	ID      string          // 任务唯一标识符
	Total   int             // 任务总数（如需要扫描的端口数、目录数等）
	Done    int             // 已完成数量
	Created time.Time       // 任务创建时间
	ch      chan SSEMessage // SSE消息通道，用于发送实时更新
	stop    chan struct{}   // 停止信号通道
	stopped bool            // 是否已停止
}

var (
	mu    sync.Mutex               // 全局互斥锁，用于保护tasks map
	tasks = make(map[string]*Task) // 全局任务映射表，key为任务ID，value为任务对象
)

// newTask 创建新任务
// @param total 任务总数
// @return 新创建的任务对象
func newTask(total int) *Task {
	// 生成唯一任务ID（基于当前时间戳）
	id := fmt.Sprintf("t-%d", time.Now().UnixNano())
	t := &Task{
		ID:      id,
		Total:   total,
		Done:    0,
		Created: time.Now(),
		ch:      make(chan SSEMessage, 1024), // 带缓冲区的通道，容量1024
		stop:    make(chan struct{}),
	}
	// 将任务添加到全局任务表
	mu.Lock()
	tasks[id] = t
	mu.Unlock()
	return t
}

// IncDone 增加已完成计数（线程安全）
// @return d 当前已完成数量
// @return tot 总数量
func (t *Task) IncDone() (d int, tot int) {
	t.m.Lock()
	defer t.m.Unlock()
	t.Done++
	return t.Done, t.Total
}

// Send 发送SSE消息到任务通道（线程安全）
// @param msg 要发送的消息
// @return 是否发送成功
func (t *Task) Send(msg SSEMessage) bool {
	if t == nil {
		return false
	}
	t.m.Lock()
	// 检查任务是否已停止或通道是否有效
	if t.stopped || t.ch == nil {
		t.m.Unlock()
		return false
	}
	// 保存通道和停止信号，避免在锁内阻塞
	ch := t.ch
	stop := t.stop
	t.m.Unlock()
	// 尝试发送消息，如果收到停止信号则返回false
	select {
	case <-stop:
		return false
	default:
		// 使用recover防止通道已关闭导致的panic
		defer func() { recover() }()
		ch <- msg
		return true
	}
}

// Close 关闭任务，停止所有操作
func (t *Task) Close() {
	if t == nil {
		return
	}
	t.m.Lock()
	if !t.stopped {
		if t.stop != nil {
			close(t.stop) // 发送停止信号
		}
		t.stopped = true
	}
	if t.ch != nil {
		close(t.ch)
		t.ch = nil
	}
	t.m.Unlock()
}

// getTask 根据任务ID获取任务对象（线程安全）
// @param id 任务ID
// @return 任务对象和是否存在的标志
func getTask(id string) (*Task, bool) {
	mu.Lock()
	defer mu.Unlock()
	t, ok := tasks[id]
	return t, ok
}

// startTaskJanitor 启动任务清理守护进程
// 后台运行，定期清理超过5分钟的老任务，避免内存泄漏
func startTaskJanitor() {
	go func() {
		for {
			time.Sleep(60 * time.Second) // 每60秒清理一次
			mu.Lock()
			for id, t := range tasks {
				if t == nil {
					delete(tasks, id)
					continue
				}
				// 清理创建时间超过5分钟的任务
				stopped := false
				dur := time.Since(t.Created)
				if dur > 5*time.Minute {
					stopped = true
				}
				if stopped {
					t.Close()
					delete(tasks, id)
				}
			}
			mu.Unlock()
		}
	}()
}

// stopTask 停止指定任务
// @param id 任务ID
func stopTask(id string) {
	mu.Lock()
	defer mu.Unlock()
	if t, ok := tasks[id]; ok {
		// 委托给Task.Close进行安全关闭
		t.Close()
	}
}

// finishTask 完成任务，关闭任务但不立即删除
// 不立即删除是为了避免SSE连接返回404，由清理守护进程稍后清理
// @param id 任务ID
func finishTask(id string) {
	mu.Lock()
	// 确保只关闭通道一次，避免重复关闭导致的panic
	if t, ok := tasks[id]; ok {
		// 委托给Task.Close进行安全关闭（它有自己的锁）
		t.Close()
		// 不要立即删除，避免SSE返回404；让清理守护进程稍后清理
	}
	mu.Unlock()
}

// writeSSE 写入SSE（Server-Sent Events）消息
// @param w HTTP响应写入器
// @param msg SSE消息
// @return 错误信息
func writeSSE(w http.ResponseWriter, msg SSEMessage) error {
	b, _ := json.Marshal(msg)
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(b))
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush() // 立即刷新，确保前端能实时收到消息
	}
	return err
}

// safeSend 安全发送消息（委托给Task.Send以避免竞争条件）
// @param t 任务对象
// @param msg 要发送的消息
func safeSend(t *Task, msg SSEMessage) {
	if t == nil {
		return
	}
	_ = t.Send(msg)
}

// --- HTTP Handlers ---

// sseHandler SSE处理器
// 处理SSE连接请求，实时推送任务进度和结果
func sseHandler(w http.ResponseWriter, r *http.Request) {
	// 从URL参数中获取任务ID
	id := r.URL.Query().Get("task")
	if id == "" {
		http.Error(w, "missing task", http.StatusBadRequest)
		return
	}

	// 获取任务对象
	t, ok := getTask(id)
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// 设置SSE响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()                         // 获取请求上下文，用于检测客户端断开
	ticker := time.NewTicker(15 * time.Second) // 创建15秒的心跳定时器
	defer ticker.Stop()

	// 发送开始消息
	if err := writeSSE(w, SSEMessage{Type: "start", TaskID: id, Progress: fmt.Sprintf("0/%d", t.Total), Percent: 0}); err != nil {
		return
	}

	// 主循环：监听任务消息和客户端断开
	for {
		select {
		case <-ctx.Done():
			// 客户端断开连接
			return
		case <-t.stop:
			// 任务停止，发送结束消息
			_ = writeSSE(w, SSEMessage{
				Type:     "end",
				TaskID:   id,
				Progress: fmt.Sprintf("%d/%d", func() int { t.m.Lock(); defer t.m.Unlock(); return t.Done }(), t.Total),
				Percent:  100,
			})
			return
		case <-ticker.C:
			// 发送心跳消息（保持连接活跃）
			if err := writeSSE(w, SSEMessage{Type: "ping", TaskID: id}); err != nil {
				return
			}
		case msg, ok := <-t.ch:
			// 接收任务消息
			if !ok {
				// 通道已关闭，发送结束消息
				_ = writeSSE(w, SSEMessage{
					Type:     "end",
					TaskID:   id,
					Progress: fmt.Sprintf("%d/%d", func() int { t.m.Lock(); defer t.m.Unlock(); return t.Done }(), t.Total),
					Percent:  100,
				})
				return
			}
			// 转发消息到前端
			if err := writeSSE(w, msg); err != nil {
				return
			}
		}
	}
}

// serveStatic 静态文件服务处理器
// 提供web目录下的静态文件服务（HTML、CSS、JS等）
func serveStatic(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	var fp string
	if p == "/" {
		// 根路径返回index.html
		fp = filepath.Join("web", "index.html")
	} else {
		// 其他路径映射到web目录下
		fp = filepath.Join("web", filepath.Clean(strings.TrimPrefix(p, "/")))
	}
	http.ServeFile(w, r, fp)
}

// stopTaskHandler 停止任务处理器
// 处理前端发送的停止任务请求
func stopTaskHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("task")
	if id == "" {
		http.Error(w, "missing task", http.StatusBadRequest)
		return
	}
	stopTask(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

/*
// --- Port Scan ---

// PortScanReq 端口扫描请求
// 前端发送的端口扫描请求参数
type PortScanReq struct {
	Host        string `json:"host"`        // 目标主机地址
	Ports       string `json:"ports"`       // 端口范围，如 "1-1024,3306,5432"
	Concurrency int    `json:"concurrency"` // 并发数（默认500）
	TimeoutMs   int    `json:"timeoutMs"`   // 连接超时时间（毫秒，默认300）
	ScanType    string `json:"scanType"`    // 扫描类型：tcp（默认）或 udp
	GrabBanner  bool   `json:"grabBanner"`  // 是否抓取Banner（仅TCP，连接后尝试读取Banner）
}

// normalizePortsString 规范化端口字符串以便比较
// 将端口规格字符串解析后重新组合成规范化的字符串表示
func normalizePortsString(spec string) string {
	ports := parsePorts(spec)
	if len(ports) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, p := range ports {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.Itoa(p))
	}
	return sb.String()
}

// parsePorts 解析端口规格字符串
// 支持格式：单个端口（"80"）、端口范围（"1-1024"）、多个端口（"80,443,3306"）
// 示例："1-1024,3306,5432" -> [1,2,3,...,1024,3306,5432]
// @param spec 端口规格字符串
// @return 解析后的端口列表（已排序）
func parsePorts(spec string) []int {
	spec = strings.TrimSpace(spec)
	set := make(map[int]struct{})
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			ab := strings.SplitN(part, "-", 2)
			a, _ := strconv.Atoi(strings.TrimSpace(ab[0]))
			b, _ := strconv.Atoi(strings.TrimSpace(ab[1]))
			if a > b {
				a, b = b, a
			}
			for i := a; i <= b; i++ {
				set[i] = struct{}{}
			}
		} else {
			v, err := strconv.Atoi(part)
			if err == nil {
				set[v] = struct{}{}
			}
		}
	}
	ports := make([]int, 0, len(set))
	for k := range set {
		ports = append(ports, k)
	}
	sort.Ints(ports)
	return ports
}

// portScanHandler 端口扫描处理器
// 处理端口扫描请求，支持TCP和UDP扫描，可选择是否抓取Banner
func portScanHandler(w http.ResponseWriter, r *http.Request) {
	var req PortScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ports := parsePorts(req.Ports)
	if len(ports) == 0 {
		http.Error(w, "no ports", http.StatusBadRequest)
		return
	}
	cc := req.Concurrency
	if cc <= 0 {
		cc = 500
	}
	timeout := time.Duration(req.TimeoutMs)
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	} else {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	t := newTask(len(ports))
	go func() {
		sem := make(chan struct{}, cc)
		var wg sync.WaitGroup
		stype := strings.ToLower(strings.TrimSpace(req.ScanType))
		if stype == "" {
			stype = "tcp"
		}
		for _, p := range ports {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			pp := p
			go func() {
				defer wg.Done()
				select {
				case <-t.stop:
					<-sem
					return
				default:
				}
				addr := net.JoinHostPort(req.Host, strconv.Itoa(pp))
				open := false
				var banner string
				if stype == "udp" {
					udpAddr, err := net.ResolveUDPAddr("udp", addr)
					if err == nil {
						c, err := net.DialUDP("udp", nil, udpAddr)
						if err == nil {
							_ = c.SetDeadline(time.Now().Add(timeout))
							_, _ = c.Write([]byte("\n"))
							buf := make([]byte, 256)
							n, _, rerr := c.ReadFrom(buf)
							if rerr == nil && n > 0 {
								open = true
								banner = string(buf[:n])
							}
							_ = c.Close()
						}
					}
				} else { // tcp
					conn, err := net.DialTimeout("tcp", addr, timeout)
					if err == nil {
						open = true
						_ = conn.SetDeadline(time.Now().Add(timeout))
						if req.GrabBanner {
							buf := make([]byte, 256)
							n, _ := conn.Read(buf)
							if n > 0 {
								banner = string(buf[:n])
							}
						}
						_ = conn.Close()
					}
				}
				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				if open {
					msg.Type = "find"
					data := map[string]interface{}{"port": pp, "status": "open", "proto": stype}
					if banner != "" {
						data["banner"] = banner
					}
					msg.Data = data
				}
				safeSend(t, msg)
				<-sem
			}()
		}
		wg.Wait()
		finishTask(t.ID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
}

// --- Dir Scan ---

// DirScanReq 目录扫描请求
// 前端发送的目录扫描请求参数
type DirScanReq struct {
	BaseURL      string   `json:"baseUrl"`      // 目标基础URL
	DictPaths    []string `json:"dictPaths"`    // 字典文件路径列表（自定义字典）
	BuiltinDicts []string `json:"builtinDicts"` // 内置字典名称列表
	Concurrency  int      `json:"concurrency"`  // 并发数（默认200）
	TimeoutMs    int      `json:"timeoutMs"`    // 请求超时时间（毫秒，默认1500）
}

// dirScanHandler 目录扫描处理器
// 根据字典文件对目标URL进行目录/文件爆破扫描
func dirScanHandler(w http.ResponseWriter, r *http.Request) {
	var req DirScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// validate base URL
	u := strings.TrimSpace(req.BaseURL)
	if u == "" {
		http.Error(w, "missing baseUrl", http.StatusBadRequest)
		return
	}
	low := strings.ToLower(u)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		u = "http://" + u
	}
	req.BaseURL = u
	// read dict files
	var paths []string
	readOne := func(fp string) error {
		b, err := os.ReadFile(fp)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			p := strings.TrimSpace(line)
			if p == "" || strings.HasPrefix(p, "#") {
				continue
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			paths = append(paths, p)
		}
		return nil
	}

	// 读取内置字典
	if len(req.BuiltinDicts) > 0 {
		dictDir := getBuiltinDictDir()
		for _, dictName := range req.BuiltinDicts {
			dictPath := filepath.Join(dictDir, dictName)
			if err := readOne(dictPath); err != nil {
				log.Printf("[目录扫描] 读取内置字典失败: %s, 错误: %v", dictPath, err)
			}
		}
	}

	// 读取自定义字典
	for _, fp := range req.DictPaths {
		if err := readOne(fp); err != nil {
			log.Printf("[目录扫描] 读取自定义字典失败: %s, 错误: %v", fp, err)
		}
	}

	if len(paths) == 0 {
		http.Error(w, "empty dict paths and builtin dicts", http.StatusBadRequest)
		return
	}
	// concurrency & timeout
	cc := req.Concurrency
	if cc <= 0 {
		cc = 200
	}
	timeout := time.Duration(req.TimeoutMs)
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	} else {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	// task
	t := newTask(len(paths))
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("dirScan top panic: %v", r)
			}
		}()
		var wg sync.WaitGroup
		jobs := make(chan string, cc*2)
		// workers
		worker := func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("dirScan worker panic: %v", r)
				}
			}()
			for path := range jobs {
				select {
				case <-t.stop:
					return
				default:
				}
				url := strings.TrimRight(req.BaseURL, "/") + path
				req0, e := http.NewRequest("GET", url, nil)
				if e != nil {
					// skip invalid URL request safely
					d, tot := t.IncDone()
					percent := int(math.Round(float64(d) / float64(tot) * 100))
					msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
					safeSend(t, msg)
					continue
				}
				resp, err := client.Do(req0)
				status := 0
				loc := ""
				bodyLen := 0
				if err == nil && resp != nil {
					status = resp.StatusCode
					loc = resp.Header.Get("Location")
					b, _ := io.ReadAll(resp.Body)
					if resp.Body != nil {
						resp.Body.Close()
					}
					bodyLen = len(b)
				}
				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				if status == 200 || status == 403 || status == 301 || status == 302 {
					msg.Type = "find"
					msg.Data = map[string]interface{}{"path": path, "url": url, "status": status, "location": loc, "length": bodyLen}
				}
				safeSend(t, msg)
			}
		}
		// start workers
		wN := cc
		if wN < 1 {
			wN = 1
		}
		if wN > 1000 {
			wN = 1000
		} // hard cap to avoid extreme goroutine count
		wg.Add(wN)
		for i := 0; i < wN; i++ {
			go worker()
		}
		// feed jobs
		for _, p := range paths {
			select {
			case <-t.stop:
				break
			default:
			}
			jobs <- p
		}
		close(jobs)
		wg.Wait()
		finishTask(t.ID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
}

// getBuiltinDictDir 获取内置字典目录路径
// 返回字典目录的绝对路径
func getBuiltinDictDir() string {
	// 尝试相对路径（当前工作目录下的 dict 目录）
	cwd, _ := os.Getwd()
	localDir := filepath.Join(cwd, "dict")
	if st, err := os.Stat(localDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(localDir)
		return abs
	}

	// 尝试上级目录中的 dict（开发环境常见结构）
	parentDir := filepath.Join(cwd, "..", "dict")
	if st, err := os.Stat(parentDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(parentDir)
		return abs
	}

	// 硬编码的字典目录路径（仅作为最后的备选，或在特定部署环境中使用）
	// 注意：硬编码绝对路径在不同机器上通常无效，建议优先使用相对路径
	dictDir := `E:\gongju\天狐渗透工具箱-社区版V2.0纪念版\tools\gui_shouji\dirscan_3.0\dict`
	if st, err := os.Stat(dictDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(dictDir)
		return abs
	}

	return ""
}

// loadDictFile 从字典文件中读取路径列表
func loadDictFile(dictPath string) ([]string, error) {
	b, err := os.ReadFile(dictPath)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(b), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// DictInfo 字典信息
type DictInfo struct {
	Name     string `json:"name"`     // 字典文件名
	Path     string `json:"path"`     // 字典文件完整路径
	Category string `json:"category"` // 分类（PHP、Java、ASP等）
}

// getBuiltinDicts 获取所有内置字典列表，按技术栈分类
func getBuiltinDicts() map[string][]DictInfo {
	dictDir := getBuiltinDictDir()
	result := make(map[string][]DictInfo)

	if dictDir == "" {
		return result
	}

	// 读取字典目录下的所有文件
	files, err := os.ReadDir(dictDir)
	if err != nil {
		return result
	}

	// 技术栈分类映射
	categoryMap := map[string]string{
		"php":        "PHP",
		"thinkphp":   "PHP",
		"laravel":    "PHP",
		"wordpress":  "PHP",
		"drupal":     "PHP",
		"jsp":        "Java",
		"java":       "Java",
		"spring":     "Java",
		"springboot": "Java",
		"struts":     "Java",
		"shiro":      "Java",
		"tomcat":     "Java",
		"jboss":      "Java",
		"weblogic":   "Java",
		"asp":        "ASP",
		"aspx":       "ASP",
		"python":     "Python",
		"django":     "Python",
		"flask":      "Python",
		"ruby":       "Ruby",
		"rails":      "Ruby",
		"nodejs":     "Node.js",
		"express":    "Node.js",
		"go":         "Go",
		"common":     "通用",
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		filePath := filepath.Join(dictDir, fileName)

		// 检查是否是有效的字典文件（文本文件）
		if info, err := file.Info(); err == nil && info.Size() > 0 {
			// 获取分类
			fileNameLower := strings.ToLower(fileName)
			category := "其他"
			for key, cat := range categoryMap {
				if strings.Contains(fileNameLower, key) {
					category = cat
					break
				}
			}

			dictInfo := DictInfo{
				Name:     fileName,
				Path:     filePath,
				Category: category,
			}

			result[category] = append(result[category], dictInfo)
		}
	}

	// 对每个分类内的字典按名称排序
	for category := range result {
		dicts := result[category]
		sort.Slice(dicts, func(i, j int) bool {
			return dicts[i].Name < dicts[j].Name
		})
		result[category] = dicts
	}

	return result
}

// getBuiltinDictsHandler 获取内置字典列表的API处理器
func getBuiltinDictsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dicts := getBuiltinDicts()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dicts)
}

// selectDictByTech 根据技术栈选择字典文件
// 返回字典文件路径列表
func selectDictByTech(tech []interface{}) []string {
	dictDir := getBuiltinDictDir()
	if dictDir == "" {
		return nil
	}

	// 技术栈名称到字典文件名的映射
	techToDict := map[string][]string{
		"php":        {"php", "common"},
		"thinkphp":   {"php", "thinkphp", "common"},
		"laravel":    {"php", "laravel", "common"},
		"wordpress":  {"php", "wordpress", "common"},
		"drupal":     {"php", "drupal", "common"},
		"jsp":        {"jsp", "java", "common"},
		"java":       {"jsp", "java", "common"},
		"spring":     {"jsp", "java", "spring", "common"},
		"springboot": {"jsp", "java", "spring", "common"},
		"struts":     {"jsp", "java", "struts", "common"},
		"shiro":      {"jsp", "java", "shiro", "common"},
		"asp":        {"asp", "aspx", "common"},
		"aspx":       {"asp", "aspx", "common"},
		"asp.net":    {"asp", "aspx", "common"},
		".net":       {"asp", "aspx", "common"},
		"python":     {"python", "common"},
		"django":     {"python", "django", "common"},
		"flask":      {"python", "flask", "common"},
		"ruby":       {"ruby", "common"},
		"rails":      {"ruby", "rails", "common"},
		"nodejs":     {"nodejs", "common"},
		"express":    {"nodejs", "express", "common"},
		"go":         {"go", "common"},
		"tomcat":     {"jsp", "java", "tomcat", "common"},
		"jboss":      {"jsp", "java", "jboss", "common"},
		"weblogic":   {"jsp", "java", "weblogic", "common"},
	}

	// 收集所有匹配的字典文件名（去重）
	dictFilesSet := make(map[string]bool)
	dictFilesSet["common"] = true // 默认包含common字典

	// 遍历技术栈，匹配字典文件
	for _, t := range tech {
		techStr, ok := t.(string)
		if !ok {
			continue
		}
		techLower := strings.ToLower(techStr)

		// 处理带前缀的技术栈名称（如 "X-Powered-By:PHP"）
		if strings.Contains(techLower, ":") {
			parts := strings.Split(techLower, ":")
			if len(parts) > 1 {
				techLower = strings.TrimSpace(parts[1])
			}
		}

		// 查找匹配的字典文件
		for techKey, dictList := range techToDict {
			if strings.Contains(techLower, techKey) {
				for _, dictName := range dictList {
					dictFilesSet[dictName] = true
				}
			}
		}

		// 直接匹配技术栈名称（如果字典文件存在）
		dictPath := filepath.Join(dictDir, techLower)
		if _, err := os.Stat(dictPath); err == nil {
			dictFilesSet[techLower] = true
		}
	}

	// 转换为列表并验证文件是否存在
	var dictFiles []string
	for dictName := range dictFilesSet {
		dictPath := filepath.Join(dictDir, dictName)
		if _, err := os.Stat(dictPath); err == nil {
			dictFiles = append(dictFiles, dictPath)
		}
	}

	// 如果没有任何匹配的字典文件，返回common字典（如果存在）
	if len(dictFiles) == 0 {
		commonPath := filepath.Join(dictDir, "common")
		if _, err := os.Stat(commonPath); err == nil {
			return []string{commonPath}
		}
	}

	return dictFiles
}

*/
/*
// pocScanHandler POC扫描处理器
// 处理POC漏洞扫描请求，支持传统POC、X-Ray风格POC和Nuclei风格POC
func pocScanHandler(w http.ResponseWriter, r *http.Request) {
	var req PocScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var pocs []POC
	var xrps []XRPOC
	var err error
	var nucs []NucleiPOC

	// 如果指定了PocPaths，使用文件列表
	if len(req.PocPaths) > 0 {
		pocs, xrps, nucs, err = loadAllPOCsFromFiles(req.PocPaths)
	} else {
		// 如果没有指定PocDir，使用内置POC目录
		if req.PocDir == "" {
			req.PocDir = getBuiltinPocDir()
		}
		pocs, xrps, nucs, err = loadAllPOCs(req.PocDir)
	}
	if err != nil || (len(pocs) == 0 && len(xrps) == 0 && len(nucs) == 0) {
		http.Error(w, "load pocs error or empty", http.StatusBadRequest)
		return
	}
	// normalize base URL (ensure scheme)
	u := strings.TrimSpace(req.BaseURL)
	if u == "" {
		http.Error(w, "missing baseUrl", http.StatusBadRequest)
		return
	}
	low := strings.ToLower(u)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		u = "http://" + u
	}
	req.BaseURL = u
	cc := req.Concurrency
	if cc <= 0 {
		cc = 50
	}
	timeout := time.Duration(req.TimeoutMs)
	if timeout <= 0 {
		timeout = 3000 * time.Millisecond
	} else {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	t := newTask(len(pocs) + len(xrps) + len(nucs))
	go func() {
		sem := make(chan struct{}, cc)
		var wg sync.WaitGroup
		for _, p := range pocs {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			pp := p
			go func() {
				defer wg.Done()
				select {
				case <-t.stop:
					<-sem
					return
				default:
				}
				url := strings.TrimRight(req.BaseURL, "/") + pp.Path
				var reqBody io.Reader
				if pp.Body != "" {
					reqBody = strings.NewReader(pp.Body)
				}
				method := strings.ToUpper(strings.TrimSpace(pp.Method))
				if method == "" {
					method = "GET"
				}
				req, _ := http.NewRequest(method, url, reqBody)
				for k, v := range pp.Headers {
					req.Header.Set(k, v)
				}
				attempts := pp.Retry + 1
				if attempts <= 0 {
					attempts = 1
				}
				matched := false
				status := 0
				var b []byte
				for i := 0; i < attempts; i++ {
					// rebuild request each attempt to ensure body resets and tolerate invalid URL
					var reqBody io.Reader
					if pp.Body != "" {
						reqBody = strings.NewReader(pp.Body)
					}
					req, e := http.NewRequest(method, url, reqBody)
					if e != nil { // malformed URL or similar; backoff if configured
						if pp.RetryDelayMs > 0 {
							time.Sleep(time.Duration(pp.RetryDelayMs) * time.Millisecond)
						}
						continue
					}
					for k, v := range pp.Headers {
						req.Header.Set(k, v)
					}
					resp, err := client.Do(req)
					if err == nil {
						status = resp.StatusCode
						b, _ = io.ReadAll(resp.Body)
						resp.Body.Close()
					}
					if err == nil {
						lb := strings.ToLower(string(b))
						// legacy match
						if pp.Match != "" && (strings.Contains(lb, strings.ToLower(pp.Match)) || strings.Contains(strings.ToLower(strings.Join(resp.Header.Values("Server"), ";")), strings.ToLower(pp.Match))) {
							matched = true
						}
						// headers match
						if len(pp.MatchHeaders) > 0 {
							okh := true
							for hk, hv := range pp.MatchHeaders {
								vals := strings.ToLower(strings.Join(resp.Header.Values(hk), ";"))
								if !strings.Contains(vals, strings.ToLower(hv)) {
									okh = false
									break
								}
							}
							matched = matched || okh
						}
						// body any/all
						if len(pp.MatchBodyAny) > 0 {
							for _, sub := range pp.MatchBodyAny {
								if strings.Contains(lb, strings.ToLower(sub)) {
									matched = true
									break
								}
							}
						}
						if len(pp.MatchBodyAll) > 0 {
							all := true
							for _, sub := range pp.MatchBodyAll {
								if !strings.Contains(lb, strings.ToLower(sub)) {
									all = false
									break
								}
							}
							matched = matched || all
						}
					}
					if matched {
						break
					}
					if pp.RetryDelayMs > 0 {
						time.Sleep(time.Duration(pp.RetryDelayMs) * time.Millisecond)
					}
				}
				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent, Data: map[string]interface{}{"current": pp.Name}}
				if matched {
					msg.Type = "find"
					curl := fmt.Sprintf("curl -i -X %s '%s'", method, url)
					// 将 info.severity 设置为 high，确保在前端置顶显示
					info := map[string]interface{}{"name": pp.Name}
					if pp.Name == "ThinkPHP 5.0.23 Remote Code Execution" {
						info["severity"] = "critical"
					}
					msg.Data = map[string]interface{}{
						"poc":    pp.Name,
						"url":    url,
						"status": status,
						"exp":    curl,
						"info":   info,
						"req": map[string]interface{}{
							"method":  method,
							"path":    pp.Path,
							"headers": pp.Headers,
							"body":    pp.Body,
						},
					}
				} else {
					// 发送未发现的结果（log类型）
					safeSend(t, SSEMessage{Type: "scan_log", TaskID: t.ID, Data: map[string]interface{}{"poc": pp.Name, "status": "safe"}})
				}
				safeSend(t, msg)
				<-sem
			}()
		}
		// process XRPOCs
		for _, xp := range xrps {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			xp := xp
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				// evaluate each rule: send request and compute rule result
				ruleResults := make(map[string]bool)
				var hitURL string
				var hitStatus int
				var hitMethod string
				var hitPath string
				var hitBody string
				var hitHeaders map[string]string
				for rname, rule := range xp.Rules {
					url := strings.TrimRight(req.BaseURL, "/") + rule.Request.Path
					method := strings.ToUpper(strings.TrimSpace(rule.Request.Method))
					if method == "" {
						method = "GET"
					}
					var reqBody io.Reader
					bodyStr := substVarsSimple(rule.Request.Body, req.BaseURL)
					if bodyStr != "" {
						reqBody = strings.NewReader(bodyStr)
					}
					req0, e := http.NewRequest(method, url, reqBody)
					if e != nil {
						ruleResults[rname] = false
						continue
					}
					for k, v := range rule.Request.Headers {
						req0.Header.Set(k, substVarsSimple(v, req.BaseURL))
					}
					resp, err := client.Do(req0)
					status := 0
					var body []byte
					var hdr http.Header
					if err == nil && resp != nil {
						status = resp.StatusCode
						body, _ = io.ReadAll(resp.Body)
						resp.Body.Close()
						hdr = resp.Header
					}
					// 严格要求：如果请求失败或响应为空，rule结果为false
					if err != nil || resp == nil {
						ruleResults[rname] = false
						continue
					}
					ok := evalRuleExpression(rule.Expression, status, body, hdr)
					ruleResults[rname] = ok
					if ok && hitURL == "" {
						hitURL = url
						hitStatus = status
						hitMethod = method
						hitPath = rule.Request.Path
						hitBody = bodyStr
						hitHeaders = make(map[string]string, len(rule.Request.Headers))
						for k, v := range rule.Request.Headers {
							hitHeaders[k] = substVarsSimple(v, req.BaseURL)
						}
					}
				}
				// evaluate global expression combining rule results
				// 严格要求：只有全局表达式完全满足才报告为可利用
				vuln := evalGlobalExpression(xp.Expression, ruleResults)

				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				pname := xp.Info.Name
				if pname == "" {
					pname = xp.ID
				}
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent, Data: map[string]interface{}{"current": pname}}

				// 只有全局表达式完全满足才报告为可利用（移除疑似逻辑，避免误报）
				if vuln {
					msg.Type = "find"
					curl := ""
					if hitURL != "" {
						curl = fmt.Sprintf("curl -i '%s'", hitURL)
					}
					reqMap := map[string]interface{}{
						"method":  hitMethod,
						"path":    hitPath,
						"headers": hitHeaders,
						"body":    hitBody,
					}
					msg.Data = map[string]interface{}{"poc": pname, "url": hitURL, "status": hitStatus, "exp": curl, "info": xp.Info, "req": reqMap}
				} else {
					safeSend(t, SSEMessage{Type: "scan_log", TaskID: t.ID, Data: map[string]interface{}{"poc": pname, "status": "safe"}})
				}
				safeSend(t, msg)
			}()
		}
		// process Nuclei subset POCs
		for _, np := range nucs {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			np := np
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				ok, hitURL, hitStatus, hitReq := runNucleiOnce(req.BaseURL, np, client)
				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				pname := np.Info.Name
				if pname == "" {
					pname = np.ID
				}
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent, Data: map[string]interface{}{"current": pname}}
				if ok {
					msg.Type = "find"
					curl := ""
					if hitURL != "" {
						curl = fmt.Sprintf("curl -i '%s'", hitURL)
					}
					// 当 runNucleiOnce 返回的状态码为 0，表示疑似命中（兜底），统一用字符串 "suspect"
					var statusVal interface{}
					if hitStatus == 0 {
						statusVal = "suspect"
					} else {
						statusVal = hitStatus
					}

					// 确保 info 对象包含 severity 字段，并对 ThinkPHP 漏洞进行特殊处理
					info := np.Info
					// 创建一个新的 map 来存储 info 数据，以便我们可以修改它而不影响原始数据
					infoMap := map[string]interface{}{
						"name":        info.Name,
						"author":      info.Author,
						"severity":    info.Severity,
						"risk":        info.Risk,
						"reference":   info.Reference,
						"description": info.Description,
						"tags":        info.Tags,
					}

					if strings.Contains(strings.ToLower(info.Name), "thinkphp") {
						infoMap["severity"] = "critical"
					}

					msg.Data = map[string]interface{}{"poc": pname, "url": hitURL, "status": statusVal, "exp": curl, "info": infoMap, "req": hitReq}
				} else {
					safeSend(t, SSEMessage{Type: "scan_log", TaskID: t.ID, Data: map[string]interface{}{"poc": pname, "status": "safe"}})
				}
				safeSend(t, msg)
			}()
		}
		wg.Wait()
		finishTask(t.ID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
}

// --- Rule expression evaluation (X-Ray style) ---
func evalRuleExpression(expr string, status int, body []byte, headers http.Header) bool {
	// 严格要求：如果响应为空，直接返回false（除非表达式明确允许空响应）
	if len(body) == 0 && strings.Contains(expr, "bcontains") {
		// 如果表达式包含bcontains但响应为空，返回false
		return false
	}

	ex := expr
	// response.status == N
	reStatus := regexp.MustCompile(`response\.status\s*==\s*(\d+)`)
	ex = reStatus.ReplaceAllStringFunc(ex, func(s string) string {
		m := reStatus.FindStringSubmatch(s)
		if len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			if status == n {
				return "true"
			}
		}
		return "false"
	})
	// response.body.bcontains(b"..." or b'...')
	reContains := regexp.MustCompile(`response\.body\.bcontains\(b?['\"]([\s\S]*?)['\"]\)`) // support single or double quotes, non-greedy
	ex = reContains.ReplaceAllStringFunc(ex, func(s string) string {
		m := reContains.FindStringSubmatch(s)
		if len(m) == 2 {
			needle := m[1]
			// 严格要求：needle不能为空，且响应体必须包含该字符串
			if needle == "" {
				return "false"
			}
			if len(body) > 0 && strings.Contains(string(body), needle) {
				return "true"
			}
		}
		return "false"
	})
	// 支持 r'pattern'.bmatches(response.body) 格式（带r前缀的正则）
	reMatchesR := regexp.MustCompile(`r['"]([\s\S]*?)['"]\.bmatches\(response\.body\)`)
	ex = reMatchesR.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatchesR.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			// 严格要求：pattern不能为空，响应体必须存在
			if pat == "" || len(body) == 0 {
				return "false"
			}
			re, e := regexp.Compile(pat)
			if e == nil && re.Match(body) {
				return "true"
			}
		}
		return "false"
	})

	// response.body.bmatches("...")
	reMatches1 := regexp.MustCompile(`response\.body\.bmatches\("([\s\S]*?)"\)`) // pattern in quotes
	ex = reMatches1.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatches1.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			// 严格要求：pattern不能为空，响应体必须存在
			if pat == "" || len(body) == 0 {
				return "false"
			}
			re, e := regexp.Compile(pat)
			if e == nil && re.Match(body) {
				return "true"
			}
		}
		return "false"
	})
	// also support "literal".bmatches(response.body)
	reMatches2 := regexp.MustCompile(`"([\s\S]*?)"\.bmatches\(response\.body\)`)
	ex = reMatches2.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatches2.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			// 严格要求：pattern不能为空，响应体必须存在
			if pat == "" || len(body) == 0 {
				return "false"
			}
			re, e := regexp.Compile(pat)
			if e == nil && re.Match(body) {
				return "true"
			}
		}
		return "false"
	})

	// 处理 oobCheck - 如果表达式包含oobCheck，当前不支持OOB验证，返回false
	if strings.Contains(ex, "oobCheck") {
		// OOB验证需要外部服务器支持，这里暂时返回false，避免误报
		// 注意：这会导致CVE-2023-38646这类需要OOB验证的POC无法被检测
		// 但可以避免误报，用户可以使用其他不依赖OOB的POC
		return false
	}

	// Normalize booleans and whitespace
	ex = strings.ReplaceAll(ex, "True", "true")
	ex = strings.ReplaceAll(ex, "False", "false")
	return evalBoolExpr(ex)
}

func evalGlobalExpression(expr string, ruleResults map[string]bool) bool {
	ex := expr
	// Replace rule calls like Linux0() with true/false
	re := regexp.MustCompile(`([A-Za-z0-9_]+)\(\)`)
	ex = re.ReplaceAllStringFunc(ex, func(s string) string {
		name := re.FindStringSubmatch(s)
		if len(name) == 2 {
			if ruleResults[name[1]] {
				return "true"
			}
		}
		return "false"
	})
	return evalBoolExpr(ex)
}

// Simple boolean evaluator supporting parentheses, &&, ||
func evalBoolExpr(expr string) bool {
	ex := strings.TrimSpace(expr)
	// 如果表达式为空，返回false
	if ex == "" {
		return false
	}
	// evaluate parentheses recursively
	for {
		re := regexp.MustCompile(`\([^()]*\)`)
		loc := re.FindStringIndex(ex)
		if loc == nil {
			break
		}
		inner := ex[loc[0]+1 : loc[1]-1]
		val := evalBoolExpr(inner)
		replacement := "false"
		if val {
			replacement = "true"
		}
		ex = ex[:loc[0]] + replacement + ex[loc[1]:]
	}
	// no parentheses: OR of AND terms
	orTerms := splitByOperator(ex, "||")
	result := false
	for _, term := range orTerms {
		andTerms := splitByOperator(term, "&&")
		// 严格要求：AND条件必须所有项都为true
		andVal := true
		hasValidTerm := false
		for _, f := range andTerms {
			v := strings.TrimSpace(f)
			if v == "" {
				continue
			}
			hasValidTerm = true
			if v != "true" {
				andVal = false
				break
			}
		}
		// 只有当有有效项且所有项都为true时，andVal才为true
		if hasValidTerm && andVal {
			result = true
			break
		}
	}
	return result
}

func splitByOperator(s, op string) []string {
	parts := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if i+len(op) <= len(s) && s[i:i+len(op)] == op {
			parts = append(parts, cur)
			cur = ""
			i += len(op) - 1
		} else {
			cur += string(s[i])
		}
	}
	if cur != "" || len(parts) == 0 {
		parts = append(parts, cur)
	}
	return parts
}

// --- Nuclei subset executor ---
func genRandStr() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func substVarsSimple(s, baseURL string) string {
	out := s
	// Hostname from baseURL
	host := strings.TrimSpace(baseURL)
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	out = strings.ReplaceAll(out, "{{Hostname}}", host)
	out = strings.ReplaceAll(out, "{{randstr}}", genRandStr())
	out = strings.ReplaceAll(out, "{{BaseURL}}", baseURL)
	out = strings.ReplaceAll(out, "{{hosturl}}", baseURL)
	return out
}

func parseRawHTTP(raw string, baseURL string) (method string, url string, headers map[string]string, body string) {
	method = "GET"
	url = strings.TrimSpace(baseURL)
	headers = map[string]string{}
	raw = substVarsSimple(raw, baseURL)
	lines := strings.Split(raw, "\n")
	// first line: METHOD PATH HTTP/1.1
	if len(lines) > 0 {
		parts := strings.Fields(strings.TrimSpace(lines[0]))
		if len(parts) >= 2 {
			method = strings.ToUpper(parts[0])
			path := parts[1]
			url = strings.TrimRight(baseURL, "/") + path
		}
	}
	// headers until empty line
	i := 1
	for ; i < len(lines); i++ {
		ln := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(ln) == "" {
			i++
			break
		}
		if idx := strings.Index(ln, ":"); idx >= 0 {
			k := strings.TrimSpace(ln[:idx])
			v := strings.TrimSpace(ln[idx+1:])
			headers[k] = v
		}
	}
	// remaining lines as body
	if i < len(lines) {
		body = strings.Join(lines[i:], "\n")
	}
	return
}

// matchNucleiMatchers 匹配 Nuclei matchers（辅助函数，用于匹配给定的 matchers 列表）
func matchNucleiMatchers(resps []*http.Response, bodies [][]byte, matchers []NucleiMatcher, cond string) bool {
	if cond == "" {
		cond = "and"
	}
	cond = strings.ToLower(strings.TrimSpace(cond))
	vals := make([]bool, 0, len(matchers))
	for _, m := range matchers {
		mt := strings.ToLower(strings.TrimSpace(m.Type))
		ok := false
		switch mt {
		case "word":
			part := strings.ToLower(strings.TrimSpace(m.Part))
			if part == "" {
				part = "body"
			}
			words := m.Words
			// 如果没有 words，视为无效匹配
			if len(words) == 0 {
				ok = false
				break
			}
			all := strings.ToLower(strings.TrimSpace(m.Condition)) == "and"
			// use last response by default
			var resp *http.Response
			var body []byte
			if len(resps) > 0 {
				resp = resps[len(resps)-1]
			}
			if len(bodies) > 0 {
				body = bodies[len(bodies)-1]
			}
			// 严格要求：响应必须存在且非空
			if resp == nil || len(body) == 0 {
				ok = false
				break
			}
			if part == "body" {
				lb := strings.ToLower(string(body))
				if all {
					ok = true
					for _, w := range words {
						if w == "" {
							continue
						}
						if !strings.Contains(lb, strings.ToLower(w)) {
							ok = false
							break
						}
					}
				} else {
					ok = false
					for _, w := range words {
						if w == "" {
							continue
						}
						if strings.Contains(lb, strings.ToLower(w)) {
							ok = true
							break
						}
					}
				}
			} else if part == "header" {
				hs := strings.ToLower(strings.Join(resp.Header.Values("Content-Type"), ";"))
				if all {
					ok = true
					for _, w := range words {
						if w == "" {
							continue
						}
						if !strings.Contains(hs, strings.ToLower(w)) {
							ok = false
							break
						}
					}
				} else {
					ok = false
					for _, w := range words {
						if w == "" {
							continue
						}
						if strings.Contains(hs, strings.ToLower(w)) {
							ok = true
							break
						}
					}
				}
			}
		case "regex":
			// 处理 regex 类型的 matcher
			part := strings.ToLower(strings.TrimSpace(m.Part))
			if part == "" {
				part = "body"
			}
			regexes := m.Regex
			// 如果没有 regex，视为无效匹配
			if len(regexes) == 0 {
				ok = false
				break
			}
			all := strings.ToLower(strings.TrimSpace(m.Condition)) == "and"
			var resp *http.Response
			var body []byte
			if len(resps) > 0 {
				resp = resps[len(resps)-1]
			}
			if len(bodies) > 0 {
				body = bodies[len(bodies)-1]
			}
			// 严格要求：响应必须存在且非空
			if resp == nil || len(body) == 0 {
				ok = false
				break
			}
			if part == "body" {
				if all {
					ok = true
					for _, reStr := range regexes {
						if reStr == "" {
							continue
						}
						re, err := regexp.Compile(reStr)
						if err != nil {
							ok = false
							break
						}
						if !re.Match(body) {
							ok = false
							break
						}
					}
				} else {
					ok = false
					for _, reStr := range regexes {
						if reStr == "" {
							continue
						}
						re, err := regexp.Compile(reStr)
						if err == nil && re.Match(body) {
							ok = true
							break
						}
					}
				}
			} else if part == "header" {
				hs := strings.ToLower(strings.Join(resp.Header.Values("Content-Type"), ";"))
				if all {
					ok = true
					for _, reStr := range regexes {
						if reStr == "" {
							continue
						}
						re, err := regexp.Compile(reStr)
						if err != nil {
							ok = false
							break
						}
						if !re.MatchString(hs) {
							ok = false
							break
						}
					}
				} else {
					ok = false
					for _, reStr := range regexes {
						if reStr == "" {
							continue
						}
						re, err := regexp.Compile(reStr)
						if err == nil && re.MatchString(hs) {
							ok = true
							break
						}
					}
				}
			}
		case "status":
			// check any response matches listed status
			// 如果没有 status 列表，视为无效匹配
			if len(m.Status) == 0 {
				ok = false
				break
			}
			// 严格要求：至少有一个响应存在且状态码匹配
			for _, r := range resps {
				if r == nil {
					continue
				}
				for _, s := range m.Status {
					if r.StatusCode == s {
						ok = true
						break
					}
				}
				if ok {
					break
				}
			}
		case "dsl":
			all := strings.ToLower(strings.TrimSpace(m.Condition)) == "and"
			if len(m.Dsl) == 0 {
				ok = true
				break
			}
			oneOK := false
			allOK := true
			for _, expr := range m.Dsl {
				expr = strings.TrimSpace(expr)
				// support: status_code_N == 200
				reStatus := regexp.MustCompile(`^status_code_(\d+)\s*==\s*(\d+)$`)
				if m := reStatus.FindStringSubmatch(expr); len(m) == 3 {
					i, _ := strconv.Atoi(m[1])
					v, _ := strconv.Atoi(m[2])
					if i >= 1 && i <= len(resps) && resps[i-1] != nil && resps[i-1].StatusCode == v {
						oneOK = true
					} else {
						allOK = false
					}
					continue
				}
				// support: body_N != ""
				reBodyNotEmpty := regexp.MustCompile(`^body_(\d+)\s*!=\s*""$`)
				if m := reBodyNotEmpty.FindStringSubmatch(expr); len(m) == 2 {
					i, _ := strconv.Atoi(m[1])
					if i >= 1 && i <= len(bodies) && len(bodies[i-1]) > 0 {
						oneOK = true
					} else {
						allOK = false
					}
					continue
				}
				// unknown expr -> treat as false
				allOK = false
			}
			ok = all
			if all {
				ok = allOK
			} else {
				ok = oneOK
			}
		}
		vals = append(vals, ok)
	}

	// 如果没有任何有效的matcher，返回false
	if len(vals) == 0 {
		return false
	}

	if cond == "and" {
		// 严格要求：所有matcher都必须匹配
		for _, v := range vals {
			if !v {
				return false
			}
		}
		return true
	}
	// or 条件：至少有一个匹配
	for _, v := range vals {
		if v {
			return true
		}
	}
	return false
}

func matchNucleiMulti(resps []*http.Response, bodies [][]byte, np NucleiPOC) bool {
	cond := strings.ToLower(strings.TrimSpace(np.MatchersCondition))
	if cond == "" {
		cond = "and"
	}
	return matchNucleiMatchers(resps, bodies, np.Matchers, cond)
}

// evalDetectionExpression 评估 detection 表达式（旧版 Nuclei 格式）
// 支持简单的表达式如: StatusCode() == 200 && StringSearch('body', 'pattern')
func evalDetectionExpression(expr string, resp *http.Response, body []byte) bool {
	// 严格要求：响应必须存在且非空
	if resp == nil || len(body) == 0 {
		return false
	}

	// 替换 StatusCode() == N
	expr = regexp.MustCompile(`StatusCode\(\)\s*==\s*(\d+)`).ReplaceAllStringFunc(expr, func(match string) string {
		re := regexp.MustCompile(`StatusCode\(\)\s*==\s*(\d+)`)
		matches := re.FindStringSubmatch(match)
		if len(matches) == 2 {
			expected, _ := strconv.Atoi(matches[1])
			if resp.StatusCode == expected {
				return "true"
			}
		}
		return "false"
	})

	// 替换 StringSearch('body', 'pattern') 或 StringSearch("body", "pattern")
	expr = regexp.MustCompile(`StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`).ReplaceAllStringFunc(expr, func(match string) string {
		re := regexp.MustCompile(`StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`)
		matches := re.FindStringSubmatch(match)
		if len(matches) == 2 {
			pattern := matches[1]
			// 严格要求：pattern 不能为空
			if pattern != "" && strings.Contains(string(body), pattern) {
				return "true"
			}
		}
		return "false"
	})

	// 替换 StringSearch('response', 'pattern') 或 StringSearch("response", "pattern")
	expr = regexp.MustCompile(`StringSearch\(['"]response['"]\s*,\s*['"]([^'"]+)['"]\)`).ReplaceAllStringFunc(expr, func(match string) string {
		re := regexp.MustCompile(`StringSearch\(['"]response['"]\s*,\s*['"]([^'"]+)['"]\)`)
		matches := re.FindStringSubmatch(match)
		if len(matches) == 2 {
			pattern := matches[1]
			// 严格要求：pattern 不能为空
			if pattern != "" && strings.Contains(string(body), pattern) {
				return "true"
			}
		}
		return "false"
	})

	// 替换 !StringSearch('body', 'pattern') 或 !StringSearch("body", "pattern")
	expr = regexp.MustCompile(`!StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`).ReplaceAllStringFunc(expr, func(match string) string {
		re := regexp.MustCompile(`!StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`)
		matches := re.FindStringSubmatch(match)
		if len(matches) == 2 {
			pattern := matches[1]
			// 严格要求：pattern 不能为空
			if pattern != "" && !strings.Contains(string(body), pattern) {
				return "true"
			}
		}
		return "false"
	})

	// 替换 RegexSearch('resBody', 'pattern') 或 RegexSearch("resBody", "pattern")
	expr = regexp.MustCompile(`RegexSearch\(['"]resBody['"]\s*,\s*['"]([^'"]+)['"]\)`).ReplaceAllStringFunc(expr, func(match string) string {
		re := regexp.MustCompile(`RegexSearch\(['"]resBody['"]\s*,\s*['"]([^'"]+)['"]\)`)
		matches := re.FindStringSubmatch(match)
		if len(matches) == 2 {
			pattern := matches[1]
			// 严格要求：pattern 不能为空
			if pattern != "" {
				matched, err := regexp.MatchString(pattern, string(body))
				if err == nil && matched {
					return "true"
				}
			}
		}
		return "false"
	})

	// 使用简单的布尔表达式评估
	return evalBoolExpr(expr)
}

func runNucleiOnce(baseURL string, np NucleiPOC, client *http.Client) (bool, string, int, map[string]interface{}) {
	if len(np.Requests) == 0 {
		return false, "", 0, nil
	}
	var resps []*http.Response
	var bodies [][]byte
	var hitURL string
	var hitStatus int
	var hitReq map[string]interface{}
	for _, reqDef := range np.Requests {
		var method, url, body string
		headers := map[string]string{}
		if len(reqDef.Raw) > 0 {
			m, u, h, b := parseRawHTTP(reqDef.Raw[0], baseURL)
			method, url, headers, body = m, u, h, b
		} else {
			method = strings.ToUpper(strings.TrimSpace(reqDef.Method))
			if method == "" {
				method = "GET"
			}
			p := baseURL
			// 优先使用 url 字段，如果没有则使用 path
			if reqDef.URL != "" {
				rawURL := strings.TrimSpace(substVarsSimple(reqDef.URL, baseURL))
				p = rawURL
				// 如果 url 不是完整 URL，则拼接 baseURL
				if !strings.HasPrefix(strings.ToLower(rawURL), "http://") &&
					!strings.HasPrefix(strings.ToLower(rawURL), "https://") &&
					!strings.HasPrefix(rawURL, "{{BaseURL}}") {
					p = strings.TrimRight(baseURL, "/") + rawURL
				}
			} else if len(reqDef.Path) > 0 {
				rawPath := strings.TrimSpace(substVarsSimple(reqDef.Path[0], baseURL))
				low := strings.ToLower(rawPath)
				if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(rawPath, "{{BaseURL}}") {
					p = rawPath
				} else {
					p = strings.TrimRight(baseURL, "/") + rawPath
				}
			}
			url = p
			// 处理 headers（可能是 map 或数组格式）
			if reqDef.Headers != nil {
				if headersMap, ok := reqDef.Headers.(map[string]string); ok {
					headers = headersMap
				} else if headersMap, ok := reqDef.Headers.(map[interface{}]interface{}); ok {
					// 处理 YAML 解析后的 map[interface{}]interface{} 格式
					headers = make(map[string]string)
					for k, v := range headersMap {
						if kStr, ok := k.(string); ok {
							if vStr, ok := v.(string); ok {
								headers[kStr] = vStr
							}
						}
					}
				} else if headersArray, ok := reqDef.Headers.([]interface{}); ok {
					// 处理数组格式的 headers（如 ["User-Agent: Mozilla/5.0 ..."]）
					headers = make(map[string]string)
					for _, item := range headersArray {
						if itemStr, ok := item.(string); ok {
							// 解析 "Key: Value" 格式
							parts := strings.SplitN(itemStr, ":", 2)
							if len(parts) == 2 {
								headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
							}
						}
					}
				}
			}
			body = substVarsSimple(reqDef.Body, baseURL)
		}
		path := url
		base := strings.TrimRight(baseURL, "/")
		if strings.HasPrefix(url, base) {
			path = url[len(base):]
			if path == "" {
				path = "/"
			}
		}
		reqMap := map[string]interface{}{
			"method":  method,
			"path":    path,
			"headers": headers,
			"body":    body,
		}
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req0, err := http.NewRequest(method, url, reader)
		if err != nil {
			continue
		}
		for k, v := range headers {
			req0.Header.Set(k, v)
		}

		resp, err := client.Do(req0)
		if err != nil {
			resps = append(resps, nil)
			bodies = append(bodies, nil)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		resps = append(resps, resp)
		bodies = append(bodies, b)
		if hitURL == "" {
			hitURL = url
			hitStatus = resp.StatusCode
			hitReq = reqMap
		}
	}

	// 检查是否有 request 内部的 matchers 或 detections
	for i, reqDef := range np.Requests {
		// 检查 request 内部的 matchers
		if len(reqDef.Matchers) > 0 || len(reqDef.Detections) > 0 {
			// 如果这个 request 有 matchers，使用它来匹配对应的响应
			// 严格要求：响应必须存在且非空
			if i < len(resps) && i < len(bodies) && resps[i] != nil && len(bodies[i]) > 0 {
				var reqMatchers []NucleiMatcher
				reqCond := reqDef.MatchersCondition
				if reqCond == "" {
					reqCond = "and"
				}

				// 使用 request 内部的 matchers
				if len(reqDef.Matchers) > 0 {
					reqMatchers = reqDef.Matchers
				} else if len(reqDef.Detections) > 0 {
					// 处理 detections 字段（旧版格式）
					// detections 是一个字符串数组，每个字符串是一个表达式
					// 例如: "StatusCode() == 200 && StringSearch('body', 'pattern')"
					// 严格要求：所有 detections 都必须匹配
					allDetectionsOK := true
					for _, det := range reqDef.Detections {
						if det == "" {
							continue
						}
						if !evalDetectionExpression(det, resps[i], bodies[i]) {
							allDetectionsOK = false
							break
						}
					}
					// 只有当所有detections都匹配且至少有一个有效的detection时，才返回true
					// 检查是否有至少一个非空的detection
					hasValidDetection := false
					for _, det := range reqDef.Detections {
						if strings.TrimSpace(det) != "" {
							hasValidDetection = true
							break
						}
					}
					if allDetectionsOK && hasValidDetection {
						return true, hitURL, hitStatus, hitReq
					}
				}

				// 使用 request 内部的 matchers 进行匹配
				if len(reqMatchers) > 0 {
					ok := matchNucleiMatchers([]*http.Response{resps[i]}, [][]byte{bodies[i]}, reqMatchers, reqCond)
					if ok {
						return true, hitURL, hitStatus, hitReq
					}
				}
			}
		}
	}

	// 使用顶级 matchers 进行匹配
	// 严格要求：必须有有效的 matchers 或 detections 才能匹配成功
	if len(np.Matchers) > 0 {
		ok := matchNucleiMulti(resps, bodies, np)
		if ok {
			return true, hitURL, hitStatus, hitReq
		}
	}

	// 如果没有任何matchers或detections，返回false（不匹配）
	hasAnyMatchers := len(np.Matchers) > 0
	for _, req := range np.Requests {
		if len(req.Matchers) > 0 || len(req.Detections) > 0 {
			hasAnyMatchers = true
			break
		}
	}
	if !hasAnyMatchers {
		return false, "", 0, nil
	}

	// 如果顶级没有matchers，但请求内部有matchers/detections，且都已经检查过，返回false
	return false, "", 0, nil
}

*/
// AIAutoScanReq 自动化扫描工作流请求
type AIAutoScanReq struct {
	Target    string                 `json:"target"`    // 目标地址（IP或域名）
	Ports     string                 `json:"ports"`     // 端口范围（默认："1-1000,3000-4000,8000-9000"）
	Provider  string                 `json:"provider"`  // AI提供商（deepseek/openai/anthropic/ollama）
	APIKey    string                 `json:"apiKey"`    // API Key
	BaseURL   string                 `json:"baseURL"`   // API Base URL（可选）
	Model     string                 `json:"model"`     // 模型名称（可选）
	EnableWAF bool                   `json:"enableWAF"` // 是否启用WAF测试
	Config    map[string]interface{} `json:"config"`    // 其他配置选项
}

// 定义AI可调用的扫描工具
func getAIScanTools() []mcp.AITool {
	return []mcp.AITool{
		{
			Type:        "function",
			Name:        "port_scan",
			Description: "执行端口扫描，检测目标主机的开放端口。通常作为第一步扫描，根据开放端口决定后续扫描策略。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target": map[string]interface{}{
						"type":        "string",
						"description": "目标IP地址或域名",
					},
					"ports": map[string]interface{}{
						"type":        "string",
						"description": "端口范围，格式如 '1-1000,3000-4000' 或 '22,80,443'",
					},
				},
				"required": []string{"target", "ports"},
			},
		},
		{
			Type:        "function",
			Name:        "web_probe",
			Description: "执行Web探针，检测Web服务信息（标题、技术栈、指纹等）。当发现HTTP/HTTPS端口（如80、443、8080等）时使用。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"urls": map[string]interface{}{
						"type":        "array",
						"description": "要探测的URL列表，如 ['http://target:8080', 'https://target:8443']",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"urls"},
			},
		},
		{
			Type:        "function",
			Name:        "dir_scan",
			Description: "执行目录扫描，发现Web服务的隐藏目录和文件。当确认存在Web服务时使用。根据Web探针识别到的技术栈，会自动选择对应的字典文件（如PHP、JSP、ASP等）。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "要扫描的Web服务URL，如 'http://target:8080'",
					},
					"tech": map[string]interface{}{
						"type":        "array",
						"description": "技术栈信息（可选），从Web探针结果中获取。系统会根据技术栈自动选择对应的字典文件（如PHP、JSP、ASP等）。",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Type:        "function",
			Name:        "poc_scan",
			Description: "执行POC漏洞扫描，检测已知漏洞。当发现特定技术栈（如ThinkPHP、Spring Boot等）时，应着重扫描相关POC。可以指定POC关键词进行针对性扫描。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "要扫描的目标URL",
					},
					"keywords": map[string]interface{}{
						"type":        "array",
						"description": "POC关键词过滤（可选），如发现ThinkPHP时使用 ['thinkphp', 'think']，发现Spring Boot时使用 ['spring', 'springboot']",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"url"},
			},
		},
	}
}

// aiAutoScanHandler AI自动化扫描工作流处理器
// 使用AI工具调用，让AI自主决策每一步扫描操作，根据扫描结果智能决定下一步操作
func aiAutoScanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AIAutoScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	if req.Target == "" {
		http.Error(w, "目标地址不能为空", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		http.Error(w, "API Key 不能为空", http.StatusBadRequest)
		return
	}

	// 设置默认端口范围
	if req.Ports == "" {
		req.Ports = "1-1000,3000-4000,8000-9000"
	}

	// 获取并发数和超时时间（从Config中读取，如果没有则使用默认值）
	concurrency := 50
	timeoutMs := 3000
	if req.Config != nil {
		if c, ok := req.Config["concurrency"].(float64); ok {
			concurrency = int(c)
		}
		if t, ok := req.Config["timeoutMs"].(float64); ok {
			timeoutMs = int(t)
		}
	}
	if concurrency <= 0 {
		concurrency = 50
	}
	if timeoutMs <= 0 {
		timeoutMs = 3000
	}

	// 创建任务用于进度跟踪
	t := newTask(100) // 总进度100%

	// 设置SSE响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	// 在goroutine中执行自动化扫描工作流
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[自动扫描] 发生panic: %v", r)
				msg := SSEMessage{
					Type:     "error",
					TaskID:   t.ID,
					Progress: "0/100",
					Percent:  0,
					Data:     map[string]interface{}{"message": fmt.Sprintf("扫描过程中发生错误: %v", r)},
				}
				safeSend(t, msg)
			}
			finishTask(t.ID)
		}()
		defer func() {
			// 发送完成消息
			msg := SSEMessage{
				Type:     "complete",
				TaskID:   t.ID,
				Progress: "100/100",
				Percent:  100,
				Data:     map[string]interface{}{"message": "自动化扫描完成"},
			}
			safeSend(t, msg)
		}()

		// 收集所有扫描结果
		allResults := make(map[string]interface{})
		allResults["target"] = req.Target
		allResults["scanTime"] = time.Now().Format("2006-01-02 15:04:05")

		// AI分析结果收集（保存所有分析过程）
		aiAnalysisSteps := []map[string]interface{}{}

		// 初始化AI Provider
		var aiProvider mcp.AIProvider
		switch req.Provider {
		case "deepseek":
			aiProvider = mcp.NewDeepSeekProvider(req.APIKey)
			if req.BaseURL != "" {
				if p, ok := aiProvider.(*mcp.DeepSeekProvider); ok {
					p.BaseURL = req.BaseURL
				}
			}
			if req.Model != "" {
				if p, ok := aiProvider.(*mcp.DeepSeekProvider); ok {
					p.Model = req.Model
				}
			}
		case "openai":
			aiProvider = mcp.NewOpenAIProvider(req.APIKey)
			if req.BaseURL != "" {
				if p, ok := aiProvider.(*mcp.OpenAIProvider); ok {
					p.BaseURL = req.BaseURL
				}
			}
			if req.Model != "" {
				if p, ok := aiProvider.(*mcp.OpenAIProvider); ok {
					p.Model = req.Model
				}
			}
		case "anthropic":
			aiProvider = mcp.NewAnthropicProvider(req.APIKey)
			if req.Model != "" {
				if p, ok := aiProvider.(*mcp.AnthropicProvider); ok {
					p.Model = req.Model
				}
			}
		case "ollama":
			baseURL := "http://localhost:11434"
			if req.BaseURL != "" {
				baseURL = req.BaseURL
			}
			model := "llama2"
			if req.Model != "" {
				model = req.Model
			}
			aiProvider = mcp.NewOllamaProvider(baseURL, model)
		default:
			sendProgress(t, 0, "不支持的AI提供商")
			return
		}

		// 获取AI可调用的工具
		scanTools := getAIScanTools()

		// 初始化AI对话消息
		messages := []mcp.ChatMessage{
			{
				Role: "system",
				Content: `你是一位专业的安全扫描专家，负责对目标进行全面的安全扫描。

【核心原则】
- 每次决策前，先回顾之前的工具调用结果，避免重复相同的操作
- 如果某个工具已经执行过并返回了结果，不要再次调用相同的工具
- 只有在确实需要获取更多数据时才调用工具

你的工作流程：
1. 首先进行端口扫描，了解目标开放的端口（只需执行一次）
2. 根据端口扫描结果，分析可能存在的服务（如22端口可能是SSH，80/443端口可能是Web服务）
3. 对于Web服务，进行Web探针，识别技术栈（如ThinkPHP、Spring Boot、Metabase、WordPress等）
4. 根据技术栈，进行针对性的POC扫描（必须使用keywords参数！）
   - 发现ThinkPHP时，使用keywords: ['thinkphp', 'think']
   - 发现Spring Boot时，使用keywords: ['spring', 'springboot']
   - 发现Metabase时，使用keywords: ['metabase']
   - 发现WordPress时，使用keywords: ['wordpress', 'wp']
   - 发现Struts时，使用keywords: ['struts']
   - 发现Shiro时，使用keywords: ['shiro']
   - 发现Fastjson时，使用keywords: ['fastjson']
   - 发现其他技术栈时，使用技术栈名称作为关键词
5. 根据需要执行目录扫描，发现隐藏目录
6. 当发现漏洞时，必须生成对应的EXP（Exploit）和使用说明

重要提示：
- 看到22端口开放，应该分析可能存在SSH爆破风险
- 看到80/443/8080等端口，应该进行Web探针
- 看到特定技术栈时，必须使用keywords参数进行针对性POC扫描，不要扫描所有POC
- Web探针结果中的技术栈信息（tech字段）会明确显示技术栈名称，请直接使用该名称作为POC扫描关键词
- 如果Web探针发现Metabase，必须使用keywords: ['metabase']进行POC扫描
- 如果Web探针发现任何技术栈，都必须进行对应的POC扫描，不要跳过
- 执行目录扫描时，如果Web探针已识别到技术栈（如PHP、JSP、ASP等），应在dir_scan工具调用时传递tech参数，系统会根据技术栈自动选择对应的字典文件，提高扫描效率
- 当POC扫描发现漏洞时，必须为该漏洞生成可用的EXP代码（Python、Bash、curl命令等），并详细说明如何使用
- EXP应包括完整的利用代码、参数说明、使用步骤和预期结果
- 输出纯文本分析，不要使用Markdown格式
- 每次分析后，决定下一步要执行什么扫描操作
- 【端口范围约束】进行端口扫描时，必须严格使用用户指定的端口范围，禁止自行决定或扩展端口范围
- 如果用户指定端口范围为8000-8100，则只扫描8000-8100范围内的端口，不得扫描其他端口（如1-1000, 3000-4000等）
- 如果用户未指定端口范围，才使用默认范围"1-1000,3000-4000,8000-9000"
- 执行port_scan工具时，只传递用户指定的端口范围，不要自行决定扫描哪些端口

现在开始对目标进行扫描。`,
				Time: time.Now().Format(time.RFC3339),
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("请对目标 %s 进行安全扫描。端口范围：%s", req.Target, req.Ports),
				Time:    time.Now().Format(time.RFC3339),
			},
		}

		// 最大迭代次数（防止无限循环）
		maxIterations := 20
		iteration := 0

		// AI驱动的循环扫描工作流
		for iteration < maxIterations {
			iteration++

			// 调用AI，获取下一步操作或分析
			sendProgress(t, 5+iteration*2, "AI正在分析并决策下一步操作...")
			msgCountBefore := len(messages)
			response, toolCalls, err := aiProvider.Chat(messages, scanTools)
			if err != nil {
				log.Printf("[ERROR] AI调用失败: %v", err)
				sendProgress(t, 0, fmt.Sprintf("AI调用失败: %v", err))
				break
			}
			log.Printf("[AI调用] 第 %d 次调用，消息数量: %d -> %d, toolCalls: %d",
				iteration, msgCountBefore, len(messages), len(toolCalls))
			if len(toolCalls) > 0 {
				// AI决定调用工具
			}

			// 如果AI返回了文本分析，保存并显示
			if response != "" && len(toolCalls) == 0 {
				// AI返回了分析结果，没有工具调用（可能是最终总结）
				analysisStep := map[string]interface{}{
					"step":    fmt.Sprintf("AI分析-%d", iteration),
					"content": response,
					"time":    time.Now().Format("2006-01-02 15:04:05"),
				}
				aiAnalysisSteps = append(aiAnalysisSteps, analysisStep)

				// 发送AI分析结果
				msg := SSEMessage{
					Type:     "ai-analysis",
					TaskID:   t.ID,
					Progress: fmt.Sprintf("%d/100", 5+iteration*2),
					Percent:  5 + iteration*2,
					Data:     map[string]interface{}{"step": fmt.Sprintf("AI分析-%d", iteration), "chunk": response},
				}
				safeSend(t, msg)

				// 添加AI回复到消息历史
				messages = append(messages, mcp.ChatMessage{
					Role:    "assistant",
					Content: response,
					Time:    time.Now().Format(time.RFC3339),
				})

				// 检查是否应该结束循环
				// 当AI返回的内容主要是最终报告/总结时结束
				shouldEnd := false
				responseLower := strings.ToLower(response)

				// 如果AI明确表示完成（使用了明确的结束语）
				hasEndPhrase := strings.Contains(responseLower, "扫描完成") ||
					strings.Contains(responseLower, "分析完成") ||
					strings.Contains(responseLower, "检测完成") ||
					strings.Contains(responseLower, "总结") ||
					strings.Contains(responseLower, "报告已生成")

				// 如果AI在提供EXP或漏洞详情，说明还在分析中
				isProvidingDetails := strings.Contains(responseLower, "漏洞描述") ||
					strings.Contains(responseLower, "exp代码") ||
					strings.Contains(responseLower, "exploit") ||
					strings.Contains(responseLower, "```python") ||
					strings.Contains(responseLower, "风险等级") ||
					strings.Contains(responseLower, "修复建议") ||
					strings.Contains(responseLower, "利用方法")

				// 统计response中换行和字符数，判断是否是长篇总结
				newlineCount := strings.Count(response, "\n")
				isLongSummary := len(response) > 500 && newlineCount > 10

				// 如果AI表示完成，且不是在详细描述漏洞/EXP，且是长篇总结，则结束
				if hasEndPhrase && !isProvidingDetails && isLongSummary {
					shouldEnd = true
				}

				// 另一个结束条件：发现了漏洞且AI返回了漏洞详情
				if hasEndPhrase && isProvidingDetails && isLongSummary && strings.Contains(responseLower, "thinkphp") {
					shouldEnd = true
				}

				if shouldEnd {
					log.Printf("[INFO] AI表示扫描完成，准备生成最终报告")
					break
				}
				continue
			}

			// 处理AI的工具调用
			if len(toolCalls) > 0 {
				// 添加 assistant 消息（包含 tool_calls），DeepSeek API 要求 tool 消息必须引用对应的 tool_calls
				messages = append(messages, mcp.ChatMessage{
					Role:      "assistant",
					Content:   response,
					ToolCalls: toolCalls,
					Time:      time.Now().Format(time.RFC3339),
				})

				// 执行AI调用的工具
				for _, toolCall := range toolCalls {
					sendProgress(t, 0, fmt.Sprintf("执行AI工具调用: %s", toolCall.Name))

					toolResult := executeAITool(toolCall, req.Target, req.Ports, concurrency, timeoutMs, t, allResults)

					// 添加工具执行结果到消息历史
					messages = append(messages, mcp.ChatMessage{
						Role:       "tool",
						ToolCallID: toolCall.ID,
						Content:    toolResult,
						Time:       time.Now().Format(time.RFC3339),
					})
				}

				// 继续循环，让AI分析工具执行结果并决定下一步
				continue
			}

			// 如果没有工具调用也没有响应，可能出错了
			if response == "" {
				break
			}
		}

		// 如果循环结束，生成最终报告
		if iteration >= maxIterations {
			sendProgress(t, 90, "达到最大迭代次数，生成最终报告...")
		}

		// 生成最终总结（只保留最后一次AI分析内容，不生成额外摘要）
		// 如果aiAnalysisSteps有内容，取最后一个AI分析作为最终总结
		var finalSummary string
		if len(aiAnalysisSteps) > 0 {
			// 找到最后一个AI分析内容
			for i := len(aiAnalysisSteps) - 1; i >= 0; i-- {
				if content, ok := aiAnalysisSteps[i]["content"].(string); ok && content != "" {
					finalSummary = content
					break
				}
			}
		}
		if finalSummary == "" {
			finalSummary = "扫描完成，未生成分析报告。"
		}

		// 发送最终分析结果
		finalMsg := SSEMessage{
			Type:     "ai-analysis",
			TaskID:   t.ID,
			Progress: "95/100",
			Percent:  95,
			Data:     map[string]interface{}{"step": "最终总结", "chunk": finalSummary},
		}
		safeSend(t, finalMsg)

		// 如果发现漏洞，生成EXP
		if pocScan, ok := allResults["pocScan"].(map[string]interface{}); ok {
			if results, ok := pocScan["results"].([]map[string]interface{}); ok && len(results) > 0 {
				sendProgress(t, 96, "正在为发现的漏洞生成EXP...")
				expContent := generateEXPForVulnerabilities(allResults, req.Provider, req.APIKey, req.BaseURL, req.Model)
				if expContent != "" {
					aiAnalysisSteps = append(aiAnalysisSteps, map[string]interface{}{
						"step":    "EXP生成",
						"content": expContent,
						"time":    time.Now().Format("2006-01-02 15:04:05"),
					})

					// 发送EXP内容
					expMsg := SSEMessage{
						Type:     "ai-analysis",
						TaskID:   t.ID,
						Progress: "98/100",
						Percent:  98,
						Data:     map[string]interface{}{"step": "EXP生成", "chunk": expContent},
					}
					safeSend(t, expMsg)

					// 保存EXP到结果中
					allResults["expGenerated"] = map[string]interface{}{
						"enabled": true,
						"content": expContent,
						"time":    time.Now().Format("2006-01-02 15:04:05"),
					}
				}

				sendProgress(t, 97, "正在生成并验证Python利用脚本...")
				pyScripts := generateAndVerifyExpScripts(allResults, req.Provider, req.APIKey, req.BaseURL, req.Model, func(msg string) {
					sendProgress(t, 97, msg)
				})
				if len(pyScripts) > 0 {
					allResults["expPythonGenerated"] = map[string]interface{}{
						"enabled": true,
						"scripts": pyScripts,
						"time":    time.Now().Format("2006-01-02 15:04:05"),
					}
					pySummary := fmt.Sprintf("已生成 %d 个 Python 利用脚本（已验证可直接使用，支持 --cmd/--shell），可在报告中下载。", len(pyScripts))
					aiAnalysisSteps = append(aiAnalysisSteps, map[string]interface{}{
						"step":    "Python利用脚本生成",
						"content": pySummary,
						"time":    time.Now().Format("2006-01-02 15:04:05"),
					})
					safeSend(t, SSEMessage{
						Type:     "ai-analysis",
						TaskID:   t.ID,
						Progress: "99/100",
						Percent:  99,
						Data:     map[string]interface{}{"step": "Python利用脚本生成", "chunk": pySummary},
					})
				}
			}
		}

		// AI驱动的工作流已完成，所有扫描结果已保存在allResults中

		// 汇总所有AI分析结果（只保存最终总结，不保存过程数据）
		// 用户只需要AI总结报告，不需要扫描过程数据
		allResults["aiAnalysis"] = map[string]interface{}{
			"enabled":      true,
			"analysisTime": time.Now().Format("2006-01-02 15:04:05"),
			"final":        finalSummary,
			"content":      finalSummary,
		}

		sendProgress(t, 100, "扫描完成，报告已生成")
		allResults["completed"] = true

		// 发送最终结果
		reportMsg := SSEMessage{
			Type:     "report",
			TaskID:   t.ID,
			Progress: "100/100",
			Percent:  100,
			Data:     allResults,
		}
		safeSend(t, reportMsg)
	}()

	// 主循环：转发SSE消息
	for {
		select {
		case <-ctx.Done():
			stopTask(t.ID)
			return
		case msg := <-t.ch:
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		case <-t.stop:
			return
		}
	}
}

// sendProgress 发送进度消息
func sendProgress(t *Task, percent int, message string) {
	msg := SSEMessage{
		Type:     "progress",
		TaskID:   t.ID,
		Progress: fmt.Sprintf("%d/100", percent),
		Percent:  percent,
		Data:     map[string]interface{}{"message": message},
	}
	safeSend(t, msg)
}

/*
// runPortScanInternal 内部端口扫描函数
// runPortScanInternalWithProgress 带进度输出的端口扫描
func runPortScanInternalWithProgress(host, ports string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	portList := parsePorts(ports)
	if len(portList) == 0 {
		return nil
	}
	sendProgress(t, 0, fmt.Sprintf("开始端口扫描，共 %d 个端口（并发：%d）...", len(portList), concurrency))

	var results []map[string]interface{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// 进度跟踪
	scanned := int32(0)
	total := int32(len(portList))

	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	}

	// 启动进度监控goroutine
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				scannedCount := int(atomic.LoadInt32(&scanned))
				if scannedCount >= int(total) {
					return
				}
				sendProgress(t, 0, fmt.Sprintf("端口扫描中: %d/%d (%.1f%%)", scannedCount, total, float64(scannedCount)/float64(total)*100))
			case <-t.stop:
				return
			}
		}
	}()

	for _, p := range portList {
		select {
		case <-t.stop:
			return results
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(port int) {
			defer wg.Done()
			defer func() {
				<-sem
				atomic.AddInt32(&scanned, 1)
			}()

			addr := net.JoinHostPort(host, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				banner := ""
				// 尝试获取banner
				conn2, err2 := net.DialTimeout("tcp", addr, timeout)
				if err2 == nil {
					conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
					buf := make([]byte, 1024)
					n, _ := conn2.Read(buf)
					if n > 0 {
						banner = strings.TrimSpace(string(buf[:n]))
						if len(banner) > 100 {
							banner = banner[:100]
						}
					}
					conn2.Close()
				}
				mu.Lock()
				results = append(results, map[string]interface{}{
					"port":   port,
					"proto":  "tcp",
					"status": "open",
					"banner": banner,
				})
				openPorts := len(results)
				mu.Unlock()

				// 立即发送发现开放端口的消息
				sendProgress(t, 0, fmt.Sprintf("发现开放端口: %d (Banner: %s) [已发现 %d 个开放端口]", port, banner, openPorts))
			}
		}(p)
	}

	wg.Wait()

	if len(results) > 0 {
		portDetails := make([]string, 0, len(results))
		for _, r := range results {
			if port, ok := r["port"].(int); ok {
				banner, _ := r["banner"].(string)
				detail := fmt.Sprintf("端口 %d", port)
				if banner != "" {
					detail += fmt.Sprintf(" (Banner: %s)", banner)
				}
				portDetails = append(portDetails, detail)
			}
		}
		sendProgress(t, 0, fmt.Sprintf("端口扫描完成，发现 %d 个开放端口: %s", len(results), strings.Join(portDetails, ", ")))
	} else {
		sendProgress(t, 0, fmt.Sprintf("端口扫描完成，未发现开放端口"))
	}

	return results
}

// runDirScanInternalWithProgress 带进度输出的目录扫描
func runDirScanInternalWithProgress(baseURL string, dictFiles []string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	var paths []string

	// 如果提供了字典文件路径，从文件读取
	if len(dictFiles) > 0 {
		for _, dictFile := range dictFiles {
			if dictPaths, err := loadDictFile(dictFile); err == nil {
				paths = append(paths, dictPaths...)
			} else {
				log.Printf("[目录扫描] 读取字典文件失败: %s, 错误: %v", dictFile, err)
			}
		}
	}

	// 如果没有成功加载任何字典，使用默认的常见目录列表
	if len(paths) == 0 {
		paths = []string{
			"/", "/admin", "/login", "/api", "/test", "/backup", "/config",
			"/.git", "/.svn", "/.env", "/robots.txt", "/sitemap.xml",
			"/wp-admin", "/wp-content", "/phpinfo.php", "/info.php",
		}
	}

	sendProgress(t, 0, fmt.Sprintf("开始目录扫描，共 %d 个目录...", len(paths)))

	var results []map[string]interface{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	scanned := int32(0)
	total := int32(len(paths))

	client := &http.Client{
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// 启动进度监控
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				scannedCount := int(atomic.LoadInt32(&scanned))
				if scannedCount >= int(total) {
					return
				}
				sendProgress(t, 0, fmt.Sprintf("目录扫描中: %d/%d (%.1f%%)", scannedCount, total, float64(scannedCount)/float64(total)*100))
			case <-t.stop:
				return
			}
		}
	}()

	for _, path := range paths {
		select {
		case <-t.stop:
			return results
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			defer func() {
				<-sem
				atomic.AddInt32(&scanned, 1)
			}()

			url := strings.TrimRight(baseURL, "/") + p
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return
			}

			resp, err := client.Do(req)
			if err == nil && resp != nil {
				status := resp.StatusCode
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				if status == 200 || status == 403 || status == 301 || status == 302 {
					mu.Lock()
					results = append(results, map[string]interface{}{
						"path":   p,
						"url":    url,
						"status": status,
						"length": len(body),
					})
					dirCount := len(results)
					mu.Unlock()

					// 立即发送发现目录的消息
					sendProgress(t, 0, fmt.Sprintf("发现目录: %s (状态码: %d) [已发现 %d 个目录]", url, status, dirCount))
				}
			}
		}(path)
	}

	wg.Wait()

	if len(results) > 0 {
		sendProgress(t, 0, fmt.Sprintf("目录扫描完成，发现 %d 个目录", len(results)))
	} else {
		sendProgress(t, 0, "目录扫描完成，未发现目录")
	}

	return results
}

*/
/*
// runPocScanInternal 内部POC扫描函数（完整实现，与pocScanHandler相同）
func runPocScanInternal(baseURL, pocDir string, pocPaths []string, keywords []string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	var results []map[string]interface{}
	var mu sync.Mutex

	// 加载POC文件
	var pocs []POC
	var xrps []XRPOC
	var nucs []NucleiPOC
	var err error

	if len(pocPaths) > 0 {
		// 从指定文件加载
		pocs, xrps, nucs, err = loadAllPOCsFromFiles(pocPaths)
	} else if pocDir != "" {
		// 从目录加载
		pocs, xrps, nucs, err = loadAllPOCs(pocDir)
	} else {
		// 使用内置POC目录
		builtinDir := getBuiltinPocDir()
		if builtinDir != "" {
			pocs, xrps, nucs, err = loadAllPOCs(builtinDir)
		}
	}

	if err != nil {
		log.Printf("[POC扫描] 加载POC失败: %v", err)
		return results
	}

	// 根据关键词过滤POC
	if len(keywords) > 0 {
		var filteredPocs []POC
		var filteredXrps []XRPOC
		var filteredNucs []NucleiPOC

		// 转换为小写以便匹配
		lowerKeywords := make([]string, len(keywords))
		for i, kw := range keywords {
			lowerKeywords[i] = strings.ToLower(kw)
		}

		// 过滤传统POC
		for _, pp := range pocs {
			match := false
			lowerName := strings.ToLower(pp.Name)
			for _, kw := range lowerKeywords {
				if strings.Contains(lowerName, kw) {
					match = true
					break
				}
			}
			if match {
				filteredPocs = append(filteredPocs, pp)
			}
		}

		// 过滤X-Ray POC
		for _, xp := range xrps {
			match := false
			lowerID := strings.ToLower(xp.ID)
			lowerName := strings.ToLower(xp.Info.Name)
			searchText := lowerID + " " + lowerName
			for _, kw := range lowerKeywords {
				if strings.Contains(searchText, kw) {
					match = true
					break
				}
			}
			if match {
				filteredXrps = append(filteredXrps, xp)
			}
		}

		// 过滤Nuclei POC（检查ID、Name、Tags）
		for _, np := range nucs {
			match := false
			lowerID := strings.ToLower(np.ID)
			lowerName := strings.ToLower(np.Info.Name)
			tagsStr := ""
			for _, tag := range np.Info.Tags {
				tagsStr += strings.ToLower(tag) + " "
			}
			searchText := lowerID + " " + lowerName + " " + tagsStr
			for _, kw := range lowerKeywords {
				if strings.Contains(searchText, kw) {
					match = true
					break
				}
			}
			if match {
				filteredNucs = append(filteredNucs, np)
			}
		}

		pocs = filteredPocs
		xrps = filteredXrps
		nucs = filteredNucs
	}

	total := len(pocs) + len(xrps) + len(nucs)
	if total == 0 {
		return results
	}

	// 更新任务总数
	t.m.Lock()
	t.Total = total
	t.Done = 0
	t.m.Unlock()

	cc := concurrency
	if cc <= 0 {
		cc = 50
	}
	timeout := time.Duration(timeoutMs)
	if timeout <= 0 {
		timeout = 3000 * time.Millisecond
	} else {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, cc)

	// 处理传统POC
	for _, pp := range pocs {
		select {
		case <-t.stop:
			return results
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		pp := pp
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			url := strings.TrimRight(baseURL, "/") + pp.Path
			var reqBody io.Reader
			if pp.Body != "" {
				reqBody = strings.NewReader(pp.Body)
			}
			method := strings.ToUpper(strings.TrimSpace(pp.Method))
			if method == "" {
				method = "GET"
			}
			req, _ := http.NewRequest(method, url, reqBody)
			for k, v := range pp.Headers {
				req.Header.Set(k, v)
			}
			attempts := pp.Retry + 1
			if attempts <= 0 {
				attempts = 1
			}
			matched := false
			status := 0
			for i := 0; i < attempts; i++ {
				var reqBody io.Reader
				if pp.Body != "" {
					reqBody = strings.NewReader(pp.Body)
				}
				req, e := http.NewRequest(method, url, reqBody)
				if e != nil {
					if pp.RetryDelayMs > 0 {
						time.Sleep(time.Duration(pp.RetryDelayMs) * time.Millisecond)
					}
					continue
				}
				for k, v := range pp.Headers {
					req.Header.Set(k, v)
				}
				resp, err := client.Do(req)
				if err == nil {
					status = resp.StatusCode
					b, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					lb := strings.ToLower(string(b))
					if pp.Match != "" && (strings.Contains(lb, strings.ToLower(pp.Match)) || strings.Contains(strings.ToLower(strings.Join(resp.Header.Values("Server"), ";")), strings.ToLower(pp.Match))) {
						matched = true
					}
					if len(pp.MatchHeaders) > 0 {
						okh := true
						for hk, hv := range pp.MatchHeaders {
							vals := strings.ToLower(strings.Join(resp.Header.Values(hk), ";"))
							if !strings.Contains(vals, strings.ToLower(hv)) {
								okh = false
								break
							}
						}
						matched = matched || okh
					}
					if len(pp.MatchBodyAny) > 0 {
						for _, sub := range pp.MatchBodyAny {
							if strings.Contains(lb, strings.ToLower(sub)) {
								matched = true
								break
							}
						}
					}
					if len(pp.MatchBodyAll) > 0 {
						all := true
						for _, sub := range pp.MatchBodyAll {
							if !strings.Contains(lb, strings.ToLower(sub)) {
								all = false
								break
							}
						}
						matched = matched || all
					}
				}
				if matched {
					break
				}
				if pp.RetryDelayMs > 0 {
					time.Sleep(time.Duration(pp.RetryDelayMs) * time.Millisecond)
				}
			}
			d, tot := t.IncDone()
			if matched {
				mu.Lock()
				info := map[string]interface{}{"name": pp.Name}
				if pp.Name == "ThinkPHP 5.0.23 Remote Code Execution" {
					info["severity"] = "critical"
				}
				results = append(results, map[string]interface{}{
					"poc":    pp.Name,
					"url":    url,
					"status": status,
					"info":   info,
					"req": map[string]interface{}{
						"method":  method,
						"path":    pp.Path,
						"headers": pp.Headers,
						"body":    pp.Body,
					},
				})
				mu.Unlock()
			}
			// 发送进度消息（可选，避免过多消息）
			if d%10 == 0 || matched {
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				safeSend(t, msg)
			}
		}()
	}

	// 处理X-Ray POC
	for _, xp := range xrps {
		select {
		case <-t.stop:
			return results
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		xp := xp
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ruleResults := make(map[string]bool)
			var hitURL string
			var hitStatus int
			var hitMethod string
			var hitPath string
			var hitBody string
			var hitHeaders map[string]string
			for rname, rule := range xp.Rules {
				url := strings.TrimRight(baseURL, "/") + rule.Request.Path
				method := strings.ToUpper(strings.TrimSpace(rule.Request.Method))
				if method == "" {
					method = "GET"
				}
				var reqBody io.Reader
				bodyStr := substVarsSimple(rule.Request.Body, baseURL)
				if bodyStr != "" {
					reqBody = strings.NewReader(bodyStr)
				}
				req0, e := http.NewRequest(method, url, reqBody)
				if e != nil {
					ruleResults[rname] = false
					continue
				}
				for k, v := range rule.Request.Headers {
					req0.Header.Set(k, substVarsSimple(v, baseURL))
				}
				resp, err := client.Do(req0)
				status := 0
				var body []byte
				var hdr http.Header
				if err == nil && resp != nil {
					status = resp.StatusCode
					body, _ = io.ReadAll(resp.Body)
					resp.Body.Close()
					hdr = resp.Header
				}
				if err != nil || resp == nil {
					ruleResults[rname] = false
					continue
				}
				ok := evalRuleExpression(rule.Expression, status, body, hdr)
				ruleResults[rname] = ok
				if ok && hitURL == "" {
					hitURL = url
					hitStatus = status
					hitMethod = method
					hitPath = rule.Request.Path
					hitBody = bodyStr
					hitHeaders = make(map[string]string, len(rule.Request.Headers))
					for k, v := range rule.Request.Headers {
						hitHeaders[k] = substVarsSimple(v, baseURL)
					}
				}
			}
			vuln := evalGlobalExpression(xp.Expression, ruleResults)
			d, tot := t.IncDone()
			if vuln {
				pname := xp.Info.Name
				if pname == "" {
					pname = xp.ID
				}
				mu.Lock()
				results = append(results, map[string]interface{}{
					"poc":    pname,
					"url":    hitURL,
					"status": hitStatus,
					"info":   xp.Info,
					"req": map[string]interface{}{
						"method":  hitMethod,
						"path":    hitPath,
						"headers": hitHeaders,
						"body":    hitBody,
					},
				})
				mu.Unlock()
			}
			if d%10 == 0 || vuln {
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				safeSend(t, msg)
			}
		}()
	}

	// 处理Nuclei POC
	for _, np := range nucs {
		select {
		case <-t.stop:
			return results
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		np := np
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ok, hitURL, hitStatus, hitReq := runNucleiOnce(baseURL, np, client)
			d, tot := t.IncDone()
			if ok {
				pname := np.Info.Name
				if pname == "" {
					pname = np.ID
				}
				var statusVal interface{}
				if hitStatus == 0 {
					statusVal = "suspect"
				} else {
					statusVal = hitStatus
				}
				mu.Lock()
				results = append(results, map[string]interface{}{
					"poc":    pname,
					"url":    hitURL,
					"status": statusVal,
					"info":   np.Info,
					"req":    hitReq,
				})
				mu.Unlock()
			}
			if d%10 == 0 || ok {
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				safeSend(t, msg)
			}
		}()
	}

	wg.Wait()
	return results
}

*/
// executeAITool 执行AI调用的工具
func executeAITool(toolCall mcp.AIToolCall, defaultTarget, defaultPorts string, concurrency, timeoutMs int, t *Task, allResults map[string]interface{}) string {

	switch toolCall.Name {
	case "port_scan":
		target := defaultTarget
		ports := defaultPorts
		if t, ok := toolCall.Arguments["target"].(string); ok && t != "" {
			target = t
		}
		// 强制使用用户指定的端口范围，忽略AI自行决定的端口参数
		// 如果AI调用时传入了ports参数但与用户指定的不符，仍然使用用户指定的端口
		if p, ok := toolCall.Arguments["ports"].(string); ok && p != "" {
			// 检查AI传入的端口是否与用户指定的一致，如果不一致则忽略AI的决策
			userPortsNormalized := normalizePortsString(defaultPorts)
			aiPortsNormalized := normalizePortsString(p)
			if userPortsNormalized == aiPortsNormalized {
				ports = p
			} else {
				log.Printf("[自动扫描] AI自行决定使用端口 %s，已强制替换为用户指定的端口范围: %s", p, defaultPorts)
			}
		}
		sendProgress(t, 0, fmt.Sprintf("AI调用端口扫描: 目标=%s, 端口=%s", target, ports))

		// 执行端口扫描，并实时输出进度
		results := runPortScanInternalWithProgress(target, ports, concurrency, timeoutMs, t)
		portScanData := map[string]interface{}{
			"enabled": len(results) > 0,
			"target":  target,
			"results": results,
		}
		allResults["portScan"] = portScanData

		if len(results) > 0 {
			details := make([]string, 0, len(results))
			for _, r := range results {
				if port, ok := r["port"].(int); ok {
					banner, _ := r["banner"].(string)
					detail := fmt.Sprintf("端口 %d: 开放", port)
					if banner != "" {
						detail += fmt.Sprintf(" (Banner: %s)", banner)
					}
					details = append(details, detail)
				}
			}
			return fmt.Sprintf("端口扫描完成，发现 %d 个开放端口：\n%s", len(results), strings.Join(details, "\n"))
		}
		return "端口扫描完成，未发现开放端口"

	case "web_probe":
		urls := []string{}
		if u, ok := toolCall.Arguments["urls"].([]interface{}); ok {
			for _, url := range u {
				if urlStr, ok := url.(string); ok {
					urls = append(urls, urlStr)
				}
			}
		}
		if len(urls) == 0 {
			return "错误：未提供URL列表"
		}
		sendProgress(t, 0, fmt.Sprintf("AI调用Web探针: %d 个URL", len(urls)))
		results := runWebProbeInternalWithProgress(urls, concurrency, timeoutMs, true, true, t)
		probeData := map[string]interface{}{
			"enabled": len(results) > 0,
			"results": results,
		}
		allResults["webProbe"] = probeData

		if len(results) > 0 {
			details := make([]string, 0, len(results))
			for _, r := range results {
				if url, ok := r["url"].(string); ok {
					title, _ := r["title"].(string)
					status, _ := r["status"].(int)
					tech, _ := r["tech"].([]interface{})
					detail := fmt.Sprintf("%s (状态码: %d", url, status)
					if title != "" {
						detail += fmt.Sprintf(", 标题: %s", title)
					}
					if len(tech) > 0 {
						techStrs := make([]string, 0, len(tech))
						for _, t := range tech {
							if ts, ok := t.(string); ok {
								techStrs = append(techStrs, ts)
							}
						}
						if len(techStrs) > 0 {
							detail += fmt.Sprintf(", 技术栈: %s", strings.Join(techStrs, ", "))
						}
					}
					detail += ")"
					details = append(details, detail)
				}
			}
			return fmt.Sprintf("Web探针完成，发现 %d 个Web服务：\n%s", len(results), strings.Join(details, "\n"))
		}
		return "Web探针完成，未发现Web服务"

	case "dir_scan":
		url := ""
		if u, ok := toolCall.Arguments["url"].(string); ok {
			url = u
		}
		if url == "" {
			return "错误：未提供URL"
		}
		// 获取技术栈信息
		var tech []interface{}
		if techArg, ok := toolCall.Arguments["tech"].([]interface{}); ok {
			tech = techArg
		} else {
			// 如果没有提供技术栈，尝试从web探针结果中查找
			if webProbe, ok := allResults["webProbe"].(map[string]interface{}); ok {
				if probeResults, ok := webProbe["results"].([]map[string]interface{}); ok {
					for _, result := range probeResults {
						if resultURL, ok := result["url"].(string); ok && resultURL == url {
							if techResult, ok := result["tech"].([]interface{}); ok {
								tech = techResult
								break
							}
						}
					}
				}
			}
		}

		// 根据技术栈选择字典文件
		dictFiles := selectDictByTech(tech)
		dictInfo := "内置常见目录列表"
		if len(dictFiles) > 0 {
			dictNames := make([]string, 0, len(dictFiles))
			for _, df := range dictFiles {
				dictNames = append(dictNames, filepath.Base(df))
			}
			dictInfo = strings.Join(dictNames, ", ")
		} else {
			// 如果selectDictByTech返回空，说明没有找到匹配的字典文件，将使用内置的常见目录列表
			// 显示更详细的信息
			dictDir := getBuiltinDictDir()
			if dictDir != "" {
				if _, err := os.Stat(dictDir); err == nil {
					dictInfo = fmt.Sprintf("内置常见目录列表（字典目录未找到匹配文件: %s）", dictDir)
				} else {
					dictInfo = fmt.Sprintf("内置常见目录列表（字典目录不存在: %s）", dictDir)
				}
			} else {
				dictInfo = "内置常见目录列表（未配置字典目录）"
			}
		}

		sendProgress(t, 0, fmt.Sprintf("AI调用目录扫描: %s (使用字典: %s)", url, dictInfo))
		results := runDirScanInternalWithProgress(url, dictFiles, concurrency, timeoutMs, t)
		dirScanData := map[string]interface{}{
			"enabled": len(results) > 0,
			"target":  url,
			"results": results,
		}
		allResults["dirScan"] = dirScanData

		if len(results) > 0 {
			details := make([]string, 0, len(results))
			for i, r := range results {
				if i >= 10 {
					details = append(details, fmt.Sprintf("... (还有 %d 个目录)", len(results)-10))
					break
				}
				if u, ok := r["url"].(string); ok {
					status, _ := r["status"].(int)
					length, _ := r["length"].(float64)
					details = append(details, fmt.Sprintf("%s (状态码: %d, 长度: %.0f)", u, status, length))
				}
			}
			return fmt.Sprintf("目录扫描完成，发现 %d 个目录：\n%s", len(results), strings.Join(details, "\n"))
		}
		return "目录扫描完成，未发现目录"

	case "poc_scan":
		url := ""
		if u, ok := toolCall.Arguments["url"].(string); ok {
			url = u
		}
		if url == "" {
			return "错误：未提供URL"
		}
		keywords := []string{}
		if k, ok := toolCall.Arguments["keywords"].([]interface{}); ok {
			for _, kw := range k {
				if kwStr, ok := kw.(string); ok {
					keywords = append(keywords, kwStr)
				}
			}
		}
		keywordStr := ""
		if len(keywords) > 0 {
			keywordStr = fmt.Sprintf("，关键词: %s", strings.Join(keywords, ", "))
		}
		sendProgress(t, 0, fmt.Sprintf("AI调用POC扫描: %s%s", url, keywordStr))
		results := runPocScanInternal(url, "", nil, keywords, concurrency, timeoutMs, t)
		pocScanData := map[string]interface{}{
			"enabled": len(results) > 0,
			"target":  url,
			"results": results,
		}
		allResults["pocScan"] = pocScanData

		if len(results) > 0 {
			details := make([]string, 0, len(results))
			for i, r := range results {
				if i >= 20 {
					details = append(details, fmt.Sprintf("... (还有 %d 个漏洞)", len(results)-20))
					break
				}
				if pocName, ok := r["poc"].(string); ok {
					u, _ := r["url"].(string)
					status, _ := r["status"]
					info, _ := r["info"].(map[string]interface{})
					severity := ""
					if sev, ok := info["severity"].(string); ok {
						severity = sev
					}
					detail := fmt.Sprintf("漏洞: %s", pocName)
					if u != "" {
						detail += fmt.Sprintf(" | URL: %s", u)
					}
					if status != nil {
						detail += fmt.Sprintf(" | 状态码: %v", status)
					}
					if severity != "" {
						detail += fmt.Sprintf(" | 严重程度: %s", severity)
					}
					details = append(details, detail)
				}
			}
			return fmt.Sprintf("POC扫描完成，发现 %d 个潜在漏洞：\n%s", len(results), strings.Join(details, "\n"))
		}
		return "POC扫描完成，未发现漏洞"

	default:
		return fmt.Sprintf("错误：未知的工具 %s", toolCall.Name)
	}
}

// generateEXPForVulnerabilities 为发现的漏洞生成EXP和使用说明
func generateEXPForVulnerabilities(allResults map[string]interface{}, provider, apiKey, baseURL, model string) string {
	// 检查是否有POC扫描结果
	pocScan, ok := allResults["pocScan"].(map[string]interface{})
	if !ok {
		return ""
	}

	results, ok := pocScan["results"].([]map[string]interface{})
	if !ok || len(results) == 0 {
		return ""
	}

	// 收集漏洞信息
	var vulnInfo strings.Builder
	vulnInfo.WriteString("发现的漏洞列表：\n\n")
	for i, result := range results {
		pocName := ""
		url := ""
		status := ""

		if name, ok := result["poc"].(string); ok {
			pocName = name
		}
		if u, ok := result["url"].(string); ok {
			url = u
		}
		if s, ok := result["status"].(int); ok {
			status = fmt.Sprintf("%d", s)
		} else if s, ok := result["status"].(string); ok {
			status = s
		}

		info := result["info"]
		severity := ""
		description := ""
		if infoMap, ok := info.(map[string]interface{}); ok {
			if sev, ok := infoMap["severity"].(string); ok {
				severity = sev
			}
			if desc, ok := infoMap["description"].(string); ok {
				description = desc
			}
		}

		vulnInfo.WriteString(fmt.Sprintf("漏洞 %d:\n", i+1))
		vulnInfo.WriteString(fmt.Sprintf("- POC名称: %s\n", pocName))
		vulnInfo.WriteString(fmt.Sprintf("- 目标URL: %s\n", url))
		vulnInfo.WriteString(fmt.Sprintf("- 状态码: %s\n", status))
		if severity != "" {
			vulnInfo.WriteString(fmt.Sprintf("- 严重程度: %s\n", severity))
		}
		if description != "" {
			vulnInfo.WriteString(fmt.Sprintf("- 描述: %s\n", description))
		}
		vulnInfo.WriteString("\n")
	}

	// 调用AI生成EXP（使用带超时的context，延长超时时间到300秒）
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var aiProvider mcp.AIProvider
	switch provider {
	case "deepseek":
		p := mcp.NewDeepSeekProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		// 为EXP生成设置更长的超时时间（300秒）
		p.SetTimeout(300 * time.Second)
		aiProvider = p
	case "openai":
		p := mcp.NewOpenAIProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		p.SetTimeout(300 * time.Second)
		aiProvider = p
	case "anthropic":
		p := mcp.NewAnthropicProvider(apiKey)
		if baseURL != "" {
			p.BaseURL = baseURL
		}
		if model != "" {
			p.Model = model
		}
		p.SetTimeout(300 * time.Second)
		aiProvider = p
	case "ollama":
		p := mcp.NewOllamaProvider(baseURL, model)
		p.SetTimeout(300 * time.Second)
		aiProvider = p
	default:
		return ""
	}

	// 限制漏洞数量，避免提示过长导致超时（最多处理前10个漏洞）
	maxVulns := 10
	if len(results) > maxVulns {
		log.Printf("[EXP生成] 发现 %d 个漏洞，仅处理前 %d 个以避免超时", len(results), maxVulns)
		results = results[:maxVulns]
		// 重新构建vulnInfo
		vulnInfo.Reset()
		vulnInfo.WriteString("发现的漏洞列表：\n\n")
		for i, result := range results {
			pocName := ""
			url := ""
			status := ""

			if name, ok := result["poc"].(string); ok {
				pocName = name
			}
			if u, ok := result["url"].(string); ok {
				url = u
			}
			if s, ok := result["status"].(int); ok {
				status = fmt.Sprintf("%d", s)
			} else if s, ok := result["status"].(string); ok {
				status = s
			}

			info := result["info"]
			severity := ""
			description := ""
			if infoMap, ok := info.(map[string]interface{}); ok {
				if sev, ok := infoMap["severity"].(string); ok {
					severity = sev
				}
				if desc, ok := infoMap["description"].(string); ok {
					description = desc
				}
			}

			vulnInfo.WriteString(fmt.Sprintf("漏洞 %d:\n", i+1))
			vulnInfo.WriteString(fmt.Sprintf("- POC名称: %s\n", pocName))
			vulnInfo.WriteString(fmt.Sprintf("- 目标URL: %s\n", url))
			vulnInfo.WriteString(fmt.Sprintf("- 状态码: %s\n", status))
			if severity != "" {
				vulnInfo.WriteString(fmt.Sprintf("- 严重程度: %s\n", severity))
			}
			if description != "" {
				vulnInfo.WriteString(fmt.Sprintf("- 描述: %s\n", description))
			}
			vulnInfo.WriteString("\n")
		}
	}

	prompt := fmt.Sprintf(`根据以下漏洞信息，为每个漏洞生成可用的EXP（Exploit）代码和使用说明。

要求：
1. 为每个漏洞生成完整的EXP代码（Python、Bash、curl命令等，根据漏洞类型选择最合适的格式）
2. EXP必须可以直接使用，包含所有必要的参数
3. 详细说明EXP的使用步骤和预期结果
4. 如果漏洞需要特定条件（如认证、特定版本等），请在说明中明确指出
5. 使用中文输出，代码部分使用代码块格式
6. 请保持简洁，每个漏洞的EXP代码不超过100行

漏洞信息：
%s

请为每个漏洞生成EXP和使用说明。`, vulnInfo.String())

	messages := []mcp.ChatMessage{
		{
			Role:    "system",
			Content: "你是一位专业的安全专家，擅长编写漏洞利用代码。请根据漏洞信息生成可直接使用的EXP代码和详细的使用说明。请保持简洁，避免冗长的描述。",
			Time:    time.Now().Format(time.RFC3339),
		},
		{
			Role:    "user",
			Content: prompt,
			Time:    time.Now().Format(time.RFC3339),
		},
	}

	// 使用context监控超时
	done := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		response, _, err := aiProvider.Chat(messages, nil)
		if err != nil {
			errChan <- err
			return
		}
		done <- response
	}()

	select {
	case response := <-done:
		return response
	case err := <-errChan:
		log.Printf("[EXP生成] AI调用失败: %v", err)
		return ""
	case <-ctx.Done():
		log.Printf("[EXP生成] AI调用超时（300秒）")
		return ""
	}
}

func generatePythonScriptsForVulnerabilities(allResults map[string]interface{}) []map[string]interface{} {
	pocScan, ok := allResults["pocScan"].(map[string]interface{})
	if !ok {
		return nil
	}
	results, ok := pocScan["results"].([]map[string]interface{})
	if !ok || len(results) == 0 {
		return nil
	}

	targetBaseURL := normalizeTargetBaseURL(allResults, pocScan)
	scripts := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		specBase, spec, ok := buildExpSpecFromPOCResult(r, targetBaseURL)
		if !ok {
			continue
		}

		py := generatePythonFromExpSpec(specBase, spec)
		keyInfo := buildExpKeyInfo(specBase, spec)
		scripts = append(scripts, map[string]interface{}{
			"name":    spec.Name,
			"keyInfo": keyInfo,
			"python":  py,
		})
	}
	return scripts
}

// runAIAnalysisInternal 内部AI分析函数（流式输出版本，保留用于兼容性）

// main 程序入口函数
// 初始化HTTP服务器，注册所有路由处理器，启动清理守护进程
func main() {
	mux := http.NewServeMux()
	startTaskJanitor()

	// 启动 JADX 会话清理器（每 1 小时清理一次）
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			// 清理 24 小时未活动的会话
			mcp.CleanupOldJADXSessions(24 * time.Hour)
			log.Printf("[清理] JADX 会话清理完成")
		}
	}()

	mux.HandleFunc("/events", sseHandler)
	mux.HandleFunc("/task/stop", stopTaskHandler)
	mux.HandleFunc("/scan/ports", portScanHandler)
	mux.HandleFunc("/scan/dirs", dirScanHandler)
	mux.HandleFunc("/api/dicts", getBuiltinDictsHandler)
	mux.HandleFunc("/scan/poc", pocScanHandler)
	mux.HandleFunc("/scan/webprobe", webProbeHandler)
	mux.HandleFunc("/scan/exp", expExecHandler)
	mux.HandleFunc("/scan/waf", wafBypassHandler)
	// AI Analysis route
	mux.HandleFunc("/ai/analyze", aiAnalyzeHandler)
	mux.HandleFunc("/ai/exp/python", aiGenPythonFromExpHandler)
	mux.HandleFunc("/ai/exp/python/batch", aiGenPythonFromExpBatchHandler)
	// AI Auto Scan route (自动化扫描工作流)
	mux.HandleFunc("/ai/auto-scan", aiAutoScanHandler)
	// Shouji integrated routes (Phase B: inline)
	shouji.RegisterRoutes(mux)
	// JADX MCP routes
	mux.HandleFunc("/mcp/jadx/connect", mcp.ConnectJADXHandler)
	mux.HandleFunc("/mcp/jadx/configure", mcp.ConfigureJADXSessionHandler)
	mux.HandleFunc("/mcp/jadx/chat/stream", mcp.JADXChatStreamHandler)
	mux.HandleFunc("/mcp/jadx/messages", mcp.GetJADXMessagesHandler)
	mux.HandleFunc("/mcp/jadx/status", mcp.GetJADXSessionStatusHandler)
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/", serveStatic)
	port := flag.String("port", "8080", "server port")
	flag.Parse()
	addr := ":" + *port

	// 注册清理函数，程序退出时停止 MCP 服务器
	defer func() {
		mcp.StopJADXServer()
		log.Printf("[清理] MCP 服务器已停止")
	}()

	log.Printf("Server listening at %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
