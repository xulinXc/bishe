#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
NeonScan系统ER图生成脚本（陈氏表示法） - 优化版V2
使用matplotlib绘制传统的实体-关系图
优化：折线绕开遮挡，清晰展示所有关系
"""

import matplotlib.pyplot as plt
import matplotlib.patches as patches
from matplotlib.patches import FancyBboxPatch, Polygon, Ellipse, Rectangle
import matplotlib.lines as mlines

# 设置中文字体
plt.rcParams['font.sans-serif'] = ['SimHei', 'Microsoft YaHei', 'Arial Unicode MS']
plt.rcParams['axes.unicode_minus'] = False

# 增大画布和调整坐标范围
fig, ax = plt.subplots(1, 1, figsize=(28, 18))
ax.set_xlim(-1, 27)
ax.set_ylim(-1, 17)
ax.axis('off')

# ==================== 辅助绘图函数 ====================

def draw_entity(ax, x, y, width, height, name):
    """绘制实体（矩形）"""
    rect = Rectangle((x - width/2, y - height/2), width, height, 
                     linewidth=2.5, edgecolor='black', facecolor='lightblue')
    ax.add_patch(rect)
    ax.text(x, y, name, ha='center', va='center', fontsize=13, fontweight='bold')
    return (x, y)

def draw_relationship(ax, x, y, size, name):
    """绘制关系（菱形）"""
    diamond = Polygon([
        (x, y + size),      # 上
        (x + size, y),      # 右
        (x, y - size),      # 下
        (x - size, y)       # 左
    ], closed=True, linewidth=2.5, edgecolor='black', facecolor='lightyellow')
    ax.add_patch(diamond)
    ax.text(x, y, name, ha='center', va='center', fontsize=11, fontweight='bold')
    return (x, y)

def draw_attribute(ax, x, y, width, height, name, is_key=False):
    """绘制属性（椭圆），主键用特殊标识"""
    # 主键用黄色背景+粗边框，普通属性用绿色
    ellipse = Ellipse((x, y), width, height, 
                     linewidth=3.0 if is_key else 1.8,
                     edgecolor='red' if is_key else 'black', 
                     facecolor='gold' if is_key else 'lightgreen')
    ax.add_patch(ellipse)
    
    # 主键文字加粗+下划线
    if is_key:
        ax.text(x, y, name, ha='center', va='center', fontsize=11, 
                fontweight='bold', style='italic',
                bbox=dict(boxstyle='round,pad=0.1', facecolor='none', edgecolor='none'))
        # 添加下划线
        ax.text(x, y - 0.15, '___', ha='center', va='top', fontsize=10, color='red')
    else:
        ax.text(x, y, name, ha='center', va='center', fontsize=10)
    return (x, y)

def draw_line(ax, x1, y1, x2, y2, style='-', color='black', width=1.8):
    """绘制连接线"""
    ax.plot([x1, x2], [y1, y2], color=color, linestyle=style, linewidth=width)

def draw_cardinality(ax, x, y, text):
    """绘制基数标注"""
    ax.text(x, y, text, ha='center', va='center', 
            fontsize=10, style='italic', fontweight='bold',
            bbox=dict(boxstyle='round,pad=0.4', facecolor='white', edgecolor='gray', linewidth=1))

# ==================== 绘制ER图（优化布局，增加间距）====================

# 第一行：扫描任务相关（y=14）- 增加横向间距
# 1. 扫描任务实体（左侧留出空间）
task_pos = draw_entity(ax, 4, 14, 2.2, 1.0, '扫描任务')
draw_attribute(ax, 2, 15.5, 0.8, 0.5, 'ID', is_key=True)
draw_attribute(ax, 6, 15.5, 0.9, 0.5, 'Total')
draw_attribute(ax, 2, 12.5, 0.8, 0.5, 'Done')
draw_attribute(ax, 6, 12.5, 1.1, 0.5, 'Created')
draw_line(ax, 2, 15.2, 2.9, 14.5)
draw_line(ax, 6, 15.2, 5.1, 14.5)
draw_line(ax, 2, 12.8, 2.9, 13.5)
draw_line(ax, 6, 12.8, 5.1, 13.5)

# 2. 扫描结果实体
result_pos = draw_entity(ax, 10, 14, 2.2, 1.0, '扫描结果')
draw_attribute(ax, 7.9, 15.5, 1.1, 0.5, 'TaskID', is_key=True)
draw_attribute(ax, 12.1, 15.5, 1.3, 0.5, 'ResultData')
draw_attribute(ax, 7.9, 12.5, 0.8, 0.5, 'Type')
draw_attribute(ax, 12.1, 12.5, 0.9, 0.5, 'Status')
draw_line(ax, 7.9, 15.2, 8.9, 14.5)
draw_line(ax, 12.1, 15.2, 11.1, 14.5)
draw_line(ax, 7.9, 12.8, 8.9, 13.5)
draw_line(ax, 12.1, 12.8, 11.1, 13.5)

# 3. POC规则实体
poc_pos = draw_entity(ax, 16, 14, 2.0, 1.0, 'POC规则')
draw_attribute(ax, 14.2, 15.5, 0.9, 0.5, 'Name', is_key=True)
draw_attribute(ax, 17.8, 15.5, 1.1, 0.5, 'Method')
draw_attribute(ax, 14.2, 12.5, 0.8, 0.5, 'Path')
draw_attribute(ax, 17.8, 12.5, 1.3, 0.5, 'MatchRules')
draw_line(ax, 14.2, 15.2, 15, 14.5)
draw_line(ax, 17.8, 15.2, 17, 14.5)
draw_line(ax, 14.2, 12.8, 15, 13.5)
draw_line(ax, 17.8, 12.8, 17, 13.5)

# 4. WAF绕过测试实体
waf_pos = draw_entity(ax, 22, 14, 2.2, 1.0, 'WAF绕过测试')
draw_attribute(ax, 19.8, 15.5, 1.1, 0.5, 'BaseURL', is_key=True)
draw_attribute(ax, 24.2, 15.5, 1.1, 0.5, 'Payload')
draw_attribute(ax, 19.8, 12.5, 1.1, 0.5, 'Strategy')
draw_attribute(ax, 24.2, 12.5, 1.0, 0.5, 'Variant')
draw_line(ax, 19.8, 15.2, 20.9, 14.5)
draw_line(ax, 24.2, 15.2, 23.1, 14.5)
draw_line(ax, 19.8, 12.8, 20.9, 13.5)
draw_line(ax, 24.2, 12.8, 23.1, 13.5)

# 第二行：AI会话相关（y=9）
# 5. AI会话实体
session_pos = draw_entity(ax, 4, 9, 2.0, 1.0, 'AI会话')
draw_attribute(ax, 2, 10.5, 1.2, 0.5, 'SessionID', is_key=True)
draw_attribute(ax, 6, 10.5, 1.1, 0.5, 'APIKey')
draw_attribute(ax, 2, 7.5, 1.1, 0.5, 'APIType')
draw_attribute(ax, 6, 7.5, 1.4, 0.5, 'LastActivity')
draw_line(ax, 2, 10.2, 3, 9.5)
draw_line(ax, 6, 10.2, 5, 9.5)
draw_line(ax, 2, 7.8, 3, 8.5)
draw_line(ax, 6, 7.8, 5, 8.5)

# 6. 聊天消息实体
message_pos = draw_entity(ax, 10, 9, 2.0, 1.0, '聊天消息')
draw_attribute(ax, 8.1, 10.5, 0.8, 0.5, 'Role', is_key=True)
draw_attribute(ax, 11.9, 10.5, 1.1, 0.5, 'Content')
draw_attribute(ax, 8.1, 7.5, 0.8, 0.5, 'Time')
draw_attribute(ax, 11.9, 7.5, 1.2, 0.5, 'ToolCallID')
draw_line(ax, 8.1, 10.2, 9, 9.5)
draw_line(ax, 11.9, 10.2, 11, 9.5)
draw_line(ax, 8.1, 7.8, 9, 8.5)
draw_line(ax, 11.9, 7.8, 11, 8.5)

# 7. MCP连接实体
mcp_pos = draw_entity(ax, 16, 9, 2.0, 1.0, 'MCP连接')
draw_attribute(ax, 14, 10.5, 1.2, 0.5, 'SessionID', is_key=True)
draw_attribute(ax, 18, 10.5, 1.2, 0.5, 'ConnType')
draw_attribute(ax, 14, 7.5, 1.3, 0.5, 'BinaryPath')
draw_attribute(ax, 18, 7.5, 0.9, 0.5, 'Status')
draw_line(ax, 14, 10.2, 15, 9.5)
draw_line(ax, 18, 10.2, 17, 9.5)
draw_line(ax, 14, 7.8, 15, 8.5)
draw_line(ax, 18, 7.8, 17, 8.5)

# 8. MCP工具实体
tool_pos = draw_entity(ax, 22, 9, 2.0, 1.0, 'MCP工具')
draw_attribute(ax, 20.1, 10.5, 0.9, 0.5, 'Name', is_key=True)
draw_attribute(ax, 23.9, 10.5, 1.3, 0.5, 'Description')
draw_attribute(ax, 20.1, 7.5, 1.2, 0.5, 'Parameters')
draw_line(ax, 20.1, 10.2, 21, 9.5)
draw_line(ax, 23.9, 10.2, 23, 9.5)
draw_line(ax, 20.1, 7.8, 21, 8.5)

# 第三行：文件和工具调用（y=4）
# 9. 上传文件实体
upload_pos = draw_entity(ax, 4, 4, 2.0, 1.0, '上传文件')
draw_attribute(ax, 2.1, 5.5, 1.0, 0.5, 'FilePath', is_key=True)
draw_attribute(ax, 5.9, 5.5, 1.1, 0.5, 'FileName')
draw_attribute(ax, 2.1, 2.5, 1.0, 0.5, 'FileSize')
draw_attribute(ax, 5.9, 2.5, 1.2, 0.5, 'UploadTime')
draw_line(ax, 2.1, 5.2, 3, 4.5)
draw_line(ax, 5.9, 5.2, 5, 4.5)
draw_line(ax, 2.1, 2.8, 3, 3.5)
draw_line(ax, 5.9, 2.8, 5, 3.5)

# 10. AI工具调用实体
toolcall_pos = draw_entity(ax, 10, 4, 2.2, 1.0, 'AI工具调用')
draw_attribute(ax, 7.9, 5.5, 0.9, 0.5, 'CallID', is_key=True)
draw_attribute(ax, 12.1, 5.5, 1.1, 0.5, 'ToolName')
draw_attribute(ax, 7.9, 2.5, 1.2, 0.5, 'Arguments')
draw_attribute(ax, 12.1, 2.5, 0.9, 0.5, 'Result')
draw_line(ax, 7.9, 5.2, 8.9, 4.5)
draw_line(ax, 12.1, 5.2, 11.1, 4.5)
draw_line(ax, 7.9, 2.8, 8.9, 3.5)
draw_line(ax, 12.1, 2.8, 11.1, 3.5)

# ==================== 绘制关系（使用折线绕开遮挡）====================

# 关系1: 扫描任务 "产生" 扫描结果 (1:N)
produce_pos = draw_relationship(ax, 7, 14, 0.55, '产生')
draw_line(ax, task_pos[0] + 1.1, task_pos[1], produce_pos[0] - 0.55, produce_pos[1])
draw_line(ax, produce_pos[0] + 0.55, produce_pos[1], result_pos[0] - 1.1, result_pos[1])
draw_cardinality(ax, 5.7, 14.4, '1')
draw_cardinality(ax, 8.3, 14.4, 'N')

# 关系2: POC规则 "应用于" 扫描任务 (1:N) - 折线绕开扫描结果
apply_pos = draw_relationship(ax, 10, 11.5, 0.55, '应用于')
# 从POC规则左下角出发
draw_line(ax, poc_pos[0] - 1.0, poc_pos[1] - 0.5, 13, 12)
draw_line(ax, 13, 12, 10, 12, color='blue', width=2)  # 蓝色折线
draw_line(ax, 10, 12, apply_pos[0], apply_pos[1] + 0.55, color='blue', width=2)
# 连接到扫描任务右下角
draw_line(ax, apply_pos[0], apply_pos[1] - 0.55, 7, 11.5, color='blue', width=2)
draw_line(ax, 7, 11.5, task_pos[0] + 1.1, task_pos[1] - 0.5, color='blue', width=2)
draw_cardinality(ax, 12, 11.8, '1')
draw_cardinality(ax, 6.5, 12.2, 'N')

# 关系3: WAF绕过测试 "基于" POC规则 (N:1)
based_pos = draw_relationship(ax, 19, 14, 0.55, '基于')
draw_line(ax, waf_pos[0] - 1.1, waf_pos[1], based_pos[0] + 0.55, based_pos[1])
draw_line(ax, based_pos[0] - 0.55, based_pos[1], poc_pos[0] + 1.0, poc_pos[1])
draw_cardinality(ax, 20.3, 14.4, 'N')
draw_cardinality(ax, 17.7, 14.4, '1')

# 关系4: AI会话 "包含" 聊天消息 (1:N)
contain_pos = draw_relationship(ax, 7, 9, 0.55, '包含')
draw_line(ax, session_pos[0] + 1.0, session_pos[1], contain_pos[0] - 0.55, contain_pos[1])
draw_line(ax, contain_pos[0] + 0.55, contain_pos[1], message_pos[0] - 1.0, message_pos[1])
draw_cardinality(ax, 5.7, 9.4, '1')
draw_cardinality(ax, 8.3, 9.4, 'N')

# 关系5: 聊天消息 "关联" MCP连接 (1:0..1)
connect_pos = draw_relationship(ax, 13, 9, 0.55, '关联')
draw_line(ax, message_pos[0] + 1.0, message_pos[1], connect_pos[0] - 0.55, connect_pos[1])
draw_line(ax, connect_pos[0] + 0.55, connect_pos[1], mcp_pos[0] - 1.0, mcp_pos[1])
draw_cardinality(ax, 11.7, 9.4, '1')
draw_cardinality(ax, 14.3, 9.4, '0..1')

# 关系6: MCP连接 "提供" MCP工具 (1:N)
provide_pos = draw_relationship(ax, 19, 9, 0.55, '提供')
draw_line(ax, mcp_pos[0] + 1.0, mcp_pos[1], provide_pos[0] - 0.55, provide_pos[1])
draw_line(ax, provide_pos[0] + 0.55, provide_pos[1], tool_pos[0] - 1.0, tool_pos[1])
draw_cardinality(ax, 17.7, 9.4, '1')
draw_cardinality(ax, 20.3, 9.4, 'N')

# 关系7: 扫描任务 "AI分析" AI会话 (M:N) - 垂直连线，左侧通道
analyze_pos = draw_relationship(ax, 1, 11.5, 0.55, 'AI分析')
# 从扫描任务左边出发
draw_line(ax, task_pos[0] - 1.1, task_pos[1], 1, 14, color='red', width=2)  # 红色折线
draw_line(ax, 1, 14, 1, 12, color='red', width=2)
draw_line(ax, 1, 12, analyze_pos[0], analyze_pos[1] + 0.55, color='red', width=2)
# 连接到AI会话上边
draw_line(ax, analyze_pos[0], analyze_pos[1] - 0.55, 1, 11, color='red', width=2)
draw_line(ax, 1, 11, 1, 9.5, color='red', width=2)
draw_line(ax, 1, 9.5, session_pos[0] - 1.0, session_pos[1], color='red', width=2)
draw_cardinality(ax, 0.3, 13, 'M')
draw_cardinality(ax, 0.3, 10, 'N')

# 关系8: 上传文件 "用于" 扫描任务 (N:M) - 垂直连线，左侧通道
usefor_pos = draw_relationship(ax, 0.3, 6.5, 0.55, '用于')
# 从上传文件左上角出发
draw_line(ax, upload_pos[0] - 1.0, upload_pos[1] + 0.5, 0.3, 5, color='green', width=2)  # 绿色折线
draw_line(ax, 0.3, 5, 0.3, 6, color='green', width=2)
draw_line(ax, 0.3, 6, usefor_pos[0], usefor_pos[1] + 0.55, color='green', width=2)
# 连接到扫描任务
draw_line(ax, usefor_pos[0], usefor_pos[1] - 0.55, 0.3, 7, color='green', width=2)
draw_line(ax, 0.3, 7, 0.3, 13, color='green', width=2)
draw_line(ax, 0.3, 13, task_pos[0] - 1.1, task_pos[1] - 0.5, color='green', width=2)
draw_cardinality(ax, -0.4, 5.5, 'N')
draw_cardinality(ax, -0.4, 12.5, 'M')

# 关系9: 聊天消息 "调用" AI工具调用 (1:N)
invoke_pos = draw_relationship(ax, 10, 6.5, 0.55, '调用')
draw_line(ax, message_pos[0], message_pos[1] - 0.5, invoke_pos[0], invoke_pos[1] + 0.55)
draw_line(ax, invoke_pos[0], invoke_pos[1] - 0.55, toolcall_pos[0], toolcall_pos[1] + 0.5)
draw_cardinality(ax, 9.3, 7.5, '1')
draw_cardinality(ax, 9.3, 5.5, 'N')

# ==================== 添加图例（底部）====================

legend_x, legend_y = 13, 0.8

# 图例标题
ax.text(legend_x, legend_y + 0.8, 'ER图符号说明（陈氏表示法）', 
        ha='center', fontsize=12, fontweight='bold',
        bbox=dict(boxstyle='round,pad=0.6', facecolor='lightyellow', edgecolor='black', linewidth=2))

# 示例符号
draw_entity(ax, legend_x - 3.5, legend_y - 0.3, 1.2, 0.5, '实体名称')
draw_relationship(ax, legend_x, legend_y - 0.3, 0.35, '关系')
draw_attribute(ax, legend_x + 3.5, legend_y - 0.3, 1.0, 0.5, 'Attribute')

# 说明文字
ax.text(legend_x, legend_y - 1.0, '矩形=实体  菱形=关系  椭圆=属性（英文字段名）', 
        ha='center', fontsize=10)
ax.text(legend_x, legend_y - 1.4, '金色椭圆+红边框+下划线=主键  彩色线=折线绕开遮挡', 
        ha='center', fontsize=10)

# 在图例中添加主键示例
draw_attribute(ax, legend_x - 5.5, legend_y - 0.3, 0.9, 0.5, 'PrimaryKey', is_key=True)

# 左侧业务模块说明
ax.text(2.5, 0.8, '业务模块:\n• 端口/目录/POC扫描\n• WAF绕过测试\n• AI安全分析\n• IDA/JADX逆向', 
        ha='left', fontsize=9,
        bbox=dict(boxstyle='round,pad=0.5', facecolor='lightcyan', edgecolor='black', linewidth=2))

# 右侧存储架构说明
ax.text(23.5, 0.8, '存储架构:\n纯内存存储\nGo map管理\nSSE实时通信', 
        ha='right', fontsize=9, style='italic',
        bbox=dict(boxstyle='round,pad=0.5', facecolor='lightgray', edgecolor='black', linewidth=2))

# 关系颜色说明
ax.text(13, legend_y - 1.8, '红色=AI分析  绿色=用于  蓝色=应用于', 
        ha='center', fontsize=9, style='italic')

# ==================== 保存图片 ====================

plt.title('NeonScan系统ER图（陈氏表示法）', fontsize=18, fontweight='bold', pad=25)
plt.tight_layout()

# 保存为高清PNG和PDF
output_path_png = 'c:/Users/86483/Desktop/桌面/bishev3/bishe/shili/论文/图/ER图/ER图_陈氏表示法.png'
output_path_pdf = 'c:/Users/86483/Desktop/桌面/bishev3/bishe/shili/论文/图/ER图/ER图_陈氏表示法_v2.pdf'

plt.savefig(output_path_png, dpi=300, bbox_inches='tight', facecolor='white')
try:
    plt.savefig(output_path_pdf, bbox_inches='tight', facecolor='white')
    print(f"   PDF: {output_path_pdf}")
except PermissionError:
    print(f"   PDF: 跳过（文件被占用）")

print(f"✅ ER图已生成（主键高亮显示）:")
print(f"   PNG: {output_path_png}")

# 显示图形（可选）
# plt.show()
