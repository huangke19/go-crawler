package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
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

type TelegramBot struct {
	bot              *tgbotapi.BotAPI
	allowedUsers     map[int64]bool
	adminUsers       map[int64]bool
	favoriteAccounts []string
	userStates       map[int64]*UserState
	statesMutex      sync.RWMutex
	workerBaseURL    string
}

func NewTelegramBot(config *Config) (*TelegramBot, error) {
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

	tb := &TelegramBot{
		bot:              bot,
		allowedUsers:     allowedUsers,
		adminUsers:       adminUsers,
		favoriteAccounts: favoriteAccounts,
		userStates:       make(map[int64]*UserState),
		workerBaseURL:    workerBaseURL,
	}

	// 启动状态清理 goroutine
	go tb.cleanupExpiredStates()

	return tb, nil
}

// cleanupExpiredStates 定期清理过期的用户状态
func (tb *TelegramBot) cleanupExpiredStates() {
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

func (tb *TelegramBot) Start() {
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

func (tb *TelegramBot) isAllowedUser(userID int64) bool {
	if len(tb.allowedUsers) == 0 {
		return true
	}
	return tb.allowedUsers[userID]
}

func (tb *TelegramBot) isAdminUser(userID int64) bool {
	if len(tb.adminUsers) == 0 {
		return tb.isAllowedUser(userID)
	}
	return tb.adminUsers[userID]
}

func (tb *TelegramBot) handleCommand(message *tgbotapi.Message) {
	command := message.Command()
	args := strings.Fields(message.CommandArguments())

	log.Printf("收到命令: /%s %v (来自: @%s)", command, args, message.From.UserName)

	switch command {
	case "start":
		tb.handleStart(message)
	case "help":
		tb.handleHelp(message)
	case "download", "dl":
		tb.handleDownload(message, args)
	case "control":
		tb.handleControl(message)
	case "status":
		tb.handleStatus(message)
	default:
		tb.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 未知命令: /%s\n使用 /help 查看帮助", command))
	}
}

func (tb *TelegramBot) handleStart(message *tgbotapi.Message) {
	text := fmt.Sprintf("👋 你好 @%s！\n\n", message.From.UserName)
	text += "我是 Instagram 下载机器人，可以帮你下载 Instagram 帖子。\n\n"
	text += "使用 /help 查看可用命令"
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramBot) handleHelp(message *tgbotapi.Message) {
	text := "📖 可用命令:\n\n"
	text += "/download - 下载指定帖子（按钮交互）\n"
	text += "/dl - download 的简写\n"
	text += "/status - 查看 bot 状态\n"
	if tb.isAdminUser(message.From.ID) {
		text += "/control - 控制 worker 启动/停止/重启\n"
	}
	text += "/help - 显示帮助信息\n\n"
	text += "💡 使用方式:\n\n"
	text += "1️⃣ 发送 /download\n"
	text += "2️⃣ 点击账户按钮（或点击\"输入其他用户\"）\n"
	text += "3️⃣ 点击帖子序号 1-10（或点击\"输入其他序号\"）\n"
	text += "4️⃣ 等待下载完成"
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramBot) handleStatus(message *tgbotapi.Message) {
	text := "✅ Bot 运行正常\n\n"
	text += fmt.Sprintf("Bot 用户名: @%s\n", tb.bot.Self.UserName)
	text += fmt.Sprintf("你的用户 ID: %d\n", message.From.ID)
	text += fmt.Sprintf("worker 地址: %s\n", tb.workerBaseURL)

	runtime, err := GetServiceRuntime("worker")
	if err == nil {
		if runtime.Running {
			text += fmt.Sprintf("worker 状态: 运行中 (PID: %d)\n", runtime.PID)
		} else {
			text += "worker 状态: 未运行\n"
		}
	}

	text += fmt.Sprintf("当前时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramBot) handleControl(message *tgbotapi.Message) {
	if !tb.isAdminUser(message.From.ID) {
		tb.sendMessage(message.Chat.ID, "❌ 仅管理员可使用 /control")
		return
	}

	text := "🎛️ Worker 控制面板\n请选择操作:"
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tb.workerControlKeyboard()
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送控制面板失败: %v", err)
	}
}

func (tb *TelegramBot) handleDownload(message *tgbotapi.Message, args []string) {
	if len(args) > 0 {
		tb.sendMessage(message.Chat.ID, "💡 提示: 请使用按钮选择账户和帖子序号\n直接使用 /download 命令即可")
		return
	}

	text := "📥 请选择要下载的账户:\n"

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for i, account := range tb.favoriteAccounts {
		btn := tgbotapi.NewInlineKeyboardButtonData(account, "account:"+account)
		currentRow = append(currentRow, btn)
		if (i+1)%3 == 0 || i == len(tb.favoriteAccounts)-1 {
			rows = append(rows, currentRow)
			currentRow = []tgbotapi.InlineKeyboardButton{}
		}
	}

	inputOtherBtn := tgbotapi.NewInlineKeyboardButtonData("📝 输入其他用户", "input_account")
	rows = append(rows, []tgbotapi.InlineKeyboardButton{inputOtherBtn})

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送账户选择失败: %v", err)
	}
}

func (tb *TelegramBot) sendMessage(chatID int64, text string) tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := tb.bot.Send(msg)
	if err != nil {
		log.Printf("发送消息失败: %v", err)
	}
	return sentMsg
}

