package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func pocScanHandler(w http.ResponseWriter, r *http.Request) {
	var req PocScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pocs []POC
	var xrps []XRPOC
	var nucs []NucleiPOC
	var err error
	if len(req.PocPaths) > 0 {
		pocs, xrps, nucs, err = loadAllPOCsFromFiles(req.PocPaths)
	} else {
		if req.PocDir == "" {
			req.PocDir = getBuiltinPocDir()
		}
		pocs, xrps, nucs, err = loadAllPOCs(req.PocDir)
	}
	if err != nil || (len(pocs) == 0 && len(xrps) == 0 && len(nucs) == 0) {
		http.Error(w, "load pocs error or empty", http.StatusBadRequest)
		return
	}

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
				method := strings.ToUpper(strings.TrimSpace(pp.Method))
				if method == "" {
					method = "GET"
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
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent, Data: map[string]interface{}{"current": pp.Name}}
				if matched {
					msg.Type = "find"
					curl := fmt.Sprintf("curl -i -X %s '%s'", method, url)
					info := map[string]interface{}{"name": pp.Name}
					if pp.Name == "ThinkPHP 5.0.23 Remote Code Execution" {
						info["severity"] = "critical"
					}
					msg.Data = map[string]interface{}{"poc": pp.Name, "url": url, "status": status, "exp": curl, "info": info, "req": map[string]interface{}{"method": method, "path": pp.Path, "headers": pp.Headers, "body": pp.Body}}
				} else {
					safeSend(t, SSEMessage{Type: "scan_log", TaskID: t.ID, Data: map[string]interface{}{"poc": pp.Name, "status": "safe"}})
				}
				safeSend(t, msg)
				<-sem
			}()
		}

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
				vuln := evalGlobalExpression(xp.Expression, ruleResults)
				d, tot := t.IncDone()
				pname := xp.Info.Name
				if pname == "" {
					pname = xp.ID
				}
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100)), Data: map[string]interface{}{"current": pname}}
				if vuln {
					msg.Type = "find"
					curl := ""
					if hitURL != "" {
						curl = fmt.Sprintf("curl -i '%s'", hitURL)
					}
					msg.Data = map[string]interface{}{"poc": pname, "url": hitURL, "status": hitStatus, "exp": curl, "info": xp.Info, "req": map[string]interface{}{"method": hitMethod, "path": hitPath, "headers": hitHeaders, "body": hitBody}}
				} else {
					safeSend(t, SSEMessage{Type: "scan_log", TaskID: t.ID, Data: map[string]interface{}{"poc": pname, "status": "safe"}})
				}
				safeSend(t, msg)
			}()
		}

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
				pname := np.Info.Name
				if pname == "" {
					pname = np.ID
				}
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100)), Data: map[string]interface{}{"current": pname}}
				if ok {
					var statusVal interface{}
					if hitStatus == 0 {
						statusVal = "suspect"
					} else {
						statusVal = hitStatus
					}
					curl := ""
					if hitURL != "" {
						curl = fmt.Sprintf("curl -i '%s'", hitURL)
					}
					info := np.Info
					infoMap := map[string]interface{}{"name": info.Name, "author": info.Author, "severity": info.Severity, "risk": info.Risk, "reference": info.Reference, "description": info.Description, "tags": info.Tags}
					if strings.Contains(strings.ToLower(info.Name), "thinkphp") {
						infoMap["severity"] = "critical"
					}
					msg.Type = "find"
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

func evalRuleExpression(expr string, status int, body []byte, headers http.Header) bool {
	if len(body) == 0 && strings.Contains(expr, "bcontains") {
		return false
	}
	ex := expr
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
	reContains := regexp.MustCompile(`response\.body\.bcontains\(b?['"]([\s\S]*?)['"]\)`)
	ex = reContains.ReplaceAllStringFunc(ex, func(s string) string {
		m := reContains.FindStringSubmatch(s)
		if len(m) == 2 {
			needle := m[1]
			if needle != "" && len(body) > 0 && strings.Contains(string(body), needle) {
				return "true"
			}
		}
		return "false"
	})
	reMatchesR := regexp.MustCompile(`r['"]([\s\S]*?)['"]\.bmatches\(response\.body\)`)
	ex = reMatchesR.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatchesR.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			if pat != "" && len(body) > 0 {
				re, e := regexp.Compile(pat)
				if e == nil && re.Match(body) {
					return "true"
				}
			}
		}
		return "false"
	})
	reMatches1 := regexp.MustCompile(`response\.body\.bmatches\("([\s\S]*?)"\)`)
	ex = reMatches1.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatches1.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			if pat != "" && len(body) > 0 {
				re, e := regexp.Compile(pat)
				if e == nil && re.Match(body) {
					return "true"
				}
			}
		}
		return "false"
	})
	reMatches2 := regexp.MustCompile(`"([\s\S]*?)"\.bmatches\(response\.body\)`)
	ex = reMatches2.ReplaceAllStringFunc(ex, func(s string) string {
		m := reMatches2.FindStringSubmatch(s)
		if len(m) == 2 {
			pat := m[1]
			if pat != "" && len(body) > 0 {
				re, e := regexp.Compile(pat)
				if e == nil && re.Match(body) {
					return "true"
				}
			}
		}
		return "false"
	})
	if strings.Contains(ex, "oobCheck") {
		return false
	}
	ex = strings.ReplaceAll(ex, "True", "true")
	ex = strings.ReplaceAll(ex, "False", "false")
	return evalBoolExpr(ex)
}

