// ============================================================================
// scraper_extractor.go - 媒体 URL 提取
// ============================================================================
//
// 职责：
//   - 通过 GraphQL API 提取媒体 URL
//   - 解析 JSON 响应
//   - 提取图片/视频 URL
//   - 处理单图/单视频/轮播
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
)

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

	// 创建 HTTP POST 请求（绑定上层上下文，支持取消/超时传递）
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
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
	startTime := time.Now()
	resp, err := httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		LogAPICall("graphql", duration, false)
		RecordAPICall("graphql", duration.Seconds(), false)
		return nil, fmt.Errorf("GraphQL API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("  API 响应状态: HTTP %d\n", resp.StatusCode)

	// 检测 Cookie 失效
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		LogAPICall("graphql", duration, false)
		RecordAPICall("graphql", duration.Seconds(), false)
		return nil, fmt.Errorf("Cookie 已失效 (HTTP %d)，请重新登录", resp.StatusCode)
	}

	if resp.StatusCode != 200 {
		LogAPICall("graphql", duration, false)
		RecordAPICall("graphql", duration.Seconds(), false)
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return nil, fmt.Errorf("GraphQL API 返回错误 HTTP %d: %s", resp.StatusCode, bodyStr)
	}

	LogAPICall("graphql", duration, true)
	RecordAPICall("graphql", duration.Seconds(), true)

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
