// ============================================================================
// telegram_handlers.go - Telegram Bot 命令与回调处理
// ============================================================================
//
// 职责：
//   - 处理所有 Telegram 命令（/start, /help, /download, /status, /control, /favorites）
//   - 处理所有 Inline Keyboard 按钮回调（账户选择、序号选择、模式选择等）
//   - 处理用户文本消息（状态机驱动的多步交互）
//   - Worker 控制面板（启动/停止/重启/状态）
//   - 常用账户管理（添加/删除/持久化）
//
// 核心结构体与生命周期函数位于 telegram_bot.go，
// Worker 交互函数位于 telegram_worker.go。
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCommand 分发并处理命令消息（以 / 开头）。
func (tb *TelegramClient) handleCommand(message *tgbotapi.Message) {
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
	case "favorites", "fav":
		tb.handleFavoritesCommand(message)
	case "status":
		tb.handleStatus(message)
	case "monitor":
		tb.handleMonitor(message)
	default:
		tb.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 未知命令: /%s\n使用 /help 查看帮助", command))
	}
}

func (tb *TelegramClient) handleStart(message *tgbotapi.Message) {
	text := fmt.Sprintf("👋 你好 @%s！\n\n", message.From.UserName)
	text += "我是 Instagram 下载机器人，可以帮你下载 Instagram 帖子。\n\n"
	text += "使用 /help 查看可用命令"
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramClient) handleHelp(message *tgbotapi.Message) {
	text := "📖 可用命令:\n\n"
	text += "/download - 下载指定帖子（按钮交互）\n"
	text += "/dl - download 的简写\n"
	text += "/status - 查看 bot 状态\n"
	if tb.isAdminUser(message.From.ID) {
		text += "/control - 控制 worker 启动/停止/重启\n"
		text += "/favorites - 管理常用账户列表\n"
		text += "/monitor - 查看监控账户状态\n"
	}
	text += "/help - 显示帮助信息\n\n"
	text += "💡 使用方式:\n\n"
	text += "1️⃣ 发送 /download\n"
	text += "2️⃣ 点击账户按钮（或点击\"输入其他用户\"）\n"
	text += "3️⃣ 点击帖子序号 1-10（或点击\"输入其他序号\"）\n"
	text += "4️⃣ 等待下载完成"
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramClient) handleStatus(message *tgbotapi.Message) {
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

func (tb *TelegramClient) handleControl(message *tgbotapi.Message) {
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

func (tb *TelegramClient) handleDownload(message *tgbotapi.Message, args []string) {
	if len(args) > 0 {
		tb.sendMessage(message.Chat.ID, "💡 提示: 请使用按钮选择账户和帖子序号\n直接使用 /download 命令即可")
		return
	}

	text := "📥 请选择要下载的账户:\n"

	tb.accountsMu.RLock()
	accounts := make([]string, len(tb.favoriteAccounts))
	copy(accounts, tb.favoriteAccounts)
	tb.accountsMu.RUnlock()

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for i, account := range accounts {
		btn := tgbotapi.NewInlineKeyboardButtonData(account, "account:"+account)
		currentRow = append(currentRow, btn)
		if (i+1)%3 == 0 || i == len(accounts)-1 {
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

// handleCallback 处理 inline button 回调。
// 注意：具体分支中通常会先 answerCallback，再异步执行耗时操作。
func (tb *TelegramClient) handleCallback(callback *tgbotapi.CallbackQuery) {
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
	case strings.HasPrefix(data, "fav:"):
		tb.handleFavoriteCallback(callback)
	}
}

// handleWorkerControl 处理 worker 启停/状态按钮。
// 该函数会先立即响应 callback，避免 Telegram "query is too old"。
func (tb *TelegramClient) handleWorkerControl(callback *tgbotapi.CallbackQuery) {
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

func (tb *TelegramClient) workerStatusSummary() (string, error) {
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

func (tb *TelegramClient) workerControlKeyboard() tgbotapi.InlineKeyboardMarkup {
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

func (tb *TelegramClient) handleAccountSelection(callback *tgbotapi.CallbackQuery, username string) {
	tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已选择: @%s", username))
	tb.showModeSelection(callback.Message.Chat.ID, username)
}

// showModeSelection 显示下载模式选择
func (tb *TelegramClient) showModeSelection(chatID int64, username string) {
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
func (tb *TelegramClient) handleModeSelection(callback *tgbotapi.CallbackQuery, username, mode string) {
	tb.answerCallback(callback.ID, "")

	if mode == "index" {
		tb.showIndexSelection(callback.Message.Chat.ID, username)
	} else if mode == "shortcode" {
		tb.showShortcodeSelection(callback.Message.Chat.ID, username)
	}
}

// showShortcodeSelection 显示 Shortcode 选择（历史下载列表）
func (tb *TelegramClient) showShortcodeSelection(chatID int64, username string) {
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
func (tb *TelegramClient) sendThumbnailGrid(chatID int64, items []*FilesCache, thumbnails map[string]string) {
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
func (tb *TelegramClient) sendShortcodeButtons(chatID int64, username string, items []*FilesCache) {
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

// handleShortcodeSelection 处理 Shortcode 选择
func (tb *TelegramClient) handleShortcodeSelection(callback *tgbotapi.CallbackQuery, shortcode string) {
	// 立即响应回调，避免超时
	tb.answerCallback(callback.ID, "✅ 已接收，开始下载...")

	// 异步执行下载任务
	go tb.executeDownloadByShortcode(callback.Message.Chat.ID, shortcode)
}

func (tb *TelegramClient) handleCancel(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "已取消")
	tb.sendMessage(callback.Message.Chat.ID, "❌ 操作已取消")
}

func (tb *TelegramClient) handleIndexSelection(callback *tgbotapi.CallbackQuery, username string, postIndex int) {
	userID := callback.From.ID

	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	// 立即响应回调，避免超时
	tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已接收，开始下载第 %d 个帖子", postIndex))

	// 异步执行下载任务
	go tb.executeDownload(callback.Message.Chat.ID, username, postIndex)
}

func (tb *TelegramClient) handleInputRequest(callback *tgbotapi.CallbackQuery, username string) {
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

func (tb *TelegramClient) handleInputAccountRequest(callback *tgbotapi.CallbackQuery) {
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

func (tb *TelegramClient) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	tb.statesMutex.RLock()
	state, exists := tb.userStates[userID]
	tb.statesMutex.RUnlock()

	if !exists {
		return
	}

	if time.Since(state.Timestamp) > stateExpiration {
		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()
		tb.sendMessage(message.Chat.ID, "❌ 操作超时，请重新使用 /download 命令")
		return
	}

	if state.Step == "waiting_add_favorite" {
		input := strings.ToLower(strings.TrimSpace(message.Text))
		if input == "" {
			tb.sendMessage(message.Chat.ID, "❌ 账户名不能为空")
			return
		}

		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()

		if err := tb.addFavoriteAccount(input); err != nil {
			tb.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 添加失败: %v", err))
		} else {
			tb.sendMessage(message.Chat.ID, fmt.Sprintf("✅ 已添加 @%s", input))
		}
		tb.sendFavoritesList(message.Chat.ID)
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

func (tb *TelegramClient) showIndexSelection(chatID int64, username string) {
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
func (tb *TelegramClient) handleRefreshCache(callback *tgbotapi.CallbackQuery, username string) {
	// 立即响应回调
	tb.answerCallback(callback.ID, "✅ 正在检查更新...")

	// 异步执行检查
	go tb.executeRefreshCache(callback.Message.Chat.ID, callback.Message.MessageID, username)
}

// handleFavoritesCommand 处理 /favorites 命令入口，仅 Admin 可用。
func (tb *TelegramClient) handleFavoritesCommand(message *tgbotapi.Message) {
	if !tb.isAdminUser(message.From.ID) {
		tb.sendMessage(message.Chat.ID, "❌ 仅管理员可使用 /favorites")
		return
	}
	tb.sendFavoritesList(message.Chat.ID)
}

// handleMonitor 显示监控账户列表及上次检测状态，仅 Admin 可用。
func (tb *TelegramClient) handleMonitor(message *tgbotapi.Message) {
	if !tb.isAdminUser(message.From.ID) {
		tb.sendMessage(message.Chat.ID, "❌ 仅管理员可使用 /monitor")
		return
	}

	config, err := LoadConfig(tb.configPath)
	if err != nil || len(config.MonitorAccounts) == 0 {
		tb.sendMessage(message.Chat.ID,
			"📡 监控状态\n\n未配置监控账户，在 config.json 中添加 monitor_accounts 字段即可启用。")
		return
	}

	// 读取监控状态
	monitorStateMu.Lock()
	states, _ := loadMonitorStates()
	monitorStateMu.Unlock()

	text := fmt.Sprintf("📡 监控状态（每 %d 分钟检查一次）\n\n", config.MonitorIntervalMin)
	for _, username := range config.MonitorAccounts {
		state, ok := states[username]
		if !ok || state.LastShortcode == "" {
			text += fmt.Sprintf("• @%s — 尚未检测\n", username)
			continue
		}
		lastCheck := state.LastCheck
		if t, err := time.Parse(time.RFC3339, state.LastCheck); err == nil {
			lastCheck = t.Format("01-02 15:04")
		}
		text += fmt.Sprintf("• @%s\n  最新: %s\n  检测: %s\n", username, state.LastShortcode, lastCheck)
	}

	tb.sendMessage(message.Chat.ID, text)
}

// sendFavoritesList 发送常用账户列表及操作按钮。
func (tb *TelegramClient) sendFavoritesList(chatID int64) {
	tb.accountsMu.RLock()
	accounts := make([]string, len(tb.favoriteAccounts))
	copy(accounts, tb.favoriteAccounts)
	tb.accountsMu.RUnlock()

	text := "📋 常用账户管理\n\n"
	if len(accounts) == 0 {
		text += "当前列表为空\n"
	} else {
		text += fmt.Sprintf("当前列表（%d 个账户）：\n", len(accounts))
		for _, a := range accounts {
			text += fmt.Sprintf("• %s\n", a)
		}
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, a := range accounts {
		btn := tgbotapi.NewInlineKeyboardButtonData(a+" ✕", "fav:rm:"+a)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{btn})
	}
	addBtn := tgbotapi.NewInlineKeyboardButtonData("➕ 添加账户", "fav:add")
	rows = append(rows, []tgbotapi.InlineKeyboardButton{addBtn})

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送常用账户列表失败: %v", err)
	}
}

// handleFavoriteCallback 处理 fav: 前缀的回调。
func (tb *TelegramClient) handleFavoriteCallback(callback *tgbotapi.CallbackQuery) {
	if !tb.isAdminUser(callback.From.ID) {
		tb.answerCallback(callback.ID, "❌ 仅管理员可操作")
		return
	}

	data := callback.Data
	switch {
	case strings.HasPrefix(data, "fav:rm:"):
		account := strings.TrimPrefix(data, "fav:rm:")
		tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已移除 @%s", account))
		if err := tb.removeFavoriteAccount(account); err != nil {
			tb.sendMessage(callback.From.ID, fmt.Sprintf("❌ 移除失败: %v", err))
			return
		}
		chatID := callback.From.ID
		if callback.Message != nil {
			chatID = callback.Message.Chat.ID
		}
		tb.sendFavoritesList(chatID)

	case data == "fav:add":
		tb.answerCallback(callback.ID, "请输入要添加的账户名")
		userID := callback.From.ID
		tb.statesMutex.Lock()
		tb.userStates[userID] = &UserState{Step: "waiting_add_favorite", Timestamp: time.Now()}
		tb.statesMutex.Unlock()

		chatID := callback.From.ID
		if callback.Message != nil {
			chatID = callback.Message.Chat.ID
		}
		msg := tgbotapi.NewMessage(chatID, "📝 请输入要添加的 Instagram 用户名：")
		msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, Selective: true}
		if _, err := tb.bot.Send(msg); err != nil {
			log.Printf("发送添加账户提示失败: %v", err)
		}
	}
}

// addFavoriteAccount 添加账户到常用列表并持久化配置。
func (tb *TelegramClient) addFavoriteAccount(account string) error {
	tb.accountsMu.Lock()
	defer tb.accountsMu.Unlock()

	for _, a := range tb.favoriteAccounts {
		if a == account {
			return fmt.Errorf("账户 @%s 已在列表中", account)
		}
	}
	tb.favoriteAccounts = append(tb.favoriteAccounts, account)
	return tb.persistFavorites()
}

// removeFavoriteAccount 从常用列表移除账户并持久化配置。
func (tb *TelegramClient) removeFavoriteAccount(account string) error {
	tb.accountsMu.Lock()
	defer tb.accountsMu.Unlock()

	filtered := tb.favoriteAccounts[:0]
	found := false
	for _, a := range tb.favoriteAccounts {
		if a == account {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !found {
		return fmt.Errorf("账户 @%s 不在列表中", account)
	}
	tb.favoriteAccounts = filtered
	return tb.persistFavorites()
}

// persistFavorites 将当前 favoriteAccounts 写回 config.json。
// 调用前必须已持有 accountsMu 写锁。
func (tb *TelegramClient) persistFavorites() error {
	config, err := LoadConfig(tb.configPath)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	config.FavoriteAccounts = tb.favoriteAccounts
	return SaveConfig(tb.configPath, config)
}
