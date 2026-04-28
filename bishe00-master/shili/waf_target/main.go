// Package main WAF绕过测试靶场
// 这是一个模拟WAF的测试靶场，用于测试WAF绕过功能
// 靶场会检测常见的攻击模式（SQL注入、XSS等），并可以被各种绕过策略绕过
package main

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

// WAFRule WAF检测规则
type WAFRule struct {
	Name        string
	Pattern     *regexp.Regexp
	Description string
}

// RequestLog 请求日志
type RequestLog struct {
	Time      string `json:"time"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	Payload   string `json:"payload"`
	Blocked   bool   `json:"blocked"`
	Rule      string `json:"rule,omitempty"`
	Bypassed  bool   `json:"bypassed"`
	UserAgent string `json:"userAgent"`
}

var (
	// WAF规则列表
	wafRules = []WAFRule{
		{
			Name:        "SQL注入检测 - OR 1=1",
			Pattern:     regexp.MustCompile(`\b(?:OR|or)\b\s*\d+\s*=\s*\d+`),
			Description: "检测 OR 1=1 类型的SQL注入（拦截常见大小写，保留少量混合大小写绕过空间）",
		},
		{
			Name:        "SQL注入检测 - UNION SELECT",
			Pattern:     regexp.MustCompile(`(?:UNION|union)\s+(?:SELECT|select)`),
			Description: "检测 UNION SELECT 类型的SQL注入（拦截常见大小写，保留少量混合大小写绕过空间）",
		},
		{
			Name:        "SQL注入检测 - 单引号",
			Pattern:     regexp.MustCompile(`'`),
			Description: "检测单引号（演示用途，真实WAF会更复杂）",
		},
		{
			Name:        "SQL注入检测 - 注释符",
			Pattern:     regexp.MustCompile(`(--|#|/\*|\*/)`),
			Description: "检测SQL注释符（--/#/注释块）",
		},
		{
			Name:        "XSS检测 - script标签",
			Pattern:     regexp.MustCompile(`<(?:script|SCRIPT)[^>]*>`),
			Description: "检测<script>标签（拦截常见大小写，保留少量混合大小写绕过空间）",
		},
		{
			Name:        "XSS检测 - alert",
			Pattern:     regexp.MustCompile(`(?:alert|ALERT)\s*\(`),
			Description: "检测alert(（拦截常见大小写，保留少量混合大小写绕过空间）",
		},
		{
			Name:        "XSS检测 - javascript协议",
			Pattern:     regexp.MustCompile(`(?:javascript|JAVASCRIPT):`),
			Description: "检测javascript:协议（拦截常见大小写，保留少量混合大小写绕过空间）",
		},
		{
			Name:        "路径遍历检测",
			Pattern:     regexp.MustCompile(`\.\./|\.\.\\`),
			Description: "检测路径遍历攻击",
		},
		{
			Name:        "命令注入检测",
			Pattern:     regexp.MustCompile(`(\||;|&|\$\(|` + "`" + `)`),
			Description: "检测命令注入符号",
		},
	}

	// 请求日志（线程安全）
	logs      []RequestLog
	logsMutex sync.RWMutex
	maxLogs   = 1000 // 最多保存1000条日志
)

// checkWAF 检查请求是否触发WAF规则
// 返回 (是否被拦截, 触发的规则名称)
func checkWAF(input string) (bool, string) {
	// 检查所有规则
	for _, rule := range wafRules {
		if rule.Pattern.MatchString(input) {
			return true, rule.Name
		}
	}

	return false, ""
}

func normalizePayload(s string) string {
	u, err := url.QueryUnescape(s)
	if err == nil {
		return u
	}
	return s
}

// addLog 添加请求日志
func addLog(log RequestLog) {
	logsMutex.Lock()
	defer logsMutex.Unlock()

	logs = append(logs, log)
	// 限制日志数量
	if len(logs) > maxLogs {
		logs = logs[len(logs)-maxLogs:]
	}
}

// getLogs 获取请求日志
func getLogs() []RequestLog {
	logsMutex.RLock()
	defer logsMutex.RUnlock()

	result := make([]RequestLog, len(logs))
	copy(result, logs)
	return result
}

