// ============================================================================
// telegram_bot.go - Telegram Bot 核心结构体、生命周期与工具函数
// ============================================================================
//
// 职责：
//   - 定义 TelegramClient 结构体与 UserState 状态机
//   - 创建 Bot 实例（NewTelegramClient）
//   - 启动 Update Loop（Start）与过期状态清理
//   - 权限校验（白名单 / 管理员）
//   - 底层消息发送工具（sendMessage, editMessage, sendFile 等）
//
// 核心概念：
//   - Bot 是"控制面"：负责交互、权限检查、文件上传
//   - Worker 是"执行面"：负责耗时的抓取与下载
//   - 分离设计：避免 Bot 因长耗时操作而无法响应用户
//
// 文件拆分：
//   - telegram_bot.go      — 本文件：结构体、生命周期、工具函数
//   - telegram_handlers.go — 命令/回调处理、常用账户管理
//   - telegram_worker.go   — Worker HTTP 交互（下载、检查更新）
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type UserState struct {
	Step      string
	Username  string
	Timestamp time.Time
}

// TelegramClient 是 Telegram 控制面：
// - 负责命令/按钮交互、权限校验、轻量状态机管理；
// - 通过 HTTP 调用本机 worker 执行下载；
// - 将 worker 返回的本地文件上传回 Telegram。
//
// 回调按钮（CallbackQuery）有严格时效，处理时需优先快速 `answerCallback`，
// 避免用户端出现"按钮无响应/超时"的体验问题。
type TelegramClient struct {
	bot              *tgbotapi.BotAPI
	allowedUsers     map[int64]bool
	adminUsers       map[int64]bool
	favoriteAccounts []string
	accountsMu       sync.RWMutex
	userStates       map[int64]*UserState
	statesMutex      sync.RWMutex
	workerBaseURL    string
	configPath       string
	shortClient      *http.Client // 短超时 HTTP 客户端（健康检查等）
	longClient       *http.Client // 长超时 HTTP 客户端（下载/更新等耗时请求）
}

// NewTelegramClient 构建 bot 实例并初始化权限与默认配置。
// - allowed_user_ids 为空则为开放模式（不做用户限制）
// - admin_user_ids 为空时通常会回退为 allowed_user_ids（见 config.go 的兼容逻辑）
func NewTelegramClient(config *Config) (*TelegramClient, error) {
	bot, err := tgbotapi.NewBotAPI(config.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("创建 bot 失败: %w", err)
	}

	allowedUsers := make(map[int64]bool)
	for _, id := range config.AllowedUserIDs {
		allowedUsers[id] = true
	}

	adminUsers := make(map[int64]bool)
	for _, id := range config.AdminUserIDs {
		adminUsers[id] = true
	}

	favoriteAccounts := config.FavoriteAccounts
	if len(favoriteAccounts) == 0 {
		favoriteAccounts = []string{"nike", "instagram", "natgeo"}
	}

	workerBaseURL := config.GetWorkerBaseURL()
	log.Printf("Telegram Bot 已启动: @%s", bot.Self.UserName)
	log.Printf("Worker 地址: %s", workerBaseURL)

	tb := &TelegramClient{
		bot:              bot,
		allowedUsers:     allowedUsers,
		adminUsers:       adminUsers,
		favoriteAccounts: favoriteAccounts,
		userStates:       make(map[int64]*UserState),
		workerBaseURL:    workerBaseURL,
		configPath:       "config.json",
		shortClient:      &http.Client{Timeout: 5 * time.Second},
		longClient:       &http.Client{Timeout: 3 * time.Minute},
	}

	// 启动状态清理 goroutine
	go tb.cleanupExpiredStates()

	return tb, nil
}

// cleanupExpiredStates 定期清理过期的用户状态
func (tb *TelegramClient) cleanupExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		tb.statesMutex.Lock()
		now := time.Now()
		for userID, state := range tb.userStates {
			if now.Sub(state.Timestamp) > 5*time.Minute {
				delete(tb.userStates, userID)
				log.Printf("清理过期状态: user=%d", userID)
			}
		}
		tb.statesMutex.Unlock()
	}
}

