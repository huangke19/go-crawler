// ============================================================================
// scraper.go - 页面爬取与媒体 URL 提取
// ============================================================================
//
// 职责：
//   - 访问 Instagram 用户主页
//   - 滚动加载更多帖子（每次加载约 12 条）
//   - 按时间线序号定位帖子（第 1 条 = 最新）
//   - 通过 GraphQL API 提取媒体 URL（图片/视频/轮播）
//
// 核心概念：
//   - 帖子定位：通过 DOM 解析获取帖子链接（/p/ 或 /reel/）
//   - 媒体提取：通过 GraphQL API 获取原始媒体 URL（不依赖页面 HTML 渲染）
//   - 三种媒体类型：单图 / 单视频 / 轮播（多图/视频混合）
//
// 关键函数：
//   - NavigateToUser()：访问用户主页
//   - ScrollToLoadMore()：滚动加载更多帖子
//   - GetPostByIndex()：获取第 N 个帖子的链接
//   - GetAllPostLinks()：获取用户主页的所有帖子链接
//   - ExtractMediaURLs()：通过 GraphQL 提取媒体 URL
//   - extractShortcode()：从 URL 提取 shortcode
//   - extractMediaFromJSON()：从 GraphQL 响应解析媒体信息
//   - extractImageURL()：从媒体数据提取图片 URL
//
// GraphQL API 说明：
//   - 端点：https://www.instagram.com/graphql/query
//   - doc_id：8845758582119845（可能随 Instagram 更新而变化）
//   - 必需请求头：X-CSRFToken（从 Cookie 中提取）
//   - 响应字段：data.xdt_shortcode_media（新版）或 data.shortcode_media（旧版）
//
// ============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		return nil, fmt.Errorf("未找到任何帖子")
	}

	fmt.Printf("✓ 找到 %d 个帖子\n", len(finalLinks))
	return finalLinks, nil
}

// MediaInfo 媒体信息
type MediaInfo struct {
	Type  string   // "image", "video" 或 "carousel"
	URLs  []string // 媒体 URL 列表
	Types []string // 每个 URL 对应的类型（"image" 或 "video"）
}

