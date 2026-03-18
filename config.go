// ============================================================================
// config.go - 配置管理
// ============================================================================
//
// 职责：
//   - 从 config.json 加载配置
//   - 配置验证与默认值填充
//   - 提供配置访问接口
//
// 配置项说明：
//   - telegram_bot_token：Telegram Bot Token（必需）
//   - allowed_user_ids：允许使用的用户 ID 列表（为空则开放模式）
//   - admin_user_ids：管理员用户 ID 列表（为空时回退为 allowed_user_ids）
//   - favorite_accounts：常用账户列表（用于 Bot 快速选择）
//   - worker_addr：Worker 服务监听地址（默认 127.0.0.1:18080）
//
// ============================================================================

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken       string   `json:"telegram_bot_token"`
	AllowedUserIDs         []int64  `json:"allowed_user_ids"`
	AdminUserIDs           []int64  `json:"admin_user_ids"`
	FavoriteAccounts       []string `json:"favorite_accounts"`
	WorkerAddr             string   `json:"worker_addr"`
	WorkerAPIToken         string   `json:"worker_api_token"`
	MonitorAccounts        []string `json:"monitor_accounts"`       // 监控的账户列表
	MonitorIntervalMin     int      `json:"monitor_interval_min"`   // 轮询间隔（分钟）
	MonitorIntervalHours   int      `json:"monitor_interval_hours"` // 轮询间隔（小时）
	MonitorCompareTopN     int      `json:"monitor_compare_top_n"`  // 监控对比前N条，默认20
	MaxConcurrentDownloads int      `json:"max_concurrent_downloads"`
	PostsCacheExpiryHours  int      `json:"posts_cache_expiry_hours"`
}

// LoadConfig 从指定路径读取 `config.json` 并做必要的兼容与默认值填充：
// - `telegram_bot_token` 必须存在且不可为占位符；
// - `worker_addr` 为空时使用默认监听地址；
// - `admin_user_ids` 未配置但 `allowed_user_ids` 有值时，自动回退为同一组 ID（兼容旧配置）。
const defaultMonitorCompareTopN = 20

func LoadConfig(path string) (*Config, error) {
	var config Config
	if err := loadJSONFile(path, &config); err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	if strings.TrimSpace(config.WorkerAddr) == "" {
		config.WorkerAddr = defaultWorkerListenAddr
	}
	config.WorkerAPIToken = strings.TrimSpace(config.WorkerAPIToken)

	if len(config.AdminUserIDs) == 0 && len(config.AllowedUserIDs) > 0 {
		config.AdminUserIDs = append([]int64(nil), config.AllowedUserIDs...)
	}

	if config.MonitorIntervalMin <= 0 && config.MonitorIntervalHours <= 0 {
		config.MonitorIntervalMin = defaultMonitorIntervalMin
	}

	if config.MonitorCompareTopN <= 0 {
		config.MonitorCompareTopN = defaultMonitorCompareTopN
	}

	if config.MaxConcurrentDownloads > 0 {
		maxConcurrentDownloads = config.MaxConcurrentDownloads
	}

	if config.PostsCacheExpiryHours > 0 {
		postsCacheExpiry = time.Duration(config.PostsCacheExpiryHours) * time.Hour
	}

	return &config, nil
}

// SaveConfig 将配置原子写入到指定路径：
// 先写入临时文件，再 rename，避免写入中途崩溃导致配置损坏。
func SaveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// GetWorkerAddr 返回 worker 监听地址；当配置缺失或为空时回退到默认值。
// 注意：这里返回的是"监听地址/host:port"形式，不保证带 scheme。
func (c *Config) GetWorkerAddr() string {
	if c == nil || strings.TrimSpace(c.WorkerAddr) == "" {
		return defaultWorkerListenAddr
	}
	return strings.TrimSpace(c.WorkerAddr)
}

// GetWorkerBaseURL 返回用于 HTTP 访问的 worker 基础地址。
// 如果 `worker_addr` 未包含 scheme，则默认使用 `http://` 前缀。
func (c *Config) GetWorkerBaseURL() string {
	addr := c.GetWorkerAddr()
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

// GetMonitorInterval 返回监控轮询间隔。
// 优先级：
// 1. monitor_interval_min
// 2. monitor_interval_hours
// 3. 默认值（30 分钟）
func (c *Config) GetMonitorInterval() time.Duration {
	if c != nil {
		if c.MonitorIntervalMin > 0 {
			return time.Duration(c.MonitorIntervalMin) * time.Minute
		}
		if c.MonitorIntervalHours > 0 {
			return time.Duration(c.MonitorIntervalHours) * time.Hour
		}
	}
	return time.Duration(defaultMonitorIntervalMin) * time.Minute
}

// GetMonitorIntervalLabel 返回用于展示的监控间隔文案。
func (c *Config) GetMonitorIntervalLabel() string {
	if c != nil {
		if c.MonitorIntervalMin > 0 {
			return fmt.Sprintf("%d 分钟", c.MonitorIntervalMin)
		}
		if c.MonitorIntervalHours > 0 {
			return fmt.Sprintf("%d 小时", c.MonitorIntervalHours)
		}
	}
	return fmt.Sprintf("%d 分钟", defaultMonitorIntervalMin)
}
