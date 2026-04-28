package main

import (
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FingerprintRule struct {
	CMS      string   `json:"cms"`
	Method   string   `json:"method"`
	Location string   `json:"location"`
	Keyword  []string `json:"keyword"`
}

type FingerprintData struct {
	Fingerprint []FingerprintRule `json:"fingerprint"`
}

var (
	fingerprintRules []FingerprintRule
	fingerprintOnce  sync.Once
)

type WebProbeReq struct {
	URLs           []string          `json:"urls"`
	Concurrency    int               `json:"concurrency"`
	TimeoutMs      int               `json:"timeoutMs"`
	Headers        map[string]string `json:"headers"`
	FollowRedirect bool              `json:"followRedirect"`
	FetchFavicon   bool              `json:"fetchFavicon"`
	FetchRobots    bool              `json:"fetchRobots"`
}

func loadFingerprints() {
	fingerprintOnce.Do(func() {
		paths := []string{
			"library/finger.json",
			"shili/library/finger.json",
			"G:/Finger-main/Finger-main/library/finger.json",
			filepath.Join(getBuiltinPocDir(), "../library/finger.json"),
		}

		var data FingerprintData
		var loaded bool
		for _, path := range paths {
			if b, err := os.ReadFile(path); err == nil {
				if err := json.Unmarshal(b, &data); err == nil {
					fingerprintRules = data.Fingerprint
					loaded = true
					log.Printf("[指纹库] 成功加载 %d 条指纹规则，来源: %s", len(fingerprintRules), path)
					break
				}
			}
		}
		if !loaded {
			log.Printf("[指纹库] 警告: 未能加载指纹库文件，将使用基础识别")
			fingerprintRules = []FingerprintRule{}
		}
	})
}

func matchFingerprintRule(rule FingerprintRule, resp *http.Response, body []byte, faviconHash string) bool {
	location := strings.ToLower(rule.Location)
	method := strings.ToLower(rule.Method)

	if method == "faviconhash" {
		if len(rule.Keyword) > 0 && faviconHash != "" {
			for _, hash := range rule.Keyword {
				if strings.EqualFold(hash, faviconHash) {
					return true
				}
			}
		}
		return false
	}

	var searchText string
	if location == "header" {
		headers := make([]string, 0)
		for k, vals := range resp.Header {
			for _, v := range vals {
				headers = append(headers, k+": "+v)
			}
		}
		searchText = strings.ToLower(strings.Join(headers, "\n"))
	} else {
		searchText = strings.ToLower(string(body))
	}

	if len(rule.Keyword) == 0 {
		return false
	}

	for _, keyword := range rule.Keyword {
		if keyword == "" {
			continue
		}
		if method == "regula" {
			re, err := regexp.Compile(keyword)
			if err != nil {
				continue
			}
			if !re.MatchString(searchText) {
				return false
			}
		} else if !strings.Contains(searchText, strings.ToLower(keyword)) {
			return false
		}
	}

	return true
}

func detectFingerprints(resp *http.Response, body []byte, faviconHash string) []string {
	loadFingerprints()
	if len(fingerprintRules) == 0 {
		return nil
	}

	detected := make(map[string]bool)
	var results []string
	for _, rule := range fingerprintRules {
		if rule.CMS == "" {
			continue
		}
		if matchFingerprintRule(rule, resp, body, faviconHash) && !detected[rule.CMS] {
			detected[rule.CMS] = true
			results = append(results, rule.CMS)
		}
	}
	return results
}

func detectTech(resp *http.Response, body []byte, faviconHash string) []string {
	var tech []string
	server := resp.Header.Get("Server")
	xpb := resp.Header.Get("X-Powered-By")
	setCookie := strings.Join(resp.Header.Values("Set-Cookie"), ";")
	if server != "" {
		tech = append(tech, "Server:"+server)
	}
	if xpb != "" {
		tech = append(tech, "X-Powered-By:"+xpb)
	}
	lowHead := strings.ToLower(server + ";" + xpb + ";" + setCookie)
	if strings.Contains(lowHead, "php") || strings.Contains(lowHead, "phpsessid") {
		tech = append(tech, "PHP")
	}
	if strings.Contains(lowHead, "asp.net") || strings.Contains(lowHead, "aspxauth") {
		tech = append(tech, ".NET")
	}
	if strings.Contains(lowHead, "jsessionid") {
		tech = append(tech, "Java")
	}
	if strings.Contains(lowHead, "laravel") || strings.Contains(lowHead, "laravel_session") {
		tech = append(tech, "Laravel")
	}
	b := strings.ToLower(string(body))
	if strings.Contains(b, "wp-content") {
		tech = append(tech, "WordPress")
	}
	if strings.Contains(b, "drupal.settings") {
		tech = append(tech, "Drupal")
	}
	if strings.Contains(b, "<meta name=\"generator\"") {
		tech = append(tech, "GeneratorMeta")
	}
	tech = append(tech, detectFingerprints(resp, body, faviconHash)...)
	return tech
}

func sha1hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

func webProbeHandler(w http.ResponseWriter, r *http.Request) {
	var req WebProbeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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

	urls2set := make(map[string]struct{})
	for _, raw := range req.URLs {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		low := strings.ToLower(u)
		if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
			u = "http://" + u
		}
		urls2set[u] = struct{}{}
	}
	urls2 := make([]string, 0, len(urls2set))
	for k := range urls2set {
		urls2 = append(urls2, k)
	}

	t := newTask(len(urls2))
	go func() {
		sem := make(chan struct{}, cc)
		var wg sync.WaitGroup
		for _, u := range urls2 {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			url := u
			go func() {
				defer wg.Done()
				select {
				case <-t.stop:
					<-sem
					return
				default:
				}
				req0, _ := http.NewRequest("GET", url, nil)
				for k, v := range req.Headers {
					req0.Header.Set(k, v)
				}
				resp, err := client.Do(req0)
				var tech []string
				status := 0
				title := ""
				finalURL := url
				proto := ""
				cl := int64(0)
				if err == nil {
					status = resp.StatusCode
					b, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					finalURL = resp.Request.URL.String()
					proto = resp.Proto
					cl = resp.ContentLength

					var faviconHash string
					if req.FetchFavicon {
						favURL := strings.TrimRight(finalURL, "/") + "/favicon.ico"
						r2, e2 := client.Get(favURL)
						if e2 == nil {
							fb, _ := io.ReadAll(r2.Body)
							r2.Body.Close()
							if len(fb) > 0 {
								faviconHash = sha1hex(fb)
								tech = append(tech, "favicon:sha1="+faviconHash)
							}
						}
					}

					tech = detectTech(resp, b, faviconHash)
					title = extractTitle(b)
					if req.FetchRobots {
						robURL := strings.TrimRight(finalURL, "/") + "/robots.txt"
						r3, e3 := client.Get(robURL)
						if e3 == nil {
							_, _ = io.Copy(io.Discard, r3.Body)
							r3.Body.Close()
							if r3.StatusCode == 200 {
								tech = append(tech, "robots.txt")
							}
						}
					}
				}

				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				if status > 0 {
					msg.Type = "find"
					msg.Data = map[string]interface{}{"url": url, "finalUrl": finalURL, "status": status, "title": title, "tech": tech, "proto": proto, "cl": cl}
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

func runWebProbeInternalWithProgress(urls []string, concurrency int, timeoutMs int, fetchFavicon, fetchRobots bool, t *Task) []map[string]interface{} {
	sendProgress(t, 0, fmt.Sprintf("开始Web探针，共 %d 个URL...", len(urls)))

	var results []map[string]interface{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	scanned := int32(0)
	total := int32(len(urls))

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
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
				sendProgress(t, 0, fmt.Sprintf("Web探针中: %d/%d (%.1f%%)", scannedCount, total, float64(scannedCount)/float64(total)*100))
			case <-t.stop:
				return
			}
		}
	}()

	for _, url := range urls {
		select {
		case <-t.stop:
			return results
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(u string) {
			defer wg.Done()
			defer func() {
				<-sem
				atomic.AddInt32(&scanned, 1)
			}()

			resp, err := client.Get(u)
			if err == nil && resp != nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				title := extractTitle(body)

				var faviconHash string
				if fetchFavicon {
					favURL := strings.TrimRight(u, "/") + "/favicon.ico"
					if r2, e2 := client.Get(favURL); e2 == nil {
						fb, _ := io.ReadAll(r2.Body)
						r2.Body.Close()
						if len(fb) > 0 {
							faviconHash = sha1hex(fb)
						}
					}
				}

				tech := detectTech(resp, body, faviconHash)
				mu.Lock()
				results = append(results, map[string]interface{}{
					"url":      u,
					"status":   resp.StatusCode,
					"title":    title,
					"tech":     tech,
					"length":   len(body),
					"server":   resp.Header.Get("Server"),
					"xpowered": resp.Header.Get("X-Powered-By"),
				})
				webCount := len(results)
				mu.Unlock()

				techStr := ""
				if len(tech) > 0 {
					techStr = strings.Join(tech, ", ")
				}
				sendProgress(t, 0, fmt.Sprintf("发现Web服务: %s (状态码: %d, 标题: %s, 技术栈: %s) [已发现 %d 个]", u, resp.StatusCode, title, techStr, webCount))
			}
		}(url)
	}

	wg.Wait()
	if len(results) > 0 {
		sendProgress(t, 0, fmt.Sprintf("Web探针完成，发现 %d 个Web服务", len(results)))
	} else {
		sendProgress(t, 0, "Web探针完成，未发现Web服务")
	}
	return results
}
