package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type UserState struct {
	Step      string    // "waiting_account" - 等待输入账户名, "waiting_index" - 等待输入帖子序号
	Username  string    // 已选择的账户名
	Timestamp time.Time // 状态创建时间
}

type TelegramBot struct {
	bot              *tgbotapi.BotAPI
	allowedUsers     map[int64]bool
	favoriteAccounts []string
	userStates       map[int64]*UserState
	statesMutex      sync.RWMutex
}

func NewTelegramBot(token string, allowedUserIDs []int64, favoriteAccounts []string) (*TelegramBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("创建 bot 失败: %w", err)
	}

	allowedUsers := make(map[int64]bool)
	for _, id := range allowedUserIDs {
		allowedUsers[id] = true
	}

	// 设置默认常用账户
	if len(favoriteAccounts) == 0 {
		favoriteAccounts = []string{"nike", "instagram", "natgeo"}
	}

	log.Printf("Telegram Bot 已启动: @%s", bot.Self.UserName)
	return &TelegramBot{
		bot:              bot,
		allowedUsers:     allowedUsers,
		favoriteAccounts: favoriteAccounts,
		userStates:       make(map[int64]*UserState),
	}, nil
}

func (tb *TelegramBot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := tb.bot.GetUpdatesChan(u)

	for update := range updates {
		// 处理回调查询（按钮点击）
		if update.CallbackQuery != nil {
			if !tb.isAllowedUser(update.CallbackQuery.From.ID) {
				tb.answerCallback(update.CallbackQuery.ID, "❌ 未授权访问")
				continue
			}
			tb.handleCallback(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		// 检查用户权限
		if !tb.isAllowedUser(update.Message.From.ID) {
			tb.sendMessage(update.Message.Chat.ID, "❌ 未授权访问")
			log.Printf("未授权用户尝试访问: %d (@%s)", update.Message.From.ID, update.Message.From.UserName)
			continue
		}

		// 处理命令
		if update.Message.IsCommand() {
			tb.handleCommand(update.Message)
		} else {
			// 处理普通消息（可能是用户回复输入）
			tb.handleMessage(update.Message)
		}
	}
}

func (tb *TelegramBot) isAllowedUser(userID int64) bool {
	// 如果没有设置白名单，允许所有用户
	if len(tb.allowedUsers) == 0 {
		return true
	}
	return tb.allowedUsers[userID]
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
	text += fmt.Sprintf("当前时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	tb.sendMessage(message.Chat.ID, text)
}

func (tb *TelegramBot) handleDownload(message *tgbotapi.Message, args []string) {
	// 取消直接参数输入，统一使用按钮交互
	if len(args) > 0 {
		tb.sendMessage(message.Chat.ID, "💡 提示: 请使用按钮选择账户和帖子序号\n直接使用 /download 命令即可")
		return
	}

	// 显示账户选择界面
	text := "📥 请选择要下载的账户:\n"

	// 创建 Inline Keyboard
	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for i, account := range tb.favoriteAccounts {
		btn := tgbotapi.NewInlineKeyboardButtonData(account, "account:"+account)
		currentRow = append(currentRow, btn)

		// 每行3个按钮
		if (i+1)%3 == 0 || i == len(tb.favoriteAccounts)-1 {
			rows = append(rows, currentRow)
			currentRow = []tgbotapi.InlineKeyboardButton{}
		}
	}

	// 添加"输入其他用户"按钮
	inputOtherBtn := tgbotapi.NewInlineKeyboardButtonData("📝 输入其他用户", "input_account")
	rows = append(rows, []tgbotapi.InlineKeyboardButton{inputOtherBtn})

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard

	tb.bot.Send(msg)
}

func (tb *TelegramBot) downloadPost(username string, postIndex int) ([]string, error) {
	log.Printf("开始下载: @%s 第 %d 个帖子", username, postIndex)

	ctx, cancel := CreateFastBrowserContext()
	defer cancel()

	if err := EnsureLoggedIn(ctx); err != nil {
		return nil, fmt.Errorf("登录验证失败: %w", err)
	}

	if err := NavigateToUser(ctx, username); err != nil {
		return nil, fmt.Errorf("访问用户主页失败: %w", err)
	}

	postURL, err := GetPostByIndex(ctx, postIndex)
	if err != nil {
		return nil, fmt.Errorf("获取帖子失败: %w", err)
	}

	mediaInfo, err := ExtractMediaURLs(ctx, postURL)
	if err != nil {
		return nil, fmt.Errorf("提取媒体失败: %w", err)
	}

	files, err := DownloadPostAndReturnPaths(username, postIndex, mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}

	return files, nil
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
	_, err := tb.bot.Send(edit)
	if err != nil {
		log.Printf("编辑消息失败: %v", err)
	}
}

func (tb *TelegramBot) sendFile(chatID int64, filePath string) error {
	// 判断文件类型
	if strings.HasSuffix(filePath, ".mp4") {
		video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))
		_, err := tb.bot.Send(video)
		return err
	} else {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(filePath))
		_, err := tb.bot.Send(photo)
		return err
	}
}

