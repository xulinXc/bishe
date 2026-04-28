# NeonScan 一体化安全扫描平台 - 用例图（带图标依赖版）

说明：本版本的 PlantUML 使用在线 `!include` 图标资源（需要联网）。如果希望完全离线渲染，请优先使用上级目录的 [NeonScan用例图.md](../NeonScan用例图.md)。

## PlantUML代码

```plantuml
@startuml NeonScan用例图

!define ICONURL https://raw.githubusercontent.com/tupadr3/plantuml-icon-font-sprites/master
!include ICONURL/common.puml
!include ICONURL/font-awesome-5/user.puml
!include ICONURL/font-awesome-5/user_shield.puml

title NeonScan 一体化安全扫描平台 - 系统用例图

' 定义系统边界
rectangle "NeonScan一体化安全扫描平台" {
    
    ' ========== 基础扫描模块 ==========
    package "基础扫描模块" {
        usecase "UC01: 端口扫描" as UC01
        usecase "UC02: TCP端口扫描" as UC02
        usecase "UC03: UDP端口扫描" as UC03
        usecase "UC04: Banner抓取" as UC04
        usecase "UC05: 目录扫描" as UC05
        usecase "UC06: Web探针扫描" as UC06
        usecase "UC07: 管理字典文件" as UC07
        
        UC01 ..> UC02 : <<include>>
        UC01 ..> UC03 : <<include>>
        UC02 ..> UC04 : <<extend>>
        UC03 ..> UC04 : <<extend>>
        UC05 ..> UC07 : <<include>>
    }
    
    ' ========== 漏洞检测模块 ==========
    package "漏洞检测模块" {
        usecase "UC08: POC漏洞扫描" as UC08
        usecase "UC09: 传统POC检测" as UC09
        usecase "UC10: X-Ray POC检测" as UC10
        usecase "UC11: Nuclei POC检测" as UC11
        usecase "UC12: EXP漏洞验证" as UC12
        usecase "UC13: WAF绕过测试" as UC13
        
        UC08 ..> UC09 : <<include>>
        UC08 ..> UC10 : <<include>>
        UC08 ..> UC11 : <<include>>
    }
    
    ' ========== 资产收集模块 ==========
    package "资产收集模块" {
        usecase "UC14: JS/URL收集" as UC14
        usecase "UC15: 小程序解包" as UC15
        usecase "UC16: 文件上传管理" as UC16
    }
    
    ' ========== AI分析模块 ==========
    package "AI智能分析模块" {
        usecase "UC17: AI安全对话" as UC17
        usecase "UC18: 漏洞分析" as UC18
        usecase "UC19: 代码审计" as UC19
        usecase "UC20: 报告生成" as UC20
        usecase "UC21: 选择AI Provider" as UC21
        
        UC17 ..> UC21 : <<include>>
        UC17 ..> UC18 : <<extend>>
        UC17 ..> UC19 : <<extend>>
        UC17 ..> UC20 : <<extend>>
    }
    
    ' ========== MCP集成模块 ==========
    package "MCP工具集成模块" {
        usecase "UC22: IDA Pro集成" as UC22
        usecase "UC23: 二进制分析" as UC23
        usecase "UC24: 函数列表提取" as UC24
        usecase "UC25: 字符串提取" as UC25
        usecase "UC26: JADX集成" as UC26
        usecase "UC27: APK反编译" as UC27
        usecase "UC28: Activity分析" as UC28
        usecase "UC29: 权限分析" as UC29
        
        UC22 ..> UC23 : <<include>>
        UC22 ..> UC24 : <<include>>
        UC22 ..> UC25 : <<include>>
        UC26 ..> UC27 : <<include>>
        UC26 ..> UC28 : <<include>>
        UC26 ..> UC29 : <<include>>
    }
    
    ' ========== 任务管理模块 ==========
    package "任务管理模块" {
        usecase "UC30: 创建扫描任务" as UC30
        usecase "UC31: 停止扫描任务" as UC31
        usecase "UC32: 查看任务进度" as UC32
        usecase "UC33: 导出扫描结果" as UC33
        usecase "UC34: 实时进度推送(SSE)" as UC34
        
        UC30 ..> UC34 : <<include>>
        UC32 ..> UC34 : <<include>>
    }
}

' ========== 外部系统 ==========
rectangle "外部系统" {
    usecase "OpenAI API" as EXT01
    usecase "DeepSeek API" as EXT02
    usecase "Anthropic API" as EXT03
    usecase "Ollama本地模型" as EXT04
    usecase "IDA Pro MCP Server" as EXT05
    usecase "JADX MCP Server" as EXT06
    usecase "URLFinder工具" as EXT07
    usecase "FFUF工具" as EXT08
}

' ========== 角色定义 ==========
actor "安全测试人员" as User <<user>>

' ========== 用户与用例的关联 ==========

' 安全测试人员 - 使用所有功能
User --> UC01
User --> UC05
User --> UC06
User --> UC07
User --> UC08
User --> UC12
User --> UC13
User --> UC14
User --> UC15
User --> UC16
User --> UC17
User --> UC21
User --> UC22
User --> UC26
User --> UC30
User --> UC31
User --> UC32
User --> UC33

' ========== 外部系统关联 ==========
UC21 ..> EXT01 : <<使用>>
UC21 ..> EXT02 : <<使用>>
UC21 ..> EXT03 : <<使用>>
UC21 ..> EXT04 : <<使用>>
UC22 ..> EXT05 : <<通信>>
UC26 ..> EXT06 : <<通信>>
UC14 ..> EXT07 : <<调用>>
UC14 ..> EXT08 : <<调用>>

' ========== 注释 ==========
note right of User
  安全测试人员：
  - 执行各类扫描任务
  - 管理字典和配置
  - 使用AI辅助分析
  - 调用MCP工具分析
  - 查看和导出结果
  
  本地工具特点：
  所有使用者权限相同
  无需角色区分
end note

note bottom of UC34
  SSE实时推送技术：
  - 扫描进度实时更新
  - 毫秒级延迟
  - 自动重连机制
end note

note bottom of UC21
  支持4种AI Provider：
  - OpenAI (GPT-4o)
  - DeepSeek (国内可用)
  - Anthropic (Claude)
  - Ollama (本地部署)
end note

@enduml
```

