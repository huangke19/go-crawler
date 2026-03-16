// ============================================================================
// telegram_handler_favorites.go - Telegram Bot 常用账户管理
// ============================================================================
//
// 职责：
//   - /favorites 命令处理
//   - 常用账户列表显示
//   - 添加/删除常用账户
//   - 配置持久化
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

// handleFavoritesCommand 处理 /favorites 命令入口，仅 Admin 可用。
func (tb *TelegramClient) handleFavoritesCommand(message *tgbotapi.Message) {
	if !tb.isAdminUser(message.From.ID) {
		tb.sendMessage(message.Chat.ID, "❌ 仅管理员可使用 /favorites")
		return
	}
	tb.sendFavoritesList(message.Chat.ID)
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

	filtered := make([]string, 0, len(tb.favoriteAccounts))
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