// handleCallback 处理 Inline Keyboard 按钮点击
func (tb *TelegramBot) handleCallback(callback *tgbotapi.CallbackQuery) {
	data := callback.Data

	// 解析回调数据
	if strings.HasPrefix(data, "account:") {
		username := strings.TrimPrefix(data, "account:")
		tb.handleAccountSelection(callback, username)
	} else if data == "input_account" {
		tb.handleInputAccountRequest(callback)
	} else if strings.HasPrefix(data, "index:") {
		// 格式: index:username:序号
		parts := strings.SplitN(data, ":", 3)
		if len(parts) == 3 {
			username := parts[1]
			postIndex, err := strconv.Atoi(parts[2])
			if err == nil && postIndex > 0 {
				tb.handleIndexSelection(callback, username, postIndex)
			}
		}
	} else if strings.HasPrefix(data, "input:") {
		username := strings.TrimPrefix(data, "input:")
		tb.handleInputRequest(callback, username)
	} else if strings.HasPrefix(data, "cancel:") {
		tb.handleCancel(callback)
	}
}

// handleAccountSelection 处理账户选择
func (tb *TelegramBot) handleAccountSelection(callback *tgbotapi.CallbackQuery, username string) {
	userID := callback.From.ID

	// 保存用户状态
	tb.statesMutex.Lock()
	tb.userStates[userID] = &UserState{
		Step:      "waiting_index",
		Username:  username,
		Timestamp: time.Now(),
	}
	tb.statesMutex.Unlock()

	// 回应回调
	tb.answerCallback(callback.ID, fmt.Sprintf("✅ 已选择: @%s", username))

	// 显示帖子序号选择界面
	tb.showIndexSelection(callback.Message.Chat.ID, username)
}

// handleCancel 处理取消操作
func (tb *TelegramBot) handleCancel(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

	// 清除用户状态
	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "已取消")
	tb.sendMessage(callback.Message.Chat.ID, "❌ 操作已取消")
}

// handleIndexSelection 处理帖子序号选择（1-10按钮）
func (tb *TelegramBot) handleIndexSelection(callback *tgbotapi.CallbackQuery, username string, postIndex int) {
	userID := callback.From.ID

	// 清除用户状态
	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	// 回应回调
	tb.answerCallback(callback.ID, fmt.Sprintf("开始下载第 %d 个帖子", postIndex))

	// 执行下载
	tb.executeDownload(callback.Message.Chat.ID, username, postIndex)
}

// handleInputRequest 处理"输入其他序号"按钮
func (tb *TelegramBot) handleInputRequest(callback *tgbotapi.CallbackQuery, username string) {
	userID := callback.From.ID

	// 更新用户状态（保持 waiting_index 状态）
	tb.statesMutex.Lock()
	tb.userStates[userID] = &UserState{
		Step:      "waiting_index",
		Username:  username,
		Timestamp: time.Now(),
	}
	tb.statesMutex.Unlock()

	// 回应回调
	tb.answerCallback(callback.ID, "请输入序号")

	// 发送提示消息
	text := fmt.Sprintf("✅ 账户: @%s\n\n", username)
	text += "请输入帖子序号 (大于 10 的数字):"

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, Selective: true}

	tb.bot.Send(msg)
}

