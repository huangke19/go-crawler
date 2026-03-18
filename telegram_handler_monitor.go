// ============================================================================
// telegram_handler_monitor.go - Telegram Bot 监控功能处理
// ============================================================================
//
// 职责：
//   - /monitor 命令处理
//   - 监控账户状态显示
//   - 立即检测按钮处理
//   - 新帖通知与上传
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

	text := fmt.Sprintf("📡 监控状态（每 %s 检查一次）\n\n", config.GetMonitorIntervalLabel())
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, username := range config.MonitorAccounts {
		postsCache, ok := GetPostsFromCacheRaw(username)
		if !ok || postsCache == nil || len(postsCache.Posts) == 0 {
			text += fmt.Sprintf("• @%s — 尚未检测\n", username)
		} else {
			latestShortcode := postsCache.Posts[0].Shortcode
			lastCheck := "-"
			if !postsCache.UpdatedAt.IsZero() {
				lastCheck = postsCache.UpdatedAt.Format("01-02 15:04")
			}
			text += fmt.Sprintf("• @%s\n  最新: %s\n  检测: %s\n", username, latestShortcode, lastCheck)
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("🔍 立即检测 @%s", username),
			"mon:check:"+username,
		)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{btn})
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if len(rows) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	if _, err := tb.bot.Send(msg); err != nil {
		log.Printf("发送监控状态失败: %v", err)
	}
}

// handleMonitorCheckCallback 处理"立即检测"按钮回调，立即对指定账户执行一次监控检测。
func (tb *TelegramClient) handleMonitorCheckCallback(callback *tgbotapi.CallbackQuery, username string) {
	if !tb.isAdminUser(callback.From.ID) {
		tb.answerCallback(callback.ID, "❌ 仅管理员可操作")
		return
	}

	tb.answerCallback(callback.ID, fmt.Sprintf("🔍 开始检测 @%s ...", username))

	chatID := callback.From.ID
	if callback.Message != nil {
		chatID = callback.Message.Chat.ID
	}

	// 异步执行检测，避免阻塞其他操作
	go tb.executeMonitorCheck(chatID, username)
}

// executeMonitorCheck 执行单个账户的监控检测并上传新帖
func (tb *TelegramClient) executeMonitorCheck(chatID int64, username string) {
	statusMsg := tb.sendMessage(chatID, fmt.Sprintf("🔍 正在检测 @%s ...", username))

	result, err := tb.requestWorkerMonitorCheck(username)
	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("❌ 检测失败: %v", err))
		return
	}

	if len(result.NewShortcodes) == 0 {
		tb.editMessage(chatID, statusMsg.MessageID, fmt.Sprintf("✅ @%s 无新帖子", username))
		return
	}

	// 限制最多显示 3 条新帖，避免一次性发送太多
	maxShow := 3
	totalNew := len(result.NewShortcodes)
	showCount := totalNew
	if showCount > maxShow {
		showCount = maxShow
	}

	if totalNew > maxShow {
		tb.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("✅ @%s 发现 %d 条新帖，显示最新 %d 条...", username, totalNew, maxShow))
	} else {
		tb.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("✅ @%s 发现 %d 条新帖，正在上传...", username, totalNew))
	}

	for i := 0; i < showCount; i++ {
		sc := result.NewShortcodes[i]
		var files []string
		if i < len(result.NewFilePaths) {
			files = result.NewFilePaths[i]
		}
		if len(files) == 0 {
			continue
		}

		postURL := fmt.Sprintf("https://www.instagram.com/p/%s/", sc)
		tb.sendMessage(chatID, fmt.Sprintf("🆕 新帖 %d/%d\n%s", i+1, showCount, postURL))

		for j, filePath := range files {
			if strings.HasSuffix(filePath, ".mp4") {
				tb.sendChatAction(chatID, "upload_video")
			} else {
				tb.sendChatAction(chatID, "upload_photo")
			}
			if err := tb.sendFile(chatID, filePath); err != nil {
				log.Printf("上传文件失败 %s: %v", filePath, err)
				tb.sendMessage(chatID, fmt.Sprintf("❌ 上传文件 %d 失败", j+1))
			}
		}
	}

	if totalNew > maxShow {
		tb.sendMessage(chatID, fmt.Sprintf("✅ 已上传最新 %d 条（共 %d 条新帖）", showCount, totalNew))
	} else {
		tb.sendMessage(chatID, fmt.Sprintf("✅ 完成！共上传 %d 条新帖", totalNew))
	}
}
