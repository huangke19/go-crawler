// ============================================================================
// config_env.go - 环境变量支持
// ============================================================================
//
// 职责：
//   - 从环境变量读取配置
//   - 环境变量优先级高于配置文件
//   - 支持敏感信息通过环境变量传递
//
// 支持的环境变量：
//   - TELEGRAM_BOT_TOKEN: Telegram Bot Token
//   - WORKER_ADDR: Worker 服务地址
//   - WORKER_API_TOKEN: Worker 接口鉴权 Token
//   - ALLOWED_USER_IDS: 允许的用户 ID（逗号分隔）
//   - ADMIN_USER_IDS: 管理员用户 ID（逗号分隔）
//
// ============================================================================

package main

import (
	"os"
	"strconv"
	"strings"
)

// LoadConfigWithEnv 从配置文件加载配置，并用环境变量覆盖
func LoadConfigWithEnv(path string) (*Config, error) {
	// 先加载配置文件
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	// 用环境变量覆盖
	applyEnvOverrides(config)

	// 验证最终配置
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// applyEnvOverrides 用环境变量覆盖配置
func applyEnvOverrides(config *Config) {
	// Telegram Bot Token
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		config.TelegramBotToken = token
	}

	// Worker 地址
	if addr := os.Getenv("WORKER_ADDR"); addr != "" {
		config.WorkerAddr = addr
	}
	if token := os.Getenv("WORKER_API_TOKEN"); token != "" {
		config.WorkerAPIToken = strings.TrimSpace(token)
	}

	// 允许的用户 ID
	if ids := os.Getenv("ALLOWED_USER_IDS"); ids != "" {
		if parsed := parseUserIDs(ids); len(parsed) > 0 {
			config.AllowedUserIDs = parsed
		}
	}

	// 管理员用户 ID
	if ids := os.Getenv("ADMIN_USER_IDS"); ids != "" {
		if parsed := parseUserIDs(ids); len(parsed) > 0 {
			config.AdminUserIDs = parsed
		}
	}

	// 监控间隔
	if interval := os.Getenv("MONITOR_INTERVAL_MIN"); interval != "" {
		if val, err := strconv.Atoi(interval); err == nil && val > 0 {
			config.MonitorIntervalMin = val
		}
	}

	// 最大并发下载数
	if concurrent := os.Getenv("MAX_CONCURRENT_DOWNLOADS"); concurrent != "" {
		if val, err := strconv.Atoi(concurrent); err == nil && val > 0 {
			config.MaxConcurrentDownloads = val
		}
	}

	// 帖子缓存过期时间
	if expiry := os.Getenv("POSTS_CACHE_EXPIRY_HOURS"); expiry != "" {
		if val, err := strconv.Atoi(expiry); err == nil && val > 0 {
			config.PostsCacheExpiryHours = val
		}
	}
}

// parseUserIDs 解析逗号分隔的用户 ID 列表
// 格式：123456789,987654321,111222333
func parseUserIDs(s string) []int64 {
	parts := strings.Split(s, ",")
	var ids []int64

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			continue
		}

		ids = append(ids, id)
	}

	return ids
}

// GetEnvExample 返回环境变量配置示例
func GetEnvExample() string {
	return `# 环境变量配置示例

# Telegram Bot Token（必需）
export TELEGRAM_BOT_TOKEN="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"

# Worker 服务地址（可选，默认 127.0.0.1:18080）
export WORKER_ADDR="127.0.0.1:18080"

# Worker 接口鉴权 Token（可选，建议开启）
export WORKER_API_TOKEN="replace_with_strong_token"

# 允许的用户 ID（可选，逗号分隔）
export ALLOWED_USER_IDS="123456789,987654321"

# 管理员用户 ID（可选，逗号分隔）
export ADMIN_USER_IDS="123456789"

# 监控间隔（分钟，可选，默认 30）
export MONITOR_INTERVAL_MIN="30"

# 最大并发下载数（可选，默认 10）
export MAX_CONCURRENT_DOWNLOADS="10"

# 帖子缓存过期时间（小时，可选，默认 24）
export POSTS_CACHE_EXPIRY_HOURS="24"
`
}

// PrintEnvStatus 打印环境变量状态（用于调试）
func PrintEnvStatus() {
	envVars := []string{
		"TELEGRAM_BOT_TOKEN",
		"WORKER_ADDR",
		"WORKER_API_TOKEN",
		"ALLOWED_USER_IDS",
		"ADMIN_USER_IDS",
		"MONITOR_INTERVAL_MIN",
		"MAX_CONCURRENT_DOWNLOADS",
		"POSTS_CACHE_EXPIRY_HOURS",
	}

	println("环境变量状态:")
	for _, key := range envVars {
		value := os.Getenv(key)
		if value != "" {
			// 隐藏敏感信息
			if key == "TELEGRAM_BOT_TOKEN" || key == "WORKER_API_TOKEN" {
				if len(value) > 10 {
					value = value[:10] + "..."
				}
			}
			println("  ✓", key, "=", value)
		} else {
			println("  ✗", key, "(未设置)")
		}
	}
}
