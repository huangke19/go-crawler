package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// loadJSONFile 从磁盘读取 JSON 文件并反序列化到目标结构。
func loadJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// saveJSONFile 将数据序列化为 JSON 并写入磁盘。
func saveJSONFile(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// extractShortcode 从帖子 URL 中提取 shortcode。
// URL 形态可能为 `/p/<shortcode>/` 或 `/reel/<shortcode>/`，这里只做最小假设提取。
func extractShortcode(postURL string) string {
	// URL 格式: https://www.instagram.com/xxx/p/SHORTCODE/ 或 /reel/SHORTCODE/
	parts := strings.Split(postURL, "/")
	for i, part := range parts {
		if (part == "p" || part == "reel") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractShortcodeFromPath(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// 从路径中提取 shortcode: downloads/cache/SHORTCODE/...
	parts := strings.Split(files[0], string(filepath.Separator))
	for i, part := range parts {
		if part == "cache" && i+1 < len(parts) {
			shortcode := parts[i+1]
			// 验证 shortcode 不为空
			if shortcode != "" && shortcode != "." && shortcode != ".." {
				return shortcode
			}
		}
	}
	return ""
}
