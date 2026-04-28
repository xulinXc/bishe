# NeonScan 系统整体功能流程图（专业版）

## PlantUML 代码

```plantuml
@startuml NeonScan整体流程图_专业版

skinparam backgroundColor #FEFEFE
skinparam activityBackgroundColor #E8F5E9
skinparam activityBorderColor #4CAF50
skinparam activityBarColor #FF9800
skinparam activityStartColor #4CAF50
skinparam activityEndColor #F44336
skinparam activityDiamondBackgroundColor #FFF9C4
skinparam activityDiamondBorderColor #FBC02D

title NeonScan 系统整体功能流程图（带泳道和决策节点）

|#AntiqueWhite|用户（前端）|
start
:访问 http://localhost:8080;
:浏览功能模块列表;

#LightBlue:选择功能模块;
note right
  11个功能页面：
  - 端口扫描、目录扫描、Web探针
  - POC扫描、EXP验证、WAF绕过
  - JS资产收集
  - AI对话、AI自动扫描
  - IDA MCP、JADX MCP
  - 扫描报告
end note

if (是否需要扫描?) then (是)
    
    if (功能类型?) then (扫描类任务)
        #E3F2FD:填写扫描参数;
        note right
          示例：端口扫描
          - 目标: 192.168.1.1
          - 端口: 1-65535
          - 并发: 500
        end note
        
        |#LightYellow|后端服务|
        :接收扫描请求;
        :创建Task对象;
        :生成TaskID;
        
        |用户（前端）|
        :收到TaskID;
        
        fork
            |用户（前端）|
            :建立SSE连接;
            floating note right: GET /events?task={taskID}
            
            repeat
                :等待SSE消息;
                
                if (消息类型?) then (progress)
                    #B3E5FC:更新进度条;
                elseif (find)
                    #C8E6C9:添加到结果表;
                elseif (end)
                    #FFCCBC:显示完成;
                    :关闭SSE连接;
                    detach
                else (error)
                    #FFCDD2:显示错误;
                    detach
                endif
                
            repeat while (连接未关闭?)
            
        fork again
            |#LightYellow|后端服务|
            partition "并发扫描执行" {
                :初始化任务队列;
                :创建goroutine池;
                
                fork
                    :Worker 1\n执行扫描;
                fork again
                    :Worker 2\n执行扫描;
                fork again
                    :Worker 3\n执行扫描;
                fork again
                    :Worker N\n执行扫描;
                end fork
                
                :收集所有结果;
            }
            
            :推送end消息;
            :任务完成;
        end fork
        
        |用户（前端）|
        :查看扫描结果;
        
        if (需要进一步操作?) then (导出报告)
            #FFE0B2:下载JSON文件;
        elseif (AI分析)
            :跳转AI对话页面;
            :粘贴扫描结果;
            goto ai_process;
        elseif (查看统一报告)
            :打开扫描报告页面;
            :查看所有模块汇总;
        else (继续扫描)
            :返回首页;
        endif
        
    elseif (AI对话/自动扫描) then
        label ai_process
        |#LightYellow|后端服务|
        
        if (AI功能类型?) then (AI对话)
            :建立流式连接;
            
            |用户（前端）|
            fork
                :输入问题/结果;
                
                repeat
                    :发送消息;
                repeat while (继续对话?)
                
            fork again
                |#LightYellow|后端服务|
                :调用AI API;
                
                |#LightGreen|AI服务|
                :流式返回响应;
                
                |用户（前端）|
                :实时显示AI回复;
            end fork
            
        else (AI自动扫描)
            |用户（前端）|
            :输入目标URL;
            
            |#LightYellow|后端服务|
            :生成TaskID;
            :AI接收任务;
            
            |#LightGreen|AI Agent|
            partition "AI自主决策" {
                :分析目标;
                :制定扫描计划;
                
                fork
                    :调用端口扫描;
                fork again
                    :调用Web探针;
                fork again
                    :调用POC检测;
                end fork
                
                :实时分析结果;
                :调整扫描策略;
                :生成分析报告;
            }
            
            |用户（前端）|
            :查看AI分析报告;
        endif
        
    elseif (MCP工具集成) then
        |用户（前端）|
        :上传二进制/APK文件;
        :选择分析工具;
        
        |#LightYellow|后端服务|
        :建立MCP Session;
        
        |#LightPink|MCP Server|
        
        if (工具类型?) then (IDA Pro)
            :加载二进制文件;
            :反汇编分析;
        else (JADX)
            :解析APK;
            :反编译代码;
        endif
        
        |用户（前端）|
        fork
            :输入分析问题;
        fork again
            |#LightPink|MCP Server|
            :执行分析;
            :返回结果;
        end fork
        
        |用户（前端）|
        :查看分析结果;
        
    else (文件上传/报告查看)
        if (操作类型?) then (上传POC/EXP)
            |#LightYellow|后端服务|
            :保存到uploads目录;
            :返回文件路径;
            
            |用户（前端）|
            :确认上传成功;
            
        else (查看扫描报告)
            |#LightYellow|后端服务|
            :聚合所有扫描结果;
            
            |用户（前端）|
            :查看统计信息;
            :导出完整报告;
        endif
    endif
    
else (否 - 仅浏览)
    :查看历史报告;
    :了解系统功能;
endif

|用户（前端）|
if (继续使用?) then (是)
    :返回首页;
    detach
else (否)
    :退出系统;
    stop
endif

@enduml
```

