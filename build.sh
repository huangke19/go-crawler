#!/bin/bash

echo "=== 编译 Instagram Crawler ==="

# 编译主程序
echo "编译 crawler..."
go build -o crawler

# 编译守护进程管理工具
echo "编译 gobot..."
go build -tags gobot -o gobot

echo ""
echo "✅ 编译完成！"
echo ""
echo "可执行文件:"
echo "  ./crawler - 主程序（登录、下载）"
echo "  ./gobot   - 守护进程管理工具"
echo ""
echo "使用方式:"
echo "  ./crawler login            # 首次登录"
echo "  ./gobot start              # 启动 bot 后台服务"
echo "  ./gobot worker start       # 启动 worker 后台服务"
echo "  ./gobot status             # 查看 bot 状态"
echo "  ./gobot worker status      # 查看 worker 状态"
echo "  ./gobot launchd install    # 将 bot 接入 launchd 常驻"
echo "  ./gobot launchd status     # 查看 launchd 托管状态"
echo ""
echo "💡 提示: bot/worker 守护进程均支持 caffeinate 防休眠"
