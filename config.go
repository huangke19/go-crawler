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
)

type Config struct {
	TelegramBotToken string   `json:"telegram_bot_token"`
	AllowedUserIDs   []int64  `json:"allowed_user_ids"`
	AdminUserIDs     []int64  `json:"admin_user_ids"`
	FavoriteAccounts []string `json:"favorite_accounts"`
	WorkerAddr       string   `json:"worker_addr"`
}

// LoadConfig 从指定路径读取 `config.json` 并做必要的兼容与默认值填充：
// - `telegram_bot_token` 必须存在且不可为占位符；
// - `worker_addr` 为空时使用默认监听地址；
// - `admin_user_ids` 未配置但 `allowed_user_ids` 有值时，自动回退为同一组 ID（兼容旧配置）。
func LoadConfig(path string) (*Config, error) {
	var config Config
	if err := loadJSONFile(path, &config); err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	if config.TelegramBotToken == "" || config.TelegramBotToken == "YOUR_BOT_TOKEN_HERE" {
		return nil, fmt.Errorf("请在 config.json 中配置有效的 telegram_bot_token")
	}

	if strings.TrimSpace(config.WorkerAddr) == "" {
		config.WorkerAddr = defaultWorkerListenAddr
	}

	if len(config.AdminUserIDs) == 0 && len(config.AllowedUserIDs) > 0 {
		config.AdminUserIDs = append([]int64(nil), config.AllowedUserIDs...)
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
