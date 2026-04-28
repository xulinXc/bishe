# NeonScan 系统整体功能流程图（优化版）

## PlantUML 代码

```plantuml
@startuml NeonScan整体流程图

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E8F5E9
skinparam activityBorderColor #4CAF50
skinparam activityStartColor #4CAF50
skinparam activityEndColor #F44336
skinparam activityDiamondBackgroundColor #FFF9C4
skinparam activityDiamondBorderColor #FBC02D

title NeonScan 系统整体功能流程图

start

:用户启动NeonScan;
:访问Web界面 http://localhost:8080;

partition "功能选择" {
    :用户选择功能模块;
    
    if (选择哪个模块?) then (基础扫描)
        if (扫描类型?) then (端口扫描)
            #E3F2FD:执行端口扫描;
        elseif (目录扫描) then
            #E8F5E9:执行目录扫描;
        else (Web探针)
            #E1F5FE:执行Web探针;
        endif
        
    elseif (漏洞检测) then
        if (检测类型?) then (POC扫描)
            #FFEBEE:执行POC扫描;
        elseif (EXP验证) then
            #FCE4EC:执行EXP验证;
        else (WAF绕过)
            #FFF3E0:执行WAF绕过测试;
        endif
        
    elseif (资产收集) then
        if (工具选择?) then (URLFinder)
            #F3E5F5:执行URL收集;
        elseif (FFUF) then
            #EDE7F6:执行目录爆破;
        else (Packer)
            #E8EAF6:执行JS资产收集;
            note right
              集成多种工具：
              - 可解包性判定
              - FFUF爆破
              - URLFinder提取
              - Packer解包
            end note
        endif
        
    elseif (AI驱动) then
        if (AI功能?) then (AI对话)
            #FFF9C4:执行AI安全对话;
            note right
              用途：
              - 咨询安全问题
              - 分析已有结果
              - 代码审计建议
            end note
            goto ai_flow;
            
        else (AI自动扫描)
            #FFF59D:AI自主决策扫描;
            note right
              **AI主动执行扫描**
              AI自动调用工具：
              - 端口扫描
              - Web探针
              - POC检测
              - 结果分析
              全流程AI驱动
            end note
            goto ai_flow;
        endif
        
    elseif (工具集成) then
        if (工具类型?) then (IDA Pro)
            #F8BBD0:IDA二进制分析;
            note right
              MCP协议
              独立Session
            end note
        else (JADX)
            #E1BEE7:JADX APK分析;
        endif
        
    elseif (扫描报告) then
        #B2DFDB:查看扫描报告;
        note right
          功能：
          - 聚合所有扫描结果
          - 统计概览
          - 导出完整报告
        end note
        stop
        
    else (文件上传)
        #E0F2F1:上传POC/EXP/二进制文件;
        :保存到uploads目录;
        stop
    endif
}

' 扫描类任务的通用流程
if (是扫描任务?) then (是)
    partition "任务执行（扫描类）" {
        :创建扫描任务;
        :后端生成TaskID;
        :返回TaskID给前端;
        :前端建立SSE连接;
        
        fork
            :后端执行扫描;
            :goroutine并发处理;
        fork again
            :SSE实时推送进度;
            :前端实时更新UI;
        end fork
        
        :扫描任务完成;
        :推送end消息;
        :关闭SSE连接;
    }
    
    partition "结果处理" {
        :查看扫描结果;
        
        if (需要AI辅助分析?) then (是)
            :打开AI对话页面;
            :发送结果到AI;
            :获取AI分析建议;
            note right
              可选操作：
              让AI分析扫描结果
              提供安全建议
            end note
        else (否)
        endif
        
        if (需要导出?) then (是)
            :导出JSON格式;
            :下载到本地;
        else (否)
        endif
    }

else (否 - AI/MCP任务)
    if (任务类型?) then (MCP分析)
        partition "MCP工具执行" {
            :建立MCP Session;
            :上传文件到MCP Server;
            :发送分析请求;
            :接收分析结果;
            :关闭Session;
        }
    else (AI任务)
        label ai_flow
        partition "AI执行流程" {
            if (AI功能?) then (AI对话)
                :建立流式连接;
                :用户输入问题/结果;
                :AI实时响应;
                :显示分析建议;
                
            else (AI自动扫描)
                :AI接收目标URL;
                :AI自主决策扫描步骤;
                
                fork
                    :AI调用端口扫描;
                    :AI调用Web探针;
                    :AI调用POC检测;
                fork again
                    :AI实时分析结果;
                    :AI决策下一步;
                end fork
                
                :AI生成完整报告;
                :返回分析结果;
            endif
        }
    endif
endif

:用户结束使用;

stop

@enduml
```

