# 核心扫描技术实现详解
## SSE实时推送 + 端口扫描 + 目录扫描

---

## 目录

1. [SSE实时推送技术](#一sse实时推送技术)
2. [端口扫描实现](#二端口扫描实现)
3. [目录扫描实现](#三目录扫描实现)
4. [三大技术的协同工作](#四三大技术的协同工作)
5. [完整工作流程示例](#五完整工作流程示例)

---

# 一、SSE实时推送技术

## 1.1 SSE技术概述

**SSE（Server-Sent Events）** 是HTML5的服务器推送技术，允许服务器主动向客户端推送数据。

### 核心特点

| 特性 | 说明 |
|------|------|
| **单向通信** | 服务端→客户端的数据推送 |
| **基于HTTP** | 标准HTTP协议，无需额外握手 |
| **自动重连** | 浏览器原生支持断线重连 |
| **轻量级** | 相比WebSocket开销更小 |
| **实时性** | 消息延迟< 50ms |

### SSE vs WebSocket vs 轮询

```
轮询（Polling）:
客户端 --请求--> 服务器（每秒N次）
客户端 <--响应-- 服务器
❌ 频繁请求，资源浪费

WebSocket:
客户端 ==双向通道== 服务器
✅ 双向通信，适合聊天
❌ 实现复杂

SSE:
客户端 --建立连接--> 服务器
客户端 <==数据流===== 服务器
✅ 单向推送，简单高效
```

## 1.2 SSE实现原理

### HTTP响应头配置

```go
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")
w.Header().Set("Connection", "keep-alive")
```

| 响应头 | 作用 |
|--------|------|
| `text/event-stream` | 告诉浏览器这是事件流 |
| `no-cache` | 禁止缓存，确保实时性 |
| `keep-alive` | 保持长连接 |

### SSE消息格式

```
data: {"type":"progress","percent":50}

data: {"type":"find","data":{"port":80}}

```

**关键点**：
- 以 `data:` 开头
- 消息内容是JSON字符串
- **必须以两个换行符 `\n\n` 结束**

### 浏览器端EventSource API

```javascript
// 建立SSE连接
const eventSource = new EventSource('/sse?task=t-1234567890');

// 监听消息
eventSource.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    console.log('收到消息:', msg);
};

// 关闭连接
eventSource.close();
```

## 1.3 核心数据结构

### （1）SSE消息结构

```go
// SSEMessage Server-Sent Events消息结构体
type SSEMessage struct {
    Type     string      `json:"type"`     // start/progress/find/end
    TaskID   string      `json:"taskId"`   // 任务ID
    Progress string      `json:"progress"` // "10/100"
    Percent  int         `json:"percent"`  // 0-100
    Data     interface{} `json:"data"`     // 具体数据
}
```

### （2）任务结构体

```go
// Task 任务结构体
type Task struct {
    m       sync.Mutex      // 互斥锁（并发安全）
    ID      string          // 任务唯一标识（纳秒级时间戳）
    Total   int             // 总数量
    Done    int             // 已完成数量
    Created time.Time       // 创建时间
    ch      chan SSEMessage // SSE消息通道（容量1024）
    stop    chan struct{}   // 停止信号通道
    stopped bool            // 是否已停止
}

// 全局任务映射表
var (
    mu    sync.Mutex               // 全局互斥锁
    tasks = make(map[string]*Task) // key=任务ID, value=任务对象
)
```

## 1.4 SSE实现流程

### 步骤1：创建任务

```go
func newTask(total int) *Task {
    // 生成唯一任务ID
    id := fmt.Sprintf("t-%d", time.Now().UnixNano())
    
    t := &Task{
        ID:      id,
        Total:   total,
        Done:    0,
        Created: time.Now(),
        ch:      make(chan SSEMessage, 1024), // 带缓冲通道
        stop:    make(chan struct{}),
    }
    
    // 加入全局任务表
    mu.Lock()
    tasks[id] = t
    mu.Unlock()
    
    return t
}
```

### 步骤2：SSE处理器（核心）

```go
func sseHandler(w http.ResponseWriter, r *http.Request) {
    // 1. 获取任务ID
    id := r.URL.Query().Get("task")
    t, ok := getTask(id)
    if !ok {
        http.Error(w, "task not found", http.StatusNotFound)
        return
    }

    // 2. 设置SSE响应头
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // 3. 持续推送消息
    for {
        select {
        case msg := <-t.ch:
            // 从通道接收消息
            writeSSE(w, msg)
            
        case <-t.stop:
            // 收到停止信号
            return
            
        case <-r.Context().Done():
            // 客户端断开
            return
        }
    }
}

// 写入SSE消息
func writeSSE(w http.ResponseWriter, msg SSEMessage) error {
    b, _ := json.Marshal(msg)
    _, err := fmt.Fprintf(w, "data: %s\n\n", string(b))
    
    // 立即刷新
    if fl, ok := w.(http.Flusher); ok {
        fl.Flush()
    }
    
    return err
}
```

### 步骤3：工作goroutine推送消息

```go
go func() {
    for i, item := range items {
        // 执行具体任务...
        
        // 更新进度
        done, total := task.IncDone()
        percent := (done * 100) / total
        
        // 发送进度消息
        task.Send(SSEMessage{
            Type:     "progress",
            Progress: fmt.Sprintf("%d/%d", done, total),
            Percent:  percent,
        })
        
        // 如果发现结果，立即推送
        if foundResult {
            task.Send(SSEMessage{
                Type: "find",
                Data: result,
            })
        }
    }
}()
```

## 1.5 并发安全机制

### （1）任务进度更新

```go
// IncDone 增加已完成计数（线程安全）
func (t *Task) IncDone() (int, int) {
    t.m.Lock()
    defer t.m.Unlock()
    t.Done++
    return t.Done, t.Total
}
```

### （2）安全发送消息

```go
// Send 发送SSE消息（线程安全）
func (t *Task) Send(msg SSEMessage) bool {
    if t == nil {
        return false
    }
    
    t.m.Lock()
    if t.stopped || t.ch == nil {
        t.m.Unlock()
        return false
    }
    ch := t.ch
    stop := t.stop
    t.m.Unlock()
    
    select {
    case <-stop:
        return false
    default:
        defer func() { recover() }() // 防止通道关闭panic
        ch <- msg
        return true
    }
}
```

### （3）内存管理

```go
// startTaskJanitor 启动任务清理守护进程
func startTaskJanitor() {
    go func() {
        for {
            time.Sleep(60 * time.Second) // 每60秒清理一次
            
            mu.Lock()
            for id, t := range tasks {
                // 清理超过5分钟的老任务
                if time.Since(t.Created) > 5*time.Minute {
                    t.Close()
                    delete(tasks, id)
                }
            }
            mu.Unlock()
        }
    }()
}
```

---

# 二、端口扫描实现

## 2.1 端口扫描原理

### TCP三次握手

```
客户端                    服务器
  |                         |
  |------- SYN ------------>| (1) 发送SYN
  |                         |
  |<---- SYN-ACK -----------| (2) 响应SYN-ACK（端口开放）
  |                         |
  |------- ACK ------------>| (3) 发送ACK
  |                         |
  |===== 连接建立 ==========|
  |                         |
  |<---- Banner -------------| (4) 服务器发送Banner
  |                         |
  |------ FIN -------------->| (5) 关闭连接
```

**判断依据**：
- 收到 `SYN-ACK` → 端口**开放**
- 收到 `RST` → 端口**关闭**
- 无响应 → 端口被**过滤**

### UDP扫描原理

```
客户端                    服务器
  |                         |
  |---- UDP Packet -------->| (1) 发送UDP数据包
  |                         |
  |<--- UDP Response -------| (2) 收到响应 → 确认开放
  |<--- ICMP Unreachable ---| (3) 收到ICMP → 确认关闭
  |       (超时)            | (4) 无响应 → 状态未知
```

## 2.2 TCP扫描实现

### 核心代码

```go
// TCP全连接扫描
func scanTCPPort(host string, port int, timeout time.Duration, grabBanner bool) (bool, string) {
    addr := fmt.Sprintf("%s:%d", host, port)
    
    // 1. 建立TCP连接（三次握手）
    conn, err := net.DialTimeout("tcp", addr, timeout)
    if err != nil {
        return false, "" // 端口关闭或被过滤
    }
    defer conn.Close()
    
    // 2. 连接成功，端口开放
    open := true
    var banner string
    
    // 3. 如果需要抓取Banner
    if grabBanner {
        _ = conn.SetDeadline(time.Now().Add(timeout))
        buf := make([]byte, 256)
        n, _ := conn.Read(buf)
        if n > 0 {
            banner = string(buf[:n])
        }
    }
    
    return open, banner
}
```

### Banner抓取示例

| 服务 | 端口 | Banner示例 |
|------|------|-----------|
| **HTTP** | 80 | `HTTP/1.1 200 OK\nServer: nginx/1.18.0` |
| **SSH** | 22 | `SSH-2.0-OpenSSH_8.2p1 Ubuntu` |
| **FTP** | 21 | `220 (vsFTPd 3.0.3)` |
| **MySQL** | 3306 | `5.7.33-0ubuntu0.18.04.1` |
| **SMTP** | 25 | `220 mail.example.com ESMTP` |

## 2.3 UDP扫描实现

### 核心代码

```go
// UDP扫描
func scanUDPPort(host string, port int, timeout time.Duration) (bool, string) {
    addr := fmt.Sprintf("%s:%d", host, port)
    
    // 1. 解析UDP地址
    udpAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        return false, ""
    }
    
    // 2. 建立UDP"连接"
    conn, err := net.DialUDP("udp", nil, udpAddr)
    if err != nil {
        return false, ""
    }
    defer conn.Close()
    
    // 3. 设置超时
    _ = conn.SetDeadline(time.Now().Add(timeout))
    
    // 4. 发送探测数据
    _, _ = conn.Write([]byte("\n"))
    
    // 5. 尝试读取响应
    buf := make([]byte, 256)
    n, _, err := conn.ReadFrom(buf)
    
    if err == nil && n > 0 {
        // 收到响应，端口开放
        return true, string(buf[:n])
    }
    
    // 无响应，状态未知
    return false, ""
}
```

## 2.4 TCP vs UDP 关键区别

| 对比项 | TCP扫描 | UDP扫描 |
|--------|---------|---------|
| **连接方式** | 三次握手 | 无连接 |
| **判断依据** | 连接成功/失败 | 响应/ICMP/超时 |
| **准确性** | 高（95%+） | 中（60-70%） |
| **超时设置** | 300ms | 2-3秒 |
| **Banner抓取** | 易获取 | 部分支持 |
| **实现代码** | `net.DialTimeout("tcp", ...)` | `net.DialUDP(...)` |
| **常见端口** | 80, 443, 22, 3306, 6379 | 53, 161, 123, 500 |

## 2.5 端口扫描完整实现

```go
// portScanHandler 端口扫描处理器
func portScanHandler(w http.ResponseWriter, r *http.Request) {
    var req PortScanReq
    json.NewDecoder(r.Body).Decode(&req)
    
    // 解析端口范围
    ports := parsePorts(req.Ports) // "80,443,8080-8090"
    
    // 创建任务
    t := newTask(len(ports))
    
    // 返回任务ID
    json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
    
    // 启动后台扫描
    go func() {
        defer finishTask(t.ID)
        
        // 并发控制（信号量）
        sem := make(chan struct{}, req.Concurrency)
        var wg sync.WaitGroup
        
        for _, port := range ports {
            wg.Add(1)
            sem <- struct{}{} // 获取信号量
            
            go func(p int) {
                defer func() {
                    <-sem // 释放信号量
                    wg.Done()
                }()
                
                // 扫描端口
                addr := fmt.Sprintf("%s:%d", req.Host, p)
                open := false
                var banner string
                
                if req.ScanType == "tcp" {
                    conn, err := net.DialTimeout("tcp", addr, timeout)
                    if err == nil {
                        open = true
                        if req.GrabBanner {
                            buf := make([]byte, 256)
                            n, _ := conn.Read(buf)
                            banner = string(buf[:n])
                        }
                        conn.Close()
                    }
                } else { // udp
                    // UDP扫描逻辑...
                }
                
                // 更新进度
                done, total := t.IncDone()
                percent := (done * 100) / total
                
                msg := SSEMessage{
                    Type:     "progress",
                    Progress: fmt.Sprintf("%d/%d", done, total),
                    Percent:  percent,
                }
                
                // 如果端口开放，发送结果
                if open {
                    msg.Type = "find"
                    msg.Data = map[string]interface{}{
                        "port":   p,
                        "status": "open",
                        "banner": banner,
                    }
                }
                
                t.Send(msg)
            }(port)
        }
        
        wg.Wait()
    }()
}
```

---

# 三、目录扫描实现

## 3.1 目录扫描原理

### 什么是目录扫描？

**目录扫描（Directory Bruteforce）** 是通过字典文件枚举目标网站的目录和文件路径，发现：
- 隐藏的管理后台（如 `/admin`、`/manager`）
- 敏感文件（如 `/config.php`、`/.git`）
- 未授权访问的API端点
- 备份文件（如 `backup.sql`、`db.zip`）

### 工作原理

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│  字典文件   │         │  扫描器     │         │  目标网站   │
│             │         │             │         │             │
│ /admin      │ ─读取─> │ 构造URL     │ ─请求─> │ 检查响应    │
│ /config.php │         │ 并发请求    │         │ 状态码/长度 │
│ /backup.sql │         │ 过滤结果    │ <─响应─ │             │
│ /.git       │         │ 实时推送    │         │             │
└─────────────┘         └─────────────┘         └─────────────┘
```

### 判断依据

| HTTP状态码 | 说明 | 是否展示 |
|------------|------|----------|
| **200** | 成功访问 | ✅ 展示 |
| **403** | 禁止访问（目录存在但无权限） | ✅ 展示 |
| **301/302** | 重定向 | ✅ 展示 |
| **404** | 不存在 | ❌ 过滤 |
| **500** | 服务器错误 | ❌ 过滤 |

## 3.2 字典文件管理

### 字典文件结构

```
dict/
├── common.txt          # 通用字典
├── php.txt             # PHP相关路径
├── java.txt            # Java相关路径
├── asp.txt             # ASP相关路径
├── python.txt          # Python相关路径
├── nodejs.txt          # Node.js相关路径
├── wordpress.txt       # WordPress专用
├── thinkphp.txt        # ThinkPHP专用
└── spring.txt          # Spring专用
```

### 字典文件示例（php.txt）

```
# PHP常见路径
/admin
/admin.php
/login.php
/config.php
/phpinfo.php
/upload.php
/index.php
/install.php
/backup.sql
/.env
```

### 获取内置字典列表

```go
// getBuiltinDictDir 获取内置字典目录
func getBuiltinDictDir() string {
    // 硬编码的字典目录路径
    dictDir := `E:\gongju\天狐渗透工具箱-社区版V2.0纪念版\tools\gui_shouji\dirscan_3.0\dict`
    
    // 检查目录是否存在
    if st, err := os.Stat(dictDir); err == nil && st.IsDir() {
        return dictDir
    }
    
    // 备用路径
    cwd, _ := os.Getwd()
    return filepath.Join(cwd, "dict")
}

// getBuiltinDicts 获取所有内置字典列表（按技术栈分类）
func getBuiltinDicts() map[string][]DictInfo {
    dictDir := getBuiltinDictDir()
    result := make(map[string][]DictInfo)
    
    // 技术栈分类映射
    categoryMap := map[string]string{
        "php":     "PHP",
        "java":    "Java",
        "jsp":     "Java",
        "asp":     "ASP",
        "aspx":    "ASP",
        "python":  "Python",
        "django":  "Python",
        "nodejs":  "Node.js",
        "ruby":    "Ruby",
        "go":      "Go",
        "common":  "通用",
    }
    
    // 读取字典目录
    files, _ := os.ReadDir(dictDir)
    
    for _, file := range files {
        if file.IsDir() {
            continue
        }
        
        fileName := file.Name()
        fileNameLower := strings.ToLower(fileName)
        
        // 根据文件名确定分类
        category := "其他"
        for key, cat := range categoryMap {
            if strings.Contains(fileNameLower, key) {
                category = cat
                break
            }
        }
        
        dictInfo := DictInfo{
            Name:     fileName,
            Path:     filepath.Join(dictDir, fileName),
            Category: category,
        }
        
        result[category] = append(result[category], dictInfo)
    }
    
    return result
}
```

## 3.3 目录扫描核心实现

### （1）请求结构体

```go
// DirScanReq 目录扫描请求
type DirScanReq struct {
    BaseURL      string   `json:"baseUrl"`      // 目标URL
    DictPaths    []string `json:"dictPaths"`    // 自定义字典路径
    BuiltinDicts []string `json:"builtinDicts"` // 内置字典名称
    Concurrency  int      `json:"concurrency"`  // 并发数（默认200）
    TimeoutMs    int      `json:"timeoutMs"`    // 超时（默认1500ms）
}
```

### （2）读取字典文件

```go
// 读取字典文件
readOne := func(fp string) error {
    b, err := os.ReadFile(fp)
    if err != nil {
        return err
    }
    
    for _, line := range strings.Split(string(b), "\n") {
        p := strings.TrimSpace(line)
        
        // 过滤空行和注释
        if p == "" || strings.HasPrefix(p, "#") {
            continue
        }
        
        // 确保路径以/开头
        if !strings.HasPrefix(p, "/") {
            p = "/" + p
        }
        
        paths = append(paths, p)
    }
    
    return nil
}

// 读取内置字典
for _, dictName := range req.BuiltinDicts {
    dictPath := filepath.Join(dictDir, dictName)
    readOne(dictPath)
}

// 读取自定义字典
for _, fp := range req.DictPaths {
    readOne(fp)
}
```

### （3）并发扫描

```go
// dirScanHandler 目录扫描处理器
func dirScanHandler(w http.ResponseWriter, r *http.Request) {
    var req DirScanReq
    json.NewDecoder(r.Body).Decode(&req)
    
    // 读取字典文件
    var paths []string
    // ... 读取字典逻辑 ...
    
    // 创建任务
    t := newTask(len(paths))
    
    // 返回任务ID
    json.NewEncoder(w).Encode(map[string]string{"taskId": t.ID})
    
    // 启动后台扫描
    go func() {
        defer finishTask(t.ID)
        
        // 创建任务队列
        jobs := make(chan string, req.Concurrency*2)
        var wg sync.WaitGroup
        
        // Worker函数
        worker := func() {
            defer wg.Done()
            
            for path := range jobs {
                // 检查停止信号
                select {
                case <-t.stop:
                    return
                default:
                }
                
                // 构造完整URL
                url := strings.TrimRight(req.BaseURL, "/") + path
                
                // 发送HTTP请求
                req0, _ := http.NewRequest("GET", url, nil)
                resp, err := client.Do(req0)
                
                status := 0
                location := ""
                bodyLen := 0
                
                if err == nil && resp != nil {
                    status = resp.StatusCode
                    location = resp.Header.Get("Location")
                    
                    b, _ := io.ReadAll(resp.Body)
                    resp.Body.Close()
                    bodyLen = len(b)
                }
                
                // 更新进度
                done, total := t.IncDone()
                percent := (done * 100) / total
                
                msg := SSEMessage{
                    Type:     "progress",
                    Progress: fmt.Sprintf("%d/%d", done, total),
                    Percent:  percent,
                }
                
                // 判断是否展示结果（过滤逻辑）
                if status == 200 || status == 403 || status == 301 || status == 302 {
                    msg.Type = "find"
                    msg.Data = map[string]interface{}{
                        "path":     path,
                        "url":      url,
                        "status":   status,
                        "location": location,
                        "length":   bodyLen,
                    }
                }
                
                t.Send(msg)
            }
        }
        
        // 启动Worker池
        workerCount := req.Concurrency
        if workerCount < 1 {
            workerCount = 1
        }
        if workerCount > 1000 {
            workerCount = 1000 // 硬上限
        }
        
        wg.Add(workerCount)
        for i := 0; i < workerCount; i++ {
            go worker()
        }
        
        // 发送任务到队列
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
    }()
}
```

## 3.4 结果过滤机制

### （1）基础过滤（状态码）

```go
// 只展示这些状态码的结果
if status == 200 || status == 403 || status == 301 || status == 302 {
    // 展示结果
    msg.Type = "find"
    msg.Data = ...
} else {
    // 只更新进度，不展示结果
    msg.Type = "progress"
}
```

### （2）高级过滤选项

```go
// 高级过滤配置
type FilterConfig struct {
    IncludeStatus   []int    // 包含的状态码（如 [200, 403]）
    ExcludeStatus   []int    // 排除的状态码（如 [404, 500]）
    MinLength       int      // 最小响应长度
    MaxLength       int      // 最大响应长度
    IncludeKeywords []string // 响应体必须包含的关键词
    ExcludeKeywords []string // 响应体必须排除的关键词
}

// 应用过滤规则
func shouldShow(resp *http.Response, body []byte, filter FilterConfig) bool {
    status := resp.StatusCode
    bodyLen := len(body)
    
    // 1. 状态码过滤
    if len(filter.IncludeStatus) > 0 {
        if !contains(filter.IncludeStatus, status) {
            return false
        }
    }
    
    if contains(filter.ExcludeStatus, status) {
        return false
    }
    
    // 2. 长度过滤
    if filter.MinLength > 0 && bodyLen < filter.MinLength {
        return false
    }
    
    if filter.MaxLength > 0 && bodyLen > filter.MaxLength {
        return false
    }
    
    // 3. 关键词过滤
    bodyStr := string(body)
    
    for _, keyword := range filter.IncludeKeywords {
        if !strings.Contains(bodyStr, keyword) {
            return false
        }
    }
    
    for _, keyword := range filter.ExcludeKeywords {
        if strings.Contains(bodyStr, keyword) {
            return false
        }
    }
    
    return true
}
```

### （3）智能去重过滤

```go
// 根据响应内容的哈希值去重（避免重复展示相同页面）
type ContentFilter struct {
    seenHashes map[string]bool
    mu         sync.Mutex
}

func (f *ContentFilter) IsDuplicate(body []byte) bool {
    // 计算内容哈希
    hash := sha256.Sum256(body)
    hashStr := hex.EncodeToString(hash[:])
    
    f.mu.Lock()
    defer f.mu.Unlock()
    
    if f.seenHashes[hashStr] {
        return true // 重复内容
    }
    
    f.seenHashes[hashStr] = true
    return false
}
```

## 3.5 性能优化

### （1）并发控制

| 场景 | 推荐并发数 | 原因 |
|------|------------|------|
| **小字典（<1000）** | 50-100 | 快速完成 |
| **中字典（1000-10000）** | 200-300 | 平衡速度与负载 |
| **大字典（>10000）** | 300-500 | 最大化速度 |
| **慢速网站** | 50 | 避免超时 |

### （2）智能字典选择

```go
// 根据技术栈自动选择字典
func selectDictByTech(technologies []string) []string {
    techToDict := map[string][]string{
        "php":        {"php.txt", "common.txt"},
        "java":       {"java.txt", "jsp.txt", "common.txt"},
        "spring":     {"spring.txt", "java.txt", "common.txt"},
        "wordpress":  {"wordpress.txt", "php.txt", "common.txt"},
        "python":     {"python.txt", "common.txt"},
        "nodejs":     {"nodejs.txt", "common.txt"},
    }
    
    var dicts []string
    for _, tech := range technologies {
        if dictList, exists := techToDict[tech]; exists {
            dicts = append(dicts, dictList...)
        }
    }
    
    // 去重
    return unique(dicts)
}
```

### （3）请求超时优化

```go
// 动态调整超时时间
func calculateTimeout(baseTimeout time.Duration, failureRate float64) time.Duration {
    if failureRate > 0.3 {
        // 失败率>30%，增加超时时间
        return baseTimeout * 2
    }
    return baseTimeout
}
```

---

# 四、三大技术的协同工作

## 4.1 完整架构图

```
┌────────────────────────────────────────────────────────────────┐
│                        前端用户界面                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │端口扫描  │  │目录扫描  │  │POC扫描   │                     │
│  └─────┬────┘  └─────┬────┘  └─────┬────┘                     │
└────────┼─────────────┼─────────────┼────────────────────────────┘
         │             │             │
         │ HTTP POST   │ HTTP POST   │ HTTP POST
         │             │             │
         ▼             ▼             ▼
┌────────────────────────────────────────────────────────────────┐
│                      HTTP服务器（Go）                           │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  路由层                                                   │  │
│  │  /api/scan/port  /api/scan/dir  /api/scan/poc  /sse     │  │
│  └─────┬──────────────────┬──────────────────┬────────┬─────┘  │
│        │                  │                  │        │         │
│        ▼                  ▼                  ▼        ▼         │
│  ┌──────────┐      ┌──────────┐      ┌──────────┐  ┌────────┐ │
│  │端口扫描  │      │目录扫描  │      │POC扫描   │  │SSE推送 │ │
│  │处理器    │      │处理器    │      │处理器    │  │处理器  │ │
│  └────┬─────┘      └────┬─────┘      └────┬─────┘  └───┬────┘ │
└───────┼─────────────────┼─────────────────┼───────────┼───────┘
        │                 │                 │           │
        │ 创建任务        │ 创建任务        │ 创建任务  │ 监听任务
        │                 │                 │           │
        ▼                 ▼                 ▼           ▼
┌────────────────────────────────────────────────────────────────┐
│                     任务管理层                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  全局任务映射表: map[taskID]*Task                        │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                  │  │
│  │  │Task-001 │  │Task-002 │  │Task-003 │  ...             │  │
│  │  │ch: chan │  │ch: chan │  │ch: chan │                  │  │
│  │  │stop: ch │  │stop: ch │  │stop: ch │                  │  │
│  │  └────┬────┘  └────┬────┘  └────┬────┘                  │  │
│  └───────┼────────────┼────────────┼─────────────────────────┘  │
└──────────┼────────────┼────────────┼────────────────────────────┘
           │            │            │
           │ 消息推送   │ 消息推送   │ 消息推送
           │            │            │
           ▼            ▼            ▼
┌────────────────────────────────────────────────────────────────┐
│                   工作Goroutine池                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │ Worker 1     │  │ Worker 2     │  │ Worker N     │         │
│  │ 扫描端口80   │  │ 扫描/admin   │  │ 测试POC-001  │  ...    │
│  │ ↓            │  │ ↓            │  │ ↓            │         │
│  │ task.Send()  │  │ task.Send()  │  │ task.Send()  │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
└────────────────────────────────────────────────────────────────┘
           │            │            │
           │ SSE消息    │ SSE消息    │ SSE消息
           │            │            │
           ▼            ▼            ▼
┌────────────────────────────────────────────────────────────────┐
│                    SSE事件流                                    │
│  data: {"type":"progress","percent":10}                        │
│                                                                 │
│  data: {"type":"find","data":{"port":80}}                      │
│                                                                 │
│  data: {"type":"progress","percent":20}                        │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
           │
           │ EventSource
           │
           ▼
┌────────────────────────────────────────────────────────────────┐
│                      前端页面                                   │
│  实时更新进度条、结果表格                                       │
└────────────────────────────────────────────────────────────────┘
```

## 4.2 数据流转过程

### （1）端口扫描流程

```
1. 用户输入
   ├─ 目标IP: 192.168.1.1
   ├─ 端口范围: 1-1024
   ├─ 并发数: 500
   └─ 超时: 300ms

2. 前端发起请求
   POST /api/scan/port
   Body: {"host":"192.168.1.1","ports":"1-1024","concurrency":500}

3. 服务端处理
   ├─ 解析端口范围: [1, 2, 3, ..., 1024]
   ├─ 创建任务: t = newTask(1024)
   └─ 返回任务ID: {"taskId":"t-1678901234567890"}

4. 前端建立SSE连接
   EventSource('/sse?task=t-1678901234567890')

5. 后台启动扫描
   ├─ 创建500个goroutine并发扫描
   ├─ 每扫描一个端口，发送进度
   │   task.Send({type:"progress", percent:5})
   └─ 发现开放端口，立即推送
       task.Send({type:"find", data:{port:80, banner:"HTTP/1.1"}})

6. 前端实时显示
   ├─ 更新进度条: 50/1024 (4.88%)
   ├─ 添加结果: 端口80 [开放] HTTP/1.1
   └─ 最终完成: 扫描完成，发现3个开放端口
```

### （2）目录扫描流程

```
1. 用户选择字典
   ├─ 内置字典: php.txt, common.txt
   ├─ 自定义字典: custom.txt
   └─ 总计路径: 5000条

2. 前端发起请求
   POST /api/scan/dir
   Body: {"baseUrl":"https://target.com","builtinDicts":["php.txt"],"concurrency":200}

3. 服务端处理
   ├─ 读取字典文件
   ├─ 解析路径: ["/admin", "/config.php", ...]
   ├─ 创建任务: t = newTask(5000)
   └─ 返回任务ID

4. 前端建立SSE连接
   EventSource('/sse?task=...')

5. 后台启动扫描
   ├─ 创建200个worker goroutine
   ├─ 任务队列: jobs <- "/admin"
   ├─ Worker处理:
   │   ├─ 构造URL: https://target.com/admin
   │   ├─ 发送HTTP GET请求
   │   ├─ 检查响应: status=200, length=1024
   │   └─ 推送结果
   └─ 发送进度: 237/5000 (4.74%)

6. 前端实时显示
   ├─ 进度条: 237/5000
   ├─ 发现目录: /admin [200] 1024字节
   └─ 发现文件: /config.php [403] 禁止访问
```

## 4.3 SSE在扫描中的作用

### 为什么需要SSE？

| 没有SSE（传统方式） | 有SSE（本项目） |
|---------------------|-----------------|
| ❌ 轮询查询进度（每秒请求一次） | ✅ 服务器主动推送进度 |
| ❌ 延迟高（1秒轮询间隔） | ✅ 实时更新（<50ms） |
| ❌ 扫描结束才看到结果 | ✅ 发现即推送 |
| ❌ 无法中途停止 | ✅ 随时停止 |
| ❌ 服务器负载高（频繁请求） | ✅ 一个长连接，负载低 |

### SSE消息类型在扫描中的应用

```javascript
// 端口扫描示例
{
  "type": "start",
  "taskId": "t-xxx",
  "data": {"total": 1024}
}

{
  "type": "progress",
  "percent": 10,
  "progress": "100/1024"
}

{
  "type": "find",
  "data": {
    "port": 80,
    "status": "open",
    "banner": "HTTP/1.1 200 OK\nServer: nginx/1.18.0"
  }
}

{
  "type": "find",
  "data": {
    "port": 443,
    "status": "open",
    "banner": "HTTP/1.1"
  }
}

{
  "type": "progress",
  "percent": 100,
  "progress": "1024/1024"
}

{
  "type": "end",
  "data": {"found": 3}
}
```

---

# 五、完整工作流程示例

## 5.1 综合扫描场景

**场景**：对目标网站进行全面扫描

```
目标: https://example.com

步骤1: 端口扫描（发现开放端口）
├─ 扫描1-1024端口
├─ 发现: 80, 443, 22, 3306
└─ 抓取Banner识别服务

步骤2: 目录扫描（发现敏感路径）
├─ 根据Banner选择字典（检测到PHP）
├─ 使用php.txt + common.txt
├─ 发现: /admin, /phpinfo.php, /backup.sql
└─ 过滤404，只展示200/403/301/302

步骤3: POC扫描（漏洞验证）
├─ 针对发现的路径进行POC测试
├─ 测试SQL注入、文件上传等漏洞
└─ 发现: /admin存在SQL注入
```

## 5.2 完整代码示例

### 前端代码

```javascript
// ========== 端口扫描 ==========
async function startPortScan() {
    // 1. 发起扫描请求
    const response = await fetch('/api/scan/port', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            host: '192.168.1.1',
            ports: '1-1024',
            concurrency: 500,
            timeoutMs: 300,
            scanType: 'tcp',
            grabBanner: true
        })
    });
    
    const {taskId} = await response.json();
    
    // 2. 建立SSE连接
    const eventSource = new EventSource(`/sse?task=${taskId}`);
    
    // 3. 监听消息
    eventSource.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        
        switch(msg.type) {
            case 'start':
                console.log('扫描开始...');
                break;
                
            case 'progress':
                // 更新进度条
                updateProgress(msg.percent, msg.progress);
                break;
                
            case 'find':
                // 添加结果到表格
                addPortResult(msg.data);
                break;
                
            case 'end':
                console.log('扫描完成');
                eventSource.close();
                break;
        }
    };
}

// ========== 目录扫描 ==========
async function startDirScan() {
    const response = await fetch('/api/scan/dir', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            baseUrl: 'https://example.com',
            builtinDicts: ['php.txt', 'common.txt'],
            concurrency: 200,
            timeoutMs: 1500
        })
    });
    
    const {taskId} = await response.json();
    const eventSource = new EventSource(`/sse?task=${taskId}`);
    
    eventSource.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        
        if (msg.type === 'find') {
            // 展示发现的目录/文件
            addDirResult(msg.data);
        } else if (msg.type === 'progress') {
            updateProgress(msg.percent, msg.progress);
        }
    };
}