func evalGlobalExpression(expr string, ruleResults map[string]bool) bool {
	ex := regexp.MustCompile(`([A-Za-z0-9_]+)\(\)`).ReplaceAllStringFunc(expr, func(s string) string {
		name := regexp.MustCompile(`([A-Za-z0-9_]+)\(\)`).FindStringSubmatch(s)
		if len(name) == 2 && ruleResults[name[1]] {
			return "true"
		}
		return "false"
	})
	return evalBoolExpr(ex)
}

func evalBoolExpr(expr string) bool {
	ex := strings.TrimSpace(expr)
	if ex == "" {
		return false
	}
	for {
		re := regexp.MustCompile(`\([^()]*\)`)
		loc := re.FindStringIndex(ex)
		if loc == nil {
			break
		}
		replacement := "false"
		if evalBoolExpr(ex[loc[0]+1 : loc[1]-1]) {
			replacement = "true"
		}
		ex = ex[:loc[0]] + replacement + ex[loc[1]:]
	}
	result := false
	for _, term := range splitByOperator(ex, "||") {
		andVal := true
		hasValidTerm := false
		for _, f := range splitByOperator(term, "&&") {
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

func genRandStr() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }

func substVarsSimple(s, baseURL string) string {
	out := s
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
	if len(lines) > 0 {
		parts := strings.Fields(strings.TrimSpace(lines[0]))
		if len(parts) >= 2 {
			method = strings.ToUpper(parts[0])
			url = strings.TrimRight(baseURL, "/") + parts[1]
		}
	}
	i := 1
	for ; i < len(lines); i++ {
		ln := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(ln) == "" {
			i++
			break
		}
		if idx := strings.Index(ln, ":"); idx >= 0 {
			headers[strings.TrimSpace(ln[:idx])] = strings.TrimSpace(ln[idx+1:])
		}
	}
	if i < len(lines) {
		body = strings.Join(lines[i:], "\n")
	}
	return
}

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
			if len(m.Words) == 0 {
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
			if resp == nil || len(body) == 0 {
				ok = false
				break
			}
			if part == "body" {
				lb := strings.ToLower(string(body))
				if all {
					ok = true
					for _, w := range m.Words {
						if w != "" && !strings.Contains(lb, strings.ToLower(w)) {
							ok = false
							break
						}
					}
				} else {
					for _, w := range m.Words {
						if w != "" && strings.Contains(lb, strings.ToLower(w)) {
							ok = true
							break
						}
					}
				}
			} else {
				hs := strings.ToLower(strings.Join(resp.Header.Values("Content-Type"), ";"))
				if all {
					ok = true
					for _, w := range m.Words {
						if w != "" && !strings.Contains(hs, strings.ToLower(w)) {
							ok = false
							break
						}
					}
				} else {
					for _, w := range m.Words {
						if w != "" && strings.Contains(hs, strings.ToLower(w)) {
							ok = true
							break
						}
					}
				}
			}
		case "regex":
			part := strings.ToLower(strings.TrimSpace(m.Part))
			if part == "" {
				part = "body"
			}
			if len(m.Regex) == 0 {
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
			if resp == nil || len(body) == 0 {
				ok = false
				break
			}
			matchRegexes := func(target []byte, regexes []string, all bool) bool {
				if all {
					for _, reStr := range regexes {
						if reStr == "" {
							continue
						}
						re, err := regexp.Compile(reStr)
						if err != nil || !re.Match(target) {
							return false
						}
					}
					return true
				}
				for _, reStr := range regexes {
					if reStr == "" {
						continue
					}
					re, err := regexp.Compile(reStr)
					if err == nil && re.Match(target) {
						return true
					}
				}
				return false
			}
			if part == "body" {
				ok = matchRegexes(body, m.Regex, all)
			} else {
				ok = matchRegexes([]byte(strings.ToLower(strings.Join(resp.Header.Values("Content-Type"), ";"))), m.Regex, all)
			}
		case "status":
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
				reStatus := regexp.MustCompile(`^status_code_(\d+)\s*==\s*(\d+)$`)
				if mm := reStatus.FindStringSubmatch(expr); len(mm) == 3 {
					i, _ := strconv.Atoi(mm[1])
					v, _ := strconv.Atoi(mm[2])
					if i >= 1 && i <= len(resps) && resps[i-1] != nil && resps[i-1].StatusCode == v {
						oneOK = true
					} else {
						allOK = false
					}
					continue
				}
				reBodyNotEmpty := regexp.MustCompile(`^body_(\d+)\s*!=\s*""$`)
				if mm := reBodyNotEmpty.FindStringSubmatch(expr); len(mm) == 2 {
					i, _ := strconv.Atoi(mm[1])
					if i >= 1 && i <= len(bodies) && len(bodies[i-1]) > 0 {
						oneOK = true
					} else {
						allOK = false
					}
					continue
				}
				allOK = false
			}
			if all {
				ok = allOK
			} else {
				ok = oneOK
			}
		}
		vals = append(vals, ok)
	}
	if len(vals) == 0 {
		return false
	}
	if cond == "and" {
		for _, v := range vals {
			if !v {
				return false
			}
		}
		return true
	}
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

func evalDetectionExpression(expr string, resp *http.Response, body []byte) bool {
	if resp == nil || len(body) == 0 {
		return false
	}
	reStatus := regexp.MustCompile(`StatusCode\(\)\s*==\s*(\d+)`)
	expr = reStatus.ReplaceAllStringFunc(expr, func(match string) string {
		matches := reStatus.FindStringSubmatch(match)
		if len(matches) == 2 {
			expected, _ := strconv.Atoi(matches[1])
			if resp.StatusCode == expected {
				return "true"
			}
		}
		return "false"
	})
	for _, pattern := range []struct {
		re     *regexp.Regexp
		negate bool
	}{
		{regexp.MustCompile(`StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`), false},
		{regexp.MustCompile(`StringSearch\(['"]response['"]\s*,\s*['"]([^'"]+)['"]\)`), false},
		{regexp.MustCompile(`!StringSearch\(['"]body['"]\s*,\s*['"]([^'"]+)['"]\)`), true},
	} {
		expr = pattern.re.ReplaceAllStringFunc(expr, func(match string) string {
			matches := pattern.re.FindStringSubmatch(match)
			if len(matches) == 2 {
				found := matches[1] != "" && strings.Contains(string(body), matches[1])
				if pattern.negate {
					found = matches[1] != "" && !found
				}
				if found {
					return "true"
				}
			}
			return "false"
		})
	}
	reRegex := regexp.MustCompile(`RegexSearch\(['"]resBody['"]\s*,\s*['"]([^'"]+)['"]\)`)
	expr = reRegex.ReplaceAllStringFunc(expr, func(match string) string {
		matches := reRegex.FindStringSubmatch(match)
		if len(matches) == 2 && matches[1] != "" {
			matched, err := regexp.MatchString(matches[1], string(body))
			if err == nil && matched {
				return "true"
			}
		}
		return "false"
	})
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
			if reqDef.URL != "" {
				rawURL := strings.TrimSpace(substVarsSimple(reqDef.URL, baseURL))
				p = rawURL
				if !strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://") && !strings.HasPrefix(rawURL, "{{BaseURL}}") {
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
			if reqDef.Headers != nil {
				switch v := reqDef.Headers.(type) {
				case map[string]string:
					headers = v
				case map[interface{}]interface{}:
					headers = map[string]string{}
					for k, vv := range v {
						if kStr, ok := k.(string); ok {
							if vStr, ok := vv.(string); ok {
								headers[kStr] = vStr
							}
						}
					}
				case []interface{}:
					headers = map[string]string{}
					for _, item := range v {
						if itemStr, ok := item.(string); ok {
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
		reqMap := map[string]interface{}{"method": method, "path": path, "headers": headers, "body": body}
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
	for i, reqDef := range np.Requests {
		if len(reqDef.Matchers) > 0 || len(reqDef.Detections) > 0 {
			if i < len(resps) && i < len(bodies) && resps[i] != nil && len(bodies[i]) > 0 {
				var reqMatchers []NucleiMatcher
				reqCond := reqDef.MatchersCondition
				if reqCond == "" {
					reqCond = "and"
				}
				if len(reqDef.Matchers) > 0 {
					reqMatchers = reqDef.Matchers
				} else if len(reqDef.Detections) > 0 {
					allDetectionsOK := true
					for _, det := range reqDef.Detections {
						if det != "" && !evalDetectionExpression(det, resps[i], bodies[i]) {
							allDetectionsOK = false
							break
						}
					}
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
				if len(reqMatchers) > 0 && matchNucleiMatchers([]*http.Response{resps[i]}, [][]byte{bodies[i]}, reqMatchers, reqCond) {
					return true, hitURL, hitStatus, hitReq
				}
			}
		}
	}
	if len(np.Matchers) > 0 && matchNucleiMulti(resps, bodies, np) {
		return true, hitURL, hitStatus, hitReq
	}
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
	return false, "", 0, nil
}

func runPocScanInternal(baseURL, pocDir string, pocPaths []string, keywords []string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	var results []map[string]interface{}
	var mu sync.Mutex
	var pocs []POC
	var xrps []XRPOC
	var nucs []NucleiPOC
	var err error
	if len(pocPaths) > 0 {
		pocs, xrps, nucs, err = loadAllPOCsFromFiles(pocPaths)
	} else if pocDir != "" {
		pocs, xrps, nucs, err = loadAllPOCs(pocDir)
	} else if builtinDir := getBuiltinPocDir(); builtinDir != "" {
		pocs, xrps, nucs, err = loadAllPOCs(builtinDir)
	}
	if err != nil {
		return results
	}
	if len(keywords) > 0 {
		lowerKeywords := make([]string, len(keywords))
		for i, kw := range keywords {
			lowerKeywords[i] = strings.ToLower(kw)
		}
		matchKeyword := func(text string) bool {
			for _, kw := range lowerKeywords {
				if strings.Contains(strings.ToLower(text), kw) {
					return true
				}
			}
			return false
		}
		var filteredPocs []POC
		for _, pp := range pocs {
			if matchKeyword(pp.Name) {
				filteredPocs = append(filteredPocs, pp)
			}
		}
		var filteredXrps []XRPOC
		for _, xp := range xrps {
			if matchKeyword(xp.ID + " " + xp.Info.Name) {
				filteredXrps = append(filteredXrps, xp)
			}
		}
		var filteredNucs []NucleiPOC
		for _, np := range nucs {
			tagsStr := strings.Join(np.Info.Tags, " ")
			if matchKeyword(np.ID + " " + np.Info.Name + " " + tagsStr) {
				filteredNucs = append(filteredNucs, np)
			}
		}
		pocs, xrps, nucs = filteredPocs, filteredXrps, filteredNucs
	}
	total := len(pocs) + len(xrps) + len(nucs)
	if total == 0 {
		return results
	}
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
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	var wg sync.WaitGroup
	sem := make(chan struct{}, cc)
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
			method := strings.ToUpper(strings.TrimSpace(pp.Method))
			if method == "" {
				method = "GET"
			}
			attempts := pp.Retry + 1
			if attempts <= 0 {
				attempts = 1
			}
			matched, status := false, 0
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
							if !strings.Contains(strings.ToLower(strings.Join(resp.Header.Values(hk), ";")), strings.ToLower(hv)) {
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
				results = append(results, map[string]interface{}{"poc": pp.Name, "url": url, "status": status, "info": info, "req": map[string]interface{}{"method": method, "path": pp.Path, "headers": pp.Headers, "body": pp.Body}})
				mu.Unlock()
			}
			if d%10 == 0 || matched {
				safeSend(t, SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))})
			}
		}()
	}
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
			ruleResults := map[string]bool{}
			var hitURL string
			var hitStatus int
			var hitMethod, hitPath, hitBody string
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
					hitURL, hitStatus, hitMethod, hitPath, hitBody = url, status, method, rule.Request.Path, bodyStr
					hitHeaders = map[string]string{}
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
				results = append(results, map[string]interface{}{"poc": pname, "url": hitURL, "status": hitStatus, "info": xp.Info, "req": map[string]interface{}{"method": hitMethod, "path": hitPath, "headers": hitHeaders, "body": hitBody}})
				mu.Unlock()
			}
			if d%10 == 0 || vuln {
				safeSend(t, SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))})
			}
		}()
	}
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
				results = append(results, map[string]interface{}{"poc": pname, "url": hitURL, "status": statusVal, "info": np.Info, "req": hitReq})
				mu.Unlock()
			}
			if d%10 == 0 || ok {
				safeSend(t, SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))})
			}
		}()
	}
	wg.Wait()
	return results
}
