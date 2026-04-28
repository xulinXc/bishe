# NeonScan 功能流程图

本文档包含NeonScan系统的核心功能流程图，使用PlantUML绘制。

---

## 1. 系统整体功能流程图

```plantuml
@startuml NeonScan整体流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E8F5E9
skinparam activityBorderColor #4CAF50
skinparam activityBarColor #FF9800
skinparam activityStartColor #4CAF50
skinparam activityEndColor #F44336
skinparam activityDiamondBackgroundColor #FFF9C4
skinparam activityDiamondBorderColor #FBC02D

title NeonScan 系统整体功能流程图

start

:用户访问 http://localhost:8080;

:浏览功能模块列表;
note right
  6大功能模块：
  • 基础扫描（端口/目录/Web探针）
  • 漏洞检测（POC/EXP/WAF绕过）
  • 资产收集（JS资产收集）
  • AI分析（对话/自动扫描）
  • 工具集成（IDA/JADX MCP）
  • 扫描报告
end note

if (选择哪类功能?) then (扫描类任务)
    
    partition "**扫描任务流程**" #LightBlue {
        :填写扫描参数;
        note right
          示例：端口扫描
          - 目标: 192.168.1.1
          - 端口: 1-65535
          - 并发: 500
        end note
        
        :提交扫描请求;
        :后端创建Task并生成TaskID;
        :前端收到TaskID;
        
        fork
            :建立SSE连接
GET /events?task={taskID};
            
            repeat
                :接收实时消息;
                
                if (消息类型?) then (进度更新)
                    #B3E5FC:更新进度条;
                elseif (发现结果)
                    #C8E6C9:添加到结果表;
                elseif (扫描完成)
                    #FFCCBC:显示完成提示;
                    :关闭SSE连接;
                    detach
                endif
                
            repeat while (连接活跃?)
            
        fork again
            :后端goroutine池
并发执行扫描;
            note right
              Worker 1, 2, 3...N
              并发处理任务
            end note
            
            :实时推送结果到SSE;
            :扫描完成后推送end消息;
        end fork
        
        :查看扫描结果;
        
        if (后续操作?) then (导出报告)
            :下载JSON文件;
        elseif (AI分析)
            :跳转AI对话页面;
            goto ai_label;
        elseif (查看统一报告)
            :打开扫描报告页面;
        endif
    }
    
elseif (AI功能) then
    label ai_label
    
    partition "**AI分析流程**" #LightYellow {
        if (AI功能类型?) then (AI对话)
            :输入安全问题或扫描结果;
            :调用AI API;
            :流式返回AI分析;
            :实时显示AI回复;
            
        else (AI自动扫描)
            :输入目标URL;
            :AI自动制定扫描计划;
            
            fork
                :AI调用端口扫描;
            fork again
                :AI调用Web探针;
            fork again
                :AI调用POC检测;
            end fork
            
            :AI实时分析结果;
            :生成完整分析报告;
            :查看AI报告;
        endif
    }
    
elseif (MCP工具集成) then
    
    partition "**MCP集成流程**" #LightPink {
        :上传二进制/APK文件;
        
        if (选择工具?) then (IDA Pro)
            :建立IDA MCP Session;
            :反汇编分析;
        else (JADX)
            :建立JADX MCP Session;
            :反编译APK;
        endif
        
        :与MCP Server对话;
        :查看分析结果;
    }
    
else (其他功能)
    
    partition "**辅助功能**" #LightGreen {
        if (功能类型?) then (文件上传)
            :上传POC/EXP文件;
            :保存到uploads目录;
            
        else (查看报告)
            :打开扫描报告页面;
            :查看所有模块汇总;
            :导出完整报告;
        endif
    }
endif

if (继续使用?) then (是)
    :返回首页;
    detach
else (否)
    stop
endif

@enduml
```

---

## 2. 端口扫描详细流程图

