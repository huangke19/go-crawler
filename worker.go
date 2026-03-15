// ============================================================================
// worker.go - Worker HTTP 服务（执行面）
// ============================================================================
//
// 职责：
//   - 启动 HTTP 服务，接收 Bot 的下载请求
//   - 复用浏览器实例，避免每个请求都启动 Chrome
//   - 优先使用三层缓存（文件 / 媒体 / 帖子）
//   - 执行实时抓取与下载
//   - 优雅关闭：等待活跃请求完成
//
// 核心概念：
//   - Worker 是"执行面"：负责耗时的抓取与下载
//   - Bot 是"控制面"：负责交互与文件上传
//   - 分离设计：避免 Bot 因长耗时操作而无法响应用户
//
// HTTP 接口：
//   - GET /health：健康检查
//   - POST /download：执行下载任务
//   - POST /check-update：检查主页是否有更新并刷新帖子列表缓存
//
// 下载请求格式：
//   {
//     "mode": "index" 或 "shortcode",
//     "username": "用户名（mode=index 时必需）",
//     "post_index": 帖子序号（mode=index 时必需）,
//     "shortcode": "shortcode（mode=shortcode 时必需）"
//   }
//
// 下载响应格式：
//   {
//     "success": true/false,
//     "message": "错误信息或成功提示",
//     "file_paths": ["文件路径1", "文件路径2", ...]
//   }
//
// 缓存命中顺序：
//   1. 文件缓存：如果文件仍存在，直接返回路径（<1秒）
//   2. 媒体缓存：如果 shortcode 的媒体 URL 已缓存，跳过 GraphQL 调用
//   3. 帖子缓存：如果用户主页帖子列表未过期，跳过滚动加载
//   4. 实时抓取：以上都未命中，则实时访问主页、调用 GraphQL、下载文件
//
// 关键函数：
//   - NewWorkerServer()：创建 Worker 服务
//   - RunWorker()：启动 Worker 服务
//   - handleHealth()：健康检查
//   - handleDownload()：处理下载请求
//   - handleCheckUpdate()：检查主页更新
//   - getWorkerListenAddr()：获取监听地址
//
// ============================================================================

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultWorkerListenAddr = "127.0.0.1:18080"
)

// WorkerServer 是下载执行面 HTTP 服务。
//
// 设计要点：
// - Worker 负责"耗时且可能阻塞"的抓取与下载逻辑；Bot 只做交互与文件上传。
// - Worker 复用一个无头浏览器实例（避免每个请求都启动 Chrome）；
// - 通过 activeReqs 跟踪活跃请求，实现优雅关闭（尽量不半途打断下载）。
// - stopCh 用于通知监控 goroutine 退出。
type WorkerServer struct {
	server        *http.Server
	config        *Config
	browserCtx    context.Context
	browserCancel context.CancelFunc
	browserMu     sync.Mutex
	activeReqs    sync.WaitGroup // 跟踪活跃请求
	stopCh        chan struct{}  // 关闭信号，通知监控 goroutine 退出
}

// WorkerDownloadRequest 是 Bot/CLI 调用 Worker 的下载请求体。
// - mode=index：按用户主页时间线序号下载（需要 username + post_index）
// - mode=shortcode：按 shortcode 下载（只需要 shortcode）
type WorkerDownloadRequest struct {
	Username  string `json:"username,omitempty"`
	PostIndex int    `json:"post_index,omitempty"`
	Shortcode string `json:"shortcode,omitempty"`
	Mode      string `json:"mode"` // "index" 或 "shortcode"
}

// WorkerDownloadResponse 是下载结果响应体。
// 成功时返回 file_paths（本机文件绝对/相对路径，供 bot 上传）。
type WorkerDownloadResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
}

// getWorkerListenAddr 获取 worker 监听地址。
// 优先级：环境变量 `CRAWLER_WORKER_ADDR` > config.json 的 worker_addr > 默认值。
func getWorkerListenAddr() string {
	if value := strings.TrimSpace(os.Getenv("CRAWLER_WORKER_ADDR")); value != "" {
		return value
	}

	if cfg, err := LoadConfig("config.json"); err == nil {
		return cfg.GetWorkerAddr()
	}

	return defaultWorkerListenAddr
}

