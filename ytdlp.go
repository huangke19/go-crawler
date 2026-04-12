// ============================================================================
// ytdlp.go - 外部平台媒体下载（YouTube / X / Twitter）
// ============================================================================
//
// 职责：
//   - 封装 yt-dlp subprocess 调用
//   - 支持 YouTube、X(Twitter) 等平台的视频/图片下载
//   - URL 类型识别（youtube / x / twitter / 其他）
//   - 下载结果文件路径收集
//
// 设计要点：
//   - yt-dlp 是外部依赖，通过 exec.Command 调用
//   - 下载文件保存到 downloads/external/<platform>/<id>/
//   - 超时控制通过 context 实现
//   - YouTube 默认限制 1080p MP4 格式
//   - Telegram 文件大小限制 50MB，超大文件会提示
//
// ============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExternalDownloadRequest 外部链接下载请求
type ExternalDownloadRequest struct {
	URL string `json:"url"`
}

// ExternalDownloadResponse 外部链接下载响应
type ExternalDownloadResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Title     string   `json:"title,omitempty"`
}

// PlatformType 平台类型
type PlatformType string

const (
	PlatformYouTube PlatformType = "youtube"
	PlatformX       PlatformType = "x"
	PlatformUnknown PlatformType = "unknown"
)

// YouTube URL 匹配
var youtubeURLPattern = regexp.MustCompile(
	`(?i)(?:https?://)?(?:www\.)?(?:youtube\.com/(?:watch\?v=|shorts/|embed/|v/)|youtu\.be/)[\w-]+`,
)

// X/Twitter URL 匹配
var xURLPattern = regexp.MustCompile(
	`(?i)(?:https?://)?(?:www\.)?(?:twitter\.com|x\.com)/\w+/status/\d+`,
)

// DetectPlatform 根据 URL 识别平台类型
func DetectPlatform(url string) PlatformType {
	if youtubeURLPattern.MatchString(url) {
		return PlatformYouTube
	}
	if xURLPattern.MatchString(url) {
		return PlatformX
	}
	return PlatformUnknown
}

// IsExternalURL 判断 URL 是否为支持的外部平台链接
func IsExternalURL(url string) bool {
	return DetectPlatform(url) != PlatformUnknown
}

// PlatformLabel 返回平台的中文标签
func PlatformLabel(p PlatformType) string {
	switch p {
	case PlatformYouTube:
		return "YouTube"
	case PlatformX:
		return "X (Twitter)"
	default:
		return "未知平台"
	}
}

// PlatformEmoji 返回平台对应的 emoji
func PlatformEmoji(p PlatformType) string {
	switch p {
	case PlatformYouTube:
		return "🎬"
	case PlatformX:
		return "🐦"
	default:
		return "🔗"
	}
}

// findYtDlp 查找 yt-dlp 可执行文件路径
func findYtDlp() (string, error) {
	// 优先查找 PATH 中的 yt-dlp
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path, nil
	}
	// macOS homebrew 常见路径
	commonPaths := []string{
		"/opt/homebrew/bin/yt-dlp",
		"/usr/local/bin/yt-dlp",
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 yt-dlp，请安装: brew install yt-dlp")
}

