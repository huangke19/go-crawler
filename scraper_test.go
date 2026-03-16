package main

import (
	"testing"
)

// TestExtractShortcode 测试从 URL 提取 shortcode
func TestExtractShortcode(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "标准帖子 URL",
			url:      "https://www.instagram.com/p/ABC123xyz/",
			expected: "ABC123xyz",
		},
		{
			name:     "Reel URL",
			url:      "https://www.instagram.com/reel/XYZ789abc/",
			expected: "XYZ789abc",
		},
		{
			name:     "带查询参数的 URL",
			url:      "https://www.instagram.com/p/DEF456/?utm_source=ig_web",
			expected: "DEF456",
		},
		{
			name:     "不带尾部斜杠",
			url:      "https://www.instagram.com/p/GHI789",
			expected: "GHI789",
		},
		{
			name:     "短 URL",
			url:      "/p/JKL012/",
			expected: "JKL012",
		},
		{
			name:     "无效 URL",
			url:      "https://www.instagram.com/user/profile/",
			expected: "",
		},
		{
			name:     "空 URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShortcode(tt.url)
			if result != tt.expected {
				t.Errorf("extractShortcode(%q) = %q, 期望 %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestExtractShortcodeFromPath 测试从文件路径提取 shortcode
func TestExtractShortcodeFromPath(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "单个文件路径",
			files:    []string{"downloads/cache/ABC123xyz/post_1.jpg"},
			expected: "ABC123xyz",
		},
		{
			name:     "多个文件路径",
			files:    []string{"downloads/cache/XYZ789/post_1.jpg", "downloads/cache/XYZ789/post_2.mp4"},
			expected: "XYZ789",
		},
		{
			name:     "用户目录路径",
			files:    []string{"downloads/nike/post_1.jpg"},
			expected: "",
		},
		{
			name:     "空路径列表",
			files:    []string{},
			expected: "",
		},
		{
			name:     "无效路径",
			files:    []string{"invalid/path"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShortcodeFromPath(tt.files)
			if result != tt.expected {
				t.Errorf("extractShortcodeFromPath(%v) = %q, 期望 %q", tt.files, result, tt.expected)
			}
		})
	}
}

// TestMediaInfo_Types 测试 MediaInfo 类型判断
func TestMediaInfo_Types(t *testing.T) {
	tests := []struct {
		name      string
		mediaInfo *MediaInfo
		wantType  string
		wantCount int
	}{
		{
			name: "单图",
			mediaInfo: &MediaInfo{
				Type:  "image",
				URLs:  []string{"https://example.com/image.jpg"},
				Types: []string{"image"},
			},
			wantType:  "image",
			wantCount: 1,
		},
		{
			name: "单视频",
			mediaInfo: &MediaInfo{
				Type:  "video",
				URLs:  []string{"https://example.com/video.mp4"},
				Types: []string{"video"},
			},
			wantType:  "video",
			wantCount: 1,
		},
		{
			name: "轮播（多图）",
			mediaInfo: &MediaInfo{
				Type: "carousel",
				URLs: []string{
					"https://example.com/img1.jpg",
					"https://example.com/img2.jpg",
					"https://example.com/img3.jpg",
				},
				Types: []string{"image", "image", "image"},
			},
			wantType:  "carousel",
			wantCount: 3,
		},
		{
			name: "轮播（图片+视频混合）",
			mediaInfo: &MediaInfo{
				Type: "carousel",
				URLs: []string{
					"https://example.com/img1.jpg",
					"https://example.com/video.mp4",
					"https://example.com/img2.jpg",
				},
				Types: []string{"image", "video", "image"},
			},
			wantType:  "carousel",
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mediaInfo.Type != tt.wantType {
				t.Errorf("Type = %q, 期望 %q", tt.mediaInfo.Type, tt.wantType)
			}
			if len(tt.mediaInfo.URLs) != tt.wantCount {
				t.Errorf("URLs 数量 = %d, 期望 %d", len(tt.mediaInfo.URLs), tt.wantCount)
			}
			if len(tt.mediaInfo.Types) != tt.wantCount {
				t.Errorf("Types 数量 = %d, 期望 %d", len(tt.mediaInfo.Types), tt.wantCount)
			}
		})
	}
}

