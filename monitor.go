// ============================================================================
// monitor.go - Instagram 账户监控
// ============================================================================
//
// 职责：
//   - 定时轮询 config.json 中的 monitor_accounts 列表
//   - 每轮实时抓取用户主页前 N 条帖子
//   - 与 posts_cache.json 中的历史列表做差集对比
//   - 对停机期间遗漏的多条新帖执行补抓并推送
//
// 并发安全：
//   - 监控 goroutine 与 HTTP 下载请求共享 ws.browserMu，天然串行
//   - stopCh 通道控制优雅退出
//
// ============================================================================

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// normalizeMonitorAccounts 规范化监控账户列表：去空白、去重、忽略空值。
func normalizeMonitorAccounts(accounts []string) []string {
	if len(accounts) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		username := strings.TrimSpace(account)
		if username == "" {
			continue
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		normalized = append(normalized, username)
	}

	return normalized
}

// loadMonitorConfigSnapshot 读取最新 monitor 相关配置（热加载）。
func (ws *WorkerServer) loadMonitorConfigSnapshot() ([]string, time.Duration, int, error) {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		return nil, 0, 0, err
	}

	accounts := normalizeMonitorAccounts(cfg.MonitorAccounts)
	interval := time.Duration(cfg.MonitorIntervalMin) * time.Minute
	if interval <= 0 {
		interval = time.Duration(defaultMonitorIntervalMin) * time.Minute
	}
	topN := cfg.MonitorCompareTopN
	if topN <= 0 {
		topN = defaultMonitorCompareTopN
	}

	return accounts, interval, topN, nil
}

// getFallbackMonitorInterval 返回配置读取失败时的兜底轮询间隔。
func (ws *WorkerServer) getFallbackMonitorInterval() time.Duration {
	if ws.config != nil && ws.config.MonitorIntervalMin > 0 {
		return time.Duration(ws.config.MonitorIntervalMin) * time.Minute
	}
	return time.Duration(defaultMonitorIntervalMin) * time.Minute
}

// runMonitorCycle 执行一轮监控：热加载配置并检测账户。
func (ws *WorkerServer) runMonitorCycle() (time.Duration, error) {
	accounts, interval, topN, err := ws.loadMonitorConfigSnapshot()
	if err != nil {
		return ws.getFallbackMonitorInterval(), fmt.Errorf("加载配置失败: %w", err)
	}

	ws.checkAllAccounts(accounts, topN)
	return interval, nil
}

// startMonitorLoop 启动监控后台 goroutine。
// 每轮检测前热加载 monitor_accounts 与 monitor_interval_min。
// 通过 ws.stopCh 接收关闭信号，优雅退出。
func (ws *WorkerServer) startMonitorLoop() {
	log.Println("监控启动：已启用配置热加载")

	go func() {
		interval, err := ws.runMonitorCycle()
		if err != nil {
			log.Printf("监控：%v", err)
		}

		for {
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				interval, err = ws.runMonitorCycle()
				if err != nil {
					log.Printf("监控：%v", err)
				}
			case <-ws.stopCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				log.Println("监控：收到停止信号，退出监控 goroutine")
				return
			}
		}
	}()
}

// checkAllAccounts 依次检测所有监控账户。
func (ws *WorkerServer) checkAllAccounts(accounts []string, topN int) {
	if len(accounts) == 0 {
		return
	}
	for _, username := range accounts {
		if err := ws.checkAccount(username, topN); err != nil {
			log.Printf("监控：@%s 检测失败: %v", username, err)
		}
	}
}

func limitPosts(posts []PostItem, topN int) []PostItem {
	if len(posts) <= topN {
		return posts
	}
	return posts[:topN]
}

// DiffNewShortcodes 对比缓存与最新帖子列表，返回新增 shortcodes。
// 导出供 check-update 和监控共用。
func DiffNewShortcodes(cachedPosts, latestPosts []PostItem) []string {
	if len(latestPosts) == 0 {
		return nil
	}

	cachedSet := make(map[string]struct{}, len(cachedPosts))
	for _, post := range cachedPosts {
		if post.Shortcode != "" {
			cachedSet[post.Shortcode] = struct{}{}
		}
	}

	var newShortcodes []string
	seen := make(map[string]struct{}, len(latestPosts))
	for _, post := range latestPosts {
		if post.Shortcode == "" {
			continue
		}
		if _, exists := seen[post.Shortcode]; exists {
			continue
		}
		seen[post.Shortcode] = struct{}{}
		if _, exists := cachedSet[post.Shortcode]; !exists {
			newShortcodes = append(newShortcodes, post.Shortcode)
		}
	}

	return newShortcodes
}

// checkAccount 检测单个账户是否有新帖。
//
// 流程：
// 1. 访问用户主页并刷新 posts_cache（前 N 条）
// 2. 将"最新前 N 条"与"旧缓存前 N 条"做差集
// 3. 对新增 shortcode 逐条补抓并通知（支持停机期间多条补抓）
func (ws *WorkerServer) checkAccount(username string, topN int) error {
	log.Printf("监控：开始检测 @%s", username)

	ctx, err := ws.getBrowser()
	if err != nil {
		return fmt.Errorf("获取浏览器失败: %w", err)
	}

	if err := EnsureLoggedIn(ctx); err != nil {
		return fmt.Errorf("登录验证失败: %w", err)
	}

	var cachedPosts []PostItem
	if postsCache, ok := GetPostsFromCacheRaw(username); ok && postsCache != nil {
		cachedPosts = limitPosts(postsCache.Posts, topN)
	}

	if _, _, err := RefreshPostsCache(ctx, username, topN); err != nil {
		return fmt.Errorf("刷新帖子缓存失败: %w", err)
	}

	latestCache, ok := GetPostsFromCacheRaw(username)
	if !ok || latestCache == nil || len(latestCache.Posts) == 0 {
		return fmt.Errorf("刷新后帖子缓存为空")
	}
	latestPosts := limitPosts(latestCache.Posts, topN)

	if len(cachedPosts) == 0 {
		log.Printf("监控：@%s 首次建立 posts_cache 基线（%d 条）", username, len(latestPosts))
		return nil
	}

	newShortcodes := DiffNewShortcodes(cachedPosts, latestPosts)
	if len(newShortcodes) == 0 {
		log.Printf("监控：@%s 无新帖（前 %d 条与缓存一致）", username, topN)
		return nil
	}

	log.Printf("监控：@%s 检测到 %d 条新帖，开始补抓", username, len(newShortcodes))

	for i := len(newShortcodes) - 1; i >= 0; i-- {
		shortcode := newShortcodes[i]

		filePaths, downloadErr := ws.downloadByShortcode(shortcode)
		if downloadErr != nil {
			log.Printf("监控：@%s 下载失败（%s）: %v", username, shortcode, downloadErr)
			continue
		}

		if ws.config != nil && len(ws.config.AdminUserIDs) > 0 {
			ws.notifyNewPost(username, shortcode, ws.config.AdminUserIDs, filePaths)
		}
	}

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
