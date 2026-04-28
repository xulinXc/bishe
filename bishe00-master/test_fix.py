#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试修复效果的脚本
"""
import requests
import json
import time

def test_no_verify():
    """测试不启用验证的情况"""
    print("=" * 60)
    print("测试1: 不启用验证，检查AI生成的代码")
    print("=" * 60)
    
    with open('test_no_verify.json', 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    try:
        response = requests.post(
            'http://localhost:8080/ai/exp/python',
            json=data,
            timeout=30
        )
        
        if response.status_code == 200:
            result = response.json()
            python_code = result.get('python', '')
            
            print(f"✓ API调用成功")
            print(f"✓ 漏洞类型: {result.get('category', '未知')}")
            print(f"✓ 代码长度: {len(python_code)} 字符")
            
            # 检查是否包含VULNERABLE标记
            if 'VULNERABLE' in python_code:
                print("✓ 生成的代码包含 VULNERABLE 标记")
            else:
                print("✗ 生成的代码不包含 VULNERABLE 标记")
                
            # 检查是否使用了POC中的标记
            if 'ThinkPHP_RCE_Vulnerable' in python_code:
                print("⚠ 生成的代码使用了POC中的标记 ThinkPHP_RCE_Vulnerable")
                print("  这可能导致验证失败")
            else:
                print("✓ 生成的代码没有使用POC中的标记")
                
            # 保存生成的代码
            with open('generated_exp_no_verify.py', 'w', encoding='utf-8') as f:
                f.write(python_code)
            print("✓ 已保存生成的代码到 generated_exp_no_verify.py")
            
        else:
            print(f"✗ API调用失败: {response.status_code}")
            print(response.text)
            
    except Exception as e:
        print(f"✗ 请求失败: {e}")

def test_with_verify():
    """测试启用验证的情况"""
    print("\n" + "=" * 60)
    print("测试2: 启用验证，检查AI自我修正机制")
    print("=" * 60)
    
    with open('test_verify_fix.json', 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    try:
        print("发送请求到API...")
        response = requests.post(
            'http://localhost:8080/ai/exp/python',
            json=data,
            timeout=120  # 验证可能需要较长时间
        )
        
        if response.status_code == 200:
            result = response.json()
            
            print(f"✓ API调用成功")
            print(f"✓ 漏洞类型: {result.get('category', '未知')}")
            print(f"✓ 验证状态: {result.get('verified', False)}")
            print(f"✓ 验证尝试次数: {result.get('verifyAttempts', 0)}")
            
            # 检查验证日志
            verify_logs = result.get('verifyLogs', [])
            if verify_logs:
                print(f"\n验证日志 (共{len(verify_logs)}条):")
                for i, log in enumerate(verify_logs[:20], 1):  # 只显示前20条
                    print(f"  {i}. {log}")
                if len(verify_logs) > 20:
                    print(f"  ... 还有 {len(verify_logs) - 20} 条日志")
            
            # 保存最终代码
            python_code = result.get('python', '')
            if python_code:
                with open('generated_exp_with_verify.py', 'w', encoding='utf-8') as f:
                    f.write(python_code)
                print(f"\n✓ 已保存最终代码到 generated_exp_with_verify.py")
                
                # 检查代码中的标记
                if 'VULNERABLE' in python_code:
                    print("✓ 最终代码包含 VULNERABLE 标记")
                if 'ThinkPHP_RCE_Vulnerable' in python_code:
                    print("⚠ 最终代码仍包含 POC标记 ThinkPHP_RCE_Vulnerable")
            
            if result.get('verified'):
                print("\n✓✓✓ 验证成功！AI自我修正机制正常工作 ✓✓✓")
            else:
                print("\n✗ 验证失败，需要进一步调试")
                
        else:
            print(f"✗ API调用失败: {response.status_code}")
            print(response.text)
            
    except Exception as e:
        print(f"✗ 请求失败: {e}")

def test_generated_exp():
    """测试生成的EXP是否可以实际使用"""
    print("\n" + "=" * 60)
    print("测试3: 测试生成的EXP是否可以实际使用")
    print("=" * 60)
    
    # 测试不启用验证生成的EXP
    try:
        with open('generated_exp_no_verify.py', 'r', encoding='utf-8') as f:
            code = f.read()
        
        print("测试 generated_exp_no_verify.py...")
        
        # 检查代码质量
        issues = []
        
        if 'VULNERABLE' not in code:
            issues.append("缺少 VULNERABLE 标记")
        
        if 'ThinkPHP_RCE_Vulnerable' in code:
            issues.append("使用了POC标记而非标准标记")
        
        if 'NEONSCAN_BEGIN' not in code or 'NEONSCAN_END' not in code:
            issues.append("缺少命令输出标记")
        
        if 'argparse' not in code:
            issues.append("缺少命令行参数解析")
        
        if issues:
            print("⚠ 发现以下问题:")
            for issue in issues:
                print(f"  - {issue}")
        else:
            print("✓ 代码质量检查通过")
            
    except FileNotFoundError:
        print("⚠ 文件不存在，请先运行测试1")

if __name__ == '__main__':
    print("开始测试修复效果...\n")
    
    # 等待服务器启动
    time.sleep(2)
    
    # 运行测试
    test_no_verify()
    test_with_verify()
    test_generated_exp()
    
    print("\n" + "=" * 60)
    print("测试完成！")
    print("=" * 60)