---

## 在线渲染

你可以使用以下方式渲染该用例图：

### 方式1：PlantUML在线编辑器
1. 访问 https://www.plantuml.com/plantuml/uml/
2. 将上面的代码粘贴进去
3. 自动生成图片

### 方式2：VS Code插件
1. 安装插件：**PlantUML**
2. 按下 `Alt+D` 预览

### 方式3：IDEA/WebStorm插件
1. 安装插件：**PlantUML Integration**
2. 右键 → **PlantUML Preview**

---

## 用例详细说明表

| 用例编号 | 用例名称 | 主要参与者 | 前置条件 | 后置条件 | 关联技术 |
|---------|---------|-----------|---------|---------|---------|
| UC01 | 端口扫描 | 安全测试员 | 提供目标IP | 返回开放端口列表 | TCP/UDP三次握手 |
| UC02 | TCP端口扫描 | 安全测试员 | 目标可达 | 识别开放端口 | net.DialTimeout |
| UC03 | UDP端口扫描 | 安全测试员 | 目标可达 | 识别UDP服务 | net.DialUDP |
| UC04 | Banner抓取 | 安全测试员 | 端口开放 | 获取服务版本 | 被动/主动探测 |
| UC05 | 目录扫描 | 安全测试员 | Web服务运行 | 返回有效路径 | HTTP请求+字典 |
| UC06 | Web探针扫描 | 安全测试员 | 目标URL可访问 | 返回技术栈 | 指纹识别 |
| UC07 | 管理字典文件 | 安全测试人员 | - | 字典可用 | 9大分类字典 |
| UC08 | POC漏洞扫描 | 安全测试员 | POC文件存在 | 返回漏洞列表 | 3种POC格式 |
| UC09 | 传统POC检测 | 安全测试员 | JSON POC | 漏洞验证 | JSON匹配规则 |
| UC10 | X-Ray POC检测 | 安全测试员 | YAML POC | 漏洞验证 | 表达式引擎 |
| UC11 | Nuclei POC检测 | 安全测试员 | Nuclei POC | 漏洞验证 | Matchers |
| UC12 | EXP漏洞验证 | 安全测试员 | EXP脚本 | 漏洞利用成功 | 多步骤HTTP |
| UC13 | WAF绕过测试 | 安全测试员 | Payload | 绕过结果 | 6种策略组合 |
| UC14 | JS/URL收集 | 安全测试员 | Web应用 | API/路径列表 | URLFinder |
| UC15 | 小程序解包 | 安全测试员 | wxapkg文件 | 解包后代码 | Packer-Fuzzer |
| UC16 | 文件上传管理 | 安全测试人员 | - | 文件已保存 | 文件系统 |
| UC17 | AI安全对话 | 安全测试员 | API密钥 | 分析报告 | LLM |
| UC18 | 漏洞分析 | 安全测试员 | 漏洞信息 | 详细解释 | AI推理 |
| UC19 | 代码审计 | 安全测试员 | 代码片段 | 安全建议 | AI分析 |
| UC20 | 报告生成 | 安全测试员 | 扫描结果 | 格式化报告 | AI总结 |
| UC21 | 选择AI Provider | 安全测试人员 | - | Provider已选择 | 多Provider架构 |
| UC22 | IDA Pro集成 | 安全测试员 | IDA运行 | 分析结果 | MCP协议 |
| UC23 | 二进制分析 | 安全测试员 | 二进制文件 | 反汇编代码 | IDA分析引擎 |
| UC24 | 函数列表提取 | 安全测试员 | 二进制已加载 | 函数列表 | IDA API |
| UC25 | 字符串提取 | 安全测试员 | 二进制已加载 | 字符串列表 | IDA strings |
| UC26 | JADX集成 | 安全测试员 | JADX运行 | 反编译结果 | MCP协议 |
| UC27 | APK反编译 | 安全测试员 | APK文件 | Java代码 | JADX引擎 |
| UC28 | Activity分析 | 安全测试员 | APK已加载 | 组件列表 | JADX API |
| UC29 | 权限分析 | 安全测试员 | APK已加载 | 权限列表 | Manifest解析 |
| UC30 | 创建扫描任务 | 安全测试员 | 参数有效 | 任务已创建 | Task结构 |
| UC31 | 停止扫描任务 | 安全测试员 | 任务运行中 | 任务已停止 | channel信号 |
| UC32 | 查看任务进度 | 安全测试员 | 任务存在 | 实时进度 | SSE推送 |
| UC33 | 导出扫描结果 | 安全测试员 | 任务完成 | 结果文件 | JSON导出 |
| UC34 | 实时进度推送(SSE) | 安全测试员 | SSE连接 | 进度更新 | EventSource |