```plantuml
@startuml 端口扫描流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E3F2FD
skinparam activityBorderColor #2196F3

title NeonScan - 端口扫描详细流程图

start

:用户输入扫描参数;
note right
  - 目标IP: 192.168.1.1
  - 端口范围: 1-65535
  - 扫描类型: TCP/UDP/全部
  - 并发数: 100
end note

:前端发送POST请求;
note right
  POST /api/portscan
  {
    "target": "192.168.1.1",
    "ports": "1-65535",
    "scan_type": "tcp",
    "concurrency": 100
  }
end note

:后端创建Task对象;
:生成唯一TaskID;
:返回TaskID给前端;

fork
    :前端建立SSE连接;
    note right
      GET /sse?task={taskID}
    end note
    
    :监听SSE事件;
    
    repeat
        :接收SSE消息;
        
        if (消息类型?) then (progress)
            :更新进度条;
        elseif (find) then
            #LightGreen:添加到结果表格;
            note right
              端口: 80
              服务: HTTP
              Banner: nginx/1.18
            end note
        elseif (end) then
            :显示扫描完成;
            :关闭SSE连接;
        else (error)
            #LightCoral:显示错误信息;
        endif
        
    repeat while (未收到end消息?)
    
fork again
    partition "后端扫描逻辑" {
        :初始化端口队列;
        :创建并发goroutine池;
        
        repeat
            :从队列取出一个端口;
            
            if (扫描类型?) then (TCP)
                :TCP三次握手连接;
                
                if (连接成功?) then (是)
                    #LightGreen:记录端口开放;
                    :尝试Banner抓取;
                    
                    if (有Banner?) then (是)
                        :识别服务类型;
                        note right
                          HTTP, FTP, SSH,
                          MySQL, Redis...
                        end note
                    else (否)
                    endif
                    
                    :推送结果到SSE;
                else (否)
                    :标记端口关闭;
                endif
                
            elseif (UDP) then
                :发送UDP探测包;
                :等待响应(超时1秒);
                
                if (有响应?) then (是)
                    #LightGreen:记录端口开放;
                    :推送结果到SSE;
                else (否)
                    :标记端口关闭/过滤;
                endif
                
            else (全部)
                :依次执行TCP和UDP;
            endif
            
            :更新扫描进度;
            :推送进度到SSE;
            
        repeat while (还有端口待扫描?)
        
        :扫描完成;
        :推送end消息到SSE;
    }
end fork

:用户查看扫描结果;

if (需要停止扫描?) then (是)
    :点击停止按钮;
    :发送停止请求;
    :后端取消goroutine;
    :推送end消息;
else (否)
endif

:导出扫描结果;

stop

@enduml
```

---

## 3. POC漏洞扫描流程图

```plantuml
@startuml POC漏洞扫描流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #FFEBEE
skinparam activityBorderColor #F44336

title NeonScan - POC漏洞扫描详细流程图

start

:用户选择POC扫描;

partition "POC准备" {
    :选择POC来源;
    
    if (POC来源?) then (本地目录)
        :浏览本地POC目录;
        :选择POC文件/文件夹;
    elseif (单个POC) then
        :上传单个POC文件;
    else (在线POC库)
        :从POC库选择;
    endif
    
    :后端加载POC文件;
    
    :识别POC格式;
    note right
      - 传统JSON格式
      - X-Ray YAML格式
      - Nuclei YAML格式
    end note
}

partition "POC解析" {
    if (POC格式?) then (传统JSON)
        :解析JSON结构;
        :提取name, method, path;
        :提取headers, body;
        :提取匹配规则;
        
    elseif (X-Ray YAML) then
        :解析YAML结构;
        :初始化表达式引擎;
        :解析rules规则;
        :解析expression表达式;
        
    else (Nuclei YAML)
        :解析YAML结构;
        :提取requests模板;
        :提取matchers匹配器;
        :提取extractors提取器;
    endif
}

:输入目标URL;
note right
  单个: http://example.com
  批量: 上传URL列表
end note

:创建扫描任务;
:计算总任务数 = POC数 × URL数;

fork
    :建立SSE连接;
    :实时接收扫描结果;
    
fork again
    partition "后端并发扫描" {
        :创建goroutine池;
        
        repeat
            :取出一个POC和URL组合;
            
            :构造HTTP请求;
            note right
              根据POC定义:
              - 设置Method
              - 设置Headers
              - 设置Body
              - 设置超时时间
            end note
            
            :发送HTTP请求;
            
            if (请求成功?) then (是)
                :获取响应;
                
                partition "漏洞匹配" {
                    if (POC类型?) then (传统)
                        :检查响应状态码;
                        :检查响应头;
                        :检查响应体关键字;
                        
                    elseif (X-Ray) then
                        :执行expression表达式;
                        :r1 = 响应包含特征;
                        :r2 = 状态码匹配;
                        :计算: r1() && r2();
                        
                    else (Nuclei)
                        :执行matchers匹配;
                        :检查word匹配;
                        :检查regex匹配;
                        :检查status匹配;
                        :检查dsl匹配;
                    endif
                    
                    if (匹配成功?) then (是)
                        #LightCoral:**发现漏洞!**;
                        :记录漏洞详情;
                        note right
                          - POC名称
                          - 漏洞类型
                          - 危险等级
                          - 匹配内容
                          - 请求/响应
                        end note
                        :推送到SSE;
                    else (否)
                        :记录无漏洞;
                    endif
                }
                
            else (否)
                :记录请求失败;
            endif
            
            :更新进度;
            :推送进度到SSE;
            
        repeat while (还有POC待测试?)
        
        :扫描完成;
        :推送end消息;
    }
end fork

:查看漏洞列表;

if (发现漏洞?) then (是)
    :查看漏洞详情;
    
    if (需要EXP验证?) then (是)
        :加载对应EXP;
        :执行漏洞利用;
        :查看利用结果;
    else (否)
    endif
    
    if (需要AI分析?) then (是)
        :发送漏洞信息到AI;
        :获取修复建议;
    else (否)
    endif
    
else (否)
    :显示无漏洞;
endif

:导出扫描报告;

stop

@enduml
```

