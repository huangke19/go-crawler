package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const cacheDir = "cache"

// MediaCache 媒体URL缓存（永久有效）
type MediaCache struct {
	Type  string   `json:"type"`
	URLs  []string `json:"urls"`
	Types []string `json:"types"`
}

// PostsCache 用户帖子列表缓存（24小时过期）
type PostsCache struct {
	Posts     []PostItem `json:"posts"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// PostItem 帖子信息
type PostItem struct {
	Index     int    `json:"index"`
	Shortcode string `json:"shortcode"`
}

// FilesCache 本地文件缓存
type FilesCache struct {
	Files        []string  `json:"files"`
	Username     string    `json:"username"`
	PostIndex    int       `json:"post_index"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

var (
	mediaCacheMu     sync.RWMutex
	postsCacheMu     sync.RWMutex
	filesCacheMu     sync.RWMutex
	mediaCacheMap    map[string]*MediaCache
	postsCacheMap    map[string]*PostsCache
	filesCacheMap    map[string]*FilesCache
	historyCacheMu   sync.RWMutex
	historyCache     []*FilesCache
	historyCacheTime time.Time
	cacheInitOnce    sync.Once
)

func init() {
	// 确保缓存目录存在
	os.MkdirAll(cacheDir, 0755)
	initCacheMaps()
}

func initCacheMaps() {
	cacheInitOnce.Do(func() {
		mediaCacheMap = make(map[string]*MediaCache)
		postsCacheMap = make(map[string]*PostsCache)
		filesCacheMap = make(map[string]*FilesCache)
	})
}

// LoadMediaCache 加载媒体URL缓存
func LoadMediaCache() (map[string]*MediaCache, error) {
	mediaCacheMu.RLock()
	if len(mediaCacheMap) > 0 {
		result := mediaCacheMap
		mediaCacheMu.RUnlock()
		return result, nil
	}
	mediaCacheMu.RUnlock()

	mediaCacheMu.Lock()
	defer mediaCacheMu.Unlock()

	// 双重检查：可能其他 goroutine 已经加载了
	if len(mediaCacheMap) > 0 {
		return mediaCacheMap, nil
	}

	path := filepath.Join(cacheDir, "media_cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			mediaCacheMap = make(map[string]*MediaCache)
			return mediaCacheMap, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &mediaCacheMap); err != nil {
		return nil, err
	}

	return mediaCacheMap, nil
}

// SaveMediaCache 保存媒体URL缓存
func SaveMediaCache(cache map[string]*MediaCache) error {
	mediaCacheMu.Lock()
	defer mediaCacheMu.Unlock()

	mediaCacheMap = cache

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(cacheDir, "media_cache.json")
	return os.WriteFile(path, data, 0644)
}

// LoadPostsCache 加载帖子列表缓存
func LoadPostsCache() (map[string]*PostsCache, error) {
	postsCacheMu.RLock()
	if len(postsCacheMap) > 0 {
		postsCacheMu.RUnlock()
		return postsCacheMap, nil
	}
	postsCacheMu.RUnlock()

	postsCacheMu.Lock()
	defer postsCacheMu.Unlock()

	path := filepath.Join(cacheDir, "posts_cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			postsCacheMap = make(map[string]*PostsCache)
			return postsCacheMap, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &postsCacheMap); err != nil {
		return nil, err
	}

	return postsCacheMap, nil
}

// SavePostsCache 保存帖子列表缓存
func SavePostsCache(cache map[string]*PostsCache) error {
	postsCacheMu.Lock()
	defer postsCacheMu.Unlock()

	postsCacheMap = cache

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(cacheDir, "posts_cache.json")
	return os.WriteFile(path, data, 0644)
}

// LoadFilesCache 加载文件缓存
func LoadFilesCache() (map[string]*FilesCache, error) {
	filesCacheMu.RLock()
	if len(filesCacheMap) > 0 {
		filesCacheMu.RUnlock()
		return filesCacheMap, nil
	}
	filesCacheMu.RUnlock()

	filesCacheMu.Lock()
	defer filesCacheMu.Unlock()

	path := filepath.Join(cacheDir, "files_cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			filesCacheMap = make(map[string]*FilesCache)
			return filesCacheMap, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &filesCacheMap); err != nil {
		return nil, err
	}

	return filesCacheMap, nil
}

// SaveFilesCache 保存文件缓存
func SaveFilesCache(cache map[string]*FilesCache) error {
	filesCacheMu.Lock()
	defer filesCacheMu.Unlock()

	filesCacheMap = cache

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(cacheDir, "files_cache.json")
	return os.WriteFile(path, data, 0644)
}

// GetMediaFromCache 从缓存获取媒体信息
func GetMediaFromCache(shortcode string) (*MediaCache, bool) {
	cache, _ := LoadMediaCache()
	media, ok := cache[shortcode]
	return media, ok
}

// SaveMediaToCache 保存媒体信息到缓存
func SaveMediaToCache(shortcode string, media *MediaCache) error {
	cache, _ := LoadMediaCache()
	cache[shortcode] = media
	return SaveMediaCache(cache)
}

// GetPostsFromCache 从缓存获取用户帖子列表
func GetPostsFromCache(username string) (*PostsCache, bool) {
	cache, _ := LoadPostsCache()
	posts, ok := cache[username]
	if !ok {
		return nil, false
	}
	// 检查是否过期
	if time.Now().After(posts.ExpiresAt) {
		return nil, false
	}
	return posts, true
}

// SavePostsToCache 保存用户帖子列表到缓存
func SavePostsToCache(username string, posts *PostsCache) error {
	cache, _ := LoadPostsCache()
	cache[username] = posts
	return SavePostsCache(cache)
}

// GetFilesFromCache 从缓存获取文件列表
func GetFilesFromCache(shortcode string) (*FilesCache, bool) {
	cache, _ := LoadFilesCache()
	files, ok := cache[shortcode]
	if !ok {
		return nil, false
	}
	// 检查文件是否都存在
	for _, file := range files.Files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return nil, false
		}
	}
	return files, true
}

