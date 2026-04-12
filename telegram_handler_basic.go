// ============================================================================
// telegram_handler_basic.go - Telegram Bot 基础命令处理
// ============================================================================
//
// 职责：
//   - 命令分发器（handleCommand）
//   - 基础命令处理（/start, /help）
//   - 回调分发器（handleCallback）
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCommand 分发并处理命令消息（以 / 开头）。
func (tb *TelegramClient) handleCommand(message *tgbotapi.Message) {
	command := message.Command()
	args := strings.Fields(message.CommandArguments())

	log.Printf("收到命令: /%s %v (来自: @%s)", command, args, message.From.UserName)
	LogBotCommand(message.From.ID, message.From.UserName, "/"+command)
	RecordBotCommand("/" + command)

	switch command {
	case "start":
		tb.handleStart(message)
	case "help":
		tb.handleHelp(message)
	case "download":
		tb.handleDownload(message, args)
	case "favorites", "fav":
		tb.handleFavoritesCommand(message)
	case "status":
		tb.handleStatus(message)
	case "monitor":
		tb.handleMonitor(message)
	case "ytdl":
		tb.handleYtdl(message, args)
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
	text += "/ytdl - 下载 YouTube / X 视频\n"
	text += "/status - 查看状态"
	if tb.isAdminUser(message.From.ID) {
		text += "与 worker 控制\n"
		text += "/favorites - 管理常用账户列表\n"
		text += "/monitor - 查看监控账户状态\n"
	} else {
		text += "\n"
	}
	text += "/help - 显示帮助信息\n\n"
	text += "💡 使用方式:\n\n"
	text += "1️⃣ 发送 /download\n"
	text += "2️⃣ 点击账户按钮（或点击\"输入其他用户\"）\n"
	text += "3️⃣ 点击帖子序号 1-10（或点击\"输入其他序号\"）\n"
	text += "4️⃣ 等待下载完成\n\n"
	text += "🎬 YouTube / X 下载:\n"
	text += "  直接发送链接即可自动识别并下载\n"
	text += "  或使用 /ytdl <url>"
	tb.sendMessage(message.Chat.ID, text)
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
	case strings.HasPrefix(data, "mon:check:"):
		username := strings.TrimPrefix(data, "mon:check:")
		tb.handleMonitorCheckCallback(callback, username)
	}
}
