//go:build !gobot
// +build !gobot

// ============================================================================
// main.go - Instagram 爬虫主入口
// ============================================================================
//
// 职责：
//   - CLI 参数解析与子命令分发
//   - 提供 5 种运行模式：login / download / bot / worker / setup-bot
//   - 每种模式对应不同的业务流程
//
// 核心流程：
//   1. login: 打开有头浏览器 -> 用户手动登录 -> 保存 Cookie 到 .instagram_session.json
//   2. download: 启动无头浏览器 -> 验证登录 -> 访问用户主页 -> 定位帖子 -> 提取媒体 URL -> 并发下载
//   3. bot: 启动 Telegram Bot -> 监听消息 -> 通过 HTTP 调用 worker 执行下载 -> 上传文件到 Telegram
//   4. worker: 启动 HTTP 服务 -> 复用浏览器实例 -> 优先使用三层缓存 -> 执行下载任务
//   5. setup-bot: 显示 Telegram Bot 命令菜单设置指南
//
// 关键设计：
//   - Bot 与 Worker 分离：Bot 负责交互，Worker 负责耗时操作（避免 Bot 阻塞）
//   - 三层缓存：媒体 URL 缓存 / 帖子列表缓存 / 文件路径缓存
//   - 并发下载：最多 10 个并发连接，提升下载速度
//   - 优雅关闭：等待活跃请求完成，避免中断下载
//
// ============================================================================

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// version 用于 `crawler version` 输出。
// 这里不做自动注入，便于直接编译/分发；如需与 CI/构建脚本集成，可改为 ldflags 注入。
const version = "1.0.0"

// main 是 `crawler` 主入口，根据子命令分发到不同的运行模式：
// - login: 手动登录并保存 Cookie 会话
// - download/dl: 本地下载（浏览器 + GraphQL + 并发下载）
// - bot: Telegram 控制面（交互/上传，不执行重下载）
// - worker: 执行面 HTTP 服务（可复用浏览器、使用缓存）
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
	case "check-update", "cu":
		handleCheckUpdate()
	case "bot":
		handleBot()
	case "worker":
		handleWorker()
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

// printUsage 输出帮助信息。CLI 的参数解析按"子命令 + 位置参数"设计，
// 以便 bot/worker/下载三种模式的入口尽量简洁明确。
func printUsage() {
	fmt.Println("Instagram Crawler - 优雅的 Instagram 内容下载工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  crawler login                           登录 Instagram 账户")
	fmt.Println("  crawler download <username> <index>     下载指定用户的第 N 个帖子")
	fmt.Println("  crawler dl <username> <index>           download 的简写")
	fmt.Println("  crawler check-update <username>         检查并刷新用户帖子缓存")
	fmt.Println("  crawler cu <username>                   check-update 的简写")
	fmt.Println("  crawler bot                             启动 Telegram Bot 服务")
	fmt.Println("  crawler worker                          启动 Worker 服务（供 Bot 调用）")
	fmt.Println("  crawler setup-bot                       显示 Telegram Bot 命令设置指南")
	fmt.Println("  crawler version                         显示版本信息")
	fmt.Println("  crawler help                            显示帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  crawler login                           # 首次使用需要登录")
	fmt.Println("  crawler download nike 1                 # 下载 @nike 的第 1 个帖子")
	fmt.Println("  crawler dl instagram 5                  # 下载 @instagram 的第 5 个帖子")
	fmt.Println("  crawler check-update nike               # 检查 @nike 是否有新帖子")
	fmt.Println("  crawler cu instagram                    # check-update 简写")
	fmt.Println("  crawler bot                             # 启动 Telegram Bot")
	fmt.Println("  crawler worker                          # 启动 Worker 服务")
	fmt.Println("  crawler setup-bot                       # 设置 Bot 命令菜单")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -h, --help                              显示帮助信息")
	fmt.Println("  -v, --version                           显示版本信息")
}

// handleLogin 触发手动登录流程：打开有头浏览器，让用户完成登录/验证码/2FA，
// 并将 Cookie 保存到 `.instagram_session.json`（敏感文件，默认不应提交到 Git）。
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

// handleDownload 是本地下载模式入口。
// 该模式会启动一个"快速无头浏览器上下文"，验证 Cookie 是否有效，然后：
// 主页定位第 N 条帖子 -> GraphQL 获取媒体 URL -> 并发下载落盘。
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

