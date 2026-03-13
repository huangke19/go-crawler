package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupCacheTest 为每个测试隔离文件系统并重置全局缓存状态
func setupCacheTest(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	os.MkdirAll("cache", 0755)
	resetCacheMaps()
	t.Cleanup(resetCacheMaps)
}

// resetCacheMaps 清空所有内存缓存，强制下次访问从磁盘重新加载
func resetCacheMaps() {
	mediaCacheMu.Lock()
	mediaCacheMap = nil
	mediaCacheMu.Unlock()

	postsCacheMu.Lock()
	postsCacheMap = nil
	postsCacheMu.Unlock()

	filesCacheMu.Lock()
	filesCacheMap = nil
	filesCacheMu.Unlock()

	historyCacheMu.Lock()
	historyCache = nil
	historyCacheMu.Unlock()
}

// ── 媒体缓存 ──────────────────────────────────────────────────────────────────

// TestMediaCache_SaveAndGet 保存后能取回相同数据
func TestMediaCache_SaveAndGet(t *testing.T) {
	setupCacheTest(t)

	media := &MediaCache{
		Type:  "carousel",
		URLs:  []string{"https://example.com/1.jpg", "https://example.com/2.mp4"},
		Types: []string{"image", "video"},
	}
	if err := SaveMediaToCache("ABC123", media); err != nil {
		t.Fatalf("SaveMediaToCache error: %v", err)
	}

	got, ok := GetMediaFromCache("ABC123")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got.Type != media.Type {
		t.Errorf("Type = %q, want %q", got.Type, media.Type)
	}
	if len(got.URLs) != 2 || got.URLs[1] != media.URLs[1] {
		t.Errorf("URLs = %v, want %v", got.URLs, media.URLs)
	}
}

// TestMediaCache_Miss 未保存的 shortcode 应返回 miss
func TestMediaCache_Miss(t *testing.T) {
	setupCacheTest(t)

	_, ok := GetMediaFromCache("nonexistent")
	if ok {
		t.Fatal("expected cache miss, got hit")
	}
}

// TestMediaCache_Persistence 保存后重置内存，从磁盘重新加载仍能命中
func TestMediaCache_Persistence(t *testing.T) {
	setupCacheTest(t)

	if err := SaveMediaToCache("PERSIST1", &MediaCache{Type: "image", URLs: []string{"u1"}}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// 清空内存缓存，模拟进程重启
	resetCacheMaps()

	got, ok := GetMediaFromCache("PERSIST1")
	if !ok {
		t.Fatal("expected hit after reload from disk, got miss")
	}
	if got.Type != "image" {
		t.Errorf("Type = %q after reload", got.Type)
	}
}

// ── 帖子缓存 ──────────────────────────────────────────────────────────────────

// TestPostsCache_NotExpired 未过期的缓存应命中
func TestPostsCache_NotExpired(t *testing.T) {
	setupCacheTest(t)

	posts := &PostsCache{
		Posts: []PostItem{
			{Index: 1, Shortcode: "sc1"},
			{Index: 2, Shortcode: "sc2"},
		},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour), // 未来过期
	}
	if err := SavePostsToCache("nike", posts); err != nil {
		t.Fatalf("save error: %v", err)
	}

	got, ok := GetPostsFromCache("nike")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if len(got.Posts) != 2 {
		t.Errorf("len(Posts) = %d, want 2", len(got.Posts))
	}
	if got.Posts[1].Shortcode != "sc2" {
		t.Errorf("Posts[1].Shortcode = %q, want %q", got.Posts[1].Shortcode, "sc2")
	}
}

// TestPostsCache_Expired 已过期的缓存应返回 miss（这是过期逻辑的核心保护）
func TestPostsCache_Expired(t *testing.T) {
	setupCacheTest(t)

	posts := &PostsCache{
		Posts:     []PostItem{{Index: 1, Shortcode: "old_sc"}},
		UpdatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Second), // 已过期
	}
	if err := SavePostsToCache("nike", posts); err != nil {
		t.Fatalf("save error: %v", err)
	}

	_, ok := GetPostsFromCache("nike")
	if ok {
		t.Fatal("expected cache miss for expired entry, got hit")
	}
}

// TestPostsCache_Miss 不存在的用户应返回 miss
func TestPostsCache_Miss(t *testing.T) {
	setupCacheTest(t)

	_, ok := GetPostsFromCache("unknown_user")
	if ok {
		t.Fatal("expected miss for unknown user")
	}
}

// ── 文件缓存 ──────────────────────────────────────────────────────────────────