// NewWorkerServer 构建 worker HTTP 服务并注册路由：
// - /health：健康检查
// - /download：执行下载任务
// - /check-update：检查主页是否有更新并刷新帖子列表缓存
func NewWorkerServer() *WorkerServer {
	mux := http.NewServeMux()
	ws := &WorkerServer{
		stopCh: make(chan struct{}),
	}

	// 尝试加载配置（用于 Cookie 失效通知）
	if cfg, err := LoadConfig("config.json"); err == nil {
		ws.config = cfg
	}

	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/download", ws.handleDownload)
	mux.HandleFunc("/check-update", ws.handleCheckUpdate)

	ws.server = &http.Server{
		Addr:              getWorkerListenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: workerReadHeaderTimeout,
		ReadTimeout:       workerReadTimeout,
		WriteTimeout:      workerWriteTimeout,
	}

	return ws
}

// Start 启动 HTTP 服务（阻塞）。
// 服务启动后同时启动监控 goroutine。
func (ws *WorkerServer) Start() error {
	log.Printf("Worker 服务启动: http://%s", ws.server.Addr)
	ws.startMonitorLoop()
	if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown 优雅关闭 worker：
// - 最多等待 30 秒让活跃请求完成\n+// - 关闭复用的浏览器上下文\n+// - 关闭 HTTP server
func (ws *WorkerServer) Shutdown(ctx context.Context) error {
	log.Println("开始优雅关闭 Worker...")

	// 停止监控 goroutine
	select {
	case <-ws.stopCh:
		// 已经关闭，忽略
	default:
		close(ws.stopCh)
	}

	// 等待所有活跃请求完成（最多等待 30 秒）
	done := make(chan struct{})
	go func() {
		ws.activeReqs.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("所有请求已完成")
	case <-time.After(workerShutdownTimeout):
		log.Println("⚠️  等待请求超时，强制关闭")
	}

	// 关闭浏览器实例
	ws.browserMu.Lock()
	if ws.browserCancel != nil {
		ws.browserCancel()
		log.Println("关闭浏览器实例")
	}
	ws.browserMu.Unlock()

	return ws.server.Shutdown(ctx)
}

// getBrowser 获取或创建浏览器实例（Worker 级别复用）。
//
// 该浏览器实例用于：
// - 访问用户主页获取帖子 shortcode 列表（按序号下载/刷新缓存）\n+// - 在媒体缓存未命中时，调用 GraphQL 获取媒体 URL
func (ws *WorkerServer) getBrowser() (context.Context, error) {
	ws.browserMu.Lock()
	defer ws.browserMu.Unlock()

	// 如果浏览器存在且健康，直接返回
	if ws.browserCtx != nil && ws.browserCtx.Err() == nil {
		return ws.browserCtx, nil
	}

	// 如果旧的上下文存在但出错，先清理
	if ws.browserCancel != nil {
		ws.browserCancel()
		log.Println("清理旧的浏览器实例")
	}

	ctx, cancel := CreateFastBrowserContext()
	ws.browserCtx = ctx
	ws.browserCancel = cancel
	log.Println("创建新的浏览器实例")

	return ws.browserCtx, nil
}

// notifyCookieExpired 发送 Cookie 失效通知到 Telegram。
// 目的：当 worker 在 GraphQL 请求中检测到 401/403 时，主动提醒管理员重新执行 `crawler login`。
func (ws *WorkerServer) notifyCookieExpired() {
	if ws.config == nil || ws.config.TelegramBotToken == "" {
		return
	}

	message := "⚠️ Instagram Cookie 已失效，请运行 ./crawler login 重新登录"

	for _, chatID := range ws.config.AdminUserIDs {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ws.config.TelegramBotToken)
		payload := map[string]interface{}{
			"chat_id": chatID,
			"text":    message,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}
}

// handleHealth 提供健康检查接口。\n+// Bot 会用它判断 worker 是否在线（避免用户点击后才发现服务未启动）。
func (ws *WorkerServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "worker",
		"time":    time.Now().Format(time.RFC3339),
	})
}

