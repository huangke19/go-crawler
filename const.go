package main

import "time"

// Instagram URLs
const (
	instagramBaseURL  = "https://www.instagram.com"
	instagramLoginURL = instagramBaseURL + "/accounts/login/"
	instagramGraphQL  = instagramBaseURL + "/graphql/query"
	graphQLDocID      = "8845758582119845"
)

// HTTP 超时
const (
	httpDownloadTimeout    = 30 * time.Second
	httpIdleConnTimeout    = 90 * time.Second
	httpHealthCheckTimeout = 5 * time.Second
	httpWorkerCallTimeout  = 3 * time.Minute
	tlsHandshakeTimeout    = 10 * time.Second
	browserFastTimeout     = 120 * time.Second // 增加到 120 秒，应对帖子多的账户
)

// Worker 服务超时
const (
	workerReadHeaderTimeout = 10 * time.Second
	workerReadTimeout       = 60 * time.Second
	workerWriteTimeout      = 10 * time.Minute
	workerShutdownTimeout   = 30 * time.Second
)

// 页面等待时间
const (
	pageLoadWait    = 2 * time.Second
	loginActionWait = 3 * time.Second
)

// Bot 状态管理
const (
	stateCleanupInterval = 5 * time.Minute
	stateExpiration      = 5 * time.Minute
)

// 缓存过期时间
const (
	defaultPostsCacheExpiry = 24 * time.Hour
	historyCacheTTL         = 1 * time.Minute
)

// 限制
const (
	defaultMaxConcurrentDownloads = 10
	maxJSONResponseSize          = 10 << 20 // 10MB
)

// 监控
const (
	defaultMonitorIntervalMin = 30
)

// 运行时可配置参数（由 config.json 覆盖）
var (
	postsCacheExpiry       = defaultPostsCacheExpiry
	maxConcurrentDownloads = defaultMaxConcurrentDownloads
)