// ExtractMediaURLs 提取帖子中的所有媒体 URL。
//
// 这里走的是 Instagram GraphQL（而不是解析页面 HTML），原因：
// - HTML 结构变化频繁；GraphQL 响应更稳定且能直接拿到原始媒体 URL；
// - 轮播/视频混合内容在 GraphQL 中更易解析。
//
// 关键约束：
// - 必须携带有效登录 Cookie，尤其是 `csrftoken`；
// - 请求头必须包含 `X-CSRFToken`，否则常见会被 403/401 拒绝；
// - 响应字段可能是 `data.xdt_shortcode_media`（新版）或 `data.shortcode_media`（旧版），需兼容。
func ExtractMediaURLs(ctx context.Context, postURL string) (*MediaInfo, error) {
	fmt.Println("正在提取媒体内容...")

	// 从 URL 提取 shortcode
	shortcode := extractShortcode(postURL)
	if shortcode == "" {
		return nil, fmt.Errorf("无法从 URL 提取 shortcode: %s", postURL)
	}
	fmt.Printf("  shortcode: %s\n", shortcode)

	// 加载 session cookies
	cookies, err := LoadSession()
	if err != nil {
		return nil, fmt.Errorf("加载 session 失败: %v", err)
	}
	fmt.Printf("  已加载 %d 个 cookies\n", len(cookies))

	// 构造 GraphQL POST 请求（参考 instaloader 的调用方式）。
	// doc_id 可能随 Instagram 更新而变化；当出现"接口返回结构变化/无数据"时，这里通常是首要排查点。
	docID := graphQLDocID
	variables := fmt.Sprintf(`{"shortcode":"%s"}`, shortcode)

	// 构造表单数据（使用 url.Values 正确编码）
	formData := url.Values{}
	formData.Set("variables", variables)
	formData.Set("doc_id", docID)
	formData.Set("server_timestamps", "true")

	apiURL := instagramGraphQL

	// 创建 HTTP POST 请求
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头（模仿 instaloader）
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-IG-App-ID", "936619743392459")
	req.Header.Set("Referer", postURL)
	req.Header.Set("Origin", instagramBaseURL)
	req.Header.Set("Accept", "*/*")

	// 添加 cookies 并提取 csrftoken
	var cookieStrs []string
	var csrfToken string
	for _, c := range cookies {
		if c.Name != "" && c.Value != "" {
			cookieStrs = append(cookieStrs, fmt.Sprintf("%s=%s", c.Name, c.Value))
			// 提取 csrftoken
			if c.Name == "csrftoken" {
				csrfToken = c.Value
			}
		}
	}
	if len(cookieStrs) > 0 {
		req.Header.Set("Cookie", strings.Join(cookieStrs, "; "))
	}

	// 添加 X-CSRFToken 请求头（关键）。
	// 没有该头部时，GraphQL 常会直接返回 403/401，即使 Cookie 本身存在。
	if csrfToken != "" {
		req.Header.Set("X-CSRFToken", csrfToken)
		fmt.Println("  ✓ 已设置 CSRF Token")
	} else {
		return nil, fmt.Errorf("未找到 csrftoken，请重新登录")
	}

	// 发送请求（使用全局客户端）
	fmt.Println("  正在调用 Instagram GraphQL API...")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("  API 响应状态: HTTP %d\n", resp.StatusCode)

	// 检测 Cookie 失效
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("Cookie 已失效 (HTTP %d)，请重新登录", resp.StatusCode)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return nil, fmt.Errorf("GraphQL API 返回错误 HTTP %d: %s", resp.StatusCode, bodyStr)
	}

	// 解析 JSON 响应（限制最大 10MB，防止内存耗尽）
	limitedReader := io.LimitReader(resp.Body, maxJSONResponseSize)
	var result map[string]interface{}
	if err := json.NewDecoder(limitedReader).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}

	// 提取 data.xdt_shortcode_media（注意是 xdt_shortcode_media）
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("响应中没有 data 字段")
	}

	// 尝试 xdt_shortcode_media（新版）或 shortcode_media（旧版）。
	// Instagram 会不定期调整字段名，保留兼容分支能显著降低线上中断概率。
	var shortcodeMedia map[string]interface{}
	if media, ok := data["xdt_shortcode_media"].(map[string]interface{}); ok {
		shortcodeMedia = media
		fmt.Println("  ✓ 使用 xdt_shortcode_media 字段")
	} else if media, ok := data["shortcode_media"].(map[string]interface{}); ok {
		// 兼容旧版
		shortcodeMedia = media
		fmt.Println("  ✓ 使用 shortcode_media 字段（旧版）")
	} else {
		return nil, fmt.Errorf("响应中没有 xdt_shortcode_media 或 shortcode_media 字段")
	}

	// 提取媒体 URL
	return extractMediaFromJSON(shortcodeMedia)
}

// parseMediaData 从提取的数据中解析媒体信息
// extractMediaFromJSON 从 GraphQL 返回的媒体 JSON 中提取媒体 URL。
//
// 分支说明：
// - 单视频：`is_video=true` 且存在 `video_url`
// - 轮播：`edge_sidecar_to_children.edges[].node`，每个 node 可能是图片或视频
// - 单图：优先 `display_url`，其次从 `display_resources` 取最大分辨率
func extractMediaFromJSON(data map[string]interface{}) (*MediaInfo, error) {
	mediaInfo := &MediaInfo{
		Type:  "image",
		URLs:  []string{},
		Types: []string{},
	}

	// 检查是否是视频
	if isVideo, ok := data["is_video"].(bool); ok && isVideo {
		mediaInfo.Type = "video"
		// 提取视频 URL
		if videoURL, ok := data["video_url"].(string); ok {
			mediaInfo.URLs = append(mediaInfo.URLs, videoURL)
			mediaInfo.Types = append(mediaInfo.Types, "video")
			return mediaInfo, nil
		}
	}

	// 检查是否是多图轮播（GraphQL 格式）
	if edgeSidecar, ok := data["edge_sidecar_to_children"].(map[string]interface{}); ok {
		if edges, ok := edgeSidecar["edges"].([]interface{}); ok && len(edges) > 0 {
			fmt.Printf("检测到多图轮播，共 %d 项\n", len(edges))
			mediaInfo.Type = "carousel"
			for _, edge := range edges {
				edgeMap, ok := edge.(map[string]interface{})
				if !ok {
					continue
				}
				if node, ok := edgeMap["node"].(map[string]interface{}); ok {
					// 检查是否是视频
					if isVideo, ok := node["is_video"].(bool); ok && isVideo {
						if videoURL, ok := node["video_url"].(string); ok {
							mediaInfo.URLs = append(mediaInfo.URLs, videoURL)
							mediaInfo.Types = append(mediaInfo.Types, "video")
							continue
						}
					}
					// 提取图片
					url := extractImageURL(node)
					if url != "" {
						mediaInfo.URLs = append(mediaInfo.URLs, url)
						mediaInfo.Types = append(mediaInfo.Types, "image")
					}
				}
			}
			return mediaInfo, nil
		}
	}

	// 单图
	url := extractImageURL(data)
	if url != "" {
		mediaInfo.URLs = append(mediaInfo.URLs, url)
		mediaInfo.Types = append(mediaInfo.Types, "image")
		return mediaInfo, nil
	}

	return nil, fmt.Errorf("未找到任何媒体 URL")
}