func (tb *TelegramBot) editMessage(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("编辑消息失败: %v", err)
	}
}

func (tb *TelegramBot) editMessageWithKeyboard(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ReplyMarkup = &keyboard
	if _, err := tb.bot.Send(edit); err != nil {
		log.Printf("编辑消息失败: %v", err)
	}
}

func (tb *TelegramBot) sendFile(chatID int64, filePath string) error {
	if strings.HasSuffix(filePath, ".mp4") {
		video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))
		_, err := tb.bot.Send(video)
		return err
	}
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(filePath))
	_, err := tb.bot.Send(photo)
	return err
}

func (tb *TelegramBot) handleCallback(callback *tgbotapi.CallbackQuery) {
	data := callback.Data

	switch {
	case strings.HasPrefix(data, "account:"):
		username := strings.TrimPrefix(data, "account:")
		tb.handleAccountSelection(callback, username)
	case data == "input_account":
		tb.handleInputAccountRequest(callback)
	case strings.HasPrefix(data, "mode:"):
		// mode:index:username 或 mode:shortcode:username
		parts := strings.SplitN(data, ":", 3)
		if len(parts) == 3 {
			mode := parts[1]
			username := parts[2]
			tb.handleModeSelection(callback, username, mode)
		}
	case strings.HasPrefix(data, "index:"):
		parts := strings.SplitN(data, ":", 3)
		if len(parts) == 3 {
			username := parts[1]
			postIndex, err := strconv.Atoi(parts[2])
			if err == nil && postIndex > 0 {
				tb.handleIndexSelection(callback, username, postIndex)
			}
		}
	case strings.HasPrefix(data, "sc:"):
		// sc:shortcode
		shortcode := strings.TrimPrefix(data, "sc:")
		tb.handleShortcodeSelection(callback, shortcode)
	case strings.HasPrefix(data, "refresh:"):
		username := strings.TrimPrefix(data, "refresh:")
		tb.handleRefreshCache(callback, username)
	case strings.HasPrefix(data, "input:"):
		username := strings.TrimPrefix(data, "input:")
		tb.handleInputRequest(callback, username)
	case strings.HasPrefix(data, "cancel:"):
		tb.handleCancel(callback)
	case strings.HasPrefix(data, "ctl:worker:"):
		tb.handleWorkerControl(callback)
	}
}

