// ============================================================================
// worker_handlers.go - Worker HTTP 请求处理器
// ============================================================================
//
// 职责：
//   - HTTP 请求处理（/health, /download, /check-update, /monitor-check）
//   - 下载任务执行（按序号/按shortcode）
//   - 缓存更新检查
//   - 监控检测
//
// ============================================================================

package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const workerAuthHeader = "X-Worker-Token"

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

// CheckUpdateResponse 检查更新接口的响应。
type CheckUpdateResponse struct {
	Success       bool       `json:"success"`
	Message       string     `json:"message,omitempty"`
	NeedRefresh   bool       `json:"need_refresh"`
	TotalPosts    int        `json:"total_posts"`
	NewShortcodes []string   `json:"new_shortcodes,omitempty"`
	NewFilePaths  [][]string `json:"new_file_paths,omitempty"`
}

// MonitorCheckResponse 单账户立即检测接口的响应。
type MonitorCheckResponse struct {
	Success       bool       `json:"success"`
	Message       string     `json:"message,omitempty"`
	NewShortcodes []string   `json:"new_shortcodes,omitempty"`
	NewFilePaths  [][]string `json:"new_file_paths,omitempty"`
}

// handleHealth 提供健康检查接口。
// Bot 会用它判断 worker 是否在线（避免用户点击后才发现服务未启动）。
func (ws *WorkerServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "worker",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	addr := strings.TrimSpace(remoteAddr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (ws *WorkerServer) authorizeRequest(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimSpace(ws.workerAPIToken)
	if token != "" {
		presented := strings.TrimSpace(r.Header.Get(workerAuthHeader))
		if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false,
				"message": "未授权访问",
			})
			return false
		}
		return true
	}

	if isLoopbackRemoteAddr(r.RemoteAddr) {
		return true
	}

	writeJSON(w, http.StatusForbidden, map[string]any{
		"success": false,
		"message": "请求来源受限：请配置 WORKER_API_TOKEN，或仅从本机访问",
	})
	return false
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
	if !ws.authorizeRequest(w, r) {
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
// 1) 文件缓存（files_cache）：若文件仍存在，直接返回路径
// 2) 媒体缓存（media_cache）：若媒体 URL 已缓存，跳过 GraphQL
// 3) GraphQL 获取媒体 URL，并回填媒体缓存
// 4) 下载到 `downloads/cache/<shortcode>/...`，并写入文件缓存
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
		ctx, cancel, err := ws.getBrowser()
		if err != nil {
			return nil, fmt.Errorf("获取浏览器失败: %w", err)
		}
		defer cancel()

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
// - 先尝试从 posts_cache 命中 shortcode（避免打开主页）
// - 若缓存不存在/过期/数量不足，则访问主页加载至少 postIndex 条并刷新缓存
// - 拿到 shortcode 后复用 downloadByShortcode（自动套用媒体/文件缓存）
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
		ctx, cancel, err := ws.getBrowser()
		if err != nil {
			return nil, fmt.Errorf("获取浏览器失败: %w", err)
		}
		defer cancel()

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

// handleCheckUpdate 处理检查更新请求。
// 用于 bot 侧"检查更新"按钮：检测新帖 → 自动下载 → 返回文件路径。
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
	if !ws.authorizeRequest(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	type CheckUpdateRequest struct {
		Username string `json:"username"`
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

	result, err := ws.checkCacheUpdate(req.Username)
	if err != nil {
		log.Printf("检查更新失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, CheckUpdateResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// checkCacheUpdate 检查缓存是否需要更新，发现新帖时自动下载。
// 采用与监控相同的 pre/post diff 模式：
// 1. 读取旧缓存基线
// 2. 刷新帖子列表
// 3. 对比差集找出新 shortcode
// 4. 逐条下载新帖
func (ws *WorkerServer) checkCacheUpdate(username string) (*CheckUpdateResponse, error) {
	log.Printf("  检查 @%s 的缓存状态...", username)

	ctx, cancel, err := ws.getBrowser()
	if err != nil {
		return nil, fmt.Errorf("获取浏览器失败: %w", err)
	}
	defer cancel()

	if err := EnsureLoggedIn(ctx); err != nil {
		return nil, fmt.Errorf("登录验证失败: %w", err)
	}

	// 1. 读取旧缓存基线
	var cachedPosts []PostItem
	if postsCache, ok := GetPostsFromCacheRaw(username); ok && postsCache != nil {
		cachedPosts = postsCache.Posts
	}

	// 2. 刷新帖子列表
	_, totalPosts, err := RefreshPostsCache(ctx, username, 12)
	if err != nil {
		return nil, err
	}

	// 3. 读取刷新后的缓存并对比
	var latestPosts []PostItem
	if latestCache, ok := GetPostsFromCacheRaw(username); ok && latestCache != nil {
		latestPosts = latestCache.Posts
	}

	newShortcodes := DiffNewShortcodes(cachedPosts, latestPosts)

	if len(newShortcodes) == 0 {
		log.Printf("  ✓ 缓存已是最新（共 %d 条）", totalPosts)
		return &CheckUpdateResponse{
			Success:    true,
			TotalPosts: totalPosts,
		}, nil
	}

	log.Printf("  ✓ 检测到 %d 条新帖，开始下载", len(newShortcodes))

	// 4. 逐条下载新帖（从旧到新）
	var allFilePaths [][]string
	var downloadedShortcodes []string
	for i := len(newShortcodes) - 1; i >= 0; i-- {
		sc := newShortcodes[i]
		files, dlErr := ws.downloadByShortcode(sc)
		if dlErr != nil {
			log.Printf("  下载失败（%s）: %v", sc, dlErr)
			continue
		}
		downloadedShortcodes = append(downloadedShortcodes, sc)
		allFilePaths = append(allFilePaths, files)
		log.Printf("  ✓ 下载完成 %s（%d 个文件）", sc, len(files))
	}

	return &CheckUpdateResponse{
		Success:       true,
		NeedRefresh:   true,
		TotalPosts:    totalPosts,
		NewShortcodes: downloadedShortcodes,
		NewFilePaths:  allFilePaths,
	}, nil
}

// handleMonitorCheck 立即对指定账户执行一次监控检测。
func (ws *WorkerServer) handleMonitorCheck(w http.ResponseWriter, r *http.Request) {
	ws.activeReqs.Add(1)
	defer ws.activeReqs.Done()

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "仅支持 POST",
		})
		return
	}
	if !ws.authorizeRequest(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	type MonitorCheckRequest struct {
		Username string `json:"username"`
	}

	var req MonitorCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MonitorCheckResponse{
			Success: false, Message: "请求体格式错误",
		})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, MonitorCheckResponse{
			Success: false, Message: "username 不能为空",
		})
		return
	}

	log.Printf("立即检测请求: @%s", req.Username)

	cfg, err := LoadConfig("config.json")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MonitorCheckResponse{
			Success: false, Message: "加载配置失败",
		})
		return
	}
	topN := cfg.MonitorCompareTopN
	if topN <= 0 {
		topN = defaultMonitorCompareTopN
	}

	ctx, cancel, ctxErr := ws.getBrowser()
	if ctxErr != nil {
		writeJSON(w, http.StatusInternalServerError, MonitorCheckResponse{
			Success: false, Message: ctxErr.Error(),
		})
		return
	}
	defer cancel()
	if loginErr := EnsureLoggedIn(ctx); loginErr != nil {
		writeJSON(w, http.StatusInternalServerError, MonitorCheckResponse{
			Success: false, Message: loginErr.Error(),
		})
		return
	}

	// 读取旧缓存基线
	var cachedPosts []PostItem
	if postsCache, ok := GetPostsFromCacheRaw(req.Username); ok && postsCache != nil {
		cachedPosts = limitPosts(postsCache.Posts, topN)
	}

	// 刷新帖子列表
	if _, _, err := RefreshPostsCache(ctx, req.Username, topN); err != nil {
		writeJSON(w, http.StatusInternalServerError, MonitorCheckResponse{
			Success: false, Message: err.Error(),
		})
		return
	}

	latestCache, ok := GetPostsFromCacheRaw(req.Username)
	if !ok || latestCache == nil {
		writeJSON(w, http.StatusOK, MonitorCheckResponse{Success: true})
		return
	}
	latestPosts := limitPosts(latestCache.Posts, topN)

	// 首次建立基线
	if len(cachedPosts) == 0 {
		writeJSON(w, http.StatusOK, MonitorCheckResponse{Success: true})
		return
	}

	newShortcodes := DiffNewShortcodes(cachedPosts, latestPosts)
	if len(newShortcodes) == 0 {
		writeJSON(w, http.StatusOK, MonitorCheckResponse{Success: true})
		return
	}

	var allFilePaths [][]string
	var downloadedShortcodes []string
	for i := len(newShortcodes) - 1; i >= 0; i-- {
		sc := newShortcodes[i]
		files, dlErr := ws.downloadByShortcode(sc)
		if dlErr != nil {
			log.Printf("立即检测：下载失败（%s）: %v", sc, dlErr)
			continue
		}
		downloadedShortcodes = append(downloadedShortcodes, sc)
		allFilePaths = append(allFilePaths, files)
	}

	writeJSON(w, http.StatusOK, MonitorCheckResponse{
		Success:       true,
		NewShortcodes: downloadedShortcodes,
		NewFilePaths:  allFilePaths,
	})
}

