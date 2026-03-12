//go:build gobot
// +build gobot

// ============================================================================
// gobot.go - 守护进程 CLI 工具
// ============================================================================
//
// 职责：
//   - 提供命令行接口管理 Bot 和 Worker 后台服务
//   - 支持启动/停止/重启/查看状态/查看日志
//   - 支持 launchd 集成（macOS 开机自启）
//
// 命令格式：
//   gobot [service] <command>
//
// 服务类型：
//   - bot：Telegram Bot 服务（默认）
//   - worker：下载执行服务
//
// 命令：
//   - start：启动服务
//   - stop：停止服务
//   - restart：重启服务
//   - status：查看服务状态
//   - logs：查看服务日志
//   - launchd：macOS 开机自启管理
//
// 使用示例：
//   gobot start                # 启动 bot
//   gobot bot start            # 启动 bot（显式指定）
//   gobot worker start         # 启动 worker
//   gobot worker status        # 查看 worker 状态
//   gobot worker logs          # 查看 worker 日志
//   gobot launchd install      # 安装 bot 开机自启
//
// ============================================================================

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printGobotUsage()
		return
	}

	if os.Args[1] == "launchd" {
		handleLaunchdCLI(os.Args[2:])
		return
	}

	service := "bot"
	commandIndex := 1

	if os.Args[1] == "bot" || os.Args[1] == "worker" {
		service = os.Args[1]
		commandIndex = 2
	}

	if len(os.Args) <= commandIndex {
		printGobotUsage()
		os.Exit(1)
	}

	command := os.Args[commandIndex]

	switch command {
	case "start":
		msg, err := StartServiceDaemon(service)
		handleServiceResult("启动失败", msg, err)
	case "stop":
		msg, err := StopServiceDaemon(service)
		handleServiceResult("停止失败", msg, err)
	case "restart":
		msg, err := RestartServiceDaemon(service)
		handleServiceResult("重启失败", msg, err)
	case "status":
		if err := PrintServiceStatus(service); err != nil {
			fmt.Printf("❌ 查询状态失败: %v\n", err)
			os.Exit(1)
		}
	case "logs":
		showLogs(service)
	case "help", "-h", "--help":
		printGobotUsage()
	default:
		fmt.Printf("未知命令: %s\n\n", command)
		printGobotUsage()
		os.Exit(1)
	}
}

func handleServiceResult(errPrefix, message string, err error) {
	if err != nil {
		fmt.Printf("❌ %s: %v\n", errPrefix, err)
		os.Exit(1)
	}
	fmt.Printf("✅ %s\n", message)
}

func printGobotUsage() {
	fmt.Println("Instagram Bot/Worker 守护进程管理工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  gobot <start|stop|restart|status|logs>")
	fmt.Println("  gobot bot <start|stop|restart|status|logs>")
	fmt.Println("  gobot worker <start|stop|restart|status|logs>")
	fmt.Println("  gobot launchd <install|uninstall|status>")
	fmt.Println("  gobot help")
	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  - 不带服务名时默认管理 bot")
	fmt.Println("  - worker 为下载执行进程，可由 Telegram /control 控制")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  gobot start            # 启动 bot")
	fmt.Println("  gobot worker start     # 启动 worker")
	fmt.Println("  gobot worker status    # 查看 worker 状态")
	fmt.Println("  gobot worker stop      # 停止 worker")
	fmt.Println("  gobot launchd install  # 安装并启用 bot 开机常驻")
	fmt.Println("  gobot launchd status   # 查看 launchd 托管状态")
}

func showLogs(service string) {
	spec, err := getServiceSpec(service)
	if err != nil {
		fmt.Printf("❌ 服务类型错误: %v\n", err)
		os.Exit(1)
	}

	logPath := spec.logFile
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		logPath = getWorkDir() + string(os.PathSeparator) + spec.logFile
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Printf("❌ 日志文件不存在: %s\n", logPath)
		os.Exit(1)
	}

	fmt.Printf("日志文件: %s\n", logPath)
	fmt.Println("---")
	if err := ShowLastLogs(logPath, 100); err != nil {
		fmt.Printf("❌ 读取日志失败: %v\n", err)
		os.Exit(1)
	}
}