func (tb *TelegramBot) handleWorkerControl(callback *tgbotapi.CallbackQuery) {
	if !tb.isAdminUser(callback.From.ID) {
		tb.answerCallback(callback.ID, "❌ 仅管理员可操作")
		return
	}

	action := strings.TrimPrefix(callback.Data, "ctl:worker:")
	// 立即响应回调，避免超时
	tb.answerCallback(callback.ID, "✅ 已接收，处理中...")

	var result string
	var err error

	switch action {
	case "start":
		result, err = StartServiceDaemon("worker")
	case "stop":
		result, err = StopServiceDaemon("worker")
	case "restart":
		result, err = RestartServiceDaemon("worker")
	case "status":
		result, err = tb.workerStatusSummary()
	default:
		err = fmt.Errorf("未知控制动作: %s", action)
	}

	if err != nil {
		result = fmt.Sprintf("❌ 操作失败: %v", err)
	} else {
		result = "✅ " + result
	}

	if callback.Message != nil {
		tb.editMessageWithKeyboard(callback.Message.Chat.ID, callback.Message.MessageID, result, tb.workerControlKeyboard())
	} else {
		tb.sendMessage(callback.From.ID, result)
	}

	log.Printf("控制操作: user=%d action=%s result=%s", callback.From.ID, action, strings.ReplaceAll(result, "\n", " | "))
}

func (tb *TelegramBot) workerStatusSummary() (string, error) {
	runtime, err := GetServiceRuntime("worker")
	if err != nil {
		return "", err
	}

	if !runtime.Running {
		return "worker 未运行", nil
	}

	healthOK := tb.checkWorkerHealth()
	healthText := "健康检查失败"
	if healthOK {
		healthText = "健康检查通过"
	}

	return fmt.Sprintf("worker 运行中 (PID: %d)\n%s\n地址: %s", runtime.PID, healthText, tb.workerBaseURL), nil
}

func (tb *TelegramBot) workerControlKeyboard() tgbotapi.InlineKeyboardMarkup {
	row1 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("▶️ 启动", "ctl:worker:start"),
		tgbotapi.NewInlineKeyboardButtonData("⏹ 停止", "ctl:worker:stop"),
	)
	row2 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 重启", "ctl:worker:restart"),
		tgbotapi.NewInlineKeyboardButtonData("📊 状态", "ctl:worker:status"),
	)
	return tgbotapi.NewInlineKeyboardMarkup(row1, row2)
}

func (tb *TelegramBot) checkWorkerHealth() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(tb.workerBaseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (tb *TelegramBot) handleAccountSelection(callback *tgbotapi.CallbackQuery, username string) {
	tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已选择: @%s", username))
	tb.showModeSelection(callback.Message.Chat.ID, username)
}

// showModeSelection 显示下载模式选择
func (tb *TelegramBot) showModeSelection(chatID int64, username string) {
	text := fmt.Sprintf("📥 @%s\n\n请选择下载模式:", username)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ 按时间线下载", "mode:index:"+username),
			tgbotapi.NewInlineKeyboardButtonData("🔖 按Shortcode下载", "mode:shortcode:"+username),
		),
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送模式选择失败: %v", err)
	}
}

// handleModeSelection 处理模式选择
func (tb *TelegramBot) handleModeSelection(callback *tgbotapi.CallbackQuery, username, mode string) {
	tb.answerCallback(callback.ID, "")

	if mode == "index" {
		tb.showIndexSelection(callback.Message.Chat.ID, username)
	} else if mode == "shortcode" {
		tb.showShortcodeSelection(callback.Message.Chat.ID, username)
	}
}

// showShortcodeSelection 显示 Shortcode 选择（历史下载列表）
func (tb *TelegramBot) showShortcodeSelection(chatID int64, username string) {
	// 获取该用户的所有下载记录
	history := GetDownloadHistory(0) // 0 = 获取全部

	// 先收集该用户的所有记录
	var userItems []*FilesCache
	for _, item := range history {
		if item.Username != username {
			continue
		}
		userItems = append(userItems, item)
	}

	// 按帖子序号正序排序（第1条、第2条...）
	sort.Slice(userItems, func(i, j int) bool {
		// PostIndex 为 0 表示按 shortcode 下载的，放到最后
		if userItems[i].PostIndex == 0 && userItems[j].PostIndex == 0 {
			return false // 保持原顺序
		}
		if userItems[i].PostIndex == 0 {
			return false // i 放后面
		}
		if userItems[j].PostIndex == 0 {
			return true // j 放后面
		}
		return userItems[i].PostIndex < userItems[j].PostIndex
	})

	// 限制最多 100 个
	if len(userItems) > 100 {
		userItems = userItems[:100]
	}

	// 批量生成缩略图（并发，5 workers）
	thumbnails := GenerateThumbnailBatch(userItems, 5)

	// 发送缩略图网格
	if len(thumbnails) > 0 {
		tb.sendThumbnailGrid(chatID, userItems, thumbnails)
	}

	// 发送按钮列表
	tb.sendShortcodeButtons(chatID, username, userItems)
}

