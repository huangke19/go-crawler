// ============================================================================
// monitor.go - Instagram 账户监控
// ============================================================================
//
// 职责：
//   - 定时轮询 config.json 中的 monitor_accounts 列表
//   - 检测最新帖子 shortcode 是否变化
//   - 有新帖时自动下载并通过 Telegram 推送文件
//
// 核心流程（每个账户）：
//   1. 使用 Worker 共享的浏览器上下文访问用户主页
//   2. 获取第 1 条帖子（= 最新帖子）的 shortcode
//   3. 与 monitor_state.json 中的上次 shortcode 对比
//   4. 首次运行（LastShortcode 为空）→ 仅初始化状态，不通知
//   5. 检测到新帖 → 下载媒体文件 → 发送 Telegram 通知 + 文件
//
// 并发安全：
//   - 监控 goroutine 与 HTTP 下载请求共享 ws.browserMu，天然串行
//   - monitorStateMu 保护 monitor_state.json 文件读写
//   - stopCh 通道控制优雅退出
//
// ============================================================================

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MonitorState 记录单个账户的上次检测状态。
type MonitorState struct {
	LastShortcode string `json:"last_shortcode"`
	LastCheck     string `json:"last_check"`
}

// monitorStates 是所有账户状态的 map（username -> MonitorState）。
type monitorStates map[string]MonitorState

// monitorStateMu 保护 monitor_state.json 的并发读写。
var monitorStateMu sync.Mutex

// loadMonitorStates 从磁盘读取 monitor_state.json。
// 文件不存在时返回空 map（正常的首次运行情况）。
func loadMonitorStates() (monitorStates, error) {
	states := make(monitorStates)
	data, err := os.ReadFile(monitorStateFile)
	if os.IsNotExist(err) {
		return states, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取监控状态文件失败: %w", err)
	}
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("解析监控状态失败: %w", err)
	}
	return states, nil
}

// saveMonitorStates 将状态原子写入 monitor_state.json。
func saveMonitorStates(states monitorStates) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化监控状态失败: %w", err)
	}
	tmp := monitorStateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("写入监控状态临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, monitorStateFile); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换监控状态文件失败: %w", err)
	}
	return nil
}

// startMonitorLoop 启动监控后台 goroutine。
// 每隔 config.MonitorIntervalMin 分钟检测一次所有 MonitorAccounts。
// 通过 ws.stopCh 接收关闭信号，优雅退出。
func (ws *WorkerServer) startMonitorLoop() {
	if ws.config == nil || len(ws.config.MonitorAccounts) == 0 {
		log.Println("监控：未配置监控账户（monitor_accounts），跳过启动")
		return
	}

	interval := time.Duration(ws.config.MonitorIntervalMin) * time.Minute
	log.Printf("监控启动：共 %d 个账户，每 %d 分钟检查一次",
		len(ws.config.MonitorAccounts), ws.config.MonitorIntervalMin)

	go func() {
		// 启动后立即执行一次，之后按 interval 轮询
		ws.checkAllAccounts()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ws.checkAllAccounts()
			case <-ws.stopCh:
				log.Println("监控：收到停止信号，退出监控 goroutine")
				return
			}
		}
	}()
}

// checkAllAccounts 依次检测所有监控账户。
func (ws *WorkerServer) checkAllAccounts() {
	if ws.config == nil {
		return
	}
	for _, username := range ws.config.MonitorAccounts {
		if err := ws.checkAccount(username); err != nil {
			log.Printf("监控：@%s 检测失败: %v", username, err)
		}
	}
}

