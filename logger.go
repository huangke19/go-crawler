// ============================================================================
// logger.go - 结构化日志
// ============================================================================
//
// 职责：
//   - 提供统一的日志接口
//   - 支持多种日志级别（Debug/Info/Warn/Error）
//   - 结构化日志输出（JSON 格式）
//   - 开发环境美化输出
//   - 生产环境文件输出
//
// 使用方式：
//   log.Info().Str("username", "nike").Msg("开始下载")
//   log.Error().Err(err).Msg("下载失败")
//   log.Debug().Int("count", 10).Msg("找到 10 个帖子")
//
// ============================================================================

package main

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	// Logger 全局日志实例
	Logger zerolog.Logger
)

// LogConfig 日志配置
type LogConfig struct {
	Level      string // 日志级别：debug, info, warn, error
	Pretty     bool   // 是否美化输出（开发环境）
	FileOutput string // 文件输出路径（为空则只输出到控制台）
}

// InitLogger 初始化日志系统
func InitLogger(config LogConfig) error {
	// 设置日志级别
	level := parseLogLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	// 设置时间格式
	zerolog.TimeFieldFormat = time.RFC3339

	var writers []io.Writer

	// 控制台输出
	if config.Pretty {
		// 开发环境：美化输出
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
			NoColor:    false,
		}
		writers = append(writers, consoleWriter)
	} else {
		// 生产环境：JSON 输出
		writers = append(writers, os.Stdout)
	}

	// 文件输出
	if config.FileOutput != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(config.FileOutput)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}

		// 打开日志文件（追加模式）
		file, err := os.OpenFile(config.FileOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}

		writers = append(writers, file)
	}

	// 创建多输出日志
	multi := io.MultiWriter(writers...)
	Logger = zerolog.New(multi).With().Timestamp().Logger()

	// 设置全局日志
	log.Logger = Logger

	return nil
}

// parseLogLevel 解析日志级别
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// GetDefaultLogConfig 获取默认日志配置
func GetDefaultLogConfig() LogConfig {
	// 检测是否在开发环境
	isDev := os.Getenv("ENV") != "production"

	return LogConfig{
		Level:      "info",
		Pretty:     isDev,
		FileOutput: "", // 默认不输出到文件
	}
}

// WithContext 创建带上下文的日志
func WithContext(fields map[string]interface{}) *zerolog.Logger {
	ctx := Logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	logger := ctx.Logger()
	return &logger
}

// LogDownloadStart 记录下载开始
func LogDownloadStart(username string, postIndex int, userID int64) {
	Logger.Info().
		Str("action", "download_start").
		Str("username", username).
		Int("post_index", postIndex).
		Int64("user_id", userID).
		Msg("开始下载")
}

// LogDownloadSuccess 记录下载成功
func LogDownloadSuccess(username string, postIndex int, fileCount int, duration time.Duration) {
	Logger.Info().
		Str("action", "download_success").
		Str("username", username).
		Int("post_index", postIndex).
		Int("file_count", fileCount).
		Dur("duration", duration).
		Msg("下载成功")
}

// LogDownloadError 记录下载失败
func LogDownloadError(username string, postIndex int, err error) {
	Logger.Error().
		Str("action", "download_error").
		Str("username", username).
		Int("post_index", postIndex).
		Err(err).
		Msg("下载失败")
}

// LogCacheHit 记录缓存命中
func LogCacheHit(cacheType string, key string) {
	Logger.Debug().
		Str("action", "cache_hit").
		Str("cache_type", cacheType).
		Str("key", key).
		Msg("缓存命中")
}

// LogCacheMiss 记录缓存未命中
func LogCacheMiss(cacheType string, key string) {
	Logger.Debug().
		Str("action", "cache_miss").
		Str("cache_type", cacheType).
		Str("key", key).
		Msg("缓存未命中")
}

// LogAPICall 记录 API 调用
func LogAPICall(api string, duration time.Duration, success bool) {
	event := Logger.Info().
		Str("action", "api_call").
		Str("api", api).
		Dur("duration", duration).
		Bool("success", success)

	if success {
		event.Msg("API 调用成功")
	} else {
		event.Msg("API 调用失败")
	}
}

// LogWorkerHealth 记录 Worker 健康状态
func LogWorkerHealth(healthy bool, message string) {
	if healthy {
		Logger.Info().
			Str("action", "worker_health").
			Bool("healthy", true).
			Msg(message)
	} else {
		Logger.Warn().
			Str("action", "worker_health").
			Bool("healthy", false).
			Msg(message)
	}
}

// LogBotCommand 记录 Bot 命令
func LogBotCommand(userID int64, username string, command string) {
	Logger.Info().
		Str("action", "bot_command").
		Int64("user_id", userID).
		Str("username", username).
		Str("command", command).
		Msg("收到 Bot 命令")
}

// LogMonitorCheck 记录监控检查
func LogMonitorCheck(account string, hasNewPost bool, newPostCount int) {
	Logger.Info().
		Str("action", "monitor_check").
		Str("account", account).
		Bool("has_new_post", hasNewPost).
		Int("new_post_count", newPostCount).
		Msg("监控检查完成")
}
