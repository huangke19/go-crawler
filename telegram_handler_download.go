// ============================================================================
// telegram_handler_download.go - Telegram Bot 下载功能处理
// ============================================================================
//
// 职责：
//   - /download 命令处理
//   - 账户选择、模式选择、序号选择、shortcode 选择
//   - 文本消息处理（状态机）
//   - 缩略图网格显示
//   - 刷新缓存处理
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

func (tb *TelegramClient) handleCancel(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	tb.statesMutex.Lock()
	delete(tb.userStates, userID)
	tb.statesMutex.Unlock()

	tb.answerCallback(callback.ID, "已取消")
	tb.sendMessage(callback.Message.Chat.ID, "❌ 操作已取消")
}

func (tb *TelegramClient) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID

	tb.statesMutex.RLock()
	state, exists := tb.userStates[userID]
	tb.statesMutex.RUnlock()

	if !exists {
		// 没有活跃的对话状态 -> 尝试识别外部平台 URL（YouTube / X）
		if tb.handleExternalURL(message) {
			return
		}
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

// handleRefreshCache 处理检查更新请求
func (tb *TelegramClient) handleRefreshCache(callback *tgbotapi.CallbackQuery, username string) {
	// 立即响应回调
	tb.answerCallback(callback.ID, "✅ 正在检查更新...")

	// 异步执行检查
	go tb.executeRefreshCache(callback.Message.Chat.ID, callback.Message.MessageID, username)
}