// checkAccount 检测单个账户是否有新帖。
//
// 流程：
// 1. 获取浏览器上下文，访问用户主页，获取第 1 条帖子 URL
// 2. 提取 latestShortcode
// 3. 与上次记录对比：
//   - 首次运行（LastShortcode 为空）→ 仅保存状态，不通知
//   - shortcode 变化 → 下载并通知
//
// 4. 更新并保存 monitor state
func (ws *WorkerServer) checkAccount(username string) error {
	log.Printf("监控：开始检测 @%s", username)

	// 获取浏览器，访问用户主页
	ctx, err := ws.getBrowser()
	if err != nil {
		return fmt.Errorf("获取浏览器失败: %w", err)
	}

	if err := EnsureLoggedIn(ctx); err != nil {
		return fmt.Errorf("登录验证失败: %w", err)
	}

	if err := NavigateToUser(ctx, username); err != nil {
		return fmt.Errorf("访问主页失败: %w", err)
	}

	// 获取最新帖子 URL（第 1 条）
	postURL, err := GetPostByIndex(ctx, 1)
	if err != nil {
		return fmt.Errorf("获取最新帖子失败: %w", err)
	}

	latestShortcode := extractShortcode(postURL)
	if latestShortcode == "" {
		return fmt.Errorf("无法从 URL 提取 shortcode: %s", postURL)
	}

	log.Printf("监控：@%s 最新帖子 shortcode = %s", username, latestShortcode)

	// 读取上次状态
	monitorStateMu.Lock()
	states, err := loadMonitorStates()
	monitorStateMu.Unlock()
	if err != nil {
		log.Printf("监控：读取状态文件失败（忽略，使用空状态）: %v", err)
		states = make(monitorStates)
	}

	prevState := states[username]

	// 对比 shortcode
	if prevState.LastShortcode == "" {
		// 首次运行：只初始化状态，不通知
		log.Printf("监控：@%s 首次运行，初始化状态（shortcode: %s）", username, latestShortcode)
	} else if prevState.LastShortcode != latestShortcode {
		// 发现新帖
		log.Printf("监控：@%s 发现新帖！旧=%s 新=%s", username, prevState.LastShortcode, latestShortcode)

		// 下载媒体文件
		filePaths, downloadErr := ws.downloadByShortcode(latestShortcode)
		if downloadErr != nil {
			log.Printf("监控：@%s 下载失败: %v（仍发送文字通知）", username, downloadErr)
		}

		// 发送通知
		if ws.config != nil && len(ws.config.AdminUserIDs) > 0 {
			ws.notifyNewPost(username, latestShortcode, ws.config.AdminUserIDs, filePaths)
		}
	} else {
		log.Printf("监控：@%s 无新帖（shortcode 未变化）", username)
	}

	// 更新状态
	states[username] = MonitorState{
		LastShortcode: latestShortcode,
		LastCheck:     time.Now().Format(time.RFC3339),
	}

	monitorStateMu.Lock()
	if err := saveMonitorStates(states); err != nil {
		log.Printf("监控：@%s 保存状态失败: %v", username, err)
	}
	monitorStateMu.Unlock()

	return nil
}

// notifyNewPost 向所有 chatID 发送新帖通知（文字 + 文件）。
func (ws *WorkerServer) notifyNewPost(username, shortcode string, chatIDs []int64, filePaths []string) {
	if ws.config == nil || ws.config.TelegramBotToken == "" {
		return
	}

	token := ws.config.TelegramBotToken
	now := time.Now().Format("2006-01-02 15:04:05")
	postURL := fmt.Sprintf("%s/p/%s/", instagramBaseURL, shortcode)
	caption := fmt.Sprintf("🆕 @%s 发布了新帖子\n%s\n⏰ %s", username, postURL, now)

	for _, chatID := range chatIDs {
		for _, fp := range filePaths {
			if err := sendTelegramFile(token, chatID, fp, caption); err != nil {
				log.Printf("监控：发送文件失败 %s: %v", fp, err)
			} else {
				log.Printf("监控：已发送文件 → chatID=%d %s", chatID, fp)
			}
		}
	}
}

// sendTelegramFile 将本地文件通过 multipart/form-data 上传到 Telegram。
// 根据扩展名自动选择 sendVideo / sendPhoto / sendDocument。
func sendTelegramFile(token string, chatID int64, filePath, caption string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(filePath))
	var method string
	var fieldName string
	switch ext {
	case ".mp4":
		method = "sendVideo"
		fieldName = "video"
	case ".jpg", ".jpeg", ".png", ".webp":
		method = "sendPhoto"
		fieldName = "photo"
	default:
		method = "sendDocument"
		fieldName = "document"
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		_ = w.WriteField("caption", caption)
	}

	part, err := w.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("创建 multipart 字段失败: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("写入文件内容失败: %w", err)
	}
	w.Close()

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
	req, err := http.NewRequest(http.MethodPost, apiURL, &buf)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Telegram API 返回 %d: %s", resp.StatusCode, body)
	}

	return nil
}
