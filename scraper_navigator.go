// ============================================================================
// scraper_navigator.go - 页面导航与帖子定位
// ============================================================================
//
// 职责：
//   - 访问 Instagram 用户主页
//   - 滚动加载更多帖子
//   - 按时间线序号定位帖子
//   - 获取所有帖子链接
//
// ============================================================================

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// NavigateToUser 访问用户主页。
//
// 说明：
// - 主页访问用于"按时间线序号"定位帖子链接（第 1 条=最新）。
// - 实际媒体 URL（图片/视频）不从主页 HTML 抓取，而是后续通过 GraphQL 接口获取。
func NavigateToUser(ctx context.Context, username string) error {
	url := fmt.Sprintf(instagramBaseURL+"/%s/", username)
	fmt.Printf("正在访问 @%s 的主页...\n", username)

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(pageLoadWait), // 增加等待时间，确保初始内容加载
	)

	if err != nil {
		return err
	}

	fmt.Println("✓ 页面加载完成")
	return nil
}

// ScrollToLoadMore 滚动页面以加载更多帖子。
//
// Instagram 主页通常会按批次懒加载帖子列表（常见每次约 12 条）。
// 这里用"滚动次数 = targetIndex/12 + 1"的粗略策略，并通过前后链接数量差异判断是否加载成功。
func ScrollToLoadMore(ctx context.Context, targetIndex int) error {
	// Instagram 通常每次加载 12 个帖子
	// 如果目标索引大于 12，需要滚动
	if targetIndex <= 12 {
		return nil
	}

	scrollTimes := (targetIndex / 12) + 1
	fmt.Printf("需要滚动 %d 次以加载更多帖子...\n", scrollTimes)

	for i := 0; i < scrollTimes; i++ {
		fmt.Printf("  滚动 %d/%d\n", i+1, scrollTimes)

		// 获取滚动前的链接数量
		var beforeCount int
		chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('a[href*="/p/"], a[href*="/reel/"]').length`, &beforeCount))

		// 滚动到底部
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		); err != nil {
			return err
		}

		// 等待新内容加载（最多等待 5 秒）
		loaded := false
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)

			var afterCount int
			chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('a[href*="/p/"], a[href*="/reel/"]').length`, &afterCount))

			if afterCount > beforeCount {
				fmt.Printf("    ✓ 加载了 %d 个新帖子\n", afterCount-beforeCount)
				loaded = true
				break
			}
		}

		if !loaded {
			fmt.Printf("    ⚠️  未检测到新内容加载\n")
		}

		// 额外等待一下，确保内容稳定
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("✓ 滚动完成")
	return nil
}

// GetPostByIndex 获取第 N 个帖子的链接（从 1 开始）。
//
// 该函数只负责定位帖子 URL（/p/ 或 /reel/），不解析媒体文件地址。
// 媒体地址需要通过 `ExtractMediaURLs` 调用 GraphQL 获取。
func GetPostByIndex(ctx context.Context, index int) (string, error) {
	fmt.Printf("正在定位第 %d 条帖子...\n", index)

	// 滚动加载更多帖子
	if err := ScrollToLoadMore(ctx, index); err != nil {
		return "", fmt.Errorf("滚动加载失败: %w", err)
	}

	// 额外等待确保 DOM 稳定
	fmt.Println("等待页面稳定...")
	time.Sleep(1 * time.Second)

	fmt.Println("正在解析页面...")
	// 获取页面 HTML
	var htmlContent string
	if err := chromedp.Run(ctx,
		chromedp.OuterHTML("html", &htmlContent, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("获取页面 HTML 失败: %v", err)
	}

	// 使用 goquery 解析
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("解析 HTML 失败: %v", err)
	}

	// 查找所有帖子链接
	var postLinks []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && (strings.Contains(href, "/p/") || strings.Contains(href, "/reel/")) {
			// 转换为完整 URL
			if strings.HasPrefix(href, "/") {
				href = instagramBaseURL + href
			}
			postLinks = append(postLinks, href)
		}
	})

	// 去重
	uniqueLinks := make(map[string]bool)
	var finalLinks []string
	for _, link := range postLinks {
		if !uniqueLinks[link] {
			uniqueLinks[link] = true
			finalLinks = append(finalLinks, link)
		}
	}

	if len(finalLinks) == 0 {
		// 记录 HTML 长度以便调试
		fmt.Printf("⚠️  未找到任何帖子链接 (HTML 长度: %d 字节)\n", len(htmlContent))
		return "", fmt.Errorf("未找到任何帖子")
	}

	fmt.Printf("✓ 找到 %d 个帖子\n", len(finalLinks))

	if index < 1 {
		return "", fmt.Errorf("帖子索引必须大于 0（请求第 %d 条）", index)
	}

	if index > len(finalLinks) {
		return "", fmt.Errorf("帖子索引超出范围（共 %d 条帖子，请求第 %d 条）", len(finalLinks), index)
	}

	return finalLinks[index-1], nil
}

// GetAllPostLinks 获取用户主页的帖子链接列表（用于缓存/刷新）。
//
// 参数 minCount 表示"至少需要加载到多少条"；实际返回可能大于 minCount（取决于页面一次性加载数量）。
func GetAllPostLinks(ctx context.Context, minCount int) ([]string, error) {
	fmt.Printf("正在获取帖子列表（至少 %d 条）...\n", minCount)

	// 滚动加载更多帖子
	if err := ScrollToLoadMore(ctx, minCount); err != nil {
		return nil, fmt.Errorf("滚动加载失败: %w", err)
	}

	// 额外等待确保 DOM 稳定
	fmt.Println("等待页面稳定...")
	time.Sleep(1 * time.Second)

	fmt.Println("正在解析页面...")
	// 获取页面 HTML
	var htmlContent string
	if err := chromedp.Run(ctx,
		chromedp.OuterHTML("html", &htmlContent, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("获取页面 HTML 失败: %v", err)
	}

	// 使用 goquery 解析
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}

	// 查找所有帖子链接
	var postLinks []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && (strings.Contains(href, "/p/") || strings.Contains(href, "/reel/")) {
			// 转换为完整 URL
			if strings.HasPrefix(href, "/") {
				href = instagramBaseURL + href
			}
			postLinks = append(postLinks, href)
		}
	})

	// 去重
	uniqueLinks := make(map[string]bool)
	var finalLinks []string
	for _, link := range postLinks {
		if !uniqueLinks[link] {
			uniqueLinks[link] = true
			finalLinks = append(finalLinks, link)
		}
	}

	if len(finalLinks) == 0 {
		fmt.Printf("⚠️  未找到任何帖子链接 (HTML 长度: %d 字节)\n", len(htmlContent))

		// 保存 HTML 到临时目录以便调试，防止在工作目录堆积敏感数据
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("debug_%s_*.html", time.Now().Format("20060102_150405")))
		if err == nil {
			_ = os.WriteFile(tmpFile.Name(), []byte(htmlContent), 0600)
			fmt.Printf("⚠️  已保存 HTML 到 %s 以便调试\n", tmpFile.Name())
		}

		// 检查是否是私密账户
		if strings.Contains(htmlContent, "This Account is Private") ||
			strings.Contains(htmlContent, "This account is private") ||
			strings.Contains(htmlContent, "该帐户为私密帐户") {
			return nil, fmt.Errorf("该账户为私密账户，需要先关注才能查看帖子")
		}

		return nil, fmt.Errorf("未找到任何帖子")
	}

	fmt.Printf("✓ 找到 %d 个帖子\n", len(finalLinks))
	return finalLinks, nil
}