// searchHandler 搜索接口 - 用于测试SQL注入等
func searchHandler(w http.ResponseWriter, r *http.Request) {
	var payload string

	if r.Method == "GET" {
		payload = r.URL.RawQuery
	} else if r.Method == "POST" {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		payload = string(bodyBytes)
	}

	payload = normalizePayload(payload)
	blocked, ruleName := checkWAF(payload)

	log := RequestLog{
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Method:    r.Method,
		URL:       r.URL.String(),
		Payload:   payload,
		Blocked:   blocked,
		Rule:      ruleName,
		Bypassed:  !blocked,
		UserAgent: r.UserAgent(),
	}
	addLog(log)

	if blocked {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "blocked",
			"message": "请求被WAF拦截",
			"rule":    ruleName,
		})
		return
	}

	// 如果没有被拦截，返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "请求成功",
		"data":    "搜索结果：找到10条记录",
		"payload": payload,
	})
}

// apiHandler API接口 - 另一个测试端点
func apiHandler(w http.ResponseWriter, r *http.Request) {
	var payload string

	if r.Method == "GET" {
		payload = r.URL.RawQuery
	} else if r.Method == "POST" {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		payload = string(bodyBytes)
	}

	payload = normalizePayload(payload)
	blocked, ruleName := checkWAF(payload)

	log := RequestLog{
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Method:    r.Method,
		URL:       r.URL.String(),
		Payload:   payload,
		Blocked:   blocked,
		Rule:      ruleName,
		Bypassed:  !blocked,
		UserAgent: r.UserAgent(),
	}
	addLog(log)

	if blocked {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "WAF Blocked: %s", ruleName)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "API Response: OK\nPayload: %s", payload)
}

// queryHandler 查询接口 - 第三个测试端点
func queryHandler(w http.ResponseWriter, r *http.Request) {
	var payload string

	if r.Method == "GET" {
		payload = r.URL.RawQuery
	} else if r.Method == "POST" {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		payload = string(bodyBytes)
	}

	payload = normalizePayload(payload)
	blocked, ruleName := checkWAF(payload)

	log := RequestLog{
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Method:    r.Method,
		URL:       r.URL.String(),
		Payload:   payload,
		Blocked:   blocked,
		Rule:      ruleName,
		Bypassed:  !blocked,
		UserAgent: r.UserAgent(),
	}
	addLog(log)

	if blocked {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"query":  payload,
	})
}

// logsHandler 获取日志接口
func logsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(getLogs())
}

// clearLogsHandler 清除日志接口
func clearLogsHandler(w http.ResponseWriter, r *http.Request) {
	logsMutex.Lock()
	defer logsMutex.Unlock()
	logs = []RequestLog{}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "日志已清除")
}

