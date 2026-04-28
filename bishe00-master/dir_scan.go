package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type DirScanReq struct {
	BaseURL      string   `json:"baseUrl"`
	DictPaths    []string `json:"dictPaths"`
	BuiltinDicts []string `json:"builtinDicts"`
	Concurrency  int      `json:"concurrency"`
	TimeoutMs    int      `json:"timeoutMs"`
}

type DictInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
}

func dirScanHandler(w http.ResponseWriter, r *http.Request) {
	var req DirScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if len(req.BuiltinDicts) > 0 {
		dictDir := getBuiltinDictDir()
		for _, dictName := range req.BuiltinDicts {
			if err := readOne(filepath.Join(dictDir, dictName)); err != nil {
				log.Printf("[目录扫描] 读取内置字典失败: %s, 错误: %v", filepath.Join(dictDir, dictName), err)
			}
		}
	}
	for _, fp := range req.DictPaths {
		if err := readOne(fp); err != nil {
			log.Printf("[目录扫描] 读取自定义字典失败: %s, 错误: %v", fp, err)
		}
	}
	if len(paths) == 0 {
		http.Error(w, "empty dict paths and builtin dicts", http.StatusBadRequest)
		return
	}
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
	t := newTask(len(paths))
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("dirScan top panic: %v", r)
			}
		}()
		var wg sync.WaitGroup
		jobs := make(chan string, cc*2)
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
					d, tot := t.IncDone()
					safeSend(t, SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))})
					continue
				}
				resp, err := client.Do(req0)
				status, loc, bodyLen := 0, "", 0
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
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))}
				if status == 200 || status == 403 || status == 301 || status == 302 {
					msg.Type = "find"
					msg.Data = map[string]interface{}{"path": path, "url": url, "status": status, "location": loc, "length": bodyLen}
				}
				safeSend(t, msg)
			}
		}
		wN := cc
		if wN < 1 {
			wN = 1
		}
		if wN > 1000 {
			wN = 1000
		}
		wg.Add(wN)
		for i := 0; i < wN; i++ {
			go worker()
		}
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

func getBuiltinDictDir() string {
	cwd, _ := os.Getwd()
	localDir := filepath.Join(cwd, "dict")
	if st, err := os.Stat(localDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(localDir)
		return abs
	}
	parentDir := filepath.Join(cwd, "..", "dict")
	if st, err := os.Stat(parentDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(parentDir)
		return abs
	}
	dictDir := `E:\gongju\天狐渗透工具箱-社区版V2.0纪念版\tools\gui_shouji\dirscan_3.0\dict`
	if st, err := os.Stat(dictDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(dictDir)
		return abs
	}
	return ""
}

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

func getBuiltinDicts() map[string][]DictInfo {
	dictDir := getBuiltinDictDir()
	result := make(map[string][]DictInfo)
	if dictDir == "" {
		return result
	}
	files, err := os.ReadDir(dictDir)
	if err != nil {
		return result
	}
	categoryMap := map[string]string{"php": "PHP", "thinkphp": "PHP", "laravel": "PHP", "wordpress": "PHP", "drupal": "PHP", "jsp": "Java", "java": "Java", "spring": "Java", "springboot": "Java", "struts": "Java", "shiro": "Java", "tomcat": "Java", "jboss": "Java", "weblogic": "Java", "asp": "ASP", "aspx": "ASP", "python": "Python", "django": "Python", "flask": "Python", "ruby": "Ruby", "rails": "Ruby", "nodejs": "Node.js", "express": "Node.js", "go": "Go", "common": "通用"}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		filePath := filepath.Join(dictDir, fileName)
		if info, err := file.Info(); err == nil && info.Size() > 0 {
			fileNameLower := strings.ToLower(fileName)
			category := "其他"
			for key, cat := range categoryMap {
				if strings.Contains(fileNameLower, key) {
					category = cat
					break
				}
			}
			result[category] = append(result[category], DictInfo{Name: fileName, Path: filePath, Category: category})
		}
	}
	for category := range result {
		sort.Slice(result[category], func(i, j int) bool { return result[category][i].Name < result[category][j].Name })
	}
	return result
}

func getBuiltinDictsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getBuiltinDicts())
}

func selectDictByTech(tech []interface{}) []string {
	dictDir := getBuiltinDictDir()
	if dictDir == "" {
		return nil
	}
	techToDict := map[string][]string{"php": {"php", "common"}, "thinkphp": {"php", "thinkphp", "common"}, "laravel": {"php", "laravel", "common"}, "wordpress": {"php", "wordpress", "common"}, "drupal": {"php", "drupal", "common"}, "jsp": {"jsp", "java", "common"}, "java": {"jsp", "java", "common"}, "spring": {"jsp", "java", "spring", "common"}, "springboot": {"jsp", "java", "spring", "common"}, "struts": {"jsp", "java", "struts", "common"}, "shiro": {"jsp", "java", "shiro", "common"}, "asp": {"asp", "aspx", "common"}, "aspx": {"asp", "aspx", "common"}, "asp.net": {"asp", "aspx", "common"}, ".net": {"asp", "aspx", "common"}, "python": {"python", "common"}, "django": {"python", "django", "common"}, "flask": {"python", "flask", "common"}, "ruby": {"ruby", "common"}, "rails": {"ruby", "rails", "common"}, "nodejs": {"nodejs", "common"}, "express": {"nodejs", "express", "common"}, "go": {"go", "common"}, "tomcat": {"jsp", "java", "tomcat", "common"}, "jboss": {"jsp", "java", "jboss", "common"}, "weblogic": {"jsp", "java", "weblogic", "common"}}
	dictFilesSet := map[string]bool{"common": true}
	for _, t := range tech {
		techStr, ok := t.(string)
		if !ok {
			continue
		}
		techLower := strings.ToLower(techStr)
		if strings.Contains(techLower, ":") {
			parts := strings.Split(techLower, ":")
			if len(parts) > 1 {
				techLower = strings.TrimSpace(parts[1])
			}
		}
		for techKey, dictList := range techToDict {
			if strings.Contains(techLower, techKey) {
				for _, dictName := range dictList {
					dictFilesSet[dictName] = true
				}
			}
		}
		if _, err := os.Stat(filepath.Join(dictDir, techLower)); err == nil {
			dictFilesSet[techLower] = true
		}
	}
	var dictFiles []string
	for dictName := range dictFilesSet {
		if dictPath := filepath.Join(dictDir, dictName); func() bool { _, err := os.Stat(dictPath); return err == nil }() {
			dictFiles = append(dictFiles, dictPath)
		}
	}
	if len(dictFiles) == 0 {
		if commonPath := filepath.Join(dictDir, "common"); func() bool { _, err := os.Stat(commonPath); return err == nil }() {
			return []string{commonPath}
		}
	}
	return dictFiles
}

func runDirScanInternalWithProgress(baseURL string, dictFiles []string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	var paths []string
	if len(dictFiles) > 0 {
		for _, dictFile := range dictFiles {
			if dictPaths, err := loadDictFile(dictFile); err == nil {
				paths = append(paths, dictPaths...)
			} else {
				log.Printf("[目录扫描] 读取字典文件失败: %s, 错误: %v", dictFile, err)
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"/", "/admin", "/login", "/api", "/test", "/backup", "/config", "/.git", "/.svn", "/.env", "/robots.txt", "/sitemap.xml", "/wp-admin", "/wp-content", "/phpinfo.php", "/info.php"}
	}
	sendProgress(t, 0, fmt.Sprintf("开始目录扫描，共 %d 个目录...", len(paths)))
	var results []map[string]interface{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	scanned := int32(0)
	total := int32(len(paths))
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
			defer func() { <-sem; atomic.AddInt32(&scanned, 1) }()
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
					results = append(results, map[string]interface{}{"path": p, "url": url, "status": status, "length": len(body)})
					dirCount := len(results)
					mu.Unlock()
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
