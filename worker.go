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

type WorkerServer struct {
	server        *http.Server
	config        *Config
	browserCtx    context.Context
	browserCancel context.CancelFunc
	browserMu     sync.Mutex
}

type WorkerDownloadRequest struct {
	Username  string `json:"username,omitempty"`
	PostIndex int    `json:"post_index,omitempty"`
	Shortcode string `json:"shortcode,omitempty"`
	Mode      string `json:"mode"` // "index" 或 "shortcode"
}

type WorkerDownloadResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
}

func getWorkerListenAddr() string {
	if value := strings.TrimSpace(os.Getenv("CRAWLER_WORKER_ADDR")); value != "" {
		return value
	}

	type workerConfig struct {
		WorkerAddr string `json:"worker_addr"`
	}
	if data, err := os.ReadFile("config.json"); err == nil {
		var cfg workerConfig
		if json.Unmarshal(data, &cfg) == nil {
			if value := strings.TrimSpace(cfg.WorkerAddr); value != "" {
				return value
			}
		}
	}

	return defaultWorkerListenAddr
}

func NewWorkerServer() *WorkerServer {
	mux := http.NewServeMux()
	ws := &WorkerServer{}

	// 尝试加载配置（用于 Cookie 失效通知）
	if cfg, err := LoadConfig("config.json"); err == nil {
		ws.config = cfg
	}

	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/download", ws.handleDownload)

	ws.server = &http.Server{
		Addr:              getWorkerListenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      10 * time.Minute,
	}

	return ws
}

func (ws *WorkerServer) Start() error {
	log.Printf("Worker 服务启动: http://%s", ws.server.Addr)
	if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (ws *WorkerServer) Shutdown(ctx context.Context) error {
	// 关闭浏览器实例
	ws.browserMu.Lock()
	if ws.browserCancel != nil {
		ws.browserCancel()
		log.Println("关闭浏览器实例")
	}
	ws.browserMu.Unlock()

	return ws.server.Shutdown(ctx)
}

// getBrowser 获取或创建浏览器实例（Worker 级别复用）
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

// notifyCookieExpired 发送 Cookie 失效通知到 Telegram
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

func (ws *WorkerServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "worker",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (ws *WorkerServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, WorkerDownloadResponse{Success: false, Message: "仅支持 POST"})
		return
	}

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

func (ws *WorkerServer) runDownloadTask(req WorkerDownloadRequest) ([]string, error) {
	if req.Mode == "shortcode" {
		return ws.downloadByShortcode(req.Shortcode)
	}
	return ws.downloadByIndex(req.Username, req.PostIndex)
}

// downloadByShortcode 通过 Shortcode 下载（使用缓存）
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
		postURL := "https://www.instagram.com/p/" + shortcode + "/"
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

// downloadByIndex 通过位置下载（使用帖子列表缓存）
func (ws *WorkerServer) downloadByIndex(username string, postIndex int) ([]string, error) {
	log.Printf("  检查帖子列表缓存...")
	var shortcode string

	// 1. 检查帖子列表缓存
	if postsCache, ok := GetPostsFromCache(username); ok {
		log.Printf("  ✓ 帖子列表缓存命中（共 %d 条）", len(postsCache.Posts))
		for _, post := range postsCache.Posts {
			if post.Index == postIndex {
				shortcode = post.Shortcode
				break
			}
		}
		if shortcode == "" && postIndex > len(postsCache.Posts) {
			return nil, fmt.Errorf("帖子索引超出范围（共 %d 条帖子，请求第 %d 条）", len(postsCache.Posts), postIndex)
		}
	}

	if shortcode == "" {
		// 2. 缓存过期或不存在，访问主页
		log.Printf("  缓存未命中，访问用户主页...")
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

		// 获取所有帖子链接
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
			ExpiresAt: time.Now().Add(24 * time.Hour),
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
			return nil, fmt.Errorf("未找到第 %d 条帖子", postIndex)
		}
	}

	log.Printf("  ✓ 获取到 shortcode: %s", shortcode)

	// 3. 使用 shortcode 下载（会使用文件和媒体缓存）
	files, err := ws.downloadByShortcode(shortcode)
	if err != nil {
		return nil, err
	}

	// 4. 更新文件缓存，添加用户名和位置信息
	if filesCache, ok := GetFilesFromCache(shortcode); ok {
		filesCache.Username = username
		filesCache.PostIndex = postIndex
		SaveFilesToCache(shortcode, filesCache)
	}

	return files, nil
}

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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("worker 优雅退出失败: %w", err)
		}
		log.Println("worker 已优雅退出")
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func buildWorkerBaseURL() string {
	addr := getWorkerListenAddr()
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

func workerControlSummary() string {
	return fmt.Sprintf("worker 地址: %s", buildWorkerBaseURL())
}

func parseWorkerPort() string {
	parts := strings.Split(getWorkerListenAddr(), ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func workerPortInt() int {
	port, _ := strconv.Atoi(parseWorkerPort())
	return port
}