// handleBot 启动 Telegram Bot（控制面）。
// Bot 负责交互与上传，不直接执行下载；实际下载由 worker 进程完成，以避免 bot 长耗时阻塞。
func handleBot() {
	fmt.Println("=== Instagram Telegram Bot ===")
	fmt.Println()

	config, err := LoadConfig("config.json")
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		fmt.Println("\n提示:")
		fmt.Println("1. 复制 config.example.json 为 config.json")
		fmt.Println("2. 在 config.json 中填入你的 Telegram Bot Token")
		fmt.Println("3. 可选：配置 allowed_user_ids 限制访问权限")
		os.Exit(1)
	}

	client, err := NewTelegramClient(config)
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

	client.Start()
}

// handleWorker 启动 Worker HTTP 服务（执行面）。
// Worker 会复用浏览器实例，并优先使用三层缓存（帖子列表/媒体 URL/文件路径）以减少重复抓取与下载。
func handleWorker() {
	fmt.Println("=== Instagram Worker 服务 ===")
	fmt.Printf("监听地址: %s\n", getWorkerListenAddr())
	fmt.Println("✅ Worker 已启动，等待任务...")

	if err := RunWorker(); err != nil {
		fmt.Printf("❌ Worker 运行失败: %v\n", err)
		os.Exit(1)
	}
}

// handleCheckUpdate 检查指定用户是否有新帖子，并刷新帖子列表缓存。
// 通过对比缓存中第 1 条 shortcode 与主页实际第 1 条来判断是否有新内容。
func handleCheckUpdate() {
	if len(os.Args) < 3 {
		fmt.Println("用法: crawler check-update <username>")
		fmt.Println("      crawler cu <username>")
		os.Exit(1)
	}

	username := strings.TrimSpace(os.Args[2])
	if username == "" {
		fmt.Println("❌ 错误: username 不能为空")
		os.Exit(1)
	}

	fmt.Printf("=== 检查更新: @%s ===\n\n", username)

	fmt.Println("正在启动浏览器...")
	ctx, cancel := CreateFastBrowserContext()
	defer cancel()

	fmt.Println("正在验证登录状态...")
	if err := EnsureLoggedIn(ctx); err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		fmt.Println("\n提示: 请先运行 'crawler login' 登录账户")
		os.Exit(1)
	}

	// 读取缓存中的第 1 条 shortcode
	var cachedFirstShortcode string
	if postsCache, ok := GetPostsFromCache(username); ok && len(postsCache.Posts) > 0 {
		cachedFirstShortcode = postsCache.Posts[0].Shortcode
		fmt.Printf("缓存中第 1 条帖子: %s（共 %d 条）\n", cachedFirstShortcode, len(postsCache.Posts))
	} else {
		fmt.Println("缓存不存在，将创建新缓存")
	}

	// 访问主页获取实际第 1 条
	fmt.Println("正在访问用户主页...")
	if err := NavigateToUser(ctx, username); err != nil {
		fmt.Printf("❌ 访问用户主页失败: %v\n", err)
		os.Exit(1)
	}

	postLinks, err := GetAllPostLinks(ctx, 12)
	if err != nil {
		fmt.Printf("❌ 获取帖子列表失败: %v\n", err)
		os.Exit(1)
	}

	if len(postLinks) == 0 {
		fmt.Println("❌ 未找到任何帖子")
		os.Exit(1)
	}

	actualFirstShortcode := extractShortcode(postLinks[0])
	fmt.Printf("主页实际第 1 条帖子: %s（共获取 %d 条）\n\n", actualFirstShortcode, len(postLinks))

	// 对比并更新缓存
	if cachedFirstShortcode == "" || cachedFirstShortcode != actualFirstShortcode {
		posts := []PostItem{}
		for i, link := range postLinks {
			sc := extractShortcode(link)
			if sc != "" {
				posts = append(posts, PostItem{
					Index:     i + 1,
					Shortcode: sc,
				})
			}
		}
		SavePostsToCache(username, &PostsCache{
			Posts:     posts,
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if cachedFirstShortcode == "" {
			fmt.Printf("✓ 已创建缓存（共 %d 条帖子）\n", len(posts))
		} else {
			fmt.Printf("✓ 检测到新帖子！已更新缓存（共 %d 条帖子）\n", len(posts))
		}
	} else {
		fmt.Println("✓ 已是最新，无新帖子")
	}
}
