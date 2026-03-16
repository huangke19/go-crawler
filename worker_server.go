// ============================================================================
// worker_server.go - Worker HTTP 服务器（生命周期管理）
// ============================================================================
//
// 职责：
//   - WorkerServer 结构体定义
//   - 服务器创建、启动、关闭
//   - 浏览器实例管理
//   - Cookie 失效通知
//   - 监听地址配置
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
	cookieNotifyCooldown    = time.Hour
)

// WorkerServer 是下载执行面 HTTP 服务。
//
// 设计要点：
// - Worker 负责"耗时且可能阻塞"的抓取与下载逻辑；Bot 只做交互与文件上传。
// - Worker 复用一个无头浏览器实例（避免每个请求都启动 Chrome）；
// - 通过 activeReqs 跟踪活跃请求，实现优雅关闭（尽量不半途打断下载）。
// - stopCh 用于通知监控 goroutine 退出。
type WorkerServer struct {
	server           *http.Server
	config           *Config
	browserCtx       context.Context
	browserCancel    context.CancelFunc
	browserMu        sync.Mutex
	activeReqs       sync.WaitGroup // 跟踪活跃请求
	stopCh           chan struct{}  // 关闭信号，通知监控 goroutine 退出
	notifyMu         sync.Mutex
	lastCookieNotify time.Time
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
	mux.HandleFunc("/monitor-check", ws.handleMonitorCheck)

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
// - 最多等待 30 秒让活跃请求完成
// - 关闭复用的浏览器上下文
// - 关闭 HTTP server
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
// - 访问用户主页获取帖子 shortcode 列表（按序号下载/刷新缓存）
// - 在媒体缓存未命中时，调用 GraphQL 获取媒体 URL
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

	ws.notifyMu.Lock()
	defer ws.notifyMu.Unlock()

	if time.Since(ws.lastCookieNotify) < cookieNotifyCooldown {
		return
	}
	ws.lastCookieNotify = time.Now()

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