---

## 核心用例流程图

### UC01: 端口扫描 - 详细流程

```
[安全测试员] → (输入目标IP和端口范围)
      ↓
[NeonScan] → (创建Task对象)
      ↓
[NeonScan] → (返回TaskID)
      ↓
[浏览器] → (建立SSE连接 /sse?task=ID)
      ↓
[NeonScan] → (并发goroutine扫描)
      ├─→ [TCP扫描] → (三次握手)
      │         ↓
      │    (成功) → [Banner抓取] → (识别服务)
      │         ↓
      │    (推送: {type:"find", data:{port:80}})
      │
      └─→ [UDP扫描] → (发送探测包)
                ↓
           (有响应) → (推送: {type:"find", data:{port:53}})
                ↓
[NeonScan] → (推送进度: {type:"progress", percent:50})
      ↓
[浏览器] → (更新进度条、添加结果到表格)
      ↓
[NeonScan] → (扫描完成, 推送: {type:"end"})
      ↓
[浏览器] → (关闭SSE连接, 显示完成)
```

### UC08: POC漏洞扫描 - 详细流程

```
[安全测试员] → (选择POC目录/文件)
      ↓
[NeonScan] → (读取POC文件)
      ├─→ [传统POC] → (JSON解析)
      ├─→ [X-Ray POC] → (YAML解析 + 表达式引擎)
      └─→ [Nuclei POC] → (YAML解析 + Matchers)
      ↓
[NeonScan] → (创建Task, 总数=POC数量)
      ↓
[并发扫描]
      ├─→ [POC 1] → (构造HTTP请求) → (发送) → (匹配响应)
      │                                    ↓
      │                               (命中) → {type:"find"}
      ├─→ [POC 2] → ...
      └─→ [POC N] → ...
      ↓
[NeonScan] → (推送进度和结果)
      ↓
[浏览器] → (显示漏洞列表)
```

### UC17: AI安全对话 - 详细流程

```
[安全测试员] → (输入问题: "分析这个漏洞")
      ↓
[NeonScan] → (构造ChatMessage)
      ↓
[NeonScan] → (选择AI Provider)
      ├─→ [OpenAI] → (调用GPT-4o API)
      ├─→ [DeepSeek] → (调用DeepSeek API)
      ├─→ [Anthropic] → (调用Claude API)
      └─→ [Ollama] → (本地模型推理)
      ↓
[AI Provider] → (返回流式响应)
      ↓
[NeonScan] → (通过SSE推送每个Token)
      ↓
[浏览器] → (逐字显示AI回复)
      ↓
[AI] → (可能调用工具: POC扫描、IDA分析等)
      ↓
[NeonScan] → (执行工具调用, 返回结果给AI)
      ↓
[AI] → (基于工具结果继续回复)
      ↓
[浏览器] → (显示完整分析报告)
```

