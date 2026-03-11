package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultWorkerListenAddr = "127.0.0.1:18080"
)

type WorkerServer struct {
	server *http.Server
}

type WorkerDownloadRequest struct {
	Username  string `json:"username"`
	PostIndex int    `json:"post_index"`
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
	return ws.server.Shutdown(ctx)
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

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "username 不能为空"})
		return
	}
	if req.PostIndex < 1 {
		writeJSON(w, http.StatusBadRequest, WorkerDownloadResponse{Success: false, Message: "post_index 必须大于 0"})
		return
	}

	log.Printf("接收下载任务: @%s 第 %d 个帖子", req.Username, req.PostIndex)
	files, err := runDownloadTask(req.Username, req.PostIndex)
	if err != nil {
		log.Printf("下载任务失败: @%s 第 %d 个帖子: %v", req.Username, req.PostIndex, err)
		writeJSON(w, http.StatusInternalServerError, WorkerDownloadResponse{Success: false, Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, WorkerDownloadResponse{
		Success:   true,
		Message:   fmt.Sprintf("下载完成，共 %d 个文件", len(files)),
		FilePaths: files,
	})
}

func runDownloadTask(username string, postIndex int) ([]string, error) {
	ctx, cancel := CreateFastBrowserContext()
	defer cancel()

	if err := EnsureLoggedIn(ctx); err != nil {
		return nil, fmt.Errorf("登录验证失败: %w", err)
	}
	if err := NavigateToUser(ctx, username); err != nil {
		return nil, fmt.Errorf("访问用户主页失败: %w", err)
	}
	postURL, err := GetPostByIndex(ctx, postIndex)
	if err != nil {
		return nil, fmt.Errorf("获取帖子失败: %w", err)
	}
	mediaInfo, err := ExtractMediaURLs(ctx, postURL)
	if err != nil {
		return nil, fmt.Errorf("提取媒体失败: %w", err)
	}
	files, err := DownloadPostAndReturnPaths(username, postIndex, mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
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
