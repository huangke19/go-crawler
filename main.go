package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("用法:")
		fmt.Println("  登录: go run . login")
		fmt.Println("  下载: go run . <目标用户名> <帖子序号>")
		fmt.Println("例如: go run . nike 5")
		return
	}

	// 处理登录命令
	if os.Args[1] == "login" {
		if err := ManualLogin(); err != nil {
			fmt.Printf("登录失败: %v\n", err)
			return
		}
		return
	}

	// 处理下载命令
	if len(os.Args) < 3 {
		fmt.Println("用法: go run . <目标用户名> <帖子序号>")
		fmt.Println("例如: go run . nike 5")
		return
	}

	targetUsername := os.Args[1]
	postIndex, err := strconv.Atoi(os.Args[2])
	if err != nil || postIndex < 1 {
		fmt.Println("错误: 帖子序号必须是大于 0 的整数")
		return
	}

	fmt.Printf("=== Instagram 帖子下载器 ===\n")
	fmt.Printf("目标用户: @%s\n", targetUsername)
	fmt.Printf("帖子序号: 第 %d 条\n\n", postIndex)

	// 创建浏览器上下文
	ctx, cancel := CreateBrowserContext()
	defer cancel()

	// 确保已登录
	if err := EnsureLoggedIn(ctx); err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}

	// 访问目标用户主页
	if err := NavigateToUser(ctx, targetUsername); err != nil {
		fmt.Printf("访问用户主页失败: %v\n", err)
		return
	}

	// 获取第 N 个帖子的链接
	postURL, err := GetPostByIndex(ctx, postIndex)
	if err != nil {
		fmt.Printf("获取帖子失败: %v\n", err)
		return
	}

	// 提取媒体 URL
	mediaInfo, err := ExtractMediaURLs(ctx, postURL)
	if err != nil {
		fmt.Printf("提取媒体失败: %v\n", err)
		return
	}

	// 下载媒体
	fmt.Println("\n开始下载...")
	if err := DownloadPost(targetUsername, postIndex, mediaInfo); err != nil {
		fmt.Printf("下载失败: %v\n", err)
		return
	}

	fmt.Printf("\n✓ 全部完成！文件已保存到 downloads/%s/\n", targetUsername)
}