// handleDownload 是 worker 的核心入口：执行下载任务并返回文件路径列表。
// 该接口设计为同步返回，调用方（Bot）在收到响应后再进行上传。
func (ws *WorkerServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	// 跟踪活跃请求
	ws.activeReqs.Add(1)
	defer ws.activeReqs.Done()

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, WorkerDownloadResponse{Success: false, Message: "仅支持 POST"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req WorkerDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "请求体格式错误"})
		return
	}

	// 设置默认模式
	if req.Mode == "" {
		req.Mode = "index"
	}

	// 验证参数
	if req.Mode == "index" {
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "username 不能为空"})
			return
		}
		if req.PostIndex < 1 {
			writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "post_index 必须大于 0"})
			return
		}
		log.Printf("接收下载任务: @%s 第 %d 个帖子 (按位置模式)", req.Username, req.PostIndex)
	} else if req.Mode == "shortcode" {
		req.Shortcode = strings.TrimSpace(req.Shortcode)
		if req.Shortcode == "" {
			writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "shortcode 不能为空"})
			return
		}
		log.Printf("接收下载任务: shortcode=%s (按Shortcode模式)", req.Shortcode)
	} else {
		writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "无效的 mode"})
		return
	}

	files, err := ws.runDownloadTask(req)
	if err != nil {
		// 检测 Cookie 失效
		if strings.Contains(err.Error(), "Cookie 已失效") {
			ws.notifyCookieExpired()
		}

		log.Printf("下载任务失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, WorkerDownloadResponse{Success: false, Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, WorkerDownloadResponse{
		Success:   true,
		Message:   fmt.Sprintf("下载完成，共 %d 个文件", len(files)),
		FilePaths: files,
	})
}

// runDownloadTask 根据 mode 分发下载策略。
func (ws *WorkerServer) runDownloadTask(req WorkerDownloadRequest) ([]string, error) {
	if req.Mode == "shortcode" {
		return ws.downloadByShortcode(req.Shortcode)
	}
	return ws.downloadByIndex(req.Username, req.PostIndex)
}

// downloadByShortcode 通过 Shortcode 下载（强依赖缓存路径语义）。
//
// 命中顺序：
// 1) 文件缓存（files_cache）：若文件仍存在，直接返回路径\n+// 2) 媒体缓存（media_cache）：若媒体 URL 已缓存，跳过 GraphQL\n+// 3) GraphQL 获取媒体 URL，并回填媒体缓存\n+// 4) 下载到 `downloads/cache/<shortcode>/...`，并写入文件缓存
func (ws *WorkerServer) downloadByShortcode(shortcode string) ([]string, error) {
	log.Printf("  检查文件缓存...")
	// 1. 检查文件缓存
	if filesCache, ok := GetFilesFromCache(shortcode); ok {
		log.Printf("  ✓ 文件缓存命中，直接返回 %d 个文件", len(filesCache.Files))
		return filesCache.Files, nil
	}

	log.Printf("  检查媒体URL缓存...")
	// 2. 检查媒体URL缓存
	var mediaInfo *MediaInfo
	if mediaCache, ok := GetMediaFromCache(shortcode); ok {
		log.Printf("  ✓ 媒体URL缓存命中")
		mediaInfo = &MediaInfo{
			Type:  mediaCache.Type,
			URLs:  mediaCache.URLs,
			Types: mediaCache.Types,
		}
	} else {
		// 3. 调用 GraphQL API 获取媒体URL
		log.Printf("  缓存未命中，调用 GraphQL API...")
		postURL := instagramBaseURL + "/p/" + shortcode + "/"
		ctx, err := ws.getBrowser()
		if err != nil {
			return nil, fmt.Errorf("获取浏览器失败: %w", err)
		}

		mediaInfo, err = ExtractMediaURLs(ctx, postURL)
		if err != nil {
			return nil, fmt.Errorf("提取媒体失败: %w", err)
		}

		// 保存到媒体缓存
		SaveMediaToCache(shortcode, &MediaCache{
			Type:  mediaInfo.Type,
			URLs:  mediaInfo.URLs,
			Types: mediaInfo.Types,
		})
		log.Printf("  ✓ 已保存媒体URL到缓存")
	}

	// 4. 下载文件（使用 shortcode 作为文件名前缀）
	log.Printf("  开始下载 %d 个文件...", len(mediaInfo.URLs))
	files, err := downloadMediaByShortcode(shortcode, mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}

	// 5. 保存文件缓存
	SaveFilesToCache(shortcode, &FilesCache{
		Files:        files,
		DownloadedAt: time.Now(),
	})
	log.Printf("  ✓ 已保存文件路径到缓存")

	return files, nil
}

