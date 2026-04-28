#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""测试Python是否可以正常运行"""

import sys
import argparse

def main():
    parser = argparse.ArgumentParser(description='测试Python环境')
    parser.add_argument('--target', required=True, help='目标URL')
    parser.add_argument('--cmd', help='测试命令')
    args = parser.parse_args()
    
    print(f"Python版本: {sys.version}")
    print(f"Python路径: {sys.executable}")
    print(f"目标: {args.target}")
    print(f"命令: {args.cmd}")
    print("VULNERABLE")
    print("测试成功！")

if __name__ == '__main__':
    main()
