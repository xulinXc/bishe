// Package main 文件上传功能模块
// 该模块实现了文件上传功能，主要用于上传扫描字典、POC文件等
// 支持大文件上传（最大512MB）和自动清理过期文件
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ensureDir 确保目录存在，如果不存在则创建
// @param p 目录路径
// @return 错误信息
func ensureDir(p string) error {
	return os.MkdirAll(p, 0755)
}

// uploadHandler 文件上传处理器
// 处理HTTP文件上传请求，支持多文件上传，每个文件最大512MB
// 上传的文件将保存到uploads目录下的时间戳子目录中
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 增大表单内存限制到512MB，以支持大文件（如大型字典文件）上传
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 创建上传目录（使用时间戳确保唯一性）
	dir := filepath.Join("uploads", fmt.Sprintf("u-%d", time.Now().UnixNano()))
	if err := ensureDir(dir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取所有上传的文件
	files := r.MultipartForm.File["files"]
	const maxSize = 512 << 20 // 每个文件最大512MB（大字典支持）
	var saved []string        // 成功保存的文件路径列表

	// 处理每个文件
	for _, f := range files {
		// 打开源文件
		src, err := f.Open()
		if err != nil {
			continue
		}

		// 构建目标文件路径
		name := filepath.Base(f.Filename)
		dstPath := filepath.Join(dir, name)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			continue
		}

		// 复制文件内容，同时限制大小（防止超大文件）
		written, copyErr := io.Copy(dst, io.LimitReader(src, maxSize))
		dst.Close()
		src.Close()

		// 检查复制是否出错
		if copyErr != nil {
			os.Remove(dstPath) // 删除不完整的文件
			continue
		}

		// 检查文件是否为空
		if written <= 0 {
			os.Remove(dstPath)
			continue
		}

		// 如果文件超过限制，会被截断，但我们仍然接受它
		saved = append(saved, dstPath)
	}

	// 异步清理旧的上传文件（超过7天的文件）
	go cleanupOldUploads("uploads", 7*24*time.Hour)

	// 返回成功保存的文件路径列表
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"paths": saved})
}

// cleanupOldUploads 清理旧的上传文件
// 异步执行，删除指定目录下超过最大年龄的文件和目录
// @param base 基础目录路径
// @param maxAge 最大保留时间（超过此时间的文件将被删除）
func cleanupOldUploads(base string, maxAge time.Duration) {
	// 读取基础目录下的所有条目
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}

	now := time.Now()
	// 遍历所有条目
	for _, e := range entries {
		// 只处理目录（上传的文件都保存在时间戳目录中）
		if !e.IsDir() {
			continue
		}

		p := filepath.Join(base, e.Name())
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}

		// 如果文件修改时间超过最大年龄，删除整个目录
		if now.Sub(fi.ModTime()) > maxAge {
			_ = os.RemoveAll(p)
		}
	}
}
