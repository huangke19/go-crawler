#!/bin/bash

echo "=== 编译 Instagram Crawler ==="

# 编译主程序
echo "编译 crawler..."
go build -o crawler main.go auth.go login.go scraper.go downloader.go bot.go config.go setup_bot.go

# 编译守护进程管理工具
echo "编译 gobot..."
go build -o gobot gobot.go daemon.go auth.go login.go scraper.go downloader.go bot.go config.go

echo ""
echo "✅ 编译完成！"
echo ""
echo "可执行文件:"
echo "  ./crawler - 主程序（登录、下载）"
echo "  ./gobot   - 守护进程管理工具"
echo ""
echo "使用方式:"
echo "  ./crawler login          # 首次登录"
echo "  ./gobot start            # 启动后台服务（自动防止 Mac 休眠）"
echo "  ./gobot status           # 查看状态"
echo "  ./gobot stop             # 停止服务"
echo ""
echo "💡 提示: Bot 已集成 caffeinate，启动时自动防止 Mac 休眠"
