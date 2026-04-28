package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type PortScanReq struct {
	Host        string `json:"host"`
	Ports       string `json:"ports"`
	Concurrency int    `json:"concurrency"`
	TimeoutMs   int    `json:"timeoutMs"`
	ScanType    string `json:"scanType"`
	GrabBanner  bool   `json:"grabBanner"`
}

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
		} else if v, err := strconv.Atoi(part); err == nil {
			set[v] = struct{}{}
		}
	}
	ports := make([]int, 0, len(set))
	for k := range set {
		ports = append(ports, k)
	}
	sortInts(ports)
	return ports
}

func sortInts(ports []int) {
	for i := 0; i < len(ports); i++ {
		for j := i + 1; j < len(ports); j++ {
			if ports[j] < ports[i] {
				ports[i], ports[j] = ports[j], ports[i]
			}
		}
	}
}

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
				} else {
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
				msg := SSEMessage{Type: "progress", TaskID: t.ID, Progress: fmt.Sprintf("%d/%d", d, tot), Percent: int(math.Round(float64(d) / float64(tot) * 100))}
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

func runPortScanInternalWithProgress(host, ports string, concurrency int, timeoutMs int, t *Task) []map[string]interface{} {
	portList := parsePorts(ports)
	sendProgress(t, 0, fmt.Sprintf("开始端口扫描，共 %d 个端口...", len(portList)))
	var results []map[string]interface{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	scanned := int32(0)
	total := int32(len(portList))
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	}
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
			defer func() { <-sem; atomic.AddInt32(&scanned, 1) }()
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				banner := ""
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
				results = append(results, map[string]interface{}{"port": port, "proto": "tcp", "status": "open", "banner": banner})
				openPorts := len(results)
				mu.Unlock()
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
		sendProgress(t, 0, "端口扫描完成，未发现开放端口")
	}
	return results
}