// handleDownloadURL 处理外部 URL 下载请求（YouTube / X）
func (ws *WorkerServer) handleDownloadURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ExternalDownloadResponse{Message: "仅支持 POST"})
		return
	}
	if !ws.authorizeRequest(w, r) {
		return
	}

	ws.activeReqs.Add(1)
	defer ws.activeReqs.Done()

	var req ExternalDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ExternalDownloadResponse{Message: "无效的请求体"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, ExternalDownloadResponse{Message: "缺少 url 参数"})
		return
	}

	platform := DetectPlatform(req.URL)
	if platform == PlatformUnknown {
		writeJSON(w, http.StatusBadRequest, ExternalDownloadResponse{
			Message: "不支持的 URL，目前仅支持 YouTube 和 X (Twitter)",
		})
		return
	}

	log.Printf("收到外部下载请求: %s (%s)", req.URL, PlatformLabel(platform))

	result, err := DownloadExternalURL(req.URL)
	if err != nil {
		log.Printf("外部下载失败: %v", err)
		ExternalDownloadTotal.WithLabelValues(string(platform), "error").Inc()
		writeJSON(w, http.StatusInternalServerError, ExternalDownloadResponse{
			Message: fmt.Sprintf("下载失败: %v", err),
		})
		return
	}

	ExternalDownloadTotal.WithLabelValues(string(platform), "success").Inc()
	writeJSON(w, http.StatusOK, result)
}

// writeJSON 写入 JSON 响应。这里忽略编码错误以避免影响主流程（一般不会发生）。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
