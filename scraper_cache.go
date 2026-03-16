// ============================================================================
// scraper_cache.go - 帖子缓存刷新
// ============================================================================
//
// 职责：
//   - 刷新用户帖子列表缓存
//   - 检查是否有新帖
//   - 对比帖子顺序
//
// ============================================================================

package main

import (
	"context"
	"fmt"
	"time"
)

// RefreshPostsCache 检查用户帖子列表是否有更新，如有则刷新缓存。
// 参数 ctx 必须是已登录的浏览器上下文，minPosts 指定最少加载的帖子数量。
// 返回 (是否有更新, 帖子总数, error)。
func RefreshPostsCache(ctx context.Context, username string, minPosts int) (bool, int, error) {
	// 获取缓存基线（不过期读取）与有效性（过期检查）
	cachedPosts, hasValidCache, _ := GetPostsCacheSnapshot(username)

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

	// 仅当最新列表中出现"缓存中不存在的 shortcode"时，判定为有新帖。
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