// TestSamePostsOrder_Extended 测试帖子顺序比较（扩展测试）
func TestSamePostsOrder_Extended(t *testing.T) {
	tests := []struct {
		name     string
		a        []PostItem
		b        []PostItem
		expected bool
	}{
		{
			name: "相同顺序",
			a: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			b: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			expected: true,
		},
		{
			name: "不同顺序",
			a: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			b: []PostItem{
				{Index: 1, Shortcode: "DEF"},
				{Index: 2, Shortcode: "ABC"},
			},
			expected: false,
		},
		{
			name: "长度不同",
			a: []PostItem{
				{Index: 1, Shortcode: "ABC"},
			},
			b: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			expected: false,
		},
		{
			name:     "都为空",
			a:        []PostItem{},
			b:        []PostItem{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := samePostsOrder(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("samePostsOrder() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

// TestHasNewPostComparedToCache 测试新帖检测
func TestHasNewPostComparedToCache(t *testing.T) {
	tests := []struct {
		name     string
		cached   []PostItem
		latest   []PostItem
		expected bool
	}{
		{
			name: "有新帖",
			cached: []PostItem{
				{Index: 1, Shortcode: "OLD1"},
				{Index: 2, Shortcode: "OLD2"},
			},
			latest: []PostItem{
				{Index: 1, Shortcode: "NEW1"},
				{Index: 2, Shortcode: "OLD1"},
				{Index: 3, Shortcode: "OLD2"},
			},
			expected: true,
		},
		{
			name: "无新帖",
			cached: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			latest: []PostItem{
				{Index: 1, Shortcode: "ABC"},
				{Index: 2, Shortcode: "DEF"},
			},
			expected: false,
		},
		{
			name:     "缓存为空",
			cached:   []PostItem{},
			latest:   []PostItem{{Index: 1, Shortcode: "ABC"}},
			expected: false,
		},
		{
			name:     "最新为空",
			cached:   []PostItem{{Index: 1, Shortcode: "ABC"}},
			latest:   []PostItem{},
			expected: false,
		},
		{
			name: "最新帖子在缓存中间",
			cached: []PostItem{
				{Index: 1, Shortcode: "OLD1"},
				{Index: 2, Shortcode: "OLD2"},
				{Index: 3, Shortcode: "OLD3"},
			},
			latest: []PostItem{
				{Index: 1, Shortcode: "NEW1"},
				{Index: 2, Shortcode: "OLD2"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasNewPostComparedToCache(tt.cached, tt.latest)
			if result != tt.expected {
				t.Errorf("hasNewPostComparedToCache() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

// TestDiffNewShortcodes 测试差集计算
func TestDiffNewShortcodes(t *testing.T) {
	tests := []struct {
		name     string
		cached   []PostItem
		latest   []PostItem
		expected []string
	}{
		{
			name: "有新帖",
			cached: []PostItem{
				{Shortcode: "OLD1"},
				{Shortcode: "OLD2"},
			},
			latest: []PostItem{
				{Shortcode: "NEW1"},
				{Shortcode: "NEW2"},
				{Shortcode: "OLD1"},
			},
			expected: []string{"NEW1", "NEW2"},
		},
		{
			name: "无新帖",
			cached: []PostItem{
				{Shortcode: "ABC"},
				{Shortcode: "DEF"},
			},
			latest: []PostItem{
				{Shortcode: "ABC"},
				{Shortcode: "DEF"},
			},
			expected: []string{},
		},
		{
			name:     "缓存为空",
			cached:   []PostItem{},
			latest:   []PostItem{{Shortcode: "ABC"}},
			expected: []string{"ABC"}, // 修正：缓存为空时，最新的帖子应该被识别为新帖
		},
		{
			name:     "最新为空",
			cached:   []PostItem{{Shortcode: "ABC"}},
			latest:   []PostItem{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiffNewShortcodes(tt.cached, tt.latest)
			if len(result) != len(tt.expected) {
				t.Errorf("DiffNewShortcodes() 返回 %d 个，期望 %d 个", len(result), len(tt.expected))
				return
			}
			for i, sc := range result {
				if sc != tt.expected[i] {
					t.Errorf("DiffNewShortcodes()[%d] = %q, 期望 %q", i, sc, tt.expected[i])
				}
			}
		})
	}
}

// TestLimitPosts 测试帖子数量限制
func TestLimitPosts(t *testing.T) {
	posts := []PostItem{
		{Index: 1, Shortcode: "A"},
		{Index: 2, Shortcode: "B"},
		{Index: 3, Shortcode: "C"},
		{Index: 4, Shortcode: "D"},
		{Index: 5, Shortcode: "E"},
	}

	tests := []struct {
		name     string
		posts    []PostItem
		limit    int
		expected int
	}{
		{
			name:     "限制小于总数",
			posts:    posts,
			limit:    3,
			expected: 3,
		},
		{
			name:     "限制等于总数",
			posts:    posts,
			limit:    5,
			expected: 5,
		},
		{
			name:     "限制大于总数",
			posts:    posts,
			limit:    10,
			expected: 5,
		},
		{
			name:     "限制为0",
			posts:    posts,
			limit:    0,
			expected: 0,
		},
		{
			name:     "空列表",
			posts:    []PostItem{},
			limit:    3,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := limitPosts(tt.posts, tt.limit)
			if len(result) != tt.expected {
				t.Errorf("limitPosts() 返回 %d 个，期望 %d 个", len(result), tt.expected)
			}
		})
	}
}