// RefreshPostsCache 检查用户帖子列表是否有更新，如有则刷新缓存。
// 参数 ctx 必须是已登录的浏览器上下文，minPosts 指定最少加载的帖子数量。
// 返回 (是否有更新, 帖子总数, error)。
func RefreshPostsCache(ctx context.Context, username string, minPosts int) (bool, int, error) {
	// 获取缓存基线（不过期读取）与有效性（过期检查）
	var cachedPosts []PostItem
	if postsCache, ok := GetPostsFromCacheRaw(username); ok && postsCache != nil {
		cachedPosts = postsCache.Posts
	}
	_, hasValidCache := GetPostsFromCache(username)

	// 访问用户主页
	if err := NavigateToUser(ctx, username); err != nil {
		return false, 0, fmt.Errorf("访问用户主页失败: %w", err)
	}

	// 获取帖子链接
	postLinks, err := GetAllPostLinks(ctx, minPosts)
	if err != nil {
		return false, 0, fmt.Errorf("获取帖子列表失败: %w", err)
	}

	if len(postLinks) == 0 {
		return false, 0, fmt.Errorf("未找到任何帖子")
	}

	// 构建新缓存
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

	// 仅当最新列表中出现“缓存中不存在的 shortcode”时，判定为有新帖。
	// 注意：缓存过期仅表示需要刷新缓存，不代表有新帖。
	needRefresh := hasNewPostComparedToCache(cachedPosts, posts)

	// 缓存失效或内容变化时都刷新落盘，避免后续重复抓取。
	if !hasValidCache || !samePostsOrder(cachedPosts, posts) {
		SavePostsToCache(username, &PostsCache{
			Posts:     posts,
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(postsCacheExpiry),
		})
	}

	return needRefresh, len(posts), nil
}

func hasNewPostComparedToCache(cached, latest []PostItem) bool {
	if len(cached) == 0 || len(latest) == 0 {
		return false
	}

	cachedSet := make(map[string]struct{}, len(cached))
	for _, item := range cached {
		if item.Shortcode != "" {
			cachedSet[item.Shortcode] = struct{}{}
		}
	}

	compareCount := len(cached)
	if len(latest) < compareCount {
		compareCount = len(latest)
	}

	for i := 0; i < compareCount; i++ {
		sc := latest[i].Shortcode
		if sc == "" {
			continue
		}
		if _, ok := cachedSet[sc]; !ok {
			return true
		}
	}

	return false
}

func samePostsOrder(a, b []PostItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Shortcode != b[i].Shortcode {
			return false
		}
	}
	return true
}

// extractImageURL 从媒体数据中提取图片 URL（优先选择较高质量）。
// GraphQL 常见字段：
// - display_url：通常为较高质量的最终展示图
// - display_resources：候选列表（一般最后一个最大）
func extractImageURL(item map[string]interface{}) string {
	// GraphQL: display_url
	if displayURL, ok := item["display_url"].(string); ok {
		return displayURL
	}

	// 备用: display_resources（选择最大的）
	if displayResources, ok := item["display_resources"].([]interface{}); ok && len(displayResources) > 0 {
		// 取最后一个（通常是最大的）
		lastResource := displayResources[len(displayResources)-1].(map[string]interface{})
		if src, ok := lastResource["src"].(string); ok {
			return src
		}
	}

	return ""
}
