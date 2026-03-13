package main

import (
	"fmt"
)

func handleSetupBot() {
	fmt.Println("=== Telegram Bot 命令设置 ===")
	fmt.Println()
	fmt.Println("请按以下步骤在 Telegram 中设置命令菜单：")
	fmt.Println()
	fmt.Println("1. 在 Telegram 中找到 @BotFather")
	fmt.Println("2. 发送 /setcommands")
	fmt.Println("3. 选择你的 bot")
	fmt.Println("4. 复制下面的命令列表并发送：")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("start - 开始使用")
	fmt.Println("help - 查看帮助信息")
	fmt.Println("download - 下载帖子（按钮交互）")
	fmt.Println("dl - download 的简写")
	fmt.Println("status - 查看 bot 与 worker 状态")
	fmt.Println("control - 管理员控制 worker 启停")
	fmt.Println("favorites - 管理常用账户列表，仅管理员")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("5. 完成后，在 Telegram 中输入 / 即可看到命令提示")
	fmt.Println()
	fmt.Println("提示：你也可以使用 /setdescription 设置 bot 描述")
	fmt.Println("      使用 /setabouttext 设置 bot 简介")
}
