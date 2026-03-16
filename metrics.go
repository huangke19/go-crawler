// ============================================================================
// metrics.go - Prometheus 指标收集
// ============================================================================
//
// 职责：
//   - 收集应用性能指标
//   - 提供 Prometheus 格式的 metrics 端点
//   - 监控下载、缓存、API 调用等关键指标
//
// 指标类型：
//   - Counter: 只增不减的计数器（下载次数、错误次数）
//   - Histogram: 分布统计（下载耗时、文件大小）
//   - Gauge: 可增可减的值（并发数、缓存大小）
//
// 使用方式：
//   metrics.DownloadTotal.WithLabelValues("success").Inc()
//   metrics.DownloadDuration.Observe(duration.Seconds())
//
// ============================================================================

package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DownloadTotal 下载总数（按状态分类）
	DownloadTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_download_total",
			Help: "Total number of downloads",
		},
		[]string{"status"}, // success, error
	)

	// DownloadDuration 下载耗时分布
	DownloadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "crawler_download_duration_seconds",
			Help:    "Download duration in seconds",
			Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
		},
		[]string{"username"},
	)

	// DownloadFileSize 下载文件大小分布
	DownloadFileSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "crawler_download_file_size_bytes",
			Help:    "Downloaded file size in bytes",
			Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 104857600}, // 1KB, 10KB, 100KB, 1MB, 10MB, 100MB
		},
	)

	// CacheHitTotal 缓存命中总数
	CacheHitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_cache_hit_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"}, // media, posts, files
	)

	// CacheMissTotal 缓存未命中总数
	CacheMissTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_cache_miss_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	// APICallTotal API 调用总数
	APICallTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_api_call_total",
			Help: "Total number of API calls",
		},
		[]string{"api", "status"}, // graphql/instagram, success/error
	)

	// APICallDuration API 调用耗时
	APICallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "crawler_api_call_duration_seconds",
			Help:    "API call duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"api"},
	)

	// ConcurrentDownloads 当前并发下载数
	ConcurrentDownloads = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "crawler_concurrent_downloads",
			Help: "Current number of concurrent downloads",
		},
	)

	// WorkerHealthy Worker 健康状态
	WorkerHealthy = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "crawler_worker_healthy",
			Help: "Worker health status (1=healthy, 0=unhealthy)",
		},
	)

	// BotCommandTotal Bot 命令总数
	BotCommandTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_bot_command_total",
			Help: "Total number of bot commands",
		},
		[]string{"command"},
	)

	// MonitorCheckTotal 监控检查总数
	MonitorCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_monitor_check_total",
			Help: "Total number of monitor checks",
		},
		[]string{"account", "has_new_post"},
	)

	// CacheSize 缓存大小（条目数）
	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "crawler_cache_size",
			Help: "Number of entries in cache",
		},
		[]string{"cache_type"},
	)
)

// RecordDownloadSuccess 记录下载成功
func RecordDownloadSuccess(username string, duration float64, fileCount int) {
	DownloadTotal.WithLabelValues("success").Inc()
	DownloadDuration.WithLabelValues(username).Observe(duration)
}

// RecordDownloadError 记录下载失败
func RecordDownloadError() {
	DownloadTotal.WithLabelValues("error").Inc()
}

// RecordCacheHit 记录缓存命中
func RecordCacheHit(cacheType string) {
	CacheHitTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss 记录缓存未命中
func RecordCacheMiss(cacheType string) {
	CacheMissTotal.WithLabelValues(cacheType).Inc()
}

// RecordAPICall 记录 API 调用
func RecordAPICall(api string, duration float64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	APICallTotal.WithLabelValues(api, status).Inc()
	APICallDuration.WithLabelValues(api).Observe(duration)
}

// RecordBotCommand 记录 Bot 命令
func RecordBotCommand(command string) {
	BotCommandTotal.WithLabelValues(command).Inc()
}

// RecordMonitorCheck 记录监控检查
func RecordMonitorCheck(account string, hasNewPost bool) {
	status := "false"
	if hasNewPost {
		status = "true"
	}
	MonitorCheckTotal.WithLabelValues(account, status).Inc()
}

// UpdateConcurrentDownloads 更新并发下载数
func UpdateConcurrentDownloads(count float64) {
	ConcurrentDownloads.Set(count)
}

// UpdateWorkerHealth 更新 Worker 健康状态
func UpdateWorkerHealth(healthy bool) {
	if healthy {
		WorkerHealthy.Set(1)
	} else {
		WorkerHealthy.Set(0)
	}
}

// UpdateCacheSize 更新缓存大小
func UpdateCacheSize(cacheType string, size float64) {
	CacheSize.WithLabelValues(cacheType).Set(size)
}

// GetCacheHitRate 计算缓存命中率
func GetCacheHitRate(cacheType string) float64 {
	// 这是一个辅助函数，实际计算需要从 Prometheus 查询
	// 这里只是示例，实际使用时应该从 Prometheus 查询
	// rate = hits / (hits + misses)
	return 0.0
}