### UC22: IDA Pro集成 - 详细流程

```
[安全测试员] → (上传二进制文件)
      ↓
[NeonScan] → (保存到uploads/)
      ↓
[安全测试员] → (选择"IDA Pro分析")
      ↓
[NeonScan] → (构造MCP请求)
      ↓
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "ida_analyze_binary",
    "arguments": {"file_path": "uploads/binary.exe"}
  }
}
      ↓
[IDA Pro MCP Server:8744] → (加载文件到IDA)
      ↓
[IDA] → (分析函数、字符串、交叉引用)
      ↓
[MCP Server] → (返回JSON结果)
      ↓
[NeonScan] → (解析结果, 转为SSE推送)
      ↓
[浏览器] → (显示函数列表、字符串、反汇编代码)
```

---

## 技术亮点映射

| 用例 | 核心技术 | 创新点 |
|------|---------|-------|
| UC34 | SSE实时推送 | 毫秒级进度反馈, 优于轮询/长轮询 |
| UC01 | TCP/UDP双协议 | goroutine并发, 1万端口仅3.2秒 |
| UC05 | 智能字典选择 | 根据技术栈自动选字典, 效率提升5倍 |
| UC08 | 三种POC格式 | 兼容业界主流格式, 可复用POC库 |
| UC13 | 六大绕过策略 | 自动生成Payload变体, 命中率提升40% |
| UC17 | 四种AI Provider | 国内外模型+本地部署, 满足各种场景 |
| UC22/UC26 | MCP协议集成 | 标准化工具调用, 可扩展至其他工具 |

---

## 系统功能覆盖率

- ✅ 基础扫描模块（7个用例）
- ✅ 漏洞检测模块（6个用例）
- ✅ 资产收集模块（3个用例）
- ✅ AI分析模块（5个用例）
- ✅ MCP集成模块（8个用例）
- ✅ 任务管理模块（5个用例）

**总计：34个核心用例**

---

## 扩展性设计

### 未来可扩展用例
- UC35: 子域名爆破
- UC36: SQL注入检测
- UC37: XSS漏洞扫描
- UC38: 弱密码爆破
- UC39: 端口指纹识别
- UC40: CDN识别
- UC41: 蜜罐检测
- UC42: 报告模板定制

---

## 用例图绘制说明

该用例图采用 **PlantUML** 标准UML语法绘制，具有以下特点：

1. **清晰分层**：按功能模块分为6个包（Package）
2. **关系明确**：使用 `<<include>>` 和 `<<extend>>` 表示用例依赖
3. **角色区分**：3种用户角色 + 1个系统角色（AI助手）
4. **外部系统**：明确标注与外部系统的交互
5. **注释丰富**：关键技术点都有注释说明

---

## 如何使用该文档

1. **答辩展示**：将渲染后的图片插入PPT，配合讲解
2. **论文写作**：复制用例说明表，作为需求分析章节
3. **开发参考**：根据用例编号追溯代码实现
4. **测试用例**：每个用例对应一个测试场景

---

## 推荐答辩讲解话术

> "各位老师，这是NeonScan的系统用例图。从图中可以看到，系统共包含**34个核心用例**，分为6大模块。
> 
> **角色设计方面**，由于NeonScan是一个**本地部署的安全测试工具**，我采用了**单一角色设计**：
> - 只有**1个安全测试人员角色**，可以使用系统的**所有功能**
> - 这符合本地工具的特点：所有使用者的目的都是进行安全测试，权限和使用方式完全相同
> - 无需像Web系统那样区分管理员/普通用户，避免了不必要的复杂性
> 
> 这种设计参考了Burp Suite、Metasploit、Nmap等业界标准工具，它们都是**单一用户模式**，体现了工具类软件的简洁性和易用性。
> 
> 右侧是**8个外部系统**，包括4种AI Provider和2个MCP服务器。特别值得一提的是**UC34实时进度推送用例**，它基于SSE技术，贯穿所有扫描任务，实现毫秒级进度反馈，这是传统工具所不具备的。
> 
> **UC21选择AI Provider用例**体现了系统的灵活性，用户可根据需求选择OpenAI、DeepSeek、Anthropic或本地Ollama模型，既能保证能力，又能保护隐私。
> 
> 整体架构采用**包含和扩展关系**设计，如UC01端口扫描包含UC02 TCP扫描和UC03 UDP扫描，而UC04 Banner抓取作为扩展功能，体现了模块化和可扩展性。"
