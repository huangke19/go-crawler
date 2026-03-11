// +build ignore

package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 测试 Shortcode 下载速度 ===\n")

	// 测试 1: GetDownloadHistory 性能
	fmt.Println("测试 1: GetDownloadHistory(50) 性能")
	start := time.Now()
	history := GetDownloadHistory(50)
	elapsed := time.Since(start)
	fmt.Printf("  结果: 获取 %d 条历史记录\n", len(history))
	fmt.Printf("  耗时: %v\n\n", elapsed)

	// 测试 2: 第二次调用（测试缓存）
	fmt.Println("测试 2: 第二次调用 GetDownloadHistory(50) - 测试缓存")
	start = time.Now()
	history2 := GetDownloadHistory(50)
	elapsed2 := time.Since(start)
	fmt.Printf("  结果: 获取 %d 条历史记录\n", len(history2))
	fmt.Printf("  耗时: %v\n", elapsed2)
	fmt.Printf("  加速比: %.2fx\n\n", float64(elapsed.Nanoseconds())/float64(elapsed2.Nanoseconds()))

	// 测试 3: 过滤特定用户
	fmt.Println("测试 3: 过滤用户 'guweiz' 的历史记录")
	start = time.Now()
	count := 0
	for _, item := range history {
		if item.Username == "guweiz" && count < 10 {
			count++
		}
	}
	elapsed3 := time.Since(start)
	fmt.Printf("  结果: 找到 %d 条记录\n", count)
	fmt.Printf("  耗时: %v\n\n", elapsed3)

	// 测试 4: 完整的 showShortcodeSelection 流程
	fmt.Println("测试 4: 完整的 shortcode 选择流程（模拟 bot）")
	start = time.Now()

	history4 := GetDownloadHistory(50)
	var shortcodes []string
	count4 := 0
	for _, item := range history4 {
		if count4 >= 10 {
			break
		}
		if item.Username == "guweiz" {
			shortcode := extractShortcodeFromPath(item.Files)
			if shortcode != "" {
				shortcodes = append(shortcodes, shortcode)
				count4++
			}
		}
	}

	elapsed4 := time.Since(start)
	fmt.Printf("  结果: 找到 %d 个 shortcode\n", len(shortcodes))
	fmt.Printf("  耗时: %v\n", elapsed4)
	if len(shortcodes) > 0 {
		fmt.Printf("  示例: %s\n", shortcodes[0])
	}

	fmt.Println("\n=== 测试完成 ===")
}

func extractShortcodeFromPath(files []string) string {
	if len(files) == 0 {
		return ""
	}

	parts := []rune(files[0])
	var result string
	inCache := false
	slashCount := 0

	for i := 0; i < len(files[0]); i++ {
		if files[0][i] == '/' {
			slashCount++
			if slashCount == 3 {
				// 找到第三个斜杠后的内容
				for j := i + 1; j < len(files[0]); j++ {
					if files[0][j] == '/' {
						break
					}
					result += string(files[0][j])
				}
				break
			}
		}
	}

	return result
}