// indexHandler 首页
func indexHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WAF绕过测试靶场</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        h1 {
            color: #333;
        }
        h2 {
            color: #666;
            border-bottom: 2px solid #4CAF50;
            padding-bottom: 10px;
        }
        .endpoint {
            background: #f9f9f9;
            padding: 15px;
            margin: 10px 0;
            border-left: 4px solid #4CAF50;
            border-radius: 4px;
        }
        .endpoint code {
            background: #e8e8e8;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: monospace;
        }
        .rule {
            background: #fff3cd;
            padding: 10px;
            margin: 5px 0;
            border-left: 4px solid #ffc107;
            border-radius: 4px;
        }
        .rule-name {
            font-weight: bold;
            color: #856404;
        }
        .stats {
            display: flex;
            gap: 20px;
            margin: 20px 0;
        }
        .stat-box {
            flex: 1;
            background: #e3f2fd;
            padding: 15px;
            border-radius: 4px;
            text-align: center;
        }
        .stat-box h3 {
            margin: 0;
            color: #1976d2;
        }
        .stat-box .number {
            font-size: 32px;
            font-weight: bold;
            color: #0d47a1;
        }
        button {
            background: #4CAF50;
            color: white;
            border: none;
            padding: 10px 20px;
            border-radius: 4px;
            cursor: pointer;
            margin: 5px;
        }
        button:hover {
            background: #45a049;
        }
        .log-entry {
            padding: 10px;
            margin: 5px 0;
            border-radius: 4px;
            font-family: monospace;
            font-size: 12px;
        }
        .log-blocked {
            background: #ffebee;
            border-left: 4px solid #f44336;
        }
        .log-bypassed {
            background: #e8f5e9;
            border-left: 4px solid #4CAF50;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 10px;
        }
        th, td {
            padding: 8px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background: #4CAF50;
            color: white;
        }
        .badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 3px;
            font-size: 11px;
            font-weight: bold;
        }
        .badge-blocked {
            background: #f44336;
            color: white;
        }
        .badge-bypassed {
            background: #4CAF50;
            color: white;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ WAF绕过测试靶场</h1>
        <p>这是一个用于测试WAF绕过功能的靶场。靶场会检测常见的攻击模式，并记录所有请求。</p>
        
        <div class="stats" id="stats">
            <div class="stat-box">
                <h3>总请求数</h3>
                <div class="number" id="total-requests">0</div>
            </div>
            <div class="stat-box">
                <h3>被拦截</h3>
                <div class="number" id="blocked-requests">0</div>
            </div>
            <div class="stat-box">
                <h3>成功绕过</h3>
                <div class="number" id="bypassed-requests">0</div>
            </div>
        </div>
        
        <button onclick="loadLogs()">刷新日志</button>
        <button onclick="clearLogs()">清除日志</button>
        <label style="margin-left:10px;font-size:12px;color:#333;">
            显示
            <select id="logs-limit" onchange="loadLogs()" style="margin:0 6px;padding:6px;border-radius:4px;border:1px solid #ddd;">
                <option value="50" selected>最近50条</option>
                <option value="200">最近200条</option>
                <option value="1000">最近1000条</option>
                <option value="all">全部</option>
            </select>
        </label>
        <label style="margin-left:10px;font-size:12px;color:#333;">
            过滤
            <input id="logs-filter" oninput="loadLogs()" placeholder="payload / url / rule 包含…" style="margin-left:6px;padding:6px;border-radius:4px;border:1px solid #ddd;min-width:260px;" />
        </label>
        <label style="margin-left:10px;font-size:12px;color:#333;">
            <input id="logs-pause" type="checkbox" style="margin-right:6px;vertical-align:middle;" />
            暂停自动刷新
        </label>
    </div>
    
    <div class="container">
        <h2>测试端点</h2>
        <div class="endpoint">
            <strong>搜索接口</strong><br>
            GET/POST: <code>http://localhost:8888/search</code><br>
            用于测试SQL注入、XSS等攻击
        </div>
        <div class="endpoint">
            <strong>API接口</strong><br>
            GET/POST: <code>http://localhost:8888/api</code><br>
            另一个测试端点
        </div>
        <div class="endpoint">
            <strong>查询接口</strong><br>
            GET/POST: <code>http://localhost:8888/query</code><br>
            第三个测试端点
        </div>
    </div>
    
    <div class="container">
        <h2>WAF检测规则</h2>
`

	for _, rule := range wafRules {
		html += fmt.Sprintf(`
        <div class="rule">
            <div class="rule-name">%s</div>
            <div>%s</div>
        </div>`, htmlpkg.EscapeString(rule.Name), htmlpkg.EscapeString(rule.Description))
	}

	html += `
    </div>
    
    <div class="container">
        <h2>请求日志</h2>
        <div id="logs-container">
            <p>正在加载日志...</p>
        </div>
    </div>
    
    <script>
        function loadLogs() {
            fetch('/logs')
                .then(res => {
                    if (!res.ok) throw new Error('HTTP ' + res.status);
                    return res.json();
                })
                .then(data => {
                    const arr = Array.isArray(data) ? data : [];
                    updateStats(arr);
                    displayLogs(arr);
                })
                .catch(err => {
                    console.error(err);
                    const container = document.getElementById('logs-container');
                    if (container) container.innerHTML = '<p style="color:#c00;">日志加载失败：' + escHtml(err && err.message ? err.message : err) + '</p>';
                });
        }

        function escHtml(s) {
            return String(s === undefined || s === null ? '' : s)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;');
        }
        
        function updateStats(logs) {
            const total = logs.length;
            const blocked = logs.filter(l => l.blocked).length;
            const bypassed = logs.filter(l => l.bypassed).length;
            
            const t = document.getElementById('total-requests');
            const b = document.getElementById('blocked-requests');
            const y = document.getElementById('bypassed-requests');
            if (t) t.textContent = total;
            if (b) b.textContent = blocked;
            if (y) y.textContent = bypassed;
        }
        
        function displayLogs(logs) {
            const container = document.getElementById('logs-container');
            if (logs.length === 0) {
                container.innerHTML = '<p>暂无日志</p>';
                return;
            }
            
            const filterText = (document.getElementById('logs-filter')?.value || '').trim();
            let filtered = logs;
            if (filterText) {
                filtered = logs.filter(l => {
                    const payload = String(l.payload || '');
                    const url = String(l.url || '');
                    const rule = String(l.rule || '');
                    return payload.includes(filterText) || url.includes(filterText) || rule.includes(filterText);
                });
            }

            const limitRaw = document.getElementById('logs-limit')?.value || '50';
            let view = filtered;
            if (limitRaw !== 'all') {
                const n = parseInt(limitRaw, 10);
                if (!isNaN(n) && n > 0) view = filtered.slice(-n);
            }
            view = view.reverse();
            
            let html = '<div style="margin:6px 0 12px 0;color:#555;font-size:12px;">' +
                '显示 ' + view.length + ' / ' + filtered.length + '（总 ' + logs.length + '）' +
                '</div>' +
                '<table><thead><tr><th>时间</th><th>方法</th><th>URL</th><th>Payload</th><th>状态</th><th>规则</th></tr></thead><tbody>';
            
            view.forEach(log => {
                const badge = log.blocked 
                    ? '<span class="badge badge-blocked">被拦截</span>'
                    : '<span class="badge badge-bypassed">成功绕过</span>';
                const rule = log.rule || '-';
                const url = log.url || '-';
                const payload = log.payload || '';
                html += '<tr>' +
                    '<td>' + escHtml(log.time) + '</td>' +
                    '<td>' + escHtml(log.method) + '</td>' +
                    '<td style="max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="' + escHtml(url) + '">' + escHtml(url) + '</td>' +
                    '<td style="max-width: 420px; overflow: hidden; text-overflow: ellipsis;" title="' + escHtml(payload) + '">' + escHtml(payload) + '</td>' +
                    '<td>' + badge + '</td>' +
                    '<td>' + escHtml(rule) + '</td>' +
                    '</tr>';
            });
            
            html += '</tbody></table>';
            container.innerHTML = html;
        }
        
        function clearLogs() {
            if (confirm('确定要清除所有日志吗？')) {
                fetch('/clear-logs', { method: 'POST' })
                    .then(() => {
                        loadLogs();
                        alert('日志已清除');
                    })
                    .catch(err => console.error(err));
            }
        }
        
        // 页面加载时自动加载日志
        loadLogs();
        // 每3秒自动刷新
        setInterval(() => {
            const paused = !!document.getElementById('logs-pause')?.checked;
            if (!paused) loadLogs();
        }, 3000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func main() {
	port := ":8888"

	// 注册路由
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/api", apiHandler)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/logs", logsHandler)
	http.HandleFunc("/clear-logs", clearLogsHandler)

	fmt.Printf("🚀 WAF绕过测试靶场启动成功！\n")
	fmt.Printf("📍 访问地址: http://localhost%s\n", port)
	fmt.Printf("📝 测试端点:\n")
	fmt.Printf("   - http://localhost%s/search\n", port)
	fmt.Printf("   - http://localhost%s/api\n", port)
	fmt.Printf("   - http://localhost%s/query\n", port)
	fmt.Printf("📊 日志接口: http://localhost%s/logs\n", port)
	fmt.Printf("\n按 Ctrl+C 停止服务器\n\n")

	log.Fatal(http.ListenAndServe(port, nil))
}