// handleInputAccountRequest 处理"输入其他用户"按钮
func (tb *TelegramBot) handleInputAccountRequest(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

	// 设置用户状态为等待输入账户名
	tb.statesMutex.Lock()
	tb.userStates[userID] = &UserState{
		Step:      "waiting_account",
		Username:  "",
		Timestamp: time.Now(),
	}
	tb.statesMutex.Unlock()

	// 回应回调
	tb.answerCallback(callback.ID, "请输入账户名")

	// 发送提示消息
	text := "📝 请输入 Instagram 账户名:\n\n"
	text += "示例: nike, tesla, spacex"

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, Selective: true}

	tb.bot.Send(msg)
}

// handleMessage 处理普通消息（用户输入）
func (tb *TelegramBot) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	// 检查用户是否有待处理的状态
	tb.statesMutex.RLock()
	state, exists := tb.userStates[userID]
	tb.statesMutex.RUnlock()

	if !exists {
		// 没有状态，忽略普通消息
		return
	}

	// 检查状态是否过期（5分钟）
	if time.Since(state.Timestamp) > 5*time.Minute {
		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()
		tb.sendMessage(message.Chat.ID, "❌ 操作超时，请重新使用 /download 命令")
		return
	}

	// 处理等待账户名的状态
	if state.Step == "waiting_account" {
		username := strings.TrimSpace(message.Text)
		if username == "" {
			tb.sendMessage(message.Chat.ID, "❌ 账户名不能为空")
			return
		}

		// 更新状态，进入选择帖子序号阶段
		tb.statesMutex.Lock()
		tb.userStates[userID] = &UserState{
			Step:      "waiting_index",
			Username:  username,
			Timestamp: time.Now(),
		}
		tb.statesMutex.Unlock()

		// 显示帖子序号选择界面
		tb.showIndexSelection(message.Chat.ID, username)
		return
	}

	// 处理等待帖子序号的状态
	if state.Step == "waiting_index" {
		postIndex, err := strconv.Atoi(strings.TrimSpace(message.Text))
		if err != nil || postIndex < 1 {
			tb.sendMessage(message.Chat.ID, "❌ 请输入有效的帖子序号（大于 0 的整数）")
			return
		}

		// 清除状态
		tb.statesMutex.Lock()
		delete(tb.userStates, userID)
		tb.statesMutex.Unlock()

		// 执行下载
		tb.executeDownload(message.Chat.ID, state.Username, postIndex)
	}
}

// executeDownload 执行下载任务
func (tb *TelegramBot) executeDownload(chatID int64, username string, postIndex int) {
	// 发送处理中消息
	statusMsg := tb.sendMessage(chatID, fmt.Sprintf("⏳ 正在下载 @%s 的第 %d 个帖子...", username, postIndex))

	// 执行下载
	startTime := time.Now()
	files, err := tb.downloadPost(username, postIndex)
	elapsed := time.Since(startTime)

	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 下载失败: %v", err))
		return
	}

	// 更新状态消息
	tb.editMessage(chatID, statusMsg.MessageID,
		fmt.Sprintf("✅ 下载完成！共 %d 个文件 (耗时: %.2f 秒)\n正在上传...", len(files), elapsed.Seconds()))

	// 上传文件到 Telegram
	for i, filePath := range files {
		if err := tb.sendFile(chatID, filePath); err != nil {
			log.Printf("上传文件失败 %s: %v", filePath, err)
			tb.sendMessage(chatID, fmt.Sprintf("❌ 上传文件 %d 失败", i+1))
		}
	}

	// 发送完成消息
	tb.sendMessage(chatID, fmt.Sprintf("✅ 全部完成！共上传 %d 个文件", len(files)))
}

// answerCallback 回应回调查询
func (tb *TelegramBot) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := tb.bot.Request(callback); err != nil {
		log.Printf("回应回调失败: %v", err)
	}
}

// showIndexSelection 显示帖子序号选择界面
func (tb *TelegramBot) showIndexSelection(chatID int64, username string) {
	text := fmt.Sprintf("✅ 已选择账户: @%s\n\n", username)
	text += "请选择帖子序号:"

	// 创建 1-10 的按钮，每行 5 个
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
		tgbotapi.NewInlineKeyboardButtonData("📝 输入其他序号", "input:"+username),
		tgbotapi.NewInlineKeyboardButtonData("❌ 取消", "cancel:"+username),
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(row1, row2, row3)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard

	tb.bot.Send(msg)
}