---

## 4. AI安全对话流程图

```plantuml
@startuml AI安全对话流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #FFF9C4
skinparam activityBorderColor #FBC02D

title NeonScan - AI安全对话详细流程图

start

:用户进入AI对话页面;

partition "AI配置检查" {
    if (已配置AI Provider?) then (否)
        :显示配置页面;
        :选择AI Provider;
        note right
          - OpenAI (GPT-4o)
          - DeepSeek
          - Anthropic (Claude)
          - Ollama (本地)
        end note
        :输入API密钥;
        :保存配置;
    else (是)
    endif
}

:用户输入问题;
note right
  示例问题:
  - "分析这个SQL注入漏洞"
  - "帮我审计这段代码"
  - "解释这个POC的原理"
  - "用IDA分析这个二进制"
end note

:前端发送聊天请求;
note right
  POST /api/ai/chat
  {
    "message": "分析漏洞",
    "provider": "openai",
    "context": {...}
  }
end note

fork
    :建立SSE连接接收回复;
    
    repeat
        :接收SSE消息;
        
        if (消息类型?) then (token)
            :逐字显示AI回复;
            
        elseif (tool_call) then
            #LightBlue:显示工具调用状态;
            note right
              "AI正在调用工具:
              POC扫描..."
            end note
            
        elseif (tool_result) then
            :显示工具返回结果;
            
        else (end)
            :对话完成;
        endif
        
    repeat while (未收到end?)
    
fork again
    partition "后端AI处理" {
        :构造ChatMessage;
        :添加系统提示词;
        note right
          你是安全测试专家,
          可以使用工具:
          - POC扫描
          - 端口扫描
          - IDA分析
          - JADX分析
        end note
        
        :调用AI Provider API;
        
        if (Provider类型?) then (OpenAI)
            :调用OpenAI API;
            :gpt-4o模型;
            
        elseif (DeepSeek) then
            :调用DeepSeek API;
            :deepseek-chat模型;
            
        elseif (Anthropic) then
            :调用Anthropic API;
            :claude-3.5-sonnet;
            
        else (Ollama)
            :调用本地Ollama;
            :qwen2.5:7b模型;
        endif
        
        :获取流式响应;
        
        repeat
            :接收Token;
            :推送Token到SSE;
            
            if (检测到工具调用?) then (是)
                :暂停Token推送;
                :解析工具调用参数;
                
                if (工具类型?) then (poc_scan)
                    :执行POC扫描;
                    :获取扫描结果;
                    
                elseif (port_scan) then
                    :执行端口扫描;
                    :获取扫描结果;
                    
                elseif (ida_analyze) then
                    :调用IDA MCP服务;
                    :获取分析结果;
                    
                elseif (jadx_decompile) then
                    :调用JADX MCP服务;
                    :获取反编译结果;
                    
                else (code_audit)
                    :执行代码审计;
                    :获取审计报告;
                endif
                
                :推送工具调用状态;
                :推送工具返回结果;
                
                :将结果返回给AI;
                :AI基于结果继续回复;
                
            else (否)
            endif
            
        repeat while (还有Token?)
        
        :推送end消息;
    }
end fork

:查看完整对话;

if (需要导出对话?) then (是)
    :导出Markdown格式;
    :保存到本地;
else (否)
endif

if (需要继续对话?) then (是)
    :输入新问题;
    backward:添加到对话历史;
else (否)
endif

stop

@enduml
```

