package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	TelegramBotToken   string   `json:"telegram_bot_token"`
	AllowedUserIDs     []int64  `json:"allowed_user_ids"`
	FavoriteAccounts   []string `json:"favorite_accounts"`
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

	return &config, nil
}