// extractVideoID 从 YouTube URL 提取视频 ID
func extractVideoID(url string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:v=|youtu\.be/|shorts/|embed/|v/)([\w-]{11})`),
	}
	for _, p := range patterns {
		if m := p.FindStringSubmatch(url); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// extractTweetID 从 X/Twitter URL 提取推文 ID
func extractTweetID(url string) string {
	p := regexp.MustCompile(`status/(\d+)`)
	if m := p.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	return ""
}

// getDownloadDir 获取下载目录路径
func getExternalDownloadDir(platform PlatformType, id string) string {
	return filepath.Join("downloads", "external", string(platform), id)
}

// DownloadExternalURL 使用 yt-dlp 下载外部平台媒体
func DownloadExternalURL(rawURL string) (*ExternalDownloadResponse, error) {
	ytdlpPath, err := findYtDlp()
	if err != nil {
		return nil, err
	}

	platform := DetectPlatform(rawURL)
	if platform == PlatformUnknown {
		return nil, fmt.Errorf("不支持的 URL: %s", rawURL)
	}

	// 确定下载 ID 和目录
	var id string
	switch platform {
	case PlatformYouTube:
		id = extractVideoID(rawURL)
	case PlatformX:
		id = extractTweetID(rawURL)
	}
	if id == "" {
		id = fmt.Sprintf("unknown_%d", time.Now().UnixMilli())
	}

	downloadDir := getExternalDownloadDir(platform, id)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建下载目录失败: %w", err)
	}

	// 构建 yt-dlp 参数
	args := buildYtDlpArgs(platform, rawURL, downloadDir)

	log.Printf("执行 yt-dlp: %s %s", ytdlpPath, strings.Join(args, " "))

	// 带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), ytdlpTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	cmd.Dir = downloadDir
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("下载超时（超过 %v）", ytdlpTimeout)
	}

	if err != nil {
		log.Printf("yt-dlp 输出:\n%s", string(output))

		// X 平台：yt-dlp 无法下载纯图片推文，回退到 API 抓取图片
		outputStr := string(output)
		if platform == PlatformX && strings.Contains(outputStr, "No video could be found") {
			log.Printf("X 推文无视频，尝试通过 API 下载图片...")
			return downloadXTweetImages(rawURL, id, downloadDir)
		}

		return nil, fmt.Errorf("yt-dlp 执行失败: %w\n输出: %s", err, truncateOutput(outputStr, 500))
	}

	// 收集下载的文件
	files, err := collectDownloadedFiles(downloadDir)
	if err != nil {
		return nil, fmt.Errorf("收集文件失败: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("yt-dlp 未下载到任何文件")
	}

	// 获取标题（可选）
	title := extractTitleFromOutput(string(output))

	log.Printf("下载完成: %s, %d 个文件", PlatformLabel(platform), len(files))

	return &ExternalDownloadResponse{
		Success:   true,
		Message:   fmt.Sprintf("下载完成，共 %d 个文件", len(files)),
		FilePaths: files,
		Platform:  string(platform),
		Title:     title,
	}, nil
}

// buildYtDlpArgs 根据平台构建 yt-dlp 参数
func buildYtDlpArgs(platform PlatformType, url, downloadDir string) []string {
	outputTemplate := filepath.Join(downloadDir, "%(title).80s_%(id)s.%(ext)s")

	switch platform {
	case PlatformYouTube:
		return []string{
			"-f", "bestvideo[ext=mp4][height<=1080]+bestaudio[ext=m4a]/best[ext=mp4]/best",
			"--merge-output-format", "mp4",
			"-o", outputTemplate,
			"--no-playlist",
			"--no-overwrites",
			"--write-thumbnail",
			"--convert-thumbnails", "jpg",
			url,
		}
	case PlatformX:
		return []string{
			"-o", outputTemplate,
			"--no-overwrites",
			url,
		}
	default:
		return []string{
			"-o", outputTemplate,
			"--no-overwrites",
			url,
		}
	}
}

// collectDownloadedFiles 扫描下载目录，收集媒体文件路径
func collectDownloadedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	mediaExts := map[string]bool{
		".mp4": true, ".webm": true, ".mkv": true,
		".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
		".mp3": true, ".m4a": true, ".ogg": true,
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if mediaExts[ext] {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

// extractTitleFromOutput 从 yt-dlp 输出中提取标题
func extractTitleFromOutput(output string) string {
	// yt-dlp 输出格式: [info] title: ...
	// 或 [download] Destination: ...
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "[info]") && strings.Contains(line, "title") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// truncateOutput 截断过长的输出
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ── X/Twitter 图片推文下载（yt-dlp 回退） ──────────────────────────

// fxTweetResponse 是 fxtwitter.com API 的响应结构
type fxTweetResponse struct {
	Tweet struct {
		Text   string `json:"text"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
		Media struct {
			Photos []struct {
				URL string `json:"url"`
			} `json:"photos"`
			Videos []struct {
				URL string `json:"url"`
			} `json:"videos"`
		} `json:"media"`
	} `json:"tweet"`
}

