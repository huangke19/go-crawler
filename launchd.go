package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const botLaunchdLabel = "com.instagram.bot"

func handleLaunchdCLI(args []string) {
	if len(args) < 1 {
		printLaunchdUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		msg, err := InstallBotLaunchd()
		handleServiceResult("安装 launchd 失败", msg, err)
	case "uninstall":
		msg, err := UninstallBotLaunchd()
		handleServiceResult("卸载 launchd 失败", msg, err)
	case "status":
		if err := PrintBotLaunchdStatus(); err != nil {
			fmt.Printf("❌ 查询 launchd 状态失败: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printLaunchdUsage()
	default:
		fmt.Printf("未知 launchd 命令: %s\n\n", args[0])
		printLaunchdUsage()
		os.Exit(1)
	}
}

func printLaunchdUsage() {
	fmt.Println("launchd 托管命令")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  gobot launchd <install|uninstall|status>")
	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  - 仅托管 crawler bot 常驻")
	fmt.Println("  - worker 不常驻，按需用 /control 或 gobot worker ... 管理")
}

func botLaunchdPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", botLaunchdLabel+".plist"), nil
}

func getCrawlerPathForLaunchd() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	crawlerPath := filepath.Join(filepath.Dir(executable), "crawler")
	if _, err := os.Stat(crawlerPath); err != nil {
		return "", fmt.Errorf("未找到 crawler 可执行文件: %s，请先执行 ./build.sh", crawlerPath)
	}
	return crawlerPath, nil
}

func buildLaunchdPlistContent(crawlerPath string) string {
	workDir := filepath.Dir(crawlerPath)
	logPath := filepath.Join(workDir, "gobot.log")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>bot</string>
    </array>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, botLaunchdLabel, crawlerPath, workDir, logPath, logPath)
}

func InstallBotLaunchd() (string, error) {
	plistPath, err := botLaunchdPlistPath()
	if err != nil {
		return "", err
	}

	crawlerPath, err := getCrawlerPathForLaunchd()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return "", fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	content := buildLaunchdPlistContent(crawlerPath)
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入 plist 失败: %w", err)
	}

	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput(); err != nil {
		return "", fmt.Errorf("加载 launchd 失败: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	return fmt.Sprintf("launchd 已安装并启用\nLabel: %s\nPlist: %s\n托管目标: crawler bot", botLaunchdLabel, plistPath), nil
}

func UninstallBotLaunchd() (string, error) {
	plistPath, err := botLaunchdPlistPath()
	if err != nil {
		return "", err
	}

	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("删除 plist 失败: %w", err)
	}

	return fmt.Sprintf("launchd 托管已卸载\nLabel: %s\nPlist: %s", botLaunchdLabel, plistPath), nil
}

func PrintBotLaunchdStatus() error {
	plistPath, err := botLaunchdPlistPath()
	if err != nil {
		return err
	}

	_, statErr := os.Stat(plistPath)
	installed := statErr == nil

	loaded, pid, detail, err := getLaunchdRuntime()
	if err != nil {
		return err
	}

	if installed {
		fmt.Printf("✅ 已安装 plist: %s\n", plistPath)
	} else {
		fmt.Printf("❌ 未安装 plist: %s\n", plistPath)
	}

	if loaded {
		if pid > 0 {
			fmt.Printf("✅ launchd 已加载: %s (PID: %d)\n", botLaunchdLabel, pid)
		} else {
			fmt.Printf("✅ launchd 已加载: %s\n", botLaunchdLabel)
		}
	} else {
		fmt.Printf("❌ launchd 未加载: %s\n", botLaunchdLabel)
		if detail != "" {
			fmt.Printf("详情: %s\n", detail)
		}
	}

	fmt.Println("托管范围: 仅 bot 常驻，worker 按需手动启动")
	return nil
}

func getLaunchdRuntime() (loaded bool, pid int, detail string, err error) {
	out, cmdErr := exec.Command("launchctl", "list", botLaunchdLabel).CombinedOutput()
	txt := strings.TrimSpace(string(out))
	if cmdErr != nil {
		if txt == "" || strings.Contains(strings.ToLower(txt), "could not find service") {
			return false, 0, "未在 launchctl 中注册", nil
		}
		return false, 0, "", fmt.Errorf("launchctl 查询失败: %v (%s)", cmdErr, txt)
	}

	return true, parseLaunchdPID(txt), "", nil
}

func parseLaunchdPID(output string) int {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "pid") {
			if v := extractFirstInt(line); v > 0 {
				return v
			}
		}
	}
	return 0
}

func extractFirstInt(s string) int {
	start := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			start = i
			break
		}
	}
	if start == -1 {
		return 0
	}

	end := start
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	v, _ := strconv.Atoi(s[start:end])
	return v
}