// sendThumbnailGrid 发送缩略图网格（使用 Telegram MediaGroup API）
func (tb *TelegramBot) sendThumbnailGrid(chatID int64, items []*FilesCache, thumbnails map[string]string) {
	// 每 10 个一组（Telegram MediaGroup 限制）
	for i := 0; i < len(items); i += 10 {
		end := i + 10
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]

		var mediaGroup []interface{}
		for _, item := range batch {
			shortcode := extractShortcodeFromPath(item.Files)
			thumbnailPath, ok := thumbnails[shortcode]
			if !ok {
				continue // 跳过无缩略图的项
			}

			// 构建 InputMediaPhoto
			photo := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(thumbnailPath))

			// 构建 caption：显示序号和 shortcode
			if item.PostIndex > 0 {
				photo.Caption = fmt.Sprintf("#%d - %s", item.PostIndex, shortcode)
			} else {
				photo.Caption = shortcode
			}

			mediaGroup = append(mediaGroup, photo)
		}

		// 发送 MediaGroup
		if len(mediaGroup) > 0 {
			msg := tgbotapi.NewMediaGroup(chatID, mediaGroup)
			if _, err := tb.bot.SendMediaGroup(msg); err != nil {
				log.Printf("发送缩略图网格失败: %v", err)
			}
			time.Sleep(500 * time.Millisecond) // 避免速率限制
		}
	}
}

// sendShortcodeButtons 发送按钮列表
func (tb *TelegramBot) sendShortcodeButtons(chatID int64, username string, items []*FilesCache) {
	// 构建按钮
	var rows [][]tgbotapi.InlineKeyboardButton
	count := 0

	for _, item := range items {
		// 获取 shortcode（从文件路径提取）
		shortcode := extractShortcodeFromPath(item.Files)
		if shortcode == "" {
			continue
		}

		// 构建按钮标签
		label := shortcode[:8] + "..."
		if item.PostIndex > 0 {
			label = fmt.Sprintf("%s (第%d条)", shortcode[:8]+"...", item.PostIndex)
		}

		btn := tgbotapi.NewInlineKeyboardButtonData(label, "sc:"+shortcode)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{btn})
		count++
	}

	// 动态生成标题
	text := ""
	if count == 0 {
		text = fmt.Sprintf("🔖 @%s - 选择 Shortcode\n\n暂无下载历史", username)
	} else if len(items) >= 100 {
		text = fmt.Sprintf("🔖 @%s - 选择 Shortcode\n\n显示前 100 个帖子:", username)
	} else {
		text = fmt.Sprintf("🔖 @%s - 选择 Shortcode\n\n共 %d 个帖子:", username, count)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	if len(rows) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送 Shortcode 选择失败: %v", err)
	}
}

// extractShortcodeFromPath 从文件路径中提取 shortcode
// handleShortcodeSelection 处理 Shortcode 选择
func (tb *TelegramBot) handleShortcodeSelection(callback *tgbotapi.CallbackQuery, shortcode string) {
	// 立即响应回调，避免超时
	tb.answerCallback(callback.ID, "✅ 已接收，开始下载...")

	// 异步执行下载任务
	go tb.executeDownloadByShortcode(callback.Message.Chat.ID, shortcode)
}

func (tb *TelegramBot) handleCancel(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "已取消")
	tb.sendMessage(callback.Message.Chat.ID, "❌ 操作已取消")
}