---

## 5. MCP工具集成流程图

```plantuml
@startuml MCP工具集成流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #F3E5F5
skinparam activityBorderColor #9C27B0

title NeonScan - MCP工具集成详细流程图

start

:用户选择MCP工具;

partition "MCP服务检查" {
    if (MCP服务运行?) then (否)
        #LightCoral:显示错误提示;
        note right
          请先启动MCP服务:
          - IDA Pro: 端口8744
          - JADX: 端口8745
        end note
        :提供启动命令;
        stop
    else (是)
        #LightGreen:服务正常;
    endif
}

if (选择哪个工具?) then (IDA Pro)
    partition "IDA Pro分析流程" {
        :上传二进制文件;
        note right
          支持格式:
          - EXE, DLL
          - ELF, SO
          - Mach-O
        end note
        
        :保存到uploads/目录;
        :选择分析功能;
        
        if (分析类型?) then (基础分析)
            :获取文件基本信息;
            
        elseif (函数列表) then
            :提取所有函数;
            :显示函数名、地址;
            
        elseif (字符串提取) then
            :提取所有字符串;
            :过滤敏感信息;
            
        elseif (反汇编) then
            :指定地址或函数;
            :获取汇编代码;
            
        else (交叉引用)
            :分析函数调用关系;
            :生成调用图;
        endif
        
        :构造MCP请求;
        note right
          {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
              "name": "ida_analyze",
              "arguments": {
                "file": "uploads/app.exe",
                "type": "functions"
              }
            }
          }
        end note
        
        :发送到IDA MCP Server:8744;
        :IDA加载文件并分析;
        :MCP Server返回JSON结果;
        
        :解析MCP响应;
        :格式化显示结果;
    }
    
elseif (JADX) then
    partition "JADX反编译流程" {
        :上传APK文件;
        :保存到uploads/目录;
        :选择分析功能;
        
        if (分析类型?) then (反编译)
            :反编译整个APK;
            :获取Java源码;
            
        elseif (Activity分析) then
            :提取所有Activity;
            :分析启动模式;
            
        elseif (Service分析) then
            :提取所有Service;
            :分析后台服务;
            
        elseif (权限分析) then
            :解析AndroidManifest.xml;
            :列出所有权限;
            :标注危险权限;
            
        else (资源分析)
            :提取资源文件;
            :分析布局、字符串;
        endif
        
        :构造MCP请求;
        note right
          {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
              "name": "jadx_decompile",
              "arguments": {
                "apk": "uploads/app.apk",
                "type": "activities"
              }
            }
          }
        end note
        
        :发送到JADX MCP Server:8745;
        :JADX加载APK并分析;
        :MCP Server返回JSON结果;
        
        :解析MCP响应;
        :格式化显示结果;
    }
    
else (其他工具)
    :待扩展;
endif

:查看分析结果;

if (需要AI辅助?) then (是)
    :发送分析结果到AI;
    :输入问题;
    note right
      - "分析这个函数的作用"
      - "这个Activity有什么安全问题?"
      - "解释这段汇编代码"
    end note
    :AI基于MCP结果回复;
else (否)
endif

:导出分析报告;

stop

@enduml
```

---

## 6. SSE实时推送机制流程图

