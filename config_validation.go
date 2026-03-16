// ============================================================================
// config_validation.go - 配置验证
// ============================================================================
//
// 职责：
//   - 配置完整性验证
//   - 配置合理性检查
//   - 详细的错误提示
//
// ============================================================================

package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ValidationError 配置验证错误
type ValidationError struct {
	Field   string // 字段名
	Value   string // 字段值
	Message string // 错误信息
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("配置验证失败 [%s=%s]: %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("配置验证失败 [%s]: %s", e.Field, e.Message)
}

// ValidationErrors 多个验证错误
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "配置验证失败"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("配置验证失败，发现 %d 个错误:\n", len(e)))
	for i, err := range e {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// Validate 验证配置的完整性和合理性
func (c *Config) Validate() error {
	var errors ValidationErrors

	// 验证 Telegram Bot Token
	if err := validateBotToken(c.TelegramBotToken); err != nil {
		errors = append(errors, err)
	}

	// 验证 Worker 地址
	if err := validateWorkerAddr(c.WorkerAddr); err != nil {
		errors = append(errors, err)
	}

	// 验证用户 ID
	if err := validateUserIDs("allowed_user_ids", c.AllowedUserIDs); err != nil {
		errors = append(errors, err)
	}
	if err := validateUserIDs("admin_user_ids", c.AdminUserIDs); err != nil {
		errors = append(errors, err)
	}

	// 验证账户名
	if err := validateAccounts("favorite_accounts", c.FavoriteAccounts); err != nil {
		errors = append(errors, err)
	}
	if err := validateAccounts("monitor_accounts", c.MonitorAccounts); err != nil {
		errors = append(errors, err)
	}

	// 验证数值范围
	if err := validateIntRange("monitor_interval_min", c.MonitorIntervalMin, 1, 1440); err != nil {
		errors = append(errors, err)
	}
	if err := validateIntRange("monitor_compare_top_n", c.MonitorCompareTopN, 1, 100); err != nil {
		errors = append(errors, err)
	}
	if err := validateIntRange("max_concurrent_downloads", c.MaxConcurrentDownloads, 0, 100); err != nil {
		errors = append(errors, err)
	}
	if err := validateIntRange("posts_cache_expiry_hours", c.PostsCacheExpiryHours, 0, 168); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validateBotToken 验证 Telegram Bot Token
func validateBotToken(token string) *ValidationError {
	token = strings.TrimSpace(token)

	if token == "" {
		return &ValidationError{
			Field:   "telegram_bot_token",
			Message: "不能为空，请在 config.json 中配置有效的 Bot Token",
		}
	}

	if token == "YOUR_BOT_TOKEN_HERE" {
		return &ValidationError{
			Field:   "telegram_bot_token",
			Value:   token,
			Message: "请替换为真实的 Bot Token（从 @BotFather 获取）",
		}
	}

	// Telegram Bot Token 格式：数字:字母数字字符
	// 例如：123456789:ABCdefGHIjklMNOpqrsTUVwxyz
	matched, _ := regexp.MatchString(`^\d+:[A-Za-z0-9_-]+$`, token)
	if !matched {
		return &ValidationError{
			Field:   "telegram_bot_token",
			Value:   token[:min(20, len(token))] + "...",
			Message: "格式不正确，应为 '数字:字母数字' 格式（例如：123456789:ABCdef...）",
		}
	}

	return nil
}

// validateWorkerAddr 验证 Worker 地址
func validateWorkerAddr(addr string) *ValidationError {
	addr = strings.TrimSpace(addr)

	// 空地址会使用默认值，不算错误
	if addr == "" {
		return nil
	}

	// 移除可能的 http:// 或 https:// 前缀
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	// 验证格式：host:port
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return &ValidationError{
			Field:   "worker_addr",
			Value:   addr,
			Message: "格式不正确，应为 'host:port' 格式（例如：127.0.0.1:18080）",
		}
	}

	host, portStr := parts[0], parts[1]

	// 验证主机名不为空
	if host == "" {
		return &ValidationError{
			Field:   "worker_addr",
			Value:   addr,
			Message: "主机名不能为空",
		}
	}

	// 验证端口号
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &ValidationError{
			Field:   "worker_addr",
			Value:   addr,
			Message: fmt.Sprintf("端口号无效: %s", portStr),
		}
	}

	if port < 1 || port > 65535 {
		return &ValidationError{
			Field:   "worker_addr",
			Value:   addr,
			Message: fmt.Sprintf("端口号超出范围 (1-65535): %d", port),
		}
	}

	return nil
}

// validateUserIDs 验证用户 ID 列表
func validateUserIDs(field string, ids []int64) *ValidationError {
	// 空列表是允许的
	if len(ids) == 0 {
		return nil
	}

	for i, id := range ids {
		if id <= 0 {
			return &ValidationError{
				Field:   field,
				Value:   fmt.Sprintf("index %d: %d", i, id),
				Message: "用户 ID 必须为正整数",
			}
		}
	}

	return nil
}

// validateAccounts 验证账户名列表
func validateAccounts(field string, accounts []string) *ValidationError {
	// 空列表是允许的
	if len(accounts) == 0 {
		return nil
	}

	// Instagram 账户名规则：
	// - 长度 1-30 个字符
	// - 只能包含字母、数字、下划线、点号
	// - 不能以点号开头或结尾
	accountRegex := regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9_.]{0,28}[a-zA-Z0-9_])?$`)

	for i, account := range accounts {
		account = strings.TrimSpace(account)

		if account == "" {
			return &ValidationError{
				Field:   field,
				Value:   fmt.Sprintf("index %d", i),
				Message: "账户名不能为空",
			}
		}

		if len(account) > 30 {
			return &ValidationError{
				Field:   field,
				Value:   account,
				Message: fmt.Sprintf("账户名过长（最多 30 个字符）: %d 个字符", len(account)),
			}
		}

		if !accountRegex.MatchString(account) {
			return &ValidationError{
				Field:   field,
				Value:   account,
				Message: "账户名格式不正确（只能包含字母、数字、下划线、点号，且不能以点号开头或结尾）",
			}
		}
	}

	return nil
}

// validateIntRange 验证整数范围
func validateIntRange(field string, value, min, max int) *ValidationError {
	// 0 值通常表示使用默认值，不算错误
	if value == 0 {
		return nil
	}

	if value < min || value > max {
		return &ValidationError{
			Field:   field,
			Value:   fmt.Sprintf("%d", value),
			Message: fmt.Sprintf("超出有效范围 (%d-%d)", min, max),
		}
	}

	return nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
