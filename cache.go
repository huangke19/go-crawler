// ============================================================================
// cache.go - 三层缓存系统
// ============================================================================
//
// 职责：
//   - 管理三层缓存：媒体 URL 缓存 / 帖子列表缓存 / 文件路径缓存
//   - 缓存持久化：读写 JSON 文件
//   - 缓存过期管理：检查过期时间，自动失效
//   - 并发安全：使用 RWMutex 保护共享状态
//
// 三层缓存说明：
//
//   1. 媒体缓存 (media_cache.json)
//      - 缓存 Instagram GraphQL API 响应
//      - key: shortcode
//      - value: 媒体类型 + URL 列表
//      - 过期时间：24 小时（可手动清理）
//      - 用途：避免重复调用 GraphQL API
//
//   2. 帖子缓存 (posts_cache.json)
//      - 缓存用户主页帖子列表
//      - key: username
//      - value: 帖子 shortcodes 列表 + 更新时间
//      - 过期时间：1 小时
//      - 用途：避免重复滚动加载主页
//
//   3. 文件缓存 (files_cache.json)
//      - 缓存已下载的文件路径
//      - key: shortcode
//      - value: 文件路径列表 + 下载时间
//      - 过期时间：永久（但会检查文件是否存在）
//      - 用途：秒回已下载的文件，跳过抓取与下载
//
// 缓存命中顺序（Worker 下载流程）：
//   1. 文件缓存：如果文件仍存在，直接返回路径（<1秒）
//   2. 媒体缓存：如果 shortcode 的媒体 URL 已缓存，跳过 GraphQL 调用
//   3. 帖子缓存：如果用户主页帖子列表未过期，跳过滚动加载
//   4. 实时抓取：以上都未命中，则实时访问主页、调用 GraphQL、下载文件
//
// 关键函数：
//   - LoadMediaCache() / SaveMediaCache()：媒体缓存读写
//   - LoadPostsCache() / SavePostsCache()：帖子缓存读写
//   - LoadFilesCache() / SaveFilesCache()：文件缓存读写
//   - GetMediaFromCache() / SaveMediaToCache()：媒体缓存查询/更新
//   - GetPostsFromCache() / SavePostsToCache()：帖子缓存查询/更新（含过期检查）
//   - GetFilesFromCache() / SaveFilesToCache()：文件缓存查询/更新（含文件存在性检查）
//   - GetDownloadHistory()：获取下载历史（按时间倒序）
//
// ============================================================================

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const cacheDir = "cache"

// MediaCache 媒体 URL 缓存（shortcode -> 媒体信息）。
//
// 该缓存用于避免重复调用 Instagram GraphQL：
// - key: shortcode
// - value: 媒体类型 + URL 列表
// 一般可视为长期有效；当 Instagram 资源 URL 失效时，可手动清理对应 key。
type MediaCache struct {
	Type  string   `json:"type"`
	URLs  []string `json:"urls"`
	Types []string `json:"types"`
}