---

## 🎯 专业版流程图的亮点

### 1. **泳道设计** 🏊
```
|用户（前端）| - 浅褐色
|后端服务|     - 浅黄色
|AI服务|       - 浅绿色
|MCP Server|   - 浅粉色
```

清晰区分不同角色的职责，符合UML规范。

### 2. **决策节点** 🔷
```plantuml
if (功能类型?) then (扫描类任务)
    ...
elseif (AI对话/自动扫描) then
    ...
elseif (MCP工具集成) then
    ...
else (文件上传/报告查看)
    ...
endif
```

所有判断都使用标准的菱形决策节点。

### 3. **并发执行** 🍴
```plantuml
fork
    :Worker 1\n执行扫描;
fork again
    :Worker 2\n执行扫描;
fork again
    :Worker 3\n执行扫描;
end fork
```

使用`fork/fork again/end fork`表示goroutine并发池。

### 4. **异步通信** 📡
```plantuml
fork
    |用户（前端）|
    :建立SSE连接;
    repeat
        :等待SSE消息;
    repeat while (连接未关闭?)
    
fork again
    |后端服务|
    :推送SSE消息;
end fork
```

清晰展示SSE的双向异步通信。

### 5. **分区模块** 📦
```plantuml
partition "并发扫描执行" {
    :初始化任务队列;
    :创建goroutine池;
    ...
}
```

用partition突出关键业务逻辑。

---

## 📊 对比效果

### 简化版（原版）
- ❌ 线性流程，看不出复杂性
- ❌ 没有角色区分
- ❌ 没有并发表示
- ✅ 简洁易懂（适合初期设计）

### 专业版（新版）
- ✅ 泳道区分前后端职责
- ✅ 菱形决策节点清晰
- ✅ fork/join表示并发
- ✅ 完整展示SSE异步通信
- ✅ 符合UML活动图标准
- ✅ 适合毕业答辩展示

---

## 💡 答辩讲解话术

> "各位老师，这是NeonScan的系统整体功能流程图。
> 
> 我采用了**UML活动图**的标准画法，使用**泳道**区分了前端、后端、AI服务和MCP Server四个角色的职责。
> 
> 流程图中可以看到：
> 1. **菱形决策节点**展示了功能选择的逻辑分支
> 2. **fork/join结构**表示了goroutine并发扫描池的设计
> 3. **双泳道并行**展示了SSE实时推送的异步通信机制
> 4. **分区模块**突出了AI自主决策等核心业务逻辑
> 
> 这种画法既符合UML规范，又能清晰展示系统的并发处理和异步通信特点。"

---

## ✅ 建议

现在你有**两个版本**的整体流程图：

1. **简化版**（`NeonScan系统整体流程图_优化版.md`）
   - 优点：简洁易懂
   - 适用：论文初稿、快速理解

2. **专业版**（`NeonScan系统整体流程图_专业版.md`）⭐
   - 优点：符合UML规范，展示复杂性
   - 适用：**毕业答辩**、专业评审

**答辩时推荐使用专业版！** 更能体现你的系统设计能力和对UML的掌握。

需要我再优化细节吗？