// downloadByIndex 通过"主页时间线序号"下载。
//
// 关键点：
// - 先尝试从 posts_cache 命中 shortcode（避免打开主页）\n+// - 若缓存不存在/过期/数量不足，则访问主页加载至少 postIndex 条并刷新缓存\n+// - 拿到 shortcode 后复用 downloadByShortcode（自动套用媒体/文件缓存）
func (ws *WorkerServer) downloadByIndex(username string, postIndex int) ([]string, error) {
	log.Printf("  检查帖子列表缓存...")
	var shortcode string
	needRefresh := false

	// 1. 检查帖子列表缓存
	if postsCache, ok := GetPostsFromCache(username); ok {
		log.Printf("  ✓ 帖子列表缓存命中（共 %d 条）", len(postsCache.Posts))
		for _, post := range postsCache.Posts {
			if post.Index == postIndex {
				shortcode = post.Shortcode
				break
			}
		}
		// 如果缓存的帖子数量不足或未匹配到，需要重新加载
		if shortcode == "" && postIndex > len(postsCache.Posts) {
			log.Printf("  ⚠️  缓存的帖子数量不足（共 %d 条，请求第 %d 条），需要重新加载", len(postsCache.Posts), postIndex)
			needRefresh = true
		} else if shortcode == "" {
			log.Printf("  ⚠️  缓存中未找到第 %d 条帖子，需要重新加载", postIndex)
			needRefresh = true
		}
	} else {
		needRefresh = true
	}

	if shortcode == "" && needRefresh {
		// 2. 缓存过期、不存在或数量不足，访问主页
		log.Printf("  访问用户主页加载更多帖子...")
		ctx, err := ws.getBrowser()
		if err != nil {
			return nil, fmt.Errorf("获取浏览器失败: %w", err)
		}

		if err := EnsureLoggedIn(ctx); err != nil {
			return nil, fmt.Errorf("登录验证失败: %w", err)
		}

		if err := NavigateToUser(ctx, username); err != nil {
			return nil, fmt.Errorf("访问用户主页失败: %w", err)
		}

		// 获取所有帖子链接（至少加载到目标索引）
		postLinks, err := GetAllPostLinks(ctx, postIndex)
		if err != nil {
			return nil, fmt.Errorf("获取帖子列表失败: %w", err)
		}

		// 更新缓存
		posts := []PostItem{}
		for i, link := range postLinks {
			sc := extractShortcode(link)
			if sc != "" {
				posts = append(posts, PostItem{
					Index:     i + 1,
					Shortcode: sc,
				})
			}
		}

		SavePostsToCache(username, &PostsCache{
			Posts:     posts,
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(postsCacheExpiry),
		})
		log.Printf("  ✓ 已保存帖子列表到缓存（共 %d 条，24小时有效）", len(posts))

		// 获取目标 shortcode
		for _, post := range posts {
			if post.Index == postIndex {
				shortcode = post.Shortcode
				break
			}
		}

		if shortcode == "" {
			return nil, fmt.Errorf("帖子索引超出范围（共 %d 条帖子，请求第 %d 条）", len(posts), postIndex)
		}
	}

	log.Printf("  ✓ 获取到 shortcode: %s", shortcode)

	// 3. 使用 shortcode 下载（会使用文件和媒体缓存）
	files, err := ws.downloadByShortcode(shortcode)
	if err != nil {
		return nil, err
	}

	// 4. 更新文件缓存，添加用户名和位置信息
	if filesCache, ok := GetFilesFromCache(shortcode); ok && filesCache != nil {
		filesCache.Username = username
		filesCache.PostIndex = postIndex
		SaveFilesToCache(shortcode, filesCache)
	}

	return files, nil
}