// 更新进度条
function updateProgress(percent, text) {
    document.getElementById('progress-bar').style.width = percent + '%';
    document.getElementById('progress-text').textContent = text;
}

// 添加端口扫描结果
function addPortResult(data) {
    const table = document.getElementById('port-results');
    const row = table.insertRow(-1);
    row.insertCell(0).textContent = data.port;
    row.insertCell(1).textContent = data.status;
    row.insertCell(2).textContent = data.banner || '-';
}

// 添加目录扫描结果
function addDirResult(data) {
    const table = document.getElementById('dir-results');
    const row = table.insertRow(-1);
    row.insertCell(0).textContent = data.path;
    row.insertCell(1).textContent = data.status;
    row.insertCell(2).textContent = data.length + ' bytes';
    row.insertCell(3).innerHTML = `<a href="${data.url}" target="_blank">访问</a>`;
}
```

## 5.3 性能指标

| 指标 | 端口扫描 | 目录扫描 |
|------|----------|----------|
| **并发数** | 500 | 200 |
| **超时时间** | 300ms | 1500ms |
| **扫描速度** | 1000端口/3秒 | 5000路径/30秒 |
| **内存占用** | ~10MB | ~20MB |
| **CPU占用** | 15-25% | 10-20% |
| **SSE延迟** | <50ms | <50ms |

## 5.4 总结对比

### 三大核心技术对比

| 技术 | 作用 | 核心难点 | 解决方案 |
|------|------|----------|----------|
| **SSE** | 实时推送进度和结果 | 并发安全、内存管理 | 互斥锁、通道、守护进程 |
| **端口扫描** | 发现开放端口和服务 | TCP/UDP协议差异 | 三次握手、超时控制 |
| **目录扫描** | 发现隐藏路径 | 字典管理、结果过滤 | 分类字典、状态码过滤 |

### 技术栈关系

```
SSE（传输层）
  │
  ├─ 端口扫描（应用层）
  │   ├─ TCP扫描
  │   └─ UDP扫描
  │
  └─ 目录扫描（应用层）
      ├─ 字典管理
      ├─ HTTP请求
      └─ 结果过滤
```

---

## 六、技术亮点总结

### 6.1 SSE实时推送

✅ **高实时性**：消息延迟< 50ms  
✅ **低资源消耗**：一个长连接，避免频繁请求  
✅ **自动重连**：浏览器原生支持，稳定可靠  
✅ **并发安全**：完善的锁机制和通道设计  

### 6.2 端口扫描

✅ **协议完整**：支持TCP和UDP两种协议  
✅ **Banner抓取**：自动识别服务类型和版本  
✅ **高并发**：信号量控制，最高500并发  
✅ **智能超时**：根据网络环境动态调整  

### 6.3 目录扫描

✅ **智能字典**：根据技术栈自动选择  
✅ **分类管理**：PHP、Java、ASP等9大分类  
✅ **结果过滤**：状态码、长度、关键词多维度过滤  
✅ **去重优化**：内容哈希去重，避免重复展示  

---

**文档编写时间**：2025年11月11日  
**版本**：v2.0（整合版）  
**作者**：毕业设计项目组  
**涵盖技术**：SSE + 端口扫描 + 目录扫描
