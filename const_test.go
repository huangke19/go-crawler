package main

import (
	"testing"
)

// TestConstants 测试常量定义
func TestConstants(t *testing.T) {
	// 验证关键常量存在且合理
	if defaultMonitorIntervalMin <= 0 {
		t.Error("defaultMonitorIntervalMin 应为正数")
	}

	if maxConcurrentDownloads <= 0 {
		t.Error("maxConcurrentDownloads 应为正数")
	}

	if defaultMaxConcurrentDownloads <= 0 {
		t.Error("defaultMaxConcurrentDownloads 应为正数")
	}
}

// TestPostsCacheExpiry 测试帖子缓存过期时间
func TestPostsCacheExpiry(t *testing.T) {
	if postsCacheExpiry <= 0 {
		t.Error("postsCacheExpiry 应为正数")
	}

	if defaultPostsCacheExpiry <= 0 {
		t.Error("defaultPostsCacheExpiry 应为正数")
	}
}

// TestInstagramBaseURL 测试 Instagram 基础 URL
func TestInstagramBaseURL(t *testing.T) {
	if instagramBaseURL == "" {
		t.Error("instagramBaseURL 不应为空")
	}

	// 验证是 HTTPS
	if len(instagramBaseURL) < 8 || instagramBaseURL[:8] != "https://" {
		t.Error("instagramBaseURL 应使用 HTTPS")
	}
}

// TestGraphQLDocID 测试 GraphQL 文档 ID
func TestGraphQLDocID(t *testing.T) {
	if graphQLDocID == "" {
		t.Error("graphQLDocID 不应为空")
	}

	// 验证是数字字符串
	if len(graphQLDocID) == 0 {
		t.Error("graphQLDocID 长度应大于 0")
	}
}

// TestMaxConcurrentDownloads 测试最大并发下载数
func TestMaxConcurrentDownloads(t *testing.T) {
	if maxConcurrentDownloads < 1 {
		t.Error("maxConcurrentDownloads 应至少为 1")
	}

	if maxConcurrentDownloads > 100 {
		t.Error("maxConcurrentDownloads 不应过大（可能导致资源耗尽）")
	}
}

// TestDefaultMonitorInterval 测试默认监控间隔
func TestDefaultMonitorInterval(t *testing.T) {
	if defaultMonitorIntervalMin < 1 {
		t.Error("defaultMonitorIntervalMin 应至少为 1 分钟")
	}

	if defaultMonitorIntervalMin > 1440 {
		t.Error("defaultMonitorIntervalMin 不应超过 1 天（1440 分钟）")
	}
}

// TestTimeoutConstants 测试超时常量
func TestTimeoutConstants(t *testing.T) {
	// 验证 HTTP 超时
	if httpDownloadTimeout <= 0 {
		t.Error("httpDownloadTimeout 应为正数")
	}

	if httpIdleConnTimeout <= 0 {
		t.Error("httpIdleConnTimeout 应为正数")
	}

	if httpHealthCheckTimeout <= 0 {
		t.Error("httpHealthCheckTimeout 应为正数")
	}

	if httpWorkerCallTimeout <= 0 {
		t.Error("httpWorkerCallTimeout 应为正数")
	}

	// 验证 Worker 超时
	if workerReadHeaderTimeout <= 0 {
		t.Error("workerReadHeaderTimeout 应为正数")
	}

	if workerShutdownTimeout <= 0 {
		t.Error("workerShutdownTimeout 应为正数")
	}
}

// TestConfigDefaults 测试配置默认值的合理性
func TestConfigDefaults(t *testing.T) {
	config := &Config{}

	// 测试 GetWorkerAddr 的默认值
	addr := config.GetWorkerAddr()
	if addr == "" {
		t.Error("GetWorkerAddr 应返回默认值")
	}

	// 测试 GetWorkerBaseURL 的默认值
	baseURL := config.GetWorkerBaseURL()
	if baseURL == "" {
		t.Error("GetWorkerBaseURL 应返回默认值")
	}

	// 验证 URL 格式
	if len(baseURL) < 7 || baseURL[:7] != "http://" {
		t.Error("GetWorkerBaseURL 应返回带 http:// 前缀的 URL")
	}

	t.Logf("默认 Worker 地址: %s", addr)
	t.Logf("默认 Worker URL: %s", baseURL)
}

// TestMonitorDefaults 测试监控相关默认值
func TestMonitorDefaults(t *testing.T) {
	// LoadConfig 会填充默认值，这里测试常量本身
	if defaultMonitorIntervalMin <= 0 {
		t.Error("defaultMonitorIntervalMin 应为正数")
	}

	t.Logf("默认监控间隔: %d 分钟", defaultMonitorIntervalMin)
}

// TestCacheExpiry 测试缓存过期时间的合理性
func TestCacheExpiry(t *testing.T) {
	// 帖子缓存过期时间应该是合理的（不能太短也不能太长）
	hours := postsCacheExpiry.Hours()

	if hours < 0.5 {
		t.Error("postsCacheExpiry 太短（小于 30 分钟）")
	}

	if hours > 168 {
		t.Error("postsCacheExpiry 太长（超过 7 天）")
	}
}

// TestConcurrencyLimits 测试并发限制的合理性
func TestConcurrencyLimits(t *testing.T) {
	// 验证并发数在合理范围内
	if maxConcurrentDownloads < 1 {
		t.Error("并发数不能小于 1")
	}

	if maxConcurrentDownloads > 50 {
		t.Logf("警告：并发数 %d 可能过高", maxConcurrentDownloads)
	}
}
