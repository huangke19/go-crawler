package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	thumbnailSize = 400
	thumbnailDir  = "cache/thumbnails"
	jpegQuality   = 85
)

// GetOrCreateThumbnail 获取或创建缩略图（带缓存检查）
func GetOrCreateThumbnail(shortcode string, files []string, postIndex int) (string, error) {
	// 生成带标签的缓存文件名
	var thumbnailPath string
	if postIndex > 0 {
		thumbnailPath = filepath.Join(thumbnailDir, fmt.Sprintf("%s_p%d.jpg", shortcode, postIndex))
	} else {
		thumbnailPath = filepath.Join(thumbnailDir, shortcode+".jpg")
	}

	// 检查缓存
	if _, err := os.Stat(thumbnailPath); err == nil {
		return thumbnailPath, nil
	}

	// 选择源文件（第一张图片或视频）
	sourceFile, ok := selectSourceFile(files)
	if !ok {
		return "", fmt.Errorf("no image or video file found")
	}

	// 检查是否为视频文件
	var img image.Image
	var err error
	ext := strings.ToLower(filepath.Ext(sourceFile))
	if ext == ".mp4" {
		// 从视频提取第一帧
		img, err = extractVideoFrame(sourceFile)
		if err != nil {
			return "", fmt.Errorf("failed to extract video frame: %w", err)
		}
	} else {
		// 打开图片文件
		img, err = imaging.Open(sourceFile)
		if err != nil {
			return "", fmt.Errorf("failed to open image: %w", err)
		}
	}

	// 调整大小并裁剪为正方形
	thumbnail := resizeAndCrop(img, thumbnailSize)

	// 在缩略图上绘制文字标签
	if postIndex > 0 {
		thumbnail = drawLabel(thumbnail, fmt.Sprintf("#%d", postIndex), shortcode)
	} else {
		thumbnail = drawLabel(thumbnail, shortcode, "")
	}

	// 保存缩略图
	err = imaging.Save(thumbnail, thumbnailPath, imaging.JPEGQuality(jpegQuality))
	if err != nil {
		return "", fmt.Errorf("failed to save thumbnail: %w", err)
	}

	return thumbnailPath, nil
}

// GenerateThumbnailBatch 批量生成缩略图（并发）
func GenerateThumbnailBatch(items []*FilesCache, maxConcurrent int) map[string]string {
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrent)

	for _, item := range items {
		wg.Add(1)
		go func(item *FilesCache) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			shortcode := extractShortcodeFromPath(item.Files)
			if shortcode == "" {
				return
			}

			thumbnailPath, err := GetOrCreateThumbnail(shortcode, item.Files, item.PostIndex)
			if err != nil {
				fmt.Printf("⚠️  生成缩略图失败 (%s): %v\n", shortcode, err)
				return
			}

			mu.Lock()
			results[shortcode] = thumbnailPath
			mu.Unlock()
		}(item)
	}

	wg.Wait()
	return results
}

// selectSourceFile 从文件列表中选择第一张图片或视频
func selectSourceFile(files []string) (string, bool) {
	var videoFile string

	// 优先选择图片: .jpg > .jpeg > .png
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			return file, true
		}
		// 记录第一个视频文件作为备选
		if videoFile == "" && ext == ".mp4" {
			videoFile = file
		}
	}

	// 如果没有图片，返回视频文件
	if videoFile != "" {
		return videoFile, true
	}

	return "", false
}

// resizeAndCrop 调整图片大小并裁剪为正方形
func resizeAndCrop(img image.Image, size int) image.Image {
	// 使用 Lanczos resampling（高质量）
	return imaging.Fill(img, size, size, imaging.Center, imaging.Lanczos)
}

// drawLabel 在图片上绘制文字标签
func drawLabel(img image.Image, line1, line2 string) image.Image {
	// 转换为 RGBA 以便绘制
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// 绘制半透明黑色背景（更大的区域，更高的不透明度）
	bgHeight := 70
	if line2 == "" {
		bgHeight = 40
	}
	bgRect := image.Rect(0, 0, bounds.Dx(), bgHeight)
	bgColor := color.RGBA{0, 0, 0, 220} // 更不透明的黑色
	draw.Draw(rgba, bgRect, &image.Uniform{bgColor}, image.Point{}, draw.Over)

	// 设置文字颜色（纯白色）
	textColor := color.RGBA{255, 255, 255, 255}

	// 绘制第一行文字（序号或 shortcode）- 使用更大的字体
	drawTextLarge(rgba, 12, 25, line1, textColor)

	// 绘制第二行文字（shortcode，如果有）
	if line2 != "" {
		drawTextLarge(rgba, 12, 55, line2, textColor)
	}

	return rgba
}

// drawTextLarge 在指定位置绘制更大更清晰的文字
func drawTextLarge(img *image.RGBA, x, y int, text string, col color.Color) {
	// 使用 basicfont.Face7x13 并放大 2 倍
	point := fixed.Point26_6{
		X: fixed.Int26_6(x * 64),
		Y: fixed.Int26_6(y * 64),
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}

	// 绘制文字阴影（增强对比度）
	shadowColor := color.RGBA{0, 0, 0, 255}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			d.Dot = fixed.Point26_6{
				X: fixed.Int26_6((x + dx) * 64),
				Y: fixed.Int26_6((y + dy) * 64),
			}
			d.Src = image.NewUniform(shadowColor)
			d.DrawString(text)
		}
	}

	// 绘制主文字
	d.Dot = point
	d.Src = image.NewUniform(col)
	d.DrawString(text)
}

// extractVideoFrame 从视频文件提取第一帧
func extractVideoFrame(videoPath string) (image.Image, error) {
	// 创建临时文件保存提取的帧
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("frame_%d.jpg", os.Getpid()))
	defer os.Remove(tmpFile)

	// 使用 ffmpeg 提取第一帧
	// -i: 输入文件
	// -vframes 1: 只提取 1 帧
	// -ss 0: 从第 0 秒开始
	// -q:v 2: 高质量 JPEG (1-31, 越小越好)
	cmd := fmt.Sprintf("ffmpeg -i %s -vframes 1 -ss 0 -q:v 2 %s -y 2>/dev/null",
		shellQuote(videoPath), shellQuote(tmpFile))

	// 执行命令
	if err := execCommand(cmd); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	// 打开提取的帧
	img, err := imaging.Open(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open extracted frame: %w", err)
	}

	return img, nil
}

// shellQuote 对文件路径进行 shell 转义
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// execCommand 执行 shell 命令
func execCommand(cmd string) error {
	// 使用 sh -c 执行命令
	process, err := os.StartProcess("/bin/sh",
		[]string{"/bin/sh", "-c", cmd},
		&os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		})
	if err != nil {
		return err
	}

	// 等待命令完成
	state, err := process.Wait()
	if err != nil {
		return err
	}

	if !state.Success() {
		return fmt.Errorf("command failed with exit code %d", state.ExitCode())
	}

	return nil
}
