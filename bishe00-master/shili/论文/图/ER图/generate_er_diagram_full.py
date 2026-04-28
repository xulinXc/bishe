#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
NeonScan系统ER图生成脚本（全字段版）
基于 matplotlib 绘制陈氏表示法 ER 图，并补全各实体的全部属性
"""

import math
import matplotlib.pyplot as plt
from matplotlib.patches import Polygon, Ellipse, Rectangle

# 设置中文字体
plt.rcParams['font.sans-serif'] = ['SimHei', 'Microsoft YaHei', 'Arial Unicode MS']
plt.rcParams['axes.unicode_minus'] = False

# 画布设置
fig, ax = plt.subplots(1, 1, figsize=(32, 20))
ax.set_xlim(-2, 30)
ax.set_ylim(-3, 19)
ax.axis('off')


# ==================== 基础绘图函数 ====================
def draw_entity(ax, x, y, width, height, name):
    """绘制实体（矩形）"""
    rect = Rectangle((x - width / 2, y - height / 2), width, height,
                     linewidth=2.5, edgecolor='black', facecolor='lightblue')
    ax.add_patch(rect)
    ax.text(x, y, name, ha='center', va='center', fontsize=14, fontweight='bold')
    return (x, y, width, height)


def draw_relationship(ax, x, y, size, name, facecolor='lightyellow'):
    """绘制关系（菱形）"""
    diamond = Polygon([
        (x, y + size),
        (x + size, y),
        (x, y - size),
        (x - size, y)
    ], closed=True, linewidth=2.5, edgecolor='black', facecolor=facecolor)
    ax.add_patch(diamond)
    ax.text(x, y, name, ha='center', va='center', fontsize=12, fontweight='bold')
    return (x, y, size)


def draw_attribute(ax, x, y, width, height, name, is_key=False):
    """绘制属性（椭圆），主键加粗高亮"""
    ellipse = Ellipse((x, y), width, height,
                      linewidth=3.0 if is_key else 1.8,
                      edgecolor='red' if is_key else 'black',
                      facecolor='gold' if is_key else 'lightgreen')
    ax.add_patch(ellipse)
    if is_key:
        ax.text(x, y, name, ha='center', va='center', fontsize=11,
                fontweight='bold', style='italic',
                bbox=dict(boxstyle='round,pad=0.1', facecolor='none', edgecolor='none'))
        ax.text(x, y - 0.2, '___', ha='center', va='top', fontsize=9, color='red')
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
            bbox=dict(boxstyle='round,pad=0.35', facecolor='white', edgecolor='gray', linewidth=1))


def distribute_attributes(ax, entity_info, attributes, column_offset=3.2, spacing=1.2):
    """将属性按照左右两列排布并连线"""
    if not attributes:
        return

    x, y, width, height = entity_info
    mid = math.ceil(len(attributes) / 2)
    left_attrs = attributes[:mid]
    right_attrs = attributes[mid:]

    def column_positions(count):
        if count == 0:
            return []
        start = (count - 1) / 2
        positions = []
        for idx in range(count):
            offset = (start - idx) * spacing
            py = y + offset
            adjust = height / 2 + 0.8
            if py >= y:
                py += adjust
            else:
                py -= adjust
            positions.append(py)
        return positions

    left_positions = column_positions(len(left_attrs))
    right_positions = column_positions(len(right_attrs))

    attr_width = 2.2
    attr_height = 1.0

    for (attr, is_key), py in zip(left_attrs, left_positions):
        px = x - (width / 2 + column_offset)
        draw_attribute(ax, px, py, attr_width, attr_height, attr, is_key=is_key)
        draw_line(ax, px + attr_width / 2, py, x - width / 2, y, width=1.4)

    for (attr, is_key), py in zip(right_attrs, right_positions):
        px = x + (width / 2 + column_offset)
        draw_attribute(ax, px, py, attr_width, attr_height, attr, is_key=is_key)
        draw_line(ax, px - attr_width / 2, py, x + width / 2, y, width=1.4)


# ==================== 实体与属性定义 ====================
entities = {
    "task": {
        "label": "扫描任务 (Task)",
        "pos": (4, 14),
        "size": (2.6, 1.1),
        "attributes": [
            ("ID (string)", True),
            ("Total (int)", False),
            ("Done (int)", False),
            ("Created (time.Time)", False),
            ("m (sync.Mutex)", False),
            ("ch (chan SSEMessage)", False),
            ("stop (chan struct{})", False),
            ("stopped (bool)", False),
        ]
    },
    "result": {
        "label": "扫描结果 (SSEMessage)",
        "pos": (10, 14),
        "size": (2.6, 1.1),
        "attributes": [
            ("TaskID (string)", True),
            ("Type (string)", False),
            ("Progress (string)", False),
            ("Percent (int)", False),
            ("Data (interface{})", False),
        ]
    },
    "poc": {
        "label": "POC规则 (POC)",
        "pos": (16, 14),
        "size": (2.6, 1.1),
        "attributes": [
            ("Name (string)", True),
            ("Method (string)", False),
            ("Path (string)", False),
            ("Body (string)", False),
            ("Headers (map[string]string)", False),
            ("Match (string)", False),
            ("MatchHeaders (map[string]string)", False),
            ("MatchBodyAny ([]string)", False),
            ("MatchBodyAll ([]string)", False),
            ("Retry (int)", False),
            ("RetryDelayMs (int)", False),
        ]
    },
    "waf": {
        "label": "WAF绕过测试 (WAFBypassReq)",
        "pos": (22, 14),
        "size": (2.8, 1.1),
        "attributes": [
            ("BaseURL (string)", True),
            ("Path (string)", False),
            ("Payloads ([]string)", False),
            ("Methods ([]string)", False),
            ("Strategies ([]string)", False),
            ("Match (string)", False),
            ("Concurrency (int)", False),
            ("TimeoutMs (int)", False),
        ]
    },
    "session": {
        "label": "AI会话 (BaseChatSession)",
        "pos": (4, 9),
        "size": (2.6, 1.1),
        "attributes": [
            ("ID (string)", True),
            ("APIKey (string)", False),
            ("APIType (string)", False),
            ("Messages ([]ChatMessage)", False),
            ("MCPConnection (MCPConnection)", False),
            ("CreatedAt (time.Time)", False),
            ("LastActivity (time.Time)", False),
            ("mu (sync.RWMutex)", False),
        ]
    },
    "message": {
        "label": "聊天消息 (ChatMessage)",
        "pos": (10, 9),
        "size": (2.6, 1.1),
        "attributes": [
            ("Role (string)", True),
            ("Content (string)", False),
            ("Time (string)", False),
            ("ToolCallID (string)", False),
            ("ToolCalls ([]AIToolCall)", False),
        ]
    },
    "mcp_conn": {
        "label": "MCP连接 (JADX/IDA)",
        "pos": (16, 9),
        "size": (2.8, 1.1),
        "attributes": [
            ("baseURL (string)", True),
            ("client (*http.Client)", False),
            ("connected (bool)", False),
            ("tools ([]MCPTool)", False),
            ("lastRequestID (int64)", False),
            ("sessionID (string)", False),
            ("sseConn (*http.Response)", False),
            ("sseReader (*bufio.Reader)", False),
            ("sseWriter (io.WriteCloser)", False),
            ("pendingReqs (map[int64]chan map[string]interface{})", False),
            ("sseReady (bool)", False),
            ("stopReader (chan struct{})", False),
            ("mu (sync.RWMutex)", False),
            ("sseMu (sync.Mutex)", False),
        ]
    },
    "mcp_tool": {
        "label": "MCP工具 (MCPTool)",
        "pos": (22, 9),
        "size": (2.4, 1.1),
        "attributes": [
            ("Name (string)", True),
            ("Description (string)", False),
            ("Parameters (map[string]interface{})", False),
        ]
    },
    "upload": {
        "label": "上传文件",
        "pos": (4, 4),
        "size": (2.6, 1.1),
        "attributes": [
            ("FilePath (string)", True),
            ("FileName (string)", False),
            ("FileSize (int64)", False),
            ("UploadTime (time.Time)", False),
        ]
    },
    "tool_call": {
        "label": "AI工具调用 (AIToolCall)",
        "pos": (10, 4),
        "size": (2.6, 1.1),
        "attributes": [
            ("ID (string)", True),
            ("Name (string)", False),
            ("Arguments (map[string]interface{})", False),
        ]
    },
}


# ==================== 绘制实体 ====================
entity_info = {}
for key, meta in entities.items():
    info = draw_entity(ax, meta["pos"][0], meta["pos"][1], meta["size"][0], meta["size"][1], meta["label"])
    entity_info[key] = info
    distribute_attributes(ax, info, meta["attributes"])


# ==================== 绘制关系 ====================
# 1. 扫描任务 -> 扫描结果 (产生)
r_produce = draw_relationship(ax, 7, 14, 0.55, '产生')
draw_line(ax, entity_info["task"][0] + entity_info["task"][2] / 2, entity_info["task"][1], r_produce[0] - r_produce[2], r_produce[1])
draw_line(ax, r_produce[0] + r_produce[2], r_produce[1], entity_info["result"][0] - entity_info["result"][2] / 2, entity_info["result"][1])
draw_cardinality(ax, 5.7, 14.6, '1')
draw_cardinality(ax, 8.3, 14.6, 'N')

# 2. POC规则 -> 扫描任务 (应用于)
r_apply = draw_relationship(ax, 10, 11.5, 0.55, '应用于', facecolor='lavender')
draw_line(ax, entity_info["poc"][0] - entity_info["poc"][2] / 2, entity_info["poc"][1] - 0.4, 13, 12.2, color='blue', width=2.2)
draw_line(ax, 13, 12.2, r_apply[0], r_apply[1] + r_apply[2], color='blue', width=2.2)
draw_line(ax, r_apply[0], r_apply[1] - r_apply[2], 7.2, 11.5, color='blue', width=2.2)
draw_line(ax, 7.2, 11.5, entity_info["task"][0] + entity_info["task"][2] / 2, entity_info["task"][1] - 0.4, color='blue', width=2.2)
draw_cardinality(ax, 12.2, 11.8, '1')
draw_cardinality(ax, 6.4, 12.0, 'N')

# 3. WAF绕过测试 -> POC规则 (基于)
r_based = draw_relationship(ax, 19, 14, 0.55, '基于', facecolor='mistyrose')
draw_line(ax, entity_info["waf"][0] - entity_info["waf"][2] / 2, entity_info["waf"][1], r_based[0] + r_based[2], r_based[1], width=2)
draw_line(ax, r_based[0] - r_based[2], r_based[1], entity_info["poc"][0] + entity_info["poc"][2] / 2, entity_info["poc"][1], width=2)
draw_cardinality(ax, 20.4, 14.5, 'N')
draw_cardinality(ax, 17.6, 14.5, '1')

# 4. AI会话 -> 聊天消息 (包含)
r_contain = draw_relationship(ax, 7, 9, 0.55, '包含')
draw_line(ax, entity_info["session"][0] + entity_info["session"][2] / 2, entity_info["session"][1], r_contain[0] - r_contain[2], r_contain[1])
draw_line(ax, r_contain[0] + r_contain[2], r_contain[1], entity_info["message"][0] - entity_info["message"][2] / 2, entity_info["message"][1])
draw_cardinality(ax, 5.7, 9.4, '1')
draw_cardinality(ax, 8.3, 9.4, 'N')

# 5. 聊天消息 -> MCP连接 (关联)
r_connect = draw_relationship(ax, 13, 9, 0.55, '关联')
draw_line(ax, entity_info["message"][0] + entity_info["message"][2] / 2, entity_info["message"][1], r_connect[0] - r_connect[2], r_connect[1])
draw_line(ax, r_connect[0] + r_connect[2], r_connect[1], entity_info["mcp_conn"][0] - entity_info["mcp_conn"][2] / 2, entity_info["mcp_conn"][1])
draw_cardinality(ax, 11.7, 9.4, '1')
draw_cardinality(ax, 14.3, 9.4, '0..1')

# 6. MCP连接 -> MCP工具 (提供)
r_provide = draw_relationship(ax, 19, 9, 0.55, '提供')
draw_line(ax, entity_info["mcp_conn"][0] + entity_info["mcp_conn"][2] / 2, entity_info["mcp_conn"][1], r_provide[0] - r_provide[2], r_provide[1])
draw_line(ax, r_provide[0] + r_provide[2], r_provide[1], entity_info["mcp_tool"][0] - entity_info["mcp_tool"][2] / 2, entity_info["mcp_tool"][1])
draw_cardinality(ax, 17.7, 9.4, '1')
draw_cardinality(ax, 20.3, 9.4, 'N')

# 7. 扫描任务 <-> AI会话 (AI分析)
r_analyze = draw_relationship(ax, 1, 11.5, 0.55, 'AI分析', facecolor='mistyrose')
draw_line(ax, entity_info["task"][0] - entity_info["task"][2] / 2, entity_info["task"][1], 1, 14.4, color='red', width=2)
draw_line(ax, 1, 14.4, 1, 12.1, color='red', width=2)
draw_line(ax, 1, 12.1, r_analyze[0], r_analyze[1] + r_analyze[2], color='red', width=2)
draw_line(ax, r_analyze[0], r_analyze[1] - r_analyze[2], 1, 9.7, color='red', width=2)
draw_line(ax, 1, 9.7, entity_info["session"][0] - entity_info["session"][2] / 2, entity_info["session"][1], color='red', width=2)
draw_cardinality(ax, 0.2, 13.1, 'M')
draw_cardinality(ax, 0.2, 10.1, 'N')

# 8. 上传文件 -> 扫描任务 (用于)
r_use = draw_relationship(ax, 0.5, 6.5, 0.55, '用于', facecolor='lightgreen')
draw_line(ax, entity_info["upload"][0] - entity_info["upload"][2] / 2, entity_info["upload"][1] + 0.4, 0.5, 5.4, color='green', width=2)
draw_line(ax, 0.5, 5.4, 0.5, 6.0, color='green', width=2)
draw_line(ax, 0.5, 6.0, r_use[0], r_use[1] + r_use[2], color='green', width=2)
draw_line(ax, r_use[0], r_use[1] - r_use[2], 0.5, 12.9, color='green', width=2)
draw_line(ax, 0.5, 12.9, entity_info["task"][0] - entity_info["task"][2] / 2, entity_info["task"][1] - 0.4, color='green', width=2)
draw_cardinality(ax, -0.3, 5.8, 'N')
draw_cardinality(ax, -0.3, 12.3, 'M')

# 9. 聊天消息 -> AI工具调用 (调用)
r_invoke = draw_relationship(ax, 10, 6.5, 0.55, '调用')
draw_line(ax, entity_info["message"][0], entity_info["message"][1] - entity_info["message"][3] / 2, r_invoke[0], r_invoke[1] + r_invoke[2])
draw_line(ax, r_invoke[0], r_invoke[1] - r_invoke[2], entity_info["tool_call"][0], entity_info["tool_call"][1] + entity_info["tool_call"][3] / 2)
draw_cardinality(ax, 9.25, 7.3, '1')
draw_cardinality(ax, 9.25, 5.7, 'N')


# ==================== 图例与说明 ====================
legend_x, legend_y = 14, 1.0
ax.text(legend_x, legend_y + 1.0, 'ER图符号说明（陈氏表示法）',
        ha='center', fontsize=13, fontweight='bold',
        bbox=dict(boxstyle='round,pad=0.6', facecolor='lightyellow', edgecolor='black', linewidth=2))

draw_entity(ax, legend_x - 5.0, legend_y - 0.2, 1.6, 0.7, '实体')
draw_relationship(ax, legend_x, legend_y - 0.2, 0.35, '关系')
draw_attribute(ax, legend_x + 5.0, legend_y - 0.2, 1.6, 0.8, '普通属性')
draw_attribute(ax, legend_x - 8.5, legend_y - 0.2, 1.6, 0.8, '主键属性', is_key=True)

ax.text(legend_x, legend_y - 1.0, '矩形=实体  菱形=关系  椭圆=属性  金色=主键',
        ha='center', fontsize=11)
ax.text(legend_x, legend_y - 1.5, '右侧往往显示引用、连接、调用关系；左侧为基础数据实体',
        ha='center', fontsize=10, style='italic')

# 左侧业务说明
ax.text(3.0, 0.4,
        '业务模块:\n• 端口/目录/POC扫描\n• WAF绕过测试\n• AI安全分析\n• IDA/JADX逆向',
        ha='left', fontsize=10,
        bbox=dict(boxstyle='round,pad=0.5', facecolor='lightcyan', edgecolor='black', linewidth=2))

# 右侧技术说明
ax.text(25.5, 0.4,
        '存储特点:\n• 运行时内存结构\n• 多通道SSE推送\n• Go语言并发同步',
        ha='right', fontsize=10, style='italic',
        bbox=dict(boxstyle='round,pad=0.5', facecolor='lightgray', edgecolor='black', linewidth=2))

# 关系颜色说明
ax.text(14, -0.6, '颜色说明: 蓝色=POC/扫描  红色=AI分析  绿色=上传关联',
        ha='center', fontsize=10, style='italic')


# ==================== 保存输出 ====================
plt.title('NeonScan系统ER图（陈氏表示法，完整属性版）', fontsize=20, fontweight='bold', pad=28)
plt.tight_layout()

output_png = 'c:/Users/86483/Desktop/桌面/bishev3/bishe/shili/论文/图/ER图/ER图_陈氏表示法_full.png'
output_pdf = 'c:/Users/86483/Desktop/桌面/bishev3/bishe/shili/论文/图/ER图/ER图_陈氏表示法_full.pdf'

plt.savefig(output_png, dpi=300, bbox_inches='tight', facecolor='white')
try:
    plt.savefig(output_pdf, bbox_inches='tight', facecolor='white')
    print(f"PDF 已生成: {output_pdf}")
except PermissionError:
    print(f"PDF 输出被跳过（文件可能被占用）: {output_pdf}")

print("✅ ER图已生成（完整属性版）:")
print(f"   PNG: {output_png}")

# 可选：显示图形
# plt.show()