func (tb *TelegramBot) handleIndexSelection(callback *tgbotapi.CallbackQuery, username string, postIndex int) {
	userID := callback.From.ID

	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	// 立即响应回调，避免超时
	tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已接收，开始下载第 %d 个帖子", postIndex))

	// 异步执行下载任务
	go tb.executeDownload(callback.Message.Chat.ID, username, postIndex)
}

func (tb *TelegramBot) handleInputRequest(callback *tgbotapi.CallbackQuery, username string) {
	userID := callback.From.ID

	tb.statesMutex.Lock()
	tb.userStates[userID] = &UserState{Step: "waiting_index", Username: username, Timestamp: time.Now()}
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "请输入序号")

	text := fmt.Sprintf("✅ 账户: @%s\n\n", username)
	text += "请输入帖子序号 (大于 10 的数字):"

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, Selective: true}
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送输入序号提示失败: %v", err)
	}
}

func (tb *TelegramBot) handleInputAccountRequest(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

	tb.statesMutex.Lock()
	tb.userStates[userID] = &UserState{Step: "waiting_account", Username: "", Timestamp: time.Now()}
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "请输入账户名")

	text := "📝 请输入 Instagram 账户名:\n\n"
	text += "示例: nike, tesla, spacex"

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, Selective: true}
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送输入账户提示失败: %v", err)
	}
}

func (tb *TelegramBot) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	tb.statesMutex.RLock()
	state, exists := tb.userStates[userID]
	tb.statesMutex.RUnlock()

	if !exists {
		return
	}

	if time.Since(state.Timestamp) > 5*time.Minute {
		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()
		tb.sendMessage(message.Chat.ID, "❌ 操作超时，请重新使用 /download 命令")
		return
	}

	if state.Step == "waiting_account" {
		username := strings.TrimSpace(message.Text)
		if username == "" {
			tb.sendMessage(message.Chat.ID, "❌ 账户名不能为空")
			return
		}

		tb.statesMutex.Lock()
		tb.userStates[userID] = &UserState{Step: "waiting_index", Username: username, Timestamp: time.Now()}
		tb.statesMutex.Unlock()

		tb.showIndexSelection(message.Chat.ID, username)
		return
	}

	if state.Step == "waiting_index" {
		postIndex, err := strconv.Atoi(strings.TrimSpace(message.Text))
		if err != nil || postIndex < 1 {
			tb.sendMessage(message.Chat.ID, "❌ 请输入有效的帖子序号（大于 0 的整数）")
			return
		}

		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()

		tb.executeDownload(message.Chat.ID, state.Username, postIndex)
	}
}

func (tb *TelegramBot) executeDownload(chatID int64, username string, postIndex int) {
	statusMsg := tb.sendMessage(chatID, fmt.Sprintf("⏳ 正在下载 @%s 的第 %d 个帖子...", username, postIndex))

	runtime, err := GetServiceRuntime("worker")
	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 无法获取 worker 状态: %v", err))
		return
	}
	if !runtime.Running {
		tb.editMessage(chatID, statusMsg.MessageID, "❌ 下载服务未启动，请联系管理员使用 /control 启动 worker")
		return
	}

	// 显示"正在输入"状态
	tb.sendChatAction(chatID, "typing")

	startTime := time.Now()
	files, err := tb.requestWorkerDownload(username, postIndex)
	elapsed := time.Since(startTime)

	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 下载失败: %v", err))
		return
	}

	tb.editMessage(chatID, statusMsg.MessageID,
		fmt.Sprintf("✅ 下载完成！共 %d 个文件 (耗时: %.2f 秒)\n正在上传...", len(files), elapsed.Seconds()))

	for i, filePath := range files {
		// 根据文件类型显示不同的上传状态
		if strings.HasSuffix(filePath, ".mp4") {
			tb.sendChatAction(chatID, "upload_video")
		} else {
			tb.sendChatAction(chatID, "upload_photo")
		}

		if err := tb.sendFile(chatID, filePath); err != nil {
			log.Printf("上传文件失败 %s: %v", filePath, err)
			tb.sendMessage(chatID, fmt.Sprintf("❌ 上传文件 %d 失败", i+1))
		}
	}

	tb.sendMessage(chatID, fmt.Sprintf("✅ 全部完成！共上传 %d 个文件", len(files)))
}