// SaveFilesToCache 保存文件列表到缓存
func SaveFilesToCache(shortcode string, files *FilesCache) error {
	cache, _ := LoadFilesCache()
	cache[shortcode] = files

	// 清除历史缓存
	historyCacheMu.Lock()
	historyCache = nil
	historyCacheMu.Unlock()

	return SaveFilesCache(cache)
}

// GetDownloadHistory 获取下载历史（按时间倒序）
func GetDownloadHistory(limit int) []*FilesCache {
	// 检查内存缓存（5秒有效期）
	historyCacheMu.RLock()
	if historyCache != nil && time.Since(historyCacheTime) < 5*time.Second {
		cached := historyCache
		historyCacheMu.RUnlock()
		if limit > 0 && len(cached) > limit {
			return cached[:limit]
		}
		return cached
	}
	historyCacheMu.RUnlock()

	// 加载并排序
	cache, _ := LoadFilesCache()

	var history []*FilesCache
	for _, files := range cache {
		history = append(history, files)
	}

	// 使用快速排序（按下载时间倒序）
	if len(history) > 1 {
		quickSortHistory(history, 0, len(history)-1)
	}

	// 更新内存缓存
	historyCacheMu.Lock()
	historyCache = history
	historyCacheTime = time.Now()
	historyCacheMu.Unlock()

	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return history
}

// quickSortHistory 快速排序（按时间倒序）
func quickSortHistory(arr []*FilesCache, low, high int) {
	if low < high {
		pivot := partitionHistory(arr, low, high)
		quickSortHistory(arr, low, pivot-1)
		quickSortHistory(arr, pivot+1, high)
	}
}

func partitionHistory(arr []*FilesCache, low, high int) int {
	pivot := arr[high].DownloadedAt
	i := low - 1
	for j := low; j < high; j++ {
		// 倒序：如果 arr[j] 的时间晚于 pivot，则交换
		if arr[j].DownloadedAt.After(pivot) {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}
