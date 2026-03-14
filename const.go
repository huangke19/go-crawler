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
	tlsHandshakeTimeout   = 10 * time.Second
	browserFastTimeout     = 60 * time.Second
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
	postsCacheExpiry    = 24 * time.Hour
	historyCacheTTL     = 1 * time.Minute
)

// 限制
const (
	maxConcurrentDownloads = 10
	maxJSONResponseSize    = 10 << 20 // 10MB
)