// TestFilesCache_AllExist 所有文件存在时应命中
func TestFilesCache_AllExist(t *testing.T) {
	setupCacheTest(t)

	// 创建真实的临时文件
	dir := t.TempDir()
	f1 := filepath.Join(dir, "post_1.jpg")
	f2 := filepath.Join(dir, "post_1_2.mp4")
	os.WriteFile(f1, []byte("img"), 0644)
	os.WriteFile(f2, []byte("vid"), 0644)

	if err := SaveFilesToCache("XYZ789", &FilesCache{
		Files:        []string{f1, f2},
		Username:     "testuser",
		PostIndex:    1,
		DownloadedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	got, ok := GetFilesFromCache("XYZ789")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if len(got.Files) != 2 {
		t.Errorf("len(Files) = %d, want 2", len(got.Files))
	}
}

// TestFilesCache_AnyMissing 任意文件被删除后，缓存应返回 miss（最关键的保护点）
func TestFilesCache_AnyMissing(t *testing.T) {
	setupCacheTest(t)

	dir := t.TempDir()
	f1 := filepath.Join(dir, "post_2.jpg")
	f2 := filepath.Join(dir, "post_2_2.jpg") // 这个文件不创建

	os.WriteFile(f1, []byte("img"), 0644)
	// f2 故意不创建

	if err := SaveFilesToCache("MISSING1", &FilesCache{
		Files:        []string{f1, f2},
		DownloadedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	_, ok := GetFilesFromCache("MISSING1")
	if ok {
		t.Fatal("expected cache miss when a file is missing, got hit")
	}
}

// TestFilesCache_DeletedAfterCache 文件先存在后被删除，缓存应失效
func TestFilesCache_DeletedAfterCache(t *testing.T) {
	setupCacheTest(t)

	dir := t.TempDir()
	f := filepath.Join(dir, "post_3.jpg")
	os.WriteFile(f, []byte("img"), 0644)

	SaveFilesToCache("DELETE1", &FilesCache{
		Files:        []string{f},
		DownloadedAt: time.Now(),
	})

	// 验证文件存在时命中
	_, ok := GetFilesFromCache("DELETE1")
	if !ok {
		t.Fatal("expected hit before deletion")
	}

	// 删除文件后应 miss
	os.Remove(f)

	_, ok = GetFilesFromCache("DELETE1")
	if ok {
		t.Fatal("expected miss after file deletion, got hit")
	}
}

// TestFilesCache_Persistence 重置内存后从磁盘恢复（文件存在时仍命中）
func TestFilesCache_Persistence(t *testing.T) {
	setupCacheTest(t)

	dir := t.TempDir()
	f := filepath.Join(dir, "post_4.jpg")
	os.WriteFile(f, []byte("img"), 0644)

	SaveFilesToCache("PERSIST2", &FilesCache{
		Files:        []string{f},
		Username:     "persistuser",
		DownloadedAt: time.Now(),
	})

	resetCacheMaps()

	got, ok := GetFilesFromCache("PERSIST2")
	if !ok {
		t.Fatal("expected hit after reload from disk")
	}
	if got.Username != "persistuser" {
		t.Errorf("Username = %q, want %q", got.Username, "persistuser")
	}
}

// ── 下载历史 ──────────────────────────────────────────────────────────────────

// TestGetDownloadHistory_TimeOrder 历史记录应按下载时间倒序排列
func TestGetDownloadHistory_TimeOrder(t *testing.T) {
	setupCacheTest(t)

	now := time.Now()
	// 故意乱序保存
	SaveFilesToCache("mid", &FilesCache{Username: "mid", DownloadedAt: now.Add(-1 * time.Hour)})
	SaveFilesToCache("old", &FilesCache{Username: "old", DownloadedAt: now.Add(-2 * time.Hour)})
	SaveFilesToCache("new", &FilesCache{Username: "new", DownloadedAt: now})

	// 重置历史缓存，强制重新从 filesCacheMap 排序
	historyCacheMu.Lock()
	historyCache = nil
	historyCacheMu.Unlock()

	history := GetDownloadHistory(0)
	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want 3", len(history))
	}

	// 验证倒序：最新的在前
	if history[0].Username != "new" {
		t.Errorf("history[0].Username = %q, want %q", history[0].Username, "new")
	}
	if history[1].Username != "mid" {
		t.Errorf("history[1].Username = %q, want %q", history[1].Username, "mid")
	}
	if history[2].Username != "old" {
		t.Errorf("history[2].Username = %q, want %q", history[2].Username, "old")
	}
}

// TestGetDownloadHistory_Limit limit 参数应截断返回数量
func TestGetDownloadHistory_Limit(t *testing.T) {
	setupCacheTest(t)

	now := time.Now()
	for i := 0; i < 5; i++ {
		SaveFilesToCache(
			"sc"+string(rune('a'+i)),
			&FilesCache{DownloadedAt: now.Add(time.Duration(i) * time.Minute)},
		)
	}

	historyCacheMu.Lock()
	historyCache = nil
	historyCacheMu.Unlock()

	result := GetDownloadHistory(3)
	if len(result) != 3 {
		t.Errorf("GetDownloadHistory(3) returned %d items, want 3", len(result))
	}
}

// TestGetDownloadHistory_ZeroLimit limit=0 应返回全部
func TestGetDownloadHistory_ZeroLimit(t *testing.T) {
	setupCacheTest(t)

	now := time.Now()
	SaveFilesToCache("h1", &FilesCache{DownloadedAt: now})
	SaveFilesToCache("h2", &FilesCache{DownloadedAt: now.Add(time.Minute)})

	historyCacheMu.Lock()
	historyCache = nil
	historyCacheMu.Unlock()

	result := GetDownloadHistory(0)
	if len(result) != 2 {
		t.Errorf("GetDownloadHistory(0) returned %d items, want 2", len(result))
	}
}
