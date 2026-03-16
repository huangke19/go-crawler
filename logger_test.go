package main

import (
	"testing"
	"time"
)

// TestInitLogger 测试日志初始化
func TestInitLogger(t *testing.T) {
	tests := []struct {
		name   string
		config LogConfig
	}{
		{
			name: "开发环境配置",
			config: LogConfig{
				Level:  "info",
				Pretty: true,
			},
		},
		{
			name: "生产环境配置",
			config: LogConfig{
				Level:  "warn",
				Pretty: false,
			},
		},
		{
			name: "Debug 级别",
			config: LogConfig{
				Level:  "debug",
				Pretty: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitLogger(tt.config)
			if err != nil {
				t.Errorf("InitLogger() error = %v", err)
			}
		})
	}
}

// TestParseLogLevel 测试日志级别解析
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{"debug", "debug", "debug"},
		{"info", "info", "info"},
		{"warn", "warn", "warn"},
		{"error", "error", "error"},
		{"invalid", "invalid", "info"}, // 默认为 info
		{"empty", "", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := parseLogLevel(tt.level)
			if level.String() != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.level, level, tt.want)
			}
		})
	}
}

// TestLogHelpers 测试日志辅助函数
func TestLogHelpers(t *testing.T) {
	// 初始化日志（不输出到文件）
	err := InitLogger(LogConfig{
		Level:  "debug",
		Pretty: false,
	})
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// 测试各种日志函数（不验证输出，只确保不崩溃）
	t.Run("LogDownloadStart", func(t *testing.T) {
		LogDownloadStart("nike", 1, 123456)
	})

	t.Run("LogDownloadSuccess", func(t *testing.T) {
		LogDownloadSuccess("nike", 1, 3, 5*time.Second)
	})

	t.Run("LogDownloadError", func(t *testing.T) {
		LogDownloadError("nike", 1, nil)
	})

	t.Run("LogCacheHit", func(t *testing.T) {
		LogCacheHit("media", "ABC123")
	})

	t.Run("LogCacheMiss", func(t *testing.T) {
		LogCacheMiss("posts", "nike")
	})

	t.Run("LogAPICall", func(t *testing.T) {
		LogAPICall("graphql", 100*time.Millisecond, true)
	})

	t.Run("LogWorkerHealth", func(t *testing.T) {
		LogWorkerHealth(true, "Worker is healthy")
	})

	t.Run("LogBotCommand", func(t *testing.T) {
		LogBotCommand(123456, "testuser", "/download")
	})

	t.Run("LogMonitorCheck", func(t *testing.T) {
		LogMonitorCheck("nike", true, 2)
	})
}

// TestMetricsRecording 测试 Metrics 记录
func TestMetricsRecording(t *testing.T) {
	t.Run("RecordDownloadSuccess", func(t *testing.T) {
		RecordDownloadSuccess("nike", 5.0, 3)
	})

	t.Run("RecordDownloadError", func(t *testing.T) {
		RecordDownloadError()
	})

	t.Run("RecordCacheHit", func(t *testing.T) {
		RecordCacheHit("media")
		RecordCacheHit("posts")
		RecordCacheHit("files")
	})

	t.Run("RecordCacheMiss", func(t *testing.T) {
		RecordCacheMiss("media")
		RecordCacheMiss("posts")
	})

	t.Run("RecordAPICall", func(t *testing.T) {
		RecordAPICall("graphql", 0.5, true)
		RecordAPICall("instagram", 1.0, false)
	})

	t.Run("RecordBotCommand", func(t *testing.T) {
		RecordBotCommand("/start")
		RecordBotCommand("/download")
		RecordBotCommand("/status")
	})

	t.Run("RecordMonitorCheck", func(t *testing.T) {
		RecordMonitorCheck("nike", true)
		RecordMonitorCheck("tesla", false)
	})

	t.Run("UpdateConcurrentDownloads", func(t *testing.T) {
		UpdateConcurrentDownloads(5)
		UpdateConcurrentDownloads(0)
	})

	t.Run("UpdateWorkerHealth", func(t *testing.T) {
		UpdateWorkerHealth(true)
		UpdateWorkerHealth(false)
	})

	t.Run("UpdateCacheSize", func(t *testing.T) {
		UpdateCacheSize("media", 100)
		UpdateCacheSize("posts", 50)
		UpdateCacheSize("files", 200)
	})
}

// TestGetDefaultLogConfig 测试默认日志配置
func TestGetDefaultLogConfig(t *testing.T) {
	config := GetDefaultLogConfig()

	if config.Level != "info" {
		t.Errorf("默认日志级别应为 info，实际为 %s", config.Level)
	}

	if config.FileOutput != "" {
		t.Errorf("默认不应输出到文件")
	}
}

// TestWithContext 测试上下文日志
func TestWithContext(t *testing.T) {
	err := InitLogger(LogConfig{
		Level:  "info",
		Pretty: false,
	})
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	fields := map[string]interface{}{
		"user_id":  123456,
		"username": "nike",
		"action":   "download",
	}

	logger := WithContext(fields)
	if logger == nil {
		t.Error("WithContext() 返回 nil")
	}

	// 使用上下文日志
	logger.Info().Msg("测试上下文日志")
}