```plantuml
@startuml SSE实时推送机制

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E0F2F1
skinparam activityBorderColor #009688

title NeonScan - SSE实时推送机制详细流程图

|前端|
start

:用户发起扫描请求;
:获取TaskID;

:建立SSE连接;
note right
  const eventSource = new EventSource(
    '/sse?task=' + taskID
  );
end note

:注册事件监听器;
note right
  eventSource.onmessage = (e) => {
    const data = JSON.parse(e.data);
    handleSSEMessage(data);
  };
end note

|后端|

:接收SSE连接请求;
:验证TaskID;

if (Task存在?) then (否)
    :返回404错误;
    |前端|
    :显示错误;
    stop
else (是)
endif

:设置响应头;
note right
  Content-Type: text/event-stream
  Cache-Control: no-cache
  Connection: keep-alive
end note

:创建消息通道;
note right
  msgChan := make(chan SSEMessage)
  task.SetChannel(msgChan)
end note

fork
    |后端扫描线程|
    partition "扫描任务执行" {
        :开始扫描;
        
        repeat
            :执行扫描逻辑;
            
            if (发现结果?) then (是)
                :构造消息;
                note right
                  {
                    "type": "find",
                    "data": {
                      "port": 80,
                      "service": "HTTP"
                    }
                  }
                end note
                :发送到msgChan;
            endif
            
            :计算进度;
            :构造进度消息;
            note right
              {
                "type": "progress",
                "percent": 45.2,
                "current": 452,
                "total": 1000
              }
            end note
            :发送到msgChan;
            
        repeat while (未完成?)
        
        :构造完成消息;
        note right
          {
            "type": "end",
            "message": "扫描完成"
          }
        end note
        :发送到msgChan;
        :关闭channel;
    }
    
fork again
    |后端SSE推送线程|
    partition "SSE消息推送" {
        repeat
            :从msgChan接收消息;
            
            :格式化SSE消息;
            note right
              data: {"type":"find",...}\n\n
            end note
            
            :写入HTTP响应流;
            :Flush立即发送;
            
            if (连接断开?) then (是)
                :清理资源;
                :退出循环;
            endif
            
        repeat while (channel未关闭?)
    }
end fork

|前端|

repeat
    :接收SSE消息;
    :解析JSON数据;
    
    if (消息类型?) then (progress)
        :更新进度条;
        :显示进度百分比;
        
    elseif (find) then
        #LightGreen:添加结果到表格;
        :播放提示音;
        
    elseif (error) then
        #LightCoral:显示错误提示;
        
    elseif (end) then
        :显示完成状态;
        :关闭SSE连接;
        note right
          eventSource.close();
        end note
    endif
    
repeat while (未收到end?)

:显示最终结果;

stop

@enduml
```

---

## 7. 目录扫描流程图

```plantuml
@startuml 目录扫描流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E8F5E9
skinparam activityBorderColor #4CAF50

title NeonScan - 目录扫描详细流程图

start

:用户输入目标URL;
note right
  示例: http://example.com
end note

:选择扫描策略;

if (字典选择?) then (自动)
    :后端识别技术栈;
    note right
      通过指纹识别:
      - PHP → php.txt
      - JSP → jsp.txt
      - ASP → asp.txt
      - 通用 → common.txt
    end note
    :自动选择对应字典;
    
elseif (手动) then
    :用户手动选择字典;
    note right
      可用字典:
      - common.txt (通用)
      - php.txt
      - jsp.txt
      - asp.txt
      - backup.txt (备份)
      - admin.txt (后台)
      - api.txt
      - .git目录
      - 敏感文件
    end note
    
else (自定义)
    :上传自定义字典;
endif

:设置扫描参数;
note right
  - 并发数: 50
  - 超时时间: 5s
  - 跟随重定向: 是
  - 过滤状态码: 404,403
end note

:创建扫描任务;

fork
    :建立SSE连接;
    :实时接收结果;
    
fork again
    partition "后端扫描逻辑" {
        :加载字典文件;
        :读取所有路径;
        note right
          /admin
          /login.php
          /api/v1
          /backup.zip
          ...
        end note
        
        :创建goroutine池;
        
        repeat
            :取出一个路径;
            :拼接完整URL;
            note right
              http://example.com/admin
            end note
            
            :发送HTTP GET请求;
            :设置超时5秒;
            
            if (请求成功?) then (是)
                :获取响应;
                
                if (状态码?) then (200 OK)
                    #LightGreen:**发现有效路径!**;
                    :记录路径信息;
                    note right
                      - 路径: /admin
                      - 状态码: 200
                      - 大小: 1234 bytes
                      - 标题: 后台登录
                    end note
                    :推送到SSE;
                    
                elseif (301/302) then
                    if (跟随重定向?) then (是)
                        :跳转到新URL;
                        :记录重定向信息;
                        :推送到SSE;
                    else (否)
                        :记录重定向;
                    endif
                    
                elseif (403 Forbidden) then
                    if (过滤403?) then (否)
                        :记录禁止访问;
                        :推送到SSE;
                    endif
                    
                elseif (401) then
                    #LightYellow:需要认证;
                    :记录需认证路径;
                    :推送到SSE;
                    
                else (404/其他)
                    :路径不存在;
                    :不推送;
                endif
                
            else (否)
                :请求失败/超时;
            endif
            
            :更新进度;
            :推送进度到SSE;
            
        repeat while (还有路径?)
        
        :扫描完成;
        :推送end消息;
    }
end fork

:查看发现的路径;

if (发现敏感路径?) then (是)
    #LightCoral:高亮显示;
    note right
      敏感路径:
      - /admin
      - /phpmyadmin
      - /backup.zip
      - /.git/config
    end note
    
    if (需要深度扫描?) then (是)
        :对敏感路径递归扫描;
        note right
          如发现 /admin
          继续扫描:
          /admin/login.php
          /admin/config.php
          ...
        end note
    else (否)
    endif
else (否)
endif

:导出扫描结果;

stop

@enduml
```

