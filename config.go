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

func LoadConfig(path string) (*Config, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
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

func (c *Config) GetWorkerAddr() string {
	if c == nil || strings.TrimSpace(c.WorkerAddr) == "" {
		return defaultWorkerListenAddr
	}
	return strings.TrimSpace(c.WorkerAddr)
}

func (c *Config) GetWorkerBaseURL() string {
	addr := c.GetWorkerAddr()
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}
