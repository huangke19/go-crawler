// ============================================================================
// telegram_worker.go - Telegram Bot 与 Worker 的交互
// ============================================================================
//
// 职责：
//   - 检查 Worker 健康状态
//   - 调用 Worker HTTP 接口执行下载（按序号 / 按 Shortcode）
//   - 调用 Worker HTTP 接口检查更新
//   - 下载完成后上传文件到 Telegram
//
// Bot 核心结构体与生命周期函数位于 telegram_bot.go，
// 命令与回调处理函数位于 telegram_handlers.go。
//
// ============================================================================

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (tb *TelegramClient) checkWorkerHealth() bool {
	resp, err := tb.shortClient.Get(tb.workerBaseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (tb *TelegramClient) executeDownload(chatID int64, username string, postIndex int) {
	statusMsg := tb.sendMessage(chatID, fmt.Sprintf("⏳ 正在下载 @%s 的第 %d 个帖子...", username, postIndex))

	// 先检查 worker 是否运行，避免用户等待后才失败。
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
func (tb *TelegramClient) executeDownloadByShortcode(chatID int64, shortcode string) {
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

// requestWorkerDownload 调用 worker 的 `/download`（按序号模式）。
// worker 返回本机文件路径列表，bot 负责后续上传这些文件。
func (tb *TelegramClient) requestWorkerDownload(username string, postIndex int) ([]string, error) {
	payload := WorkerDownloadRequest{Username: username, PostIndex: postIndex, Mode: "index"}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	resp, err := tb.longClient.Post(tb.workerBaseURL+"/download", "application/json", bytes.NewReader(body))
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
func (tb *TelegramClient) requestWorkerDownloadByShortcode(shortcode string) ([]string, error) {
	payload := WorkerDownloadRequest{Shortcode: shortcode, Mode: "shortcode"}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	resp, err := tb.longClient.Post(tb.workerBaseURL+"/download", "application/json", bytes.NewReader(body))
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

// executeRefreshCache 执行缓存刷新检查，发现新帖时自动下载并上传。
func (tb *TelegramClient) executeRefreshCache(chatID int64, messageID int, username string) {
	tb.editMessage(chatID, messageID, fmt.Sprintf("🔄 正在检查 @%s 的更新...", username))

	result, err := tb.requestWorkerCheckUpdate(username)
	if err != nil {
		tb.editMessage(chatID, messageID, fmt.Sprintf("❌ 检查更新失败: %v", err))
		return
	}

	if !result.NeedRefresh || len(result.NewShortcodes) == 0 {
		tb.editMessage(chatID, messageID, fmt.Sprintf("✅ @%s 已是最新", username))
		time.Sleep(500 * time.Millisecond)
		tb.showIndexSelection(chatID, username)
		return
	}

	tb.editMessage(chatID, messageID,
		fmt.Sprintf("✅ @%s 检测到 %d 条新帖，正在上传...", username, len(result.NewShortcodes)))

	// 逐条上传新帖文件
	for i, sc := range result.NewShortcodes {
		var files []string
		if i < len(result.NewFilePaths) {
			files = result.NewFilePaths[i]
		}
		if len(files) == 0 {
			continue
		}

		postURL := fmt.Sprintf("https://www.instagram.com/p/%s/", sc)
		tb.sendMessage(chatID, fmt.Sprintf("🆕 新帖 %d/%d\n%s", i+1, len(result.NewShortcodes), postURL))

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

	tb.sendMessage(chatID, fmt.Sprintf("✅ 全部完成！共 %d 条新帖已上传", len(result.NewShortcodes)))

	time.Sleep(500 * time.Millisecond)
	tb.showIndexSelection(chatID, username)
}

// requestWorkerCheckUpdate 请求 Worker 检查更新
func (tb *TelegramClient) requestWorkerCheckUpdate(username string) (*CheckUpdateResponse, error) {
	type CheckUpdateRequest struct {
		Username string `json:"username"`
	}

	payload := CheckUpdateRequest{Username: username}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	resp, err := tb.longClient.Post(tb.workerBaseURL+"/check-update", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("worker 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 worker 响应失败: %w", err)
	}

	var result CheckUpdateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 worker 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if result.Message == "" {
			result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", result.Message)
	}

	return &result, nil
}