// PostsCache 用户帖子列表缓存（username -> 帖子 shortcodes 列表）。
//
// 该缓存用于"按时间线序号下载"的第一步：主页定位第 N 条帖子。
// - UpdatedAt: 最近一次刷新时间
// - ExpiresAt: 到期后视为无效（避免长期依赖旧主页结构）
type PostsCache struct {
	Posts     []PostItem `json:"posts"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// PostItem 是帖子在"时间线序号"语义下的定位信息。
// Index 从 1 开始，1 表示主页最新的一条。
type PostItem struct {
	Index     int    `json:"index"`
	Shortcode string `json:"shortcode"`
}

// FilesCache 本地文件缓存（shortcode -> 已下载文件路径）。
//
// 这是最快的一层缓存：如果文件仍存在，worker 可以直接返回路径，跳过抓取与下载。
// Username/PostIndex 用于 bot 侧展示历史下载与"按序号"的溯源信息；按 shortcode 下载时 PostIndex 可能为 0。
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
	os.MkdirAll(filepath.Join(cacheDir, "thumbnails"), 0755)
	initCacheMaps()
}

func initCacheMaps() {
	cacheInitOnce.Do(func() {
		mediaCacheMap = make(map[string]*MediaCache)
		postsCacheMap = make(map[string]*PostsCache)
		filesCacheMap = make(map[string]*FilesCache)
	})
}

// LoadMediaCache 加载媒体 URL 缓存。
// 使用"惰性加载 + 双重检查 + RWMutex"：
// - 多读少写场景下减少锁竞争；
// - 第一次读取时从磁盘加载，后续直接走内存 map。
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

// SaveMediaCache 保存媒体 URL 缓存到 `cache/media_cache.json`。
// 写入会替换内存 map 引用；调用方应避免持有旧 map 的长生命周期引用。
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

// LoadPostsCache 加载帖子列表缓存。
// 与媒体缓存一致：惰性加载到内存，后续读取无需反复读盘。
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

// SavePostsCache 保存帖子列表缓存到 `cache/posts_cache.json`。
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

// LoadFilesCache 加载文件缓存（shortcode -> 本地文件路径）。
// 文件缓存除了读盘外，在 `GetFilesFromCache` 还会做"文件存在性校验"，避免返回已被删除的路径。
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

// SaveFilesCache 保存文件缓存到 `cache/files_cache.json`。
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

// GetMediaFromCache 从媒体缓存获取媒体信息。
func GetMediaFromCache(shortcode string) (*MediaCache, bool) {
	cache, _ := LoadMediaCache()
	media, ok := cache[shortcode]
	return media, ok
}

// SaveMediaToCache 保存媒体信息到媒体缓存。
func SaveMediaToCache(shortcode string, media *MediaCache) error {
	cache, _ := LoadMediaCache()
	cache[shortcode] = media
	return SaveMediaCache(cache)
}

// GetPostsFromCache 从帖子列表缓存获取用户帖子列表（会检查过期时间）。
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

// SavePostsToCache 保存用户帖子列表到缓存。
func SavePostsToCache(username string, posts *PostsCache) error {
	cache, _ := LoadPostsCache()
	cache[username] = posts
	return SavePostsCache(cache)
}

// GetFilesFromCache 从文件缓存获取文件列表。
// 除了缓存命中外，这里会验证每个文件路径是否仍存在：
// - 若任意文件缺失，则视为缓存失效，促使 worker 走"重新下载"路径。
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

// SaveFilesToCache 保存文件列表到缓存，并同步更新"下载历史"的内存缓存。
// 该函数是事件驱动更新：当写入新下载记录时，尽量避免下一次 bot 侧读取历史还要重新排序全量数据。
func SaveFilesToCache(shortcode string, files *FilesCache) error {
	cache, _ := LoadFilesCache()
	cache[shortcode] = files

	// 主动更新历史缓存（事件驱动）
	historyCacheMu.Lock()
	if historyCache != nil {
		// 插入新记录到头部（已按时间倒序）
		historyCache = append([]*FilesCache{files}, historyCache...)
		historyCacheTime = time.Now()
	} else {
		// 首次加载，下次 GetDownloadHistory 会重新加载并排序
		historyCache = nil
	}
	historyCacheMu.Unlock()

	return SaveFilesCache(cache)
}

// GetDownloadHistory 获取下载历史（按下载时间倒序）。
//
// 设计：
// - 内存缓存 1 分钟，避免 bot 频繁刷新时每次都读盘 + 排序；
// - 读取时把 `files_cache.json` 的 value 集合拉平为 slice，再按 DownloadedAt 倒序排序。
func GetDownloadHistory(limit int) []*FilesCache {
	// 检查内存缓存（1分钟有效期）
	historyCacheMu.RLock()
	if historyCache != nil && time.Since(historyCacheTime) < 1*time.Minute {
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

// quickSortHistory 快速排序（按时间倒序）。
// 这里用手写排序主要是为了减少额外分配；数据规模一般不大（历史记录数量有限）。
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
