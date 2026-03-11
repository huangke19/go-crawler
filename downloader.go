package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 全局 HTTP 客户端，复用连接
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	},
}

// downloadTask 下载任务
type downloadTask struct {
	url      string
	savePath string
}

// CreateUserDirectory 创建用户下载目录
func CreateUserDirectory(username string) (string, error) {
	dirPath := filepath.Join("downloads", username)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	return dirPath, nil
}

// DownloadMedia 下载单个媒体文件，支持重试
func DownloadMedia(url, savePath string, retries int) error {
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			fmt.Printf("  重试 %d/%d: %s\n", attempt, retries, filepath.Base(savePath))
		}

		// 发送 HTTP 请求（使用全局客户端）
		resp, err := httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("下载失败: %v", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
			continue
		}

		// 创建文件
		file, err := os.Create(savePath)
		if err != nil {
			lastErr = fmt.Errorf("创建文件失败: %v", err)
			continue
		}
		defer file.Close()

		// 写入文件
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("写入文件失败: %v", err)
			continue
		}

		return nil
	}

	return lastErr
}

// DownloadPost 下载帖子的所有媒体（并发下载）
func DownloadPost(username string, postIndex int, mediaInfo *MediaInfo) error {
	_, err := downloadPostInternal(username, postIndex, mediaInfo)
	return err
}

// DownloadPostAndReturnPaths 下载帖子并返回文件路径列表（用于 Telegram Bot）
func DownloadPostAndReturnPaths(username string, postIndex int, mediaInfo *MediaInfo) ([]string, error) {
	return downloadPostInternal(username, postIndex, mediaInfo)
}

// downloadPostInternal 内部下载函数，返回文件路径列表
func downloadPostInternal(username string, postIndex int, mediaInfo *MediaInfo) ([]string, error) {
	// 创建用户目录
	userDir, err := CreateUserDirectory(username)
	if err != nil {
		return nil, err
	}

	// 准备下载任务列表
	var tasks []downloadTask

	if mediaInfo.Type == "video" {
		// 单个视频
		filename := fmt.Sprintf("post_%d.mp4", postIndex)
		savePath := filepath.Join(userDir, filename)
		tasks = append(tasks, downloadTask{url: mediaInfo.URLs[0], savePath: savePath})
	} else if mediaInfo.Type == "carousel" {
		// 轮播内容（可能包含图片和视频）
		for i, url := range mediaInfo.URLs {
			var ext string
			if i < len(mediaInfo.Types) && mediaInfo.Types[i] == "video" {
				ext = ".mp4"
			} else {
				ext = ".jpg"
			}
			filename := fmt.Sprintf("post_%d_%d%s", postIndex, i+1, ext)
			savePath := filepath.Join(userDir, filename)
			tasks = append(tasks, downloadTask{url: url, savePath: savePath})
		}
	} else {
		// 图片（单图或多图）
		for i, url := range mediaInfo.URLs {
			var filename string
			if len(mediaInfo.URLs) == 1 {
				filename = fmt.Sprintf("post_%d.jpg", postIndex)
			} else {
				filename = fmt.Sprintf("post_%d_%d.jpg", postIndex, i+1)
			}
			savePath := filepath.Join(userDir, filename)
			tasks = append(tasks, downloadTask{url: url, savePath: savePath})
		}
	}

	// 并发下载（提升并发数到 10）
	if err := downloadConcurrently(tasks, 10, 1); err != nil {
		return nil, err
	}

	// 收集文件路径
	var filePaths []string
	for _, task := range tasks {
		filePaths = append(filePaths, task.savePath)
	}

	return filePaths, nil
}

// downloadMediaByShortcode 通过 shortcode 下载媒体（用于缓存模式）
func downloadMediaByShortcode(shortcode string, mediaInfo *MediaInfo) ([]string, error) {
	// 创建 cache 目录下的 shortcode 子目录
	cacheDir := filepath.Join("downloads", "cache", shortcode)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %v", err)
	}

	var tasks []downloadTask
	var filePaths []string

	for i, url := range mediaInfo.URLs {
		var ext string
		if i < len(mediaInfo.Types) && mediaInfo.Types[i] == "video" {
			ext = ".mp4"
		} else {
			ext = ".jpg"
		}

		var filename string
		if len(mediaInfo.URLs) == 1 {
			// 单个文件
			filename = shortcode + ext
		} else {
			// 多个文件
			filename = fmt.Sprintf("%s_%d%s", shortcode, i+1, ext)
		}

		savePath := filepath.Join(cacheDir, filename)
		tasks = append(tasks, downloadTask{url: url, savePath: savePath})
		filePaths = append(filePaths, savePath)
	}

	// 并发下载
	if err := downloadConcurrently(tasks, 10, 1); err != nil {
		return nil, err
	}

	return filePaths, nil
}

// downloadConcurrently 并发下载多个文件
func downloadConcurrently(tasks []downloadTask, maxConcurrent int, retries int) error {
	totalFiles := len(tasks)
	var completed int32
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrent)
	errChan := make(chan error, totalFiles)

	fmt.Printf("开始下载 %d 个文件...\n", totalFiles)

	for _, task := range tasks {
		wg.Add(1)
		go func(t downloadTask) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 下载文件
			if err := DownloadMedia(t.url, t.savePath, retries); err != nil {
				errChan <- fmt.Errorf("%s: %v", filepath.Base(t.savePath), err)
				return
			}

			// 更新进度
			atomic.AddInt32(&completed, 1)
		}(task)
	}

	// 等待所有下载完成
	wg.Wait()
	close(errChan)

	// 收集错误
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("下载失败 %d 个文件", len(errors))
	}

	fmt.Printf("✓ 下载完成，共 %d 个文件\n", totalFiles)
	return nil
}

// GetFileExtension 从 URL 获取文件扩展名
func GetFileExtension(url string) string {
	// 移除查询参数
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}

	ext := filepath.Ext(url)
	if ext == "" {
		return ".jpg" // 默认扩展名
	}
	return ext
}
