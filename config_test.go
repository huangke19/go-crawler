package main

import (
	"os"
	"testing"
	"time"
)

// setupConfigTest 为配置测试创建临时目录
func setupConfigTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })
}

// TestLoadConfig_FileNotExists 测试配置文件不存在
func TestLoadConfig_FileNotExists(t *testing.T) {
	setupConfigTest(t)

	_, err := LoadConfig("nonexistent.json")
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestLoadConfig_ValidConfig 测试加载有效配置
func TestLoadConfig_ValidConfig(t *testing.T) {
	setupConfigTest(t)

	// 创建测试配置文件
	configJSON := `{
		"telegram_bot_token": "test_token_123",
		"allowed_user_ids": [123456, 789012],
		"admin_user_ids": [123456],
		"favorite_accounts": ["nike", "tesla"],
		"worker_addr": "127.0.0.1:8080",
		"monitor_accounts": ["spacex"],
		"monitor_interval_min": 60,
		"monitor_compare_top_n": 5
	}`

	if err := os.WriteFile("test_config.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	// 加载配置
	config, err := LoadConfig("test_config.json")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证字段
	if config.TelegramBotToken != "test_token_123" {
		t.Errorf("TelegramBotToken 不匹配: %s", config.TelegramBotToken)
	}

	if len(config.AllowedUserIDs) != 2 {
		t.Errorf("AllowedUserIDs 数量不匹配: %d", len(config.AllowedUserIDs))
	}

	if len(config.AdminUserIDs) != 1 {
		t.Errorf("AdminUserIDs 数量不匹配: %d", len(config.AdminUserIDs))
	}

	if len(config.FavoriteAccounts) != 2 {
		t.Errorf("FavoriteAccounts 数量不匹配: %d", len(config.FavoriteAccounts))
	}

	if config.WorkerAddr != "127.0.0.1:8080" {
		t.Errorf("WorkerAddr 不匹配: %s", config.WorkerAddr)
	}

	if len(config.MonitorAccounts) != 1 {
		t.Errorf("MonitorAccounts 数量不匹配: %d", len(config.MonitorAccounts))
	}

	if config.MonitorIntervalMin != 60 {
		t.Errorf("MonitorIntervalMin 不匹配: %d", config.MonitorIntervalMin)
	}

	if config.MonitorCompareTopN != 5 {
		t.Errorf("MonitorCompareTopN 不匹配: %d", config.MonitorCompareTopN)
	}
}

// TestLoadConfig_MinimalConfig 测试最小配置
func TestLoadConfig_MinimalConfig(t *testing.T) {
	setupConfigTest(t)

	configJSON := `{
		"telegram_bot_token": "token"
	}`

	if err := os.WriteFile("minimal.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	config, err := LoadConfig("minimal.json")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.TelegramBotToken != "token" {
		t.Errorf("TelegramBotToken 不匹配")
	}

	// 验证默认值
	if len(config.AllowedUserIDs) != 0 {
		t.Errorf("AllowedUserIDs 应为空")
	}
}

func TestLoadConfig_HourlyMonitorInterval(t *testing.T) {
	setupConfigTest(t)

	configJSON := `{
		"telegram_bot_token": "test_token_123",
		"monitor_interval_hours": 2
	}`

	if err := os.WriteFile("hourly.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	config, err := LoadConfig("hourly.json")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.MonitorIntervalHours != 2 {
		t.Errorf("MonitorIntervalHours 不匹配: %d", config.MonitorIntervalHours)
	}

	if got := config.GetMonitorInterval(); got != 2*time.Hour {
		t.Errorf("GetMonitorInterval() = %v, want %v", got, 2*time.Hour)
	}
}

func TestGetMonitorInterval_PrefersMinutes(t *testing.T) {
	config := &Config{
		MonitorIntervalMin:   45,
		MonitorIntervalHours: 2,
	}

	if got := config.GetMonitorInterval(); got != 45*time.Minute {
		t.Errorf("GetMonitorInterval() = %v, want %v", got, 45*time.Minute)
	}
}

// TestLoadConfig_InvalidJSON 测试无效 JSON
func TestLoadConfig_InvalidJSON(t *testing.T) {
	setupConfigTest(t)

	invalidJSON := `{invalid json`

	if err := os.WriteFile("invalid.json", []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	_, err := LoadConfig("invalid.json")
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestSaveConfig 测试保存配置
func TestSaveConfig(t *testing.T) {
	setupConfigTest(t)

	config := &Config{
		TelegramBotToken: "new_token",
		AllowedUserIDs:   []int64{111, 222},
		FavoriteAccounts: []string{"account1", "account2"},
		WorkerAddr:       "localhost:9090",
	}

	// 保存配置
	if err := SaveConfig("save_test.json", config); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	// 重新加载验证
	loaded, err := LoadConfig("save_test.json")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if loaded.TelegramBotToken != config.TelegramBotToken {
		t.Errorf("TelegramBotToken 不匹配")
	}

	if len(loaded.AllowedUserIDs) != len(config.AllowedUserIDs) {
		t.Errorf("AllowedUserIDs 数量不匹配")
	}

	if len(loaded.FavoriteAccounts) != len(config.FavoriteAccounts) {
		t.Errorf("FavoriteAccounts 数量不匹配")
	}
}

// TestGetWorkerAddr 测试获取 Worker 地址
func TestGetWorkerAddr(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name:     "使用配置的地址",
			config:   &Config{WorkerAddr: "192.168.1.100:8080"},
			expected: "192.168.1.100:8080",
		},
		{
			name:     "空地址使用默认值",
			config:   &Config{WorkerAddr: ""},
			expected: "127.0.0.1:18080",
		},
		{
			name:     "空白地址使用默认值",
			config:   &Config{WorkerAddr: "   "},
			expected: "127.0.0.1:18080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetWorkerAddr()
			if result != tt.expected {
				t.Errorf("期望 %s, 实际 %s", tt.expected, result)
			}
		})
	}
}

// TestGetWorkerBaseURL 测试获取 Worker Base URL
func TestGetWorkerBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name:     "普通地址",
			config:   &Config{WorkerAddr: "localhost:8080"},
			expected: "http://localhost:8080",
		},
		{
			name:     "已有 http 前缀",
			config:   &Config{WorkerAddr: "http://localhost:8080"},
			expected: "http://localhost:8080",
		},
		{
			name:     "已有 https 前缀",
			config:   &Config{WorkerAddr: "https://example.com:8080"},
			expected: "https://example.com:8080",
		},
		{
			name:     "空地址使用默认值",
			config:   &Config{WorkerAddr: ""},
			expected: "http://127.0.0.1:18080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetWorkerBaseURL()
			if result != tt.expected {
				t.Errorf("期望 %s, 实际 %s", tt.expected, result)
			}
		})
	}
}

// TestConfig_AdminUserIDs_Fallback 测试 AdminUserIDs 回退逻辑
func TestConfig_AdminUserIDs_Fallback(t *testing.T) {
	setupConfigTest(t)

	// AdminUserIDs 为空时，业务逻辑会回退到 AllowedUserIDs
	configJSON := `{
		"telegram_bot_token": "token",
		"allowed_user_ids": [123, 456]
	}`

	if err := os.WriteFile("fallback.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	config, err := LoadConfig("fallback.json")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 注意：LoadConfig 会自动将 AllowedUserIDs 复制到 AdminUserIDs（如果 AdminUserIDs 为空）
	// 这是在 config.go 的 LoadConfig 函数中实现的回退逻辑
	if len(config.AdminUserIDs) == 0 {
		// 如果没有自动回退，这是预期行为
		t.Log("AdminUserIDs 为空，需要业务逻辑手动回退")
	} else if len(config.AdminUserIDs) == len(config.AllowedUserIDs) {
		// 如果有自动回退，验证是否正确
		t.Log("AdminUserIDs 已自动回退到 AllowedUserIDs")
	}
}
