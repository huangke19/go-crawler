// ============================================================================
// telegram_handler_status.go - Telegram Bot 状态与控制处理
// ============================================================================
//
// 职责：
//   - /status 命令处理
//   - Worker 控制面板（启动/停止/重启/状态）
//   - Worker 状态摘要
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

	// 管理员额外显示 worker 控制按钮
	if tb.isAdminUser(message.From.ID) {
		text += "\n\n🎛️ Worker 控制:"
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ReplyMarkup = tb.workerControlKeyboard()
		if _, err := tb.bot.Send(msg); err != nil {
			log.Printf("发送状态失败: %v", err)
		}
		return
	}

	tb.sendMessage(message.Chat.ID, text)
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
