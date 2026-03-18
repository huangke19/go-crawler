#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

echo "=== 启动 go-crawler 全部服务 ==="

start_with_gobot() {
	local label="$1"
	shift

	if [[ ! -x ./gobot ]]; then
		echo "❌ 未找到可执行文件 ./gobot，请先运行 ./build.sh"
		exit 1
	fi

	if output=$("$@" 2>&1); then
		echo "✅ ${label} 启动成功"
		echo "$output"
	else
		echo "❌ ${label} 启动失败"
		echo "$output"
		exit 1
	fi
}

start_with_gobot "Bot" ./gobot start
echo ""
start_with_gobot "Worker" ./gobot worker start

echo ""
echo "✅ 全部服务已启动"