// Start 启动 bot 的 update loop。
//
// 健壮性策略：
// - 外层与每条 update 都有 panic 恢复，避免单条异常导致 bot 整体退出；
// - callback 与 message 分开处理，callback 优先响应以避免 Telegram 超时。
func (tb *TelegramClient) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := tb.bot.GetUpdatesChan(u)

	// 添加 panic 恢复，确保 bot 不会崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Bot panic 恢复: %v", r)
			// 可以在这里添加重启逻辑或通知管理员
		}
	}()

	for update := range updates {
		// 为每个更新添加 panic 保护
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ 处理更新时 panic: %v", r)
				}
			}()

			if update.CallbackQuery != nil {
				if !tb.isAllowedUser(update.CallbackQuery.From.ID) {
					tb.answerCallback(update.CallbackQuery.ID, "❌ 未授权访问")
					return
				}
				tb.handleCallback(update.CallbackQuery)
				return
			}

			if update.Message == nil {
				return
			}

			if !tb.isAllowedUser(update.Message.From.ID) {
				tb.sendMessage(update.Message.Chat.ID, "❌ 未授权访问")
				log.Printf("未授权用户尝试访问: %d (@%s)", update.Message.From.ID, update.Message.From.UserName)
				return
			}

			if update.Message.IsCommand() {
				tb.handleCommand(update.Message)
			} else {
				tb.handleMessage(update.Message)
			}
		}()
	}
}

// isAllowedUser 判断 userID 是否有使用权限。\n+// allowedUsers 为空表示不限制。
func (tb *TelegramClient) isAllowedUser(userID int64) bool {
	if len(tb.allowedUsers) == 0 {
		return true
	}
	return tb.allowedUsers[userID]
}

// isAdminUser 判断 userID 是否有管理员权限。\n+// adminUsers 为空时回退为 allowedUsers 的规则（保持兼容）。
func (tb *TelegramClient) isAdminUser(userID int64) bool {
	if len(tb.adminUsers) == 0 {
		return tb.isAllowedUser(userID)
	}
	return tb.adminUsers[userID]
}
// sendMessage 发送普通文本消息。\n+// 发送失败仅记录日志，不中断主流程。
func (tb *TelegramClient) sendMessage(chatID int64, text string) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := tb.bot.Send(msg)
	if err != nil {
		log.Printf("发送消息失败: %v", err)
	}
	return sentMsg
}

// editMessage 编辑消息，用于更新进度/结果展示。
func (tb *TelegramClient) editMessage(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("编辑消息失败: %v", err)
	}
}

// editMessageWithKeyboard 编辑消息并附带 inline keyboard。\n+// 常用于控制面板"原地刷新"。
func (tb *TelegramClient) editMessageWithKeyboard(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ReplyMarkup = &keyboard
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("编辑消息失败: %v", err)
	}
}

// getVideoResolution 使用 ffprobe 获取视频的宽高信息。
// 如果 ffprobe 不可用或获取失败，返回 0, 0。
func getVideoResolution(filePath string) (int, int) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", filePath)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) != 2 {
		return 0, 0
	}

	width, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return width, height
}

// sendFile 上传文件到 Telegram。
// 对于视频，会尝试获取宽高信息以保持原有的长宽比显示。
// 如果 ffprobe 不可用，会降级到不设置宽高（Telegram 会使用默认显示）。
func (tb *TelegramClient) sendFile(chatID int64, filePath string) error {
	if strings.HasSuffix(filePath, ".mp4") {
		video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))

		// 尝试获取视频宽高，以保持原有长宽比
		_, _ = getVideoResolution(filePath)

		_, err := tb.bot.Send(video)
		return err
	}
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(filePath))
	_, err := tb.bot.Send(photo)
	return err
}
func (tb *TelegramClient) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := tb.bot.Request(callback); err != nil {
		// 忽略超时错误，避免日志污染
		if !strings.Contains(err.Error(), "query is too old") {
			log.Printf("回应回调失败: %v", err)
		}
	}
}

// sendChatAction 发送聊天动作状态（显示"正在输入"、"正在上传"等）
func (tb *TelegramClient) sendChatAction(chatID int64, action string) {
	chatAction := tgbotapi.NewChatAction(chatID, action)
	if _, err := tb.bot.Request(chatAction); err != nil {
		// 忽略错误，不影响主流程
		log.Printf("发送 ChatAction 失败: %v", err)
	}
}