// RunWorker 启动 worker 并监听退出信号（SIGINT/SIGTERM），支持优雅关闭。
func RunWorker() error {
	server := NewWorkerServer()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return fmt.Errorf("worker 服务异常退出: %w", err)
	case sig := <-sigCh:
		log.Printf("收到退出信号: %s", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), workerReadHeaderTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("worker 优雅退出失败: %w", err)
		}
		log.Println("worker 已优雅退出")
		return nil
	}
}

// writeJSON 写入 JSON 响应。这里忽略编码错误以避免影响主流程（一般不会发生）。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// buildWorkerBaseURL 由监听地址构建可访问的 base URL（默认 http://）。
func buildWorkerBaseURL() string {
	addr := getWorkerListenAddr()
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

// workerControlSummary 返回 worker 控制面摘要文本（用于 status 输出）。
func workerControlSummary() string {
	return fmt.Sprintf("worker 地址: %s", buildWorkerBaseURL())
}

// parseWorkerPort 从监听地址中提取端口部分（用于守护/控制逻辑）。
func parseWorkerPort() string {
	parts := strings.Split(getWorkerListenAddr(), ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// workerPortInt 返回端口的 int 形式（转换失败返回 0）。
func workerPortInt() int {
	port, _ := strconv.Atoi(parseWorkerPort())
	return port
}

// handleCheckUpdate 处理检查更新请求。\n+// 用于 bot 侧"检查更新"按钮：判断主页最新帖子是否变化，必要时刷新 posts_cache。
func (ws *WorkerServer) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	ws.activeReqs.Add(1)
	defer ws.activeReqs.Done()

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "仅支持 POST",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	type CheckUpdateRequest struct {
		Username string `json:"username"`
	}
	type CheckUpdateResponse struct {
		Success     bool   `json:"success"`
		Message     string `json:"message,omitempty"`
		NeedRefresh bool   `json:"need_refresh"`
		TotalPosts  int    `json:"total_posts"`
	}

	var req CheckUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, CheckUpdateResponse{
			Success: false,
			Message: "请求体格式错误",
		})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, CheckUpdateResponse{
			Success: false,
			Message: "username 不能为空",
		})
		return
	}

	log.Printf("接收检查更新请求: @%s", req.Username)

	needRefresh, totalPosts, err := ws.checkCacheUpdate(req.Username)
	if err != nil {
		log.Printf("检查更新失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, CheckUpdateResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, CheckUpdateResponse{
		Success:     true,
		NeedRefresh: needRefresh,
		TotalPosts:  totalPosts,
	})
}

// checkCacheUpdate 检查缓存是否需要更新。
// 核心逻辑委托给 RefreshPostsCache（scraper.go），此处仅负责浏览器获取与日志输出。
func (ws *WorkerServer) checkCacheUpdate(username string) (needRefresh bool, totalPosts int, err error) {
	log.Printf("  检查 @%s 的缓存状态...", username)

	ctx, err := ws.getBrowser()
	if err != nil {
		return false, 0, fmt.Errorf("获取浏览器失败: %w", err)
	}

	if err := EnsureLoggedIn(ctx); err != nil {
		return false, 0, fmt.Errorf("登录验证失败: %w", err)
	}

	needRefresh, totalPosts, err = RefreshPostsCache(ctx, username, 12)
	if err != nil {
		return false, 0, err
	}

	if needRefresh {
		log.Printf("  ✓ 检测到更新，已刷新缓存（共 %d 条）", totalPosts)
	} else {
		log.Printf("  ✓ 缓存已是最新（共 %d 条）", totalPosts)
	}

	return needRefresh, totalPosts, nil
}
