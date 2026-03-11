# Mac 防休眠与常驻建议

## 目标

保证手机端 Telegram 始终可控：

- `bot` 长期在线（接收命令）
- `worker` 按需启停（执行下载）

## 推荐策略

### 1) 使用 `gobot` 启动服务（已集成 `caffeinate`）

```bash
./gobot start           # 启动 bot
./gobot worker start    # 启动 worker
```

守护启动时使用 `caffeinate -i`，在服务运行期间防止系统空闲休眠。

### 2) 使用 `launchd` 托管 `crawler bot`（推荐）

核心原则：`KeepAlive` 应该盯住长驻进程 `crawler bot`，不要盯 `gobot start` 这种一次性命令。

现在可以直接用 `gobot` 一键接入：

```bash
./gobot launchd install
./gobot launchd status
./gobot launchd uninstall
```

说明：

- `install` 会自动生成并加载 `~/Library/LaunchAgents/com.instagram.bot.plist`
- 托管目标仅 `crawler bot`
- `worker` 不常驻，继续通过 `/control` 或 `./gobot worker ...` 按需管理

## 验证

```bash
./gobot status
./gobot worker status
ps aux | grep caffeinate | grep -v grep
```

## 关键结论

- 只有 `bot` 在线时，手机消息才能被处理。
- 因此建议：`bot` 常驻，`worker` 用 `/control` 或 `gobot worker ...` 管理。