// executeDownloadByShortcode 通过 Shortcode 下载
func (tb *TelegramBot) executeDownloadByShortcode(chatID int64, shortcode string) {
	statusMsg := tb.sendMessage(chatID, fmt.Sprintf("⏳ 正在下载 shortcode: %s...", shortcode))

	runtime, err := GetServiceRuntime("worker")
	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 无法获取 worker 状态: %v", err))
		return
	}
	if !runtime.Running {
		tb.editMessage(chatID, statusMsg.MessageID, "❌ 下载服务未启动，请联系管理员使用 /control 启动 worker")
		return
	}

	// 显示"正在输入"状态
	tb.sendChatAction(chatID, "typing")

	startTime := time.Now()
	files, err := tb.requestWorkerDownloadByShortcode(shortcode)
	elapsed := time.Since(startTime)

	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 下载失败: %v", err))
		return
	}

	tb.editMessage(chatID, statusMsg.MessageID,
		fmt.Sprintf("✅ 下载完成！共 %d 个文件 (耗时: %.2f 秒)\n正在上传...", len(files), elapsed.Seconds()))

	for i, filePath := range files {
		// 根据文件类型显示不同的上传状态
		if strings.HasSuffix(filePath, ".mp4") {
			tb.sendChatAction(chatID, "upload_video")
		} else {
			tb.sendChatAction(chatID, "upload_photo")
		}

		if err := tb.sendFile(chatID, filePath); err != nil {
			log.Printf("上传文件失败 %s: %v", filePath, err)
			tb.sendMessage(chatID, fmt.Sprintf("❌ 上传文件 %d 失败", i+1))
		}
	}

	tb.sendMessage(chatID, fmt.Sprintf("✅ 全部完成！共上传 %d 个文件", len(files)))
}

func (tb *TelegramBot) requestWorkerDownload(username string, postIndex int) ([]string, error) {
	payload := WorkerDownloadRequest{Username: username, PostIndex: postIndex, Mode: "index"}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Post(tb.workerBaseURL+"/download", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("worker 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 worker 响应失败: %w", err)
	}

	var result WorkerDownloadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 worker 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if result.Message == "" {
			result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", result.Message)
	}

	if len(result.FilePaths) == 0 {
		return nil, fmt.Errorf("worker 未返回文件")
	}

	return result.FilePaths, nil
}

// requestWorkerDownloadByShortcode 请求 Worker 通过 Shortcode 下载
func (tb *TelegramBot) requestWorkerDownloadByShortcode(shortcode string) ([]string, error) {
	payload := WorkerDownloadRequest{Shortcode: shortcode, Mode: "shortcode"}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Post(tb.workerBaseURL+"/download", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("worker 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 worker 响应失败: %w", err)
	}

	var result WorkerDownloadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 worker 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if result.Message == "" {
			result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", result.Message)
	}

	if len(result.FilePaths) == 0 {
		return nil, fmt.Errorf("worker 未返回文件")
	}

	return result.FilePaths, nil
}

func (tb *TelegramBot) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := tb.bot.Request(callback); err != nil {
		// 忽略超时错误，避免日志污染
		if !strings.Contains(err.Error(), "query is too old") {
			log.Printf("回应回调失败: %v", err)
		}
	}
}

// sendChatAction 发送聊天动作状态（显示"正在输入"、"正在上传"等）
func (tb *TelegramBot) sendChatAction(chatID int64, action string) {
	chatAction := tgbotapi.NewChatAction(chatID, action)
	if _, err := tb.bot.Send(chatAction); err != nil {
		// 忽略错误，不影响主流程
		log.Printf("发送 ChatAction 失败: %v", err)
	}
}