---

## 使用说明

### 在线渲染这些流程图

#### 方式1: PlantUML在线编辑器
1. 访问 https://www.plantuml.com/plantuml/uml/
2. 复制上面任意一个流程图代码
3. 粘贴后自动生成图片
4. 右键保存图片

#### 方式2: VS Code插件
```bash
# 1. 安装插件
搜索并安装 "PlantUML"

# 2. 预览
打开本文件
按 Alt+D 预览
```

#### 方式3: 导出图片
```bash
# 安装PlantUML
java -jar plantuml.jar NeonScan功能流程图.md

# 批量生成PNG图片
```

---

## 流程图说明

### 1. 系统整体流程图
- **用途**: 展示NeonScan的整体功能架构
- **适用场景**: 答辩开场、论文系统设计章节
- **核心要素**: 6大功能模块、任务执行流程、结果处理

### 2. 端口扫描流程图
- **用途**: 详细展示端口扫描的完整流程
- **技术亮点**: SSE实时推送、TCP/UDP双协议、Banner抓取
- **适用场景**: 技术实现章节、核心功能讲解

### 3. POC漏洞扫描流程图
- **用途**: 展示3种POC格式的解析和匹配流程
- **技术亮点**: 支持传统/X-Ray/Nuclei格式、智能匹配引擎
- **适用场景**: 创新点讲解、漏洞检测模块说明

### 4. AI安全对话流程图
- **用途**: 展示AI集成和工具调用流程
- **技术亮点**: 多Provider支持、工具调用机制、流式推送
- **适用场景**: AI模块讲解、创新点展示

### 5. MCP工具集成流程图
- **用途**: 展示IDA Pro和JADX集成的详细流程
- **技术亮点**: MCP协议、专业工具联动、二进制/APK分析
- **适用场景**: 高级功能讲解、扩展性展示

### 6. SSE实时推送机制流程图
- **用途**: 深入展示SSE的前后端交互细节
- **技术亮点**: 双向通信、goroutine并发、实时更新
- **适用场景**: 技术实现细节、性能优化讲解

### 7. 目录扫描流程图
- **用途**: 展示智能字典选择和目录扫描流程
- **技术亮点**: 自动识别技术栈、9大分类字典、递归扫描
- **适用场景**: Web扫描模块讲解

---

## 答辩展示建议

### PPT使用方案