// downloadXTweetImages 通过 fxtwitter API 获取推文图片并下载
func downloadXTweetImages(rawURL, tweetID, downloadDir string) (*ExternalDownloadResponse, error) {
	// 从 URL 中提取用户名
	username := extractXUsername(rawURL)
	if username == "" {
		username = "tweet"
	}

	// 调用 fxtwitter API
	apiURL := fmt.Sprintf("https://api.fxtwitter.com/%s/status/%s", username, tweetID)
	log.Printf("请求 fxtwitter API: %s", apiURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "go-crawler/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 fxtwitter API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fxtwitter API 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var fxResp fxTweetResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return nil, fmt.Errorf("解析 fxtwitter 响应失败: %w", err)
	}

	// 收集所有媒体 URL
	var mediaURLs []string
	for _, photo := range fxResp.Tweet.Media.Photos {
		if photo.URL != "" {
			mediaURLs = append(mediaURLs, photo.URL)
		}
	}
	for _, video := range fxResp.Tweet.Media.Videos {
		if video.URL != "" {
			mediaURLs = append(mediaURLs, video.URL)
		}
	}

	if len(mediaURLs) == 0 {
		return nil, fmt.Errorf("推文中未找到任何媒体内容")
	}

	log.Printf("从推文中提取到 %d 个媒体文件", len(mediaURLs))

	// 下载所有媒体文件
	var filePaths []string
	for i, mediaURL := range mediaURLs {
		ext := guessExtFromURL(mediaURL)
		var filename string
		if len(mediaURLs) == 1 {
			filename = fmt.Sprintf("%s%s", tweetID, ext)
		} else {
			filename = fmt.Sprintf("%s_%d%s", tweetID, i+1, ext)
		}
		savePath := filepath.Join(downloadDir, filename)

		log.Printf("下载 [%d/%d]: %s", i+1, len(mediaURLs), mediaURL)
		if err := DownloadMedia(mediaURL, savePath, 2); err != nil {
			log.Printf("下载失败 %s: %v", mediaURL, err)
			continue
		}
		filePaths = append(filePaths, savePath)
	}

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("所有媒体文件下载失败")
	}

	title := fxResp.Tweet.Text
	if len(title) > 100 {
		title = title[:100] + "..."
	}

	return &ExternalDownloadResponse{
		Success:   true,
		Message:   fmt.Sprintf("下载完成，共 %d 个文件", len(filePaths)),
		FilePaths: filePaths,
		Platform:  string(PlatformX),
		Title:     title,
	}, nil
}

// extractXUsername 从 X/Twitter URL 中提取用户名
func extractXUsername(url string) string {
	p := regexp.MustCompile(`(?:twitter\.com|x\.com)/(\w+)/status/`)
	if m := p.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	return ""
}

// guessExtFromURL 从 URL 猜测文件扩展名
func guessExtFromURL(url string) string {
	// 去掉查询参数
	clean := url
	if idx := strings.Index(clean, "?"); idx != -1 {
		clean = clean[:idx]
	}
	ext := strings.ToLower(filepath.Ext(clean))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".mp4", ".webm":
		return ext
	default:
		return ".jpg"
	}
}

// GetYtDlpVersion 获取 yt-dlp 版本（用于 /status 展示）
func GetYtDlpVersion() string {
	ytdlpPath, err := findYtDlp()
	if err != nil {
		return "未安装"
	}
	out, err := exec.Command(ytdlpPath, "--version").Output()
	if err != nil {
		return "未知"
	}
	return strings.TrimSpace(string(out))
}
