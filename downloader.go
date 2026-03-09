package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CreateUserDirectory 创建用户下载目录
func CreateUserDirectory(username string) (string, error) {
	dirPath := filepath.Join("downloads", username)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	return dirPath, nil
}

// DownloadMedia 下载单个媒体文件
func DownloadMedia(url, savePath string) error {
	fmt.Printf("正在下载: %s\n", filepath.Base(savePath))

	// 发送 HTTP 请求
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	// 创建文件
	file, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 写入文件
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("✓ 下载完成: %s\n", savePath)
	return nil
}

// DownloadPost 下载帖子的所有媒体
func DownloadPost(username string, postIndex int, mediaInfo *MediaInfo) error {
	// 创建用户目录
	userDir, err := CreateUserDirectory(username)
	if err != nil {
		return err
	}

	if mediaInfo.Type == "video" {
		// 下载单个视频
		filename := fmt.Sprintf("post_%d.mp4", postIndex)
		savePath := filepath.Join(userDir, filename)
		return DownloadMedia(mediaInfo.URLs[0], savePath)
	}

	if mediaInfo.Type == "carousel" {
		// 下载轮播内容（可能包含图片和视频）
		for i, url := range mediaInfo.URLs {
			var filename string
			var ext string

			// 根据媒体类型确定扩展名
			if i < len(mediaInfo.Types) && mediaInfo.Types[i] == "video" {
				ext = ".mp4"
			} else {
				ext = ".jpg"
			}

			filename = fmt.Sprintf("post_%d_%d%s", postIndex, i+1, ext)
			savePath := filepath.Join(userDir, filename)

			if err := DownloadMedia(url, savePath); err != nil {
				return err
			}
		}
		return nil
	}

	// 下载图片（单图或多图）
	for i, url := range mediaInfo.URLs {
		var filename string
		if len(mediaInfo.URLs) == 1 {
			// 单图
			filename = fmt.Sprintf("post_%d.jpg", postIndex)
		} else {
			// 多图
			filename = fmt.Sprintf("post_%d_%d.jpg", postIndex, i+1)
		}

		savePath := filepath.Join(userDir, filename)
		if err := DownloadMedia(url, savePath); err != nil {
			return err
		}
	}

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
