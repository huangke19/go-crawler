package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// botCommand 对应 Telegram setMyCommands API 的单条命令。
type botCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// botCommands 是要注册到 Telegram 的完整命令列表。
var botCommands = []botCommand{
	{"start", "开始使用"},
	{"help", "查看帮助信息"},
	{"download", "下载帖子（按钮交互）"},
	{"dl", "download 的简写"},
	{"status", "查看 bot 与 worker 状态"},
	{"control", "管理员控制 worker 启停"},
	{"favorites", "管理常用账户列表，仅管理员"},
	{"monitor", "查看监控账户状态，仅管理员"},
}

func handleSetupBot() {
	fmt.Println("=== Telegram Bot 命令设置 ===")
	fmt.Println()

	config, err := LoadConfig("config.json")
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		fmt.Println("\n请先配置 config.json，或手动将以下命令列表发送给 @BotFather (/setcommands)：")
		printCommandList()
		os.Exit(1)
	}

	if err := setMyCommands(config.TelegramBotToken); err != nil {
		fmt.Printf("❌ 设置命令失败: %v\n", err)
		fmt.Println("\n可以手动将以下命令列表发送给 @BotFather (/setcommands)：")
		printCommandList()
		os.Exit(1)
	}

	fmt.Println("✅ Bot 命令菜单已更新！")
	fmt.Println()
	fmt.Println("已注册以下命令：")
	for _, cmd := range botCommands {
		fmt.Printf("  /%s — %s\n", cmd.Command, cmd.Description)
	}
	fmt.Println()
	fmt.Println("在 Telegram 中输入 / 即可看到命令提示。")
}

// setMyCommands 调用 Telegram Bot API 的 setMyCommands 接口。
func setMyCommands(token string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", token)

	payload, err := json.Marshal(map[string]any{
		"commands": botCommands,
	})
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("Telegram API 返回错误: %s", result.Description)
	}

	return nil
}

// printCommandList 打印供手动复制给 BotFather 的命令列表。
func printCommandList() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	for _, cmd := range botCommands {
		fmt.Printf("%s - %s\n", cmd.Command, cmd.Description)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