```
第1页: 标题页
  "NeonScan - AI驱动的安全测试工具"

第2页: 系统整体流程图
  - 展示6大功能模块
  - 讲解整体架构设计

第3页: 核心技术 - SSE实时推送
  - 使用SSE实时推送流程图
  - 对比传统轮询方式
  - 强调技术优势

第4页: 基础扫描功能
  - 端口扫描流程图
  - 目录扫描流程图
  - 展示并发性能

第5页: 漏洞检测功能
  - POC漏洞扫描流程图
  - 强调3种POC格式支持
  - 展示漏洞匹配引擎

第6页: AI智能分析
  - AI安全对话流程图
  - 展示工具调用机制
  - 强调4种Provider支持

第7页: MCP工具集成
  - MCP工具集成流程图
  - 展示专业工具联动
  - 体现扩展性设计

第8页: 技术总结
  - 核心技术对比表
  - 创新点汇总
```

### 讲解话术模板

#### 开场（系统整体流程图）
> "各位老师，这是NeonScan的系统整体流程图。用户启动工具后，可以选择6大功能模块：端口扫描、目录扫描、POC漏洞扫描、AI安全对话、MCP工具集成和Web探针。每个功能都基于**任务-SSE推送-结果处理**的统一架构，保证了系统的一致性和可维护性。"

#### 技术亮点（SSE流程图）
> "这张图展示了SSE实时推送的完整机制。前端建立EventSource连接后，后端通过goroutine并发执行扫描任务，扫描结果通过channel传递到SSE推送线程，实现**毫秒级**的进度更新。相比传统的轮询方式，SSE减少了90%的无效请求，大幅提升了用户体验。"

#### 创新功能（POC扫描流程图）
> "POC漏洞扫描是系统的核心功能之一。我们支持**3种主流POC格式**：传统JSON格式、X-Ray的YAML格式和Nuclei格式。通过统一的解析引擎和智能匹配机制，用户可以直接使用GitHub上的开源POC库，大大降低了使用门槛。"

#### 高级特性（AI+MCP流程图）
> "AI安全对话模块是本系统的最大创新点。用户提问后，AI不仅能回答问题，还能**主动调用系统工具**，比如执行POC扫描、调用IDA分析二进制文件。这种**AI+工具联动**的方式，让安全测试从手工操作变成了智能对话，极大提升了测试效率。"

---

## 论文写作建议

### 第3章 需求分析
引用: **系统整体流程图**
```
图3.1 NeonScan系统整体功能流程图

如图3.1所示，系统采用模块化设计，包含6大功能模块...
```

### 第4章 系统设计
引用: **SSE实时推送机制流程图**
```
图4.2 SSE实时推送机制设计

系统采用SSE技术实现实时推送，如图4.2所示，前端通过EventSource建立连接...
```

### 第5章 详细设计与实现

#### 5.1 端口扫描模块
引用: **端口扫描流程图**
```
图5.1 端口扫描详细流程图

端口扫描模块的实现流程如图5.1所示，首先用户输入扫描参数...
```

#### 5.2 POC漏洞检测模块
引用: **POC漏洞扫描流程图**
```
图5.3 POC漏洞扫描流程图

POC漏洞检测模块支持3种主流格式，如图5.3所示...
```

#### 5.3 AI智能分析模块
引用: **AI安全对话流程图**
```
图5.5 AI安全对话流程图

AI模块的核心是工具调用机制，如图5.5所示...
```

#### 5.4 MCP工具集成模块
引用: **MCP工具集成流程图**
```
图5.7 MCP工具集成流程图

系统通过MCP协议集成专业工具，如图5.7所示...
```

---

## 流程图特点总结

### ✅ 7个核心流程图
1. **系统整体流程图** - 宏观架构
2. **端口扫描流程图** - 基础功能
3. **POC漏洞扫描流程图** - 核心功能
4. **AI安全对话流程图** - 创新功能
5. **MCP工具集成流程图** - 高级功能
6. **SSE实时推送流程图** - 技术细节
7. **目录扫描流程图** - 完整功能

### ✅ 覆盖所有核心模块
- 基础扫描 ✓
- 漏洞检测 ✓
- AI分析 ✓
- MCP集成 ✓
- 实时推送 ✓

### ✅ 适用多个场景
- 毕业答辩PPT
- 毕业论文插图
- 技术文档
- 开发参考

---

需要我帮你进一步优化这些流程图，或者绘制其他类型的UML图（如时序图、类图、部署图）吗？
