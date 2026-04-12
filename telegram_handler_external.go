// ============================================================================
// telegram_handler_external.go - Telegram Bot 外部平台下载处理
// ============================================================================
//
// 职责：
//   - /ytdl 命令处理
//   - YouTube / X (Twitter) URL 自动识别与下载
//   - 调用 Worker 的 /download-url 接口
//   - 下载完成后上传文件到 Telegram
//
// ============================================================================

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleYtdl 处理 /ytdl 命令
func (tb *TelegramClient) handleYtdl(message *tgbotapi.Message, args []string) {
	if len(args) == 0 {
		tb.sendMessage(message.Chat.ID,
			"📖 用法: /ytdl <url>\n\n"+
				"支持的平台:\n"+
				"  🎬 YouTube (视频/Shorts)\n"+
				"  🐦 X / Twitter (视频/图片)\n\n"+
				"示例:\n"+
				"  /ytdl https://youtube.com/watch?v=xxx\n"+
				"  /ytdl https://x.com/user/status/123\n\n"+
				"💡 也可以直接发送链接，会自动识别")
		return
	}

	url := args[0]
	tb.executeExternalDownload(message.Chat.ID, url)
}

// handleExternalURL 处理用户直接发送的外部平台链接
func (tb *TelegramClient) handleExternalURL(message *tgbotapi.Message) bool {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return false
	}

	// 尝试从消息文本中提取支持的 URL
	url := extractExternalURL(text)
	if url == "" {
		return false
	}

	go tb.executeExternalDownload(message.Chat.ID, url)
	return true
}

// extractExternalURL 从文本中提取支持的外部平台 URL
func extractExternalURL(text string) string {
	// 先检查 YouTube
	if m := youtubeURLPattern.FindString(text); m != "" {
		return m
	}
	// 再检查 X/Twitter
	if m := xURLPattern.FindString(text); m != "" {
		return m
	}
	return ""
}

// executeExternalDownload 执行外部平台下载全流程
func (tb *TelegramClient) executeExternalDownload(chatID int64, rawURL string) {
	platform := DetectPlatform(rawURL)
	emoji := PlatformEmoji(platform)
	label := PlatformLabel(platform)

	statusMsg := tb.sendMessage(chatID,
		fmt.Sprintf("%s 正在下载 %s 内容...\n🔗 %s", emoji, label, rawURL))

	// 检查 worker 状态
	runtime, err := GetServiceRuntime("worker")
	if err != nil {
		tb.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("❌ 无法获取 worker 状态: %v", err))
		return
	}
	if !runtime.Running {
		tb.editMessage(chatID, statusMsg.MessageID,
			"❌ 下载服务未启动，请联系管理员使用 /control 启动 worker")
		return
	}

	tb.sendChatAction(chatID, "typing")

	startTime := time.Now()
	result, err := tb.requestWorkerDownloadURL(rawURL)
	elapsed := time.Since(startTime)

	if err != nil {
		errMsg := err.Error()
		// 只显示第一行（关键原因），去掉冗长的 yt-dlp 原始输出
		if idx := strings.Index(errMsg, "\n输出:"); idx != -1 {
			errMsg = errMsg[:idx]
		}
		tb.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("❌ %s", errMsg))
		return
	}

	// 显示下载完成信息
	titleInfo := ""
	if result.Title != "" {
		titleInfo = fmt.Sprintf("\n📝 %s", result.Title)
	}
	tb.editMessage(chatID, statusMsg.MessageID,
		fmt.Sprintf("%s 下载完成！共 %d 个文件 (耗时: %.1f 秒)%s\n正在上传...",
			emoji, len(result.FilePaths), elapsed.Seconds(), titleInfo))

	// 上传文件
	uploaded := 0
	for i, filePath := range result.FilePaths {
		// 检查文件大小
		fi, err := os.Stat(filePath)
		if err != nil {
			log.Printf("获取文件信息失败 %s: %v", filePath, err)
			tb.sendMessage(chatID, fmt.Sprintf("❌ 文件 %d 不可访问", i+1))
			continue
		}

		if fi.Size() > telegramFileSizeLimit {
			tb.sendMessage(chatID, fmt.Sprintf(
				"⚠️ 文件 %d 太大 (%.1f MB)，超过 Telegram 50MB 限制\n📁 本地路径: %s",
				i+1, float64(fi.Size())/(1024*1024), filePath))
			continue
		}

		// 根据文件类型选择上传方式
		if strings.HasSuffix(filePath, ".mp4") || strings.HasSuffix(filePath, ".webm") || strings.HasSuffix(filePath, ".mkv") {
			tb.sendChatAction(chatID, "upload_video")
		} else {
			tb.sendChatAction(chatID, "upload_photo")
		}

		if err := tb.sendFile(chatID, filePath); err != nil {
			log.Printf("上传文件失败 %s: %v", filePath, err)
			tb.sendMessage(chatID, fmt.Sprintf("❌ 上传文件 %d 失败", i+1))
		} else {
			uploaded++
		}
	}

	tb.sendMessage(chatID, fmt.Sprintf("✅ 全部完成！共上传 %d 个文件", uploaded))
}
