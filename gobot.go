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

	command := os.Args[1]

	switch command {
	case "start":
		if err := StartDaemon(); err != nil {
			fmt.Printf("❌ 启动失败: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := StopDaemon(); err != nil {
			fmt.Printf("❌ 停止失败: %v\n", err)
			os.Exit(1)
		}
	case "restart":
		if err := RestartDaemon(); err != nil {
			fmt.Printf("❌ 重启失败: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := StatusDaemon(); err != nil {
			fmt.Printf("❌ 查询状态失败: %v\n", err)
			os.Exit(1)
		}
	case "logs":
		showLogs()
	case "help", "-h", "--help":
		printGobotUsage()
	default:
		fmt.Printf("未知命令: %s\n\n", command)
		printGobotUsage()
		os.Exit(1)
	}
}

func printGobotUsage() {
	fmt.Println("Instagram Bot 守护进程管理工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  gobot start      启动 Bot 服务（后台运行）")
	fmt.Println("  gobot stop       停止 Bot 服务")
	fmt.Println("  gobot restart    重启 Bot 服务")
	fmt.Println("  gobot status     查看 Bot 运行状态")
	fmt.Println("  gobot logs       查看完整日志")
	fmt.Println("  gobot help       显示帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  gobot start      # 启动服务")
	fmt.Println("  gobot status     # 查看状态")
	fmt.Println("  gobot restart    # 重启服务")
	fmt.Println("  gobot stop       # 停止服务")
}

func showLogs() {
	executable, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ 获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	logPath := logFile
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Println("❌ 日志文件不存在")
		os.Exit(1)
	}

	fmt.Printf("日志文件: %s\n", logPath)
	fmt.Println("---")

	// 使用 tail 命令显示日志（macOS/Linux）
	cmd := fmt.Sprintf("tail -f %s", logPath)
	fmt.Printf("执行: %s\n", cmd)
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println()

	_ = executable
	os.Exit(0)
}
