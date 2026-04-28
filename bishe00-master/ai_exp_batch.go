package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"

	"bishe/internal/mcp"
)

type AIGenPythonFromExpBatchReq struct {
	Provider      string    `json:"provider"`
	APIKey        string    `json:"apiKey"`
	BaseURL       string    `json:"baseUrl"`
	Model         string    `json:"model"`
	TargetBaseURL string    `json:"targetBaseUrl"`
	TimeoutMs     int       `json:"timeoutMs"`
	ExpDir        string    `json:"expDir"`
	ExpPaths      []string  `json:"expPaths"`
	InlineExps    []ExpSpec `json:"inlineExps"`
	Concurrency   int       `json:"concurrency"`
}

func aiGenPythonFromExpBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AIGenPythonFromExpBatchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.TargetBaseURL = strings.TrimSpace(req.TargetBaseURL)

	exps, err := loadRequestedExps(req.InlineExps, req.ExpPaths, req.ExpDir)
	if err != nil || len(exps) == 0 {
		http.Error(w, "load exps error or empty", http.StatusBadRequest)
		return
	}

	cc := req.Concurrency
	if cc <= 0 {
		cc = 20
	}

	var provider mcp.AIProvider
	if strings.TrimSpace(req.Provider) != "" {
		if p, err := newAIProvider(req.Provider, req.APIKey, req.BaseURL, req.Model); err == nil {
			provider = p
		}
	}

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

				py, keyInfo, _ := generatePythonForSpec(req.TargetBaseURL, es, provider, nil)

				d, tot := t.IncDone()
				percent := int(math.Round(float64(d) / float64(tot) * 100))
				safeSend(t, SSEMessage{
					Type:     "find",
					TaskID:   t.ID,
					Progress: fmt.Sprintf("%d/%d", d, tot),
					Percent:  percent,
					Data: map[string]interface{}{
						"name":    es.Name,
						"keyInfo": keyInfo,
						"python":  py,
					},
				})

				<-sem
			}()
		}
		wg.Wait()
		finishTask(t.ID)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
}
