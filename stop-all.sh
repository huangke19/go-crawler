#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

echo "=== 停止 go-crawler 相关进程 ==="

stop_with_gobot() {
	local label="$1"
	shift

	if [[ -x ./gobot ]]; then
		if "$@" >/dev/null 2>&1; then
			echo "✅ 已停止 ${label}"
		else
			echo "ℹ️  ${label} 未运行或停止时出现提示，继续兜底清理"
		fi
	else
		echo "ℹ️  未找到 ./gobot，跳过 ${label} 的守护停止"
	fi
}

kill_pattern_if_running() {
	local label="$1"
	local pattern="$2"

	if pkill -f "$pattern" >/dev/null 2>&1; then
		echo "✅ 已清理直接启动的 ${label} 进程"
	else
		echo "ℹ️  未发现直接启动的 ${label} 进程"
	fi
}

kill_service_processes() {
	local service="$1"
	local pids
	pids="$(ps ax -o pid=,command= | awk -v svc="$service" '
		{
			pid = $1
			$1 = ""
			cmd = substr($0, 2)
			if (cmd ~ /(^|\/)(crawler|go-crawler)([[:space:]]|$)/ && cmd ~ ("(^|[[:space:]])" svc "([[:space:]]|$)")) {
				print pid
			}
		}
	')"

	if [[ -z "$pids" ]]; then
		echo "ℹ️  未发现 ${service} 相关进程"
		return
	fi

	echo "🧹 清理 ${service} 进程: ${pids//$'\n'/ }"
	echo "$pids" | xargs kill -TERM >/dev/null 2>&1 || true
	sleep 1

	# 二次确认，残留则强制结束
	local remain
	remain="$(echo "$pids" | xargs -I{} sh -c 'kill -0 {} >/dev/null 2>&1 && echo {}' 2>/dev/null || true)"
	if [[ -n "$remain" ]]; then
		echo "$remain" | xargs kill -KILL >/dev/null 2>&1 || true
	fi
}

stop_with_gobot "Bot" ./gobot stop
stop_with_gobot "Worker" ./gobot worker stop

kill_pattern_if_running "Bot" "$PROJECT_DIR/crawler bot"
kill_pattern_if_running "Worker" "$PROJECT_DIR/crawler worker"
kill_service_processes "bot"
kill_service_processes "worker"

echo ""
echo "✅ 停止完成"
