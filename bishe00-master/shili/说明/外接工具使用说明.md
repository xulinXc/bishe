# 工具配置说明

本项目已配置以下外部工具，位于 `tools/` 目录中：

## 已配置的工具

### 1. ffuf
- **仓库**: https://github.com/ffuf/ffuf.git
- **位置**: `tools/ffuf/`
- **可执行文件**: `tools/bin/ffuf.exe`
- **版本**: 2.1.0-dev
- **用途**: Web 路径爆破工具，用于发现隐藏的 JS 文件路径

### 2. URLFinder
- **仓库**: https://github.com/pingc0y/URLFinder.git
- **位置**: `tools/URLFinder/`
- **可执行文件**: `tools/bin/urlfinder.exe`
- **版本**: 2023.9.9
- **用途**: URL 发现工具，用于从网页中提取 URL 和 JS 文件链接

### 3. Packer-Fuzzer
- **仓库**: https://github.com/rtcatc/Packer-Fuzzer.git
- **位置**: `tools/Packer-Fuzzer/`
- **主脚本**: `tools/Packer-Fuzzer/PackerFuzzer.py`
- **版本**: v1.4
- **用途**: JavaScript 打包器安全检测工具，用于检测由 Webpack 等打包器构建的网站的安全漏洞
- **依赖**: Python 3.x 及 requirements.txt 中的包

## 工具使用方式

主程序会自动优先使用 `tools/bin/` 目录中的可执行文件（对于 Go 工具）或 `tools/` 目录中的脚本（对于 Python 工具），如果找不到再尝试使用系统 PATH 中的工具。

### 编译方式

如果需要重新编译 Go 工具：

```powershell
# 编译 ffuf
cd tools\ffuf
go build -o ..\bin\ffuf.exe .
cd ..\..

# 编译 URLFinder
cd tools\URLFinder
go mod tidy  # 修复依赖（如果需要）
go build -o ..\bin\urlfinder.exe .
cd ..\..
```

### Packer-Fuzzer 依赖安装

Packer-Fuzzer 是 Python 工具，需要安装依赖：

```powershell
# 进入 Packer-Fuzzer 目录
cd tools\Packer-Fuzzer

# 安装依赖（推荐使用虚拟环境）
python -m venv venv
.\venv\Scripts\activate  # Windows
# 或
source venv/bin/activate  # Linux/Mac

# 安装依赖包
pip install -r requirements.txt
```

**依赖包列表**:
- bs4 (BeautifulSoup4)
- urllib3
- requests
- docx2pdf
- docx2txt
- node_vm2
- python-docx==0.8.11

## 依赖要求

- Go 1.17+ (用于编译 ffuf)
- Go 1.19+ (用于编译 URLFinder)
- Python 3.x (用于运行 Packer-Fuzzer)

## 注意事项

1. 已编译的可执行文件位于 `tools/bin/` 目录
2. Packer-Fuzzer 脚本位于 `tools/Packer-Fuzzer/` 目录
3. 主程序会自动查找并使用这些工具，无需额外配置 PATH
4. 如果系统 PATH 中已有这些工具，也会作为备选方案使用
5. Packer-Fuzzer 的输出结果会保存在：
   - `tools/Packer-Fuzzer/tmp/` - 临时数据和数据库
   - `tools/Packer-Fuzzer/reports/` - 生成的报告文件