---

## 📊 优化说明

### 1. **功能模块完整性** ✅

| 模块 | 原流程图 | 优化后 |
|------|---------|--------|
| **基础扫描** | 端口、目录、Web探针 | ✅ 保留 |
| **漏洞检测** | 只有POC | ✅ 补充EXP、WAF绕过 |
| **资产收集** | ❌ 缺失 | ✅ 新增URLFinder/FFUF/Packer |
| **AI分析** | 只有AI对话 | ✅ 补充AI自动扫描 |
| **工具集成** | 笼统的"MCP" | ✅ 细分IDA/JADX |
| **文件上传** | ❌ 缺失 | ✅ 新增独立流程 |

### 2. **流程逻辑优化** ✅

**原流程图问题**：所有功能都走"任务执行"流程

**优化后**：
- **扫描类任务**（端口/目录/POC等）→ TaskID + SSE流程
- **AI对话** → 实时流式响应，无TaskID
- **MCP工具** → 独立Session管理
- **文件上传** → 直接结束，不进入任务流程

### 3. **新增关键注释** ✅

```plantuml
note right
  AI对话：实时流式对话，无需TaskID
end note

note right
  MCP工具：MCP协议，独立Session
end note
```

---

## 🎯 与实际代码的对应关系

| 流程图功能 | 对应路由 | 说明 |
|----------|---------|------|
| **基础扫描** |
| 端口扫描 | `/scan/ports` | TaskID + SSE ✅ |
| 目录扫描 | `/scan/dirs` | TaskID + SSE ✅ |
| Web探针 | `/scan/webprobe` | TaskID + SSE ✅ |
| **漏洞检测** |
| POC扫描 | `/scan/poc` | TaskID + SSE ✅ |
| EXP验证 | `/scan/exp` | TaskID + SSE ✅ |
| WAF绕过 | `/scan/waf` | TaskID + SSE ✅ |
| **资产收集** |
| URLFinder | `/scan/shouji/urlfinder` | TaskID + SSE ✅ |
| FFUF爆破 | `/scan/shouji/ffuf` | TaskID + SSE ✅ |
| Packer解包 | `/scan/shouji/packer` | TaskID + SSE ✅ |
| **AI分析** |
| AI对话 | `/ai/analyze` | 流式响应，无TaskID ✅ |
| AI自动扫描 | `/ai/auto-scan` | TaskID + SSE ✅ |
| **工具集成** |
| IDA分析 | `/mcp/ida/chat/stream` | MCP Session ✅ |
| JADX分析 | `/mcp/jadx/chat/stream` | MCP Session ✅ |
| **文件管理** |
| 文件上传 | `/upload` | 直接返回 ✅ |

---

## 💡 答辩讲解话术

> "各位老师，这是NeonScan的系统整体功能流程图。
> 
> 系统分为**6大功能模块**：
> 1. **基础扫描**：端口、目录、Web探针
> 2. **漏洞检测**：POC、EXP、WAF绕过
> 3. **资产收集**：集成URLFinder、FFUF、Packer三大工具
> 4. **AI分析**：智能对话和自动化扫描
> 5. **工具集成**：IDA Pro二进制分析、JADX APK分析
> 6. **文件管理**：POC/EXP/二进制文件上传
> 
> 在流程设计上，我做了**分类处理**：
> - **扫描类任务**采用TaskID + SSE实时推送架构
> - **AI对话**采用流式响应，无需TaskID
> - **MCP工具**有独立的Session管理机制
> - **文件上传**是独立的文件管理功能
> 
> 这种设计既保证了功能的完整性，又体现了不同任务类型的特点。"

---

## ✅ 总结

### 原流程图的问题：
1. ❌ 缺少EXP验证、WAF绕过
2. ❌ 缺少整个资产收集模块
3. ❌ 缺少AI自动扫描
4. ❌ MCP工具集成太笼统
5. ❌ 缺少文件上传
6. ❌ AI对话和扫描任务流程混在一起

### 优化后的效果：
1. ✅ 功能模块从6个→15个核心功能
2. ✅ 流程逻辑更清晰（扫描/AI/MCP分类）
3. ✅ 与实际代码完全对应
4. ✅ 适合答辩展示

文件已保存到：`NeonScan系统整体流程图_优化版.md` 🎉
