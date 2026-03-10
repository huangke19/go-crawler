package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]

	switch command {
	case "login":
		handleLogin()
	case "download", "dl":
		handleDownload()
	case "bot":
		handleBot()
	case "setup-bot":
		handleSetupBot()
	case "version", "-v", "--version":
		fmt.Printf("Instagram Crawler v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("未知命令: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Instagram Crawler - 优雅的 Instagram 内容下载工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  crawler login                           登录 Instagram 账户")
	fmt.Println("  crawler download <username> <index>     下载指定用户的第 N 个帖子")
	fmt.Println("  crawler dl <username> <index>           download 的简写")
	fmt.Println("  crawler bot                             启动 Telegram Bot 服务")
	fmt.Println("  crawler setup-bot                       显示 Telegram Bot 命令设置指南")
	fmt.Println("  crawler version                         显示版本信息")
	fmt.Println("  crawler help                            显示帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  crawler login                           # 首次使用需要登录")
	fmt.Println("  crawler download nike 1                 # 下载 @nike 的第 1 个帖子")
	fmt.Println("  crawler dl instagram 5                  # 下载 @instagram 的第 5 个帖子")
	fmt.Println("  crawler bot                             # 启动 Telegram Bot")
	fmt.Println("  crawler setup-bot                       # 设置 Bot 命令菜单")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -h, --help                              显示帮助信息")
	fmt.Println("  -v, --version                           显示版本信息")
}

func handleLogin() {
	fmt.Println("=== Instagram 登录 ===")
	fmt.Println("浏览器将自动打开，请手动完成登录...")
	fmt.Println()

	if err := ManualLogin(); err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ 登录成功！会话已保存")
}

func handleDownload() {
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	downloadCmd.Usage = func() {
		fmt.Println("用法: crawler download <username> <index>")
		fmt.Println()
		fmt.Println("参数:")
		fmt.Println("  username    Instagram 用户名（不含 @ 符号）")
		fmt.Println("  index       帖子序号（从 1 开始）")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  crawler download nike 1")
		fmt.Println("  crawler dl instagram 5")
	}

	if len(os.Args) < 4 {
		downloadCmd.Usage()
		os.Exit(1)
	}

	targetUsername := os.Args[2]
	postIndexStr := os.Args[3]

	postIndex, err := strconv.Atoi(postIndexStr)
	if err != nil || postIndex < 1 {
		fmt.Println("❌ 错误: 帖子序号必须是大于 0 的整数")
		fmt.Println()
		downloadCmd.Usage()
		os.Exit(1)
	}

	startTime := time.Now()

	fmt.Printf("=== Instagram 帖子下载器 ===\n")
	fmt.Printf("目标用户: @%s\n", targetUsername)
	fmt.Printf("帖子序号: 第 %d 条\n\n", postIndex)

	fmt.Println("正在启动浏览器...")
	ctx, cancel := CreateFastBrowserContext()
	defer cancel()

	fmt.Println("正在验证登录状态...")
	if err := EnsureLoggedIn(ctx); err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		fmt.Println("\n提示: 请先运行 'crawler login' 登录账户")
		os.Exit(1)
	}

	if err := NavigateToUser(ctx, targetUsername); err != nil {
		fmt.Printf("❌ 访问用户主页失败: %v\n", err)
		os.Exit(1)
	}

	postURL, err := GetPostByIndex(ctx, postIndex)
	if err != nil {
		fmt.Printf("❌ 获取帖子失败: %v\n", err)
		os.Exit(1)
	}

	mediaInfo, err := ExtractMediaURLs(ctx, postURL)
	if err != nil {
		fmt.Printf("❌ 提取媒体失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n开始下载...")
	if err := DownloadPost(targetUsername, postIndex, mediaInfo); err != nil {
		fmt.Printf("❌ 下载失败: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n✓ 全部完成！文件已保存到 downloads/%s/\n", targetUsername)
	fmt.Printf("总耗时: %.2f 秒\n", elapsed.Seconds())
}

func handleBot() {
	fmt.Println("=== Instagram Telegram Bot ===")
	fmt.Println()

	// 加载配置
	config, err := LoadConfig("config.json")
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		fmt.Println("\n提示:")
		fmt.Println("1. 复制 config.example.json 为 config.json")
		fmt.Println("2. 在 config.json 中填入你的 Telegram Bot Token")
		fmt.Println("3. 可选：配置 allowed_user_ids 限制访问权限")
		os.Exit(1)
	}

	// 创建 bot
	bot, err := NewTelegramBot(config.TelegramBotToken, config.AllowedUserIDs, config.FavoriteAccounts)
	if err != nil {
		fmt.Printf("❌ 启动 Bot 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Bot 已启动，等待消息...")
	if len(config.AllowedUserIDs) > 0 {
		fmt.Printf("🔒 已启用白名单模式，允许的用户 ID: %v\n", config.AllowedUserIDs)
	} else {
		fmt.Println("⚠️  未配置白名单，所有用户都可以使用")
	}
	fmt.Println()

	// 启动 bot（阻塞）
	bot.Start()
}
