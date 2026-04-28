# MCP 集成文档整理

本目录用于记录 MCP 与逆向工具链（JADX/IDA）的集成、调试、排障与最终方案。

## 推荐阅读顺序（从“能快速解决问题”到“方案总览”）

1. [JADX_MCP_502错误问题总结.md](./JADX_MCP_502错误问题总结.md)（最常见：服务器在线但插件离线）
2. [JADX_406错误和连接慢问题修复.md](./JADX_406错误和连接慢问题修复.md)（常见：请求头/轮询导致 406 与慢连接）
3. [IDA_MCP问题完整总结.md](./IDA_MCP问题完整总结.md)（完整实现与踩坑汇总）

## 其他资料（作为补充/背景）

- [JADX MCP Server 配置说明.md](./JADX%20MCP%20Server%20%E9%85%8D%E7%BD%AE%E8%AF%B4%E6%98%8E.md)
- [JADX_MCP_连接方案完整文档.md](./JADX_MCP_%E8%BF%9E%E6%8E%A5%E6%96%B9%E6%A1%88%E5%AE%8C%E6%95%B4%E6%96%87%E6%A1%A3.md)
- [JADX_MCP_问题分析与解决方案.md](./JADX_MCP_%E9%97%AE%E9%A2%98%E5%88%86%E6%9E%90%E4%B8%8E%E8%A7%A3%E5%86%B3%E6%96%B9%E6%A1%88.md)
- [IDA_MCP使用说明.md](./IDA_MCP%E4%BD%BF%E7%94%A8%E8%AF%B4%E6%98%8E.md)
- [IDA_MCP实现思路.md](./IDA_MCP%E5%AE%9E%E7%8E%B0%E6%80%9D%E8%B7%AF.md)
- [IDA_MCP完美解决方案.md](./IDA_MCP%E5%AE%8C%E7%BE%8E%E8%A7%A3%E5%86%B3%E6%96%B9%E6%A1%88.md)
- [IDA_MCP问题总结.md](./IDA_MCP%E9%97%AE%E9%A2%98%E6%80%BB%E7%BB%93.md)
- [启动JADX_MCP服务器.md](./%E5%90%AF%E5%8A%A8JADX_MCP%E6%9C%8D%E5%8A%A1%E5%99%A8.md)
- [端口说明.md](./%E7%AB%AF%E5%8F%A3%E8%AF%B4%E6%98%8E.md)
- [IDA和JADX连接速度差异分析.md](./IDA%E5%92%8CJADX%E8%BF%9E%E6%8E%A5%E9%80%9F%E5%BA%A6%E5%B7%AE%E5%BC%82%E5%88%86%E6%9E%90.md)
- [AI+MCP思路.txt](./AI%2BMCP%E6%80%9D%E8%B7%AF.txt)
