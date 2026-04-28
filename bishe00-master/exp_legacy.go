package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

func execExp(base string, spec ExpSpec, client *http.Client) (bool, int, int, string, string) {
	matchedSteps := 0
	lastStatus := 0
	vars := make(map[string]string)
	cookieJar := make(map[string]string)

	for _, st := range spec.Steps {
		rawPath := substVars(st.Path, vars)
		low := strings.ToLower(strings.TrimSpace(rawPath))
		url := ""
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			url = rawPath
		} else {
			url = strings.TrimRight(base, "/") + rawPath
		}
		body := substVars(st.Body, vars)
		method := strings.ToUpper(strings.TrimSpace(st.Method))
		if method == "" {
			method = "GET"
		}

		attempts := st.Retry + 1
		if attempts <= 0 {
			attempts = 1
		}

		var resp *http.Response
		var err error
		var responseBody []byte
		for i := 0; i < attempts; i++ {
			var reqBody io.Reader
			if body != "" {
				reqBody = strings.NewReader(body)
			}
			req, buildErr := http.NewRequest(method, url, reqBody)
			if buildErr != nil {
				err = buildErr
				if st.RetryDelayMs > 0 {
					time.Sleep(time.Duration(st.RetryDelayMs) * time.Millisecond)
				}
				continue
			}

			for k, v := range st.Headers {
				req.Header.Set(k, substVars(v, vars))
			}
			if len(cookieJar) > 0 {
				var pairs []string
				for ck, cv := range cookieJar {
					pairs = append(pairs, fmt.Sprintf("%s=%s", ck, cv))
				}
				req.Header.Set("Cookie", strings.Join(pairs, "; "))
			}

			resp, err = client.Do(req)
			if err == nil {
				responseBody, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				lastStatus = resp.StatusCode
				break
			}
			if st.RetryDelayMs > 0 {
				time.Sleep(time.Duration(st.RetryDelayMs) * time.Millisecond)
			}
		}
		if err != nil {
			continue
		}
		if st.SleepMs > 0 {
			time.Sleep(time.Duration(st.SleepMs) * time.Millisecond)
		}

		for _, sc := range resp.Header.Values("Set-Cookie") {
			parts := strings.Split(sc, ";")
			kv := strings.SplitN(parts[0], "=", 2)
			if len(kv) == 2 {
				cookieJar[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}

		extractVars(resp, responseBody, st.Extract, vars)
		if validate(resp, responseBody, st.Validate) {
			matchedSteps++
		}
	}

	usage := buildUsageFromVars(vars)
	suggestion := substVars(spec.ExploitSuggestion, vars)
	return matchedSteps == len(spec.Steps) && len(spec.Steps) > 0, matchedSteps, lastStatus, usage, suggestion
}

func expExecHandler(w http.ResponseWriter, r *http.Request) {
	var req ExpExecReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var exps []ExpSpec
	var err error
	if len(req.InlineExps) > 0 {
		exps = req.InlineExps
	} else if len(req.ExpPaths) > 0 {
		exps, err = loadExpsFromFiles(req.ExpPaths)
	} else {
		exps, err = loadExps(req.ExpDir)
	}
	if err != nil || len(exps) == 0 {
		http.Error(w, "load exps error or empty", http.StatusBadRequest)
		return
	}

	cc := req.Concurrency
	if cc <= 0 {
		cc = 50
	}
	timeout := time.Duration(req.TimeoutMs)
	if timeout <= 0 {
		timeout = 5000 * time.Millisecond
	} else {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	client := &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	t := newTask(len(exps))
	go func() {
		sem := make(chan struct{}, cc)
		var wg sync.WaitGroup
		for _, e := range exps {
			select {
			case <-t.stop:
				break
			default:
			}
			wg.Add(1)
			sem <- struct{}{}
			es := e
			go func() {
				defer wg.Done()
				select {
				case <-t.stop:
					<-sem
					return
				default:
				}
				success, matched, lastStatus, usage, suggestion := execExp(req.BaseURL, es, client)
				keyInfo := buildExpKeyInfo(req.BaseURL, es)
				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: percent}
				if success {
					msg.Type = "find"
					msg.Data = map[string]interface{}{
						"name":         es.Name,
						"matchedSteps": matched,
						"lastStatus":   lastStatus,
						"usage":        usage,
						"suggestion":   suggestion,
						"keyInfo":      keyInfo,
					}
				} else {
					safeSend(t, SSEMessage{
						Type:   "scan_log",
						TaskID: t.ID,
						Data: map[string]interface{}{
							"name":         es.Name,
							"status":       "failed",
							"matchedSteps": matched,
							"lastStatus":   lastStatus,
							"keyInfo":      keyInfo,
						},
					})
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