func (tb *TelegramBot) showIndexSelection(chatID int64, username string) {
	text := fmt.Sprintf("⏰ @%s - 按时间线下载\n\n", username)
	text += "请选择帖子序号 (1=最新):"

	row1 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("1", fmt.Sprintf("index:%s:1", username)),
		tgbotapi.NewInlineKeyboardButtonData("2", fmt.Sprintf("index:%s:2", username)),
		tgbotapi.NewInlineKeyboardButtonData("3", fmt.Sprintf("index:%s:3", username)),
		tgbotapi.NewInlineKeyboardButtonData("4", fmt.Sprintf("index:%s:4", username)),
		tgbotapi.NewInlineKeyboardButtonData("5", fmt.Sprintf("index:%s:5", username)),
	}
	row2 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("6", fmt.Sprintf("index:%s:6", username)),
		tgbotapi.NewInlineKeyboardButtonData("7", fmt.Sprintf("index:%s:7", username)),
		tgbotapi.NewInlineKeyboardButtonData("8", fmt.Sprintf("index:%s:8", username)),
		tgbotapi.NewInlineKeyboardButtonData("9", fmt.Sprintf("index:%s:9", username)),
		tgbotapi.NewInlineKeyboardButtonData("10", fmt.Sprintf("index:%s:10", username)),
	}
	row3 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔄 检查更新", "refresh:"+username),
		tgbotapi.NewInlineKeyboardButtonData("📝 输入其他序号", "input:"+username),
	}
	row4 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("❌ 取消", "cancel:"+username),
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(row1, row2, row3, row4)
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送序号选择失败: %v", err)
	}
}

// handleRefreshCache 处理检查更新请求
func (tb *TelegramBot) handleRefreshCache(callback *tgbotapi.CallbackQuery, username string) {
	// 立即响应回调
	tb.answerCallback(callback.ID, "✅ 正在检查更新...")

	// 异步执行检查
	go tb.executeRefreshCache(callback.Message.Chat.ID, callback.Message.MessageID, username)
}

// executeRefreshCache 执行缓存刷新检查
func (tb *TelegramBot) executeRefreshCache(chatID int64, messageID int, username string) {
	// 更新消息状态
	tb.editMessage(chatID, messageID, fmt.Sprintf("🔄 正在检查 @%s 的更新...", username))

	// 请求 worker 检查更新
	needRefresh, totalPosts, err := tb.requestWorkerCheckUpdate(username)
	if err != nil {
		tb.editMessage(chatID, messageID, fmt.Sprintf("❌ 检查更新失败: %v", err))
		return
	}

	if needRefresh {
		// 有更新，显示结果
		text := fmt.Sprintf("✅ @%s 有新帖子！\n\n", username)
		text += fmt.Sprintf("已更新缓存，当前共 %d 条帖子\n", totalPosts)
		text += "请重新选择帖子序号:"
		tb.editMessage(chatID, messageID, text)
	} else {
		// 无更新
		text := fmt.Sprintf("✅ @%s 已是最新\n\n", username)
		text += fmt.Sprintf("当前共 %d 条帖子\n", totalPosts)
		text += "请选择帖子序号:"
		tb.editMessage(chatID, messageID, text)
	}

	// 重新显示序号选择按钮（发送新消息）
	time.Sleep(500 * time.Millisecond)
	tb.showIndexSelection(chatID, username)
}

// requestWorkerCheckUpdate 请求 Worker 检查更新
func (tb *TelegramBot) requestWorkerCheckUpdate(username string) (needRefresh bool, totalPosts int, err error) {
	type CheckUpdateRequest struct {
		Username string `json:"username"`
	}
	type CheckUpdateResponse struct {
		Success     bool   `json:"success"`
		Message     string `json:"message,omitempty"`
		NeedRefresh bool   `json:"need_refresh"`
		TotalPosts  int    `json:"total_posts"`
	}

	payload := CheckUpdateRequest{Username: username}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, 0, fmt.Errorf("构建请求失败: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Post(tb.workerBaseURL+"/check-update", "application/json", bytes.NewReader(body))
	if err != nil {
		return false, 0, fmt.Errorf("worker 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, fmt.Errorf("读取 worker 响应失败: %w", err)
	}

	var result CheckUpdateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, 0, fmt.Errorf("解析 worker 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if result.Message == "" {
			result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return false, 0, fmt.Errorf("%s", result.Message)
	}

	return result.NeedRefresh, result.TotalPosts, nil
}
