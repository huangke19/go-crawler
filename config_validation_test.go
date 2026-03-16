package main

import (
	"os"
	"testing"
)

// TestValidateBotToken 测试 Bot Token 验证
func TestValidateBotToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "有效的 Token",
			token:   "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			wantErr: false,
		},
		{
			name:    "空 Token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "占位符 Token",
			token:   "YOUR_BOT_TOKEN_HERE",
			wantErr: true,
		},
		{
			name:    "格式错误（缺少冒号）",
			token:   "123456789ABCdef",
			wantErr: true,
		},
		{
			name:    "格式错误（冒号前不是数字）",
			token:   "abc:ABCdef",
			wantErr: true,
		},
		{
			name:    "带空格的 Token",
			token:   "  123456789:ABCdef  ",
			wantErr: false, // TrimSpace 后应该有效
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBotToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBotToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateWorkerAddr 测试 Worker 地址验证
func TestValidateWorkerAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "有效地址",
			addr:    "127.0.0.1:18080",
			wantErr: false,
		},
		{
			name:    "有效地址（localhost）",
			addr:    "localhost:8080",
			wantErr: false,
		},
		{
			name:    "空地址（使用默认值）",
			addr:    "",
			wantErr: false,
		},
		{
			name:    "带 http 前缀",
			addr:    "http://127.0.0.1:18080",
			wantErr: false,
		},
		{
			name:    "缺少端口号",
			addr:    "127.0.0.1",
			wantErr: true,
		},
		{
			name:    "端口号无效",
			addr:    "127.0.0.1:abc",
			wantErr: true,
		},
		{
			name:    "端口号超出范围（太小）",
			addr:    "127.0.0.1:0",
			wantErr: true,
		},
		{
			name:    "端口号超出范围（太大）",
			addr:    "127.0.0.1:99999",
			wantErr: true,
		},
		{
			name:    "主机名为空",
			addr:    ":8080",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkerAddr(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkerAddr() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateUserIDs 测试用户 ID 验证
func TestValidateUserIDs(t *testing.T) {
	tests := []struct {
		name    string
		ids     []int64
		wantErr bool
	}{
		{
			name:    "有效的用户 ID",
			ids:     []int64{123456789, 987654321},
			wantErr: false,
		},
		{
			name:    "空列表",
			ids:     []int64{},
			wantErr: false,
		},
		{
			name:    "包含零值",
			ids:     []int64{123456789, 0},
			wantErr: true,
		},
		{
			name:    "包含负数",
			ids:     []int64{123456789, -1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUserIDs("test_field", tt.ids)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUserIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateAccounts 测试账户名验证
func TestValidateAccounts(t *testing.T) {
	tests := []struct {
		name     string
		accounts []string
		wantErr  bool
	}{
		{
			name:     "有效的账户名",
			accounts: []string{"nike", "instagram", "natgeo"},
			wantErr:  false,
		},
		{
			name:     "带下划线的账户名",
			accounts: []string{"user_name", "test_123"},
			wantErr:  false,
		},
		{
			name:     "带点号的账户名",
			accounts: []string{"user.name", "test.123"},
			wantErr:  false,
		},
		{
			name:     "空列表",
			accounts: []string{},
			wantErr:  false,
		},
		{
			name:     "包含空字符串",
			accounts: []string{"nike", ""},
			wantErr:  true,
		},
		{
			name:     "账户名过长",
			accounts: []string{"a123456789012345678901234567890"},
			wantErr:  true,
		},
		{
			name:     "包含非法字符",
			accounts: []string{"user@name"},
			wantErr:  true,
		},
		{
			name:     "以点号开头",
			accounts: []string{".username"},
			wantErr:  true,
		},
		{
			name:     "以点号结尾",
			accounts: []string{"username."},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccounts("test_field", tt.accounts)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAccounts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateIntRange 测试整数范围验证
func TestValidateIntRange(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{
			name:    "有效值",
			value:   50,
			min:     1,
			max:     100,
			wantErr: false,
		},
		{
			name:    "最小值",
			value:   1,
			min:     1,
			max:     100,
			wantErr: false,
		},
		{
			name:    "最大值",
			value:   100,
			min:     1,
			max:     100,
			wantErr: false,
		},
		{
			name:    "零值（使用默认值）",
			value:   0,
			min:     1,
			max:     100,
			wantErr: false,
		},
		{
			name:    "小于最小值",
			value:   -1,
			min:     1,
			max:     100,
			wantErr: true,
		},
		{
			name:    "大于最大值",
			value:   101,
			min:     1,
			max:     100,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIntRange("test_field", tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIntRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfigValidate 测试完整配置验证
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "有效配置",
			config: &Config{
				TelegramBotToken:   "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
				AllowedUserIDs:     []int64{123456789},
				AdminUserIDs:       []int64{123456789},
				FavoriteAccounts:   []string{"nike", "instagram"},
				WorkerAddr:         "127.0.0.1:18080",
				MonitorAccounts:    []string{"nasa"},
				MonitorIntervalMin: 30,
				MonitorCompareTopN: 10,
			},
			wantErr: false,
		},
		{
			name: "无效 Token",
			config: &Config{
				TelegramBotToken: "",
			},
			wantErr: true,
		},
		{
			name: "无效 Worker 地址",
			config: &Config{
				TelegramBotToken: "123456789:ABCdef",
				WorkerAddr:       "invalid",
			},
			wantErr: true,
		},
		{
			name: "无效用户 ID",
			config: &Config{
				TelegramBotToken: "123456789:ABCdef",
				AllowedUserIDs:   []int64{0},
			},
			wantErr: true,
		},
		{
			name: "无效账户名",
			config: &Config{
				TelegramBotToken: "123456789:ABCdef",
				FavoriteAccounts: []string{"@invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidationErrors 测试多个验证错误
func TestValidationErrors(t *testing.T) {
	config := &Config{
		TelegramBotToken: "",
		WorkerAddr:       "invalid",
		AllowedUserIDs:   []int64{0},
	}

	err := config.Validate()
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}

	// 验证错误信息包含多个错误
	errMsg := err.Error()
	if !contains(errMsg, "telegram_bot_token") {
		t.Error("错误信息应包含 telegram_bot_token")
	}
	if !contains(errMsg, "worker_addr") {
		t.Error("错误信息应包含 worker_addr")
	}
	if !contains(errMsg, "allowed_user_ids") {
		t.Error("错误信息应包含 allowed_user_ids")
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParseUserIDs 测试用户 ID 解析
func TestParseUserIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int64
	}{
		{
			name:     "单个 ID",
			input:    "123456789",
			expected: []int64{123456789},
		},
		{
			name:     "多个 ID",
			input:    "123456789,987654321,111222333",
			expected: []int64{123456789, 987654321, 111222333},
		},
		{
			name:     "带空格",
			input:    "123456789, 987654321, 111222333",
			expected: []int64{123456789, 987654321, 111222333},
		},
		{
			name:     "空字符串",
			input:    "",
			expected: []int64{},
		},
		{
			name:     "包含无效值",
			input:    "123456789,invalid,987654321",
			expected: []int64{123456789, 987654321},
		},
		{
			name:     "包含负数",
			input:    "123456789,-1,987654321",
			expected: []int64{123456789, 987654321},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseUserIDs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseUserIDs() 返回 %d 个，期望 %d 个", len(result), len(tt.expected))
				return
			}
			for i, id := range result {
				if id != tt.expected[i] {
					t.Errorf("parseUserIDs()[%d] = %d, 期望 %d", i, id, tt.expected[i])
				}
			}
		})
	}
}

// TestApplyEnvOverrides 测试环境变量覆盖
func TestApplyEnvOverrides(t *testing.T) {
	// 保存原始环境变量
	oldToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	oldAddr := os.Getenv("WORKER_ADDR")
	oldWorkerToken := os.Getenv("WORKER_API_TOKEN")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", oldToken)
		os.Setenv("WORKER_ADDR", oldAddr)
		os.Setenv("WORKER_API_TOKEN", oldWorkerToken)
	}()

	// 设置测试环境变量
	os.Setenv("TELEGRAM_BOT_TOKEN", "999999999:TestToken")
	os.Setenv("WORKER_ADDR", "localhost:9999")
	os.Setenv("WORKER_API_TOKEN", "test-worker-token")

	config := &Config{
		TelegramBotToken: "123456789:OriginalToken",
		WorkerAddr:       "127.0.0.1:8080",
	}

	applyEnvOverrides(config)

	if config.TelegramBotToken != "999999999:TestToken" {
		t.Errorf("TelegramBotToken 未被环境变量覆盖")
	}

	if config.WorkerAddr != "localhost:9999" {
		t.Errorf("WorkerAddr 未被环境变量覆盖")
	}
	if config.WorkerAPIToken != "test-worker-token" {
		t.Errorf("WorkerAPIToken 未被环境变量覆盖")
	}
}
