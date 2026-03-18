# Instagram 爬虫工具 (go-crawler)

一个基于 Go 的 Instagram 下载工具，支持本地 CLI 下载，以及 Telegram Bot 远程控制（Bot + Worker 双进程）。

## 特性

- 支持图片、视频、轮播下载
- 会话持久化（`crawler login` 后复用 Cookie）
- Bot/Worker 分离，下载任务不阻塞 Bot 交互
- 三级缓存（媒体/帖子/文件）提升重复任务速度
- `gobot` 守护管理（start/stop/restart/status/logs）
- Worker 接口安全增强：
  - 配置 `WORKER_API_TOKEN` 时，必须携带 `X-Worker-Token`
  - 未配置 token 时，仅允许本机来源访问
- macOS 防休眠：优先使用 `caffeinate`，不可用时自动降级直接启动

## 环境要求

- Go 1.24+
- Chrome/Chromium
- Instagram 账号（用于登录态）

## 快速开始

```bash
git clone <your-repo-url>
cd go-crawler
go mod download
./build.sh
```

首次登录：

```bash
./crawler login
```

## 配置

复制模板：

```bash
cp config.example.json config.json
```

示例：

```json
{
  "telegram_bot_token": "YOUR_BOT_TOKEN_HERE",
  "allowed_user_ids": [123456789],
  "admin_user_ids": [123456789],
  "favorite_accounts": ["nike", "instagram", "natgeo"],
  "worker_addr": "127.0.0.1:18080",
  "worker_api_token": "",
  "monitor_accounts": ["example_account"],
  "monitor_interval_min": 30,
  "monitor_interval_hours": 0,
  "monitor_compare_top_n": 10,
  "max_concurrent_downloads": 10,
  "posts_cache_expiry_hours": 24
}
```

关键字段：

- `worker_addr`: Worker 监听地址（默认 `127.0.0.1:18080`）
- `worker_api_token`: Worker API 鉴权 token（建议配置；Bot 与 Worker 需一致）
- `monitor_interval_min`: 监控轮询间隔（分钟，优先级高于 `monitor_interval_hours`）
- `monitor_interval_hours`: 监控轮询间隔（小时，当未配置分钟时生效）

环境变量（会覆盖配置文件）：

- `TELEGRAM_BOT_TOKEN`
- `WORKER_ADDR`
- `WORKER_API_TOKEN`
- `ALLOWED_USER_IDS`
- `ADMIN_USER_IDS`
- `MONITOR_INTERVAL_MIN`
- `MONITOR_INTERVAL_HOURS`
- `MAX_CONCURRENT_DOWNLOADS`
- `POSTS_CACHE_EXPIRY_HOURS`

## 命令

### crawler

```bash
./crawler login
./crawler download <username> <index>
./crawler dl <username> <index>
./crawler check-update <username>
./crawler cu <username>
./crawler bot
./crawler worker
./crawler setup-bot
```

### gobot

```bash
./gobot start
./gobot stop
./gobot restart
./gobot status
./gobot logs

./gobot worker start
./gobot worker stop
./gobot worker restart
./gobot worker status
./gobot worker logs
```

说明：不带服务名前缀时默认管理 `bot`。

## Telegram 命令

- `/download` 或 `/dl`
- `/status`
- `/control`（管理员）
- `/favorites`
- `/monitor`

## Bot + Worker 架构

```text
Telegram User
  -> Bot (控制面)
  -> HTTP POST /download (/check-update, /monitor-check)
  -> Worker (执行面，浏览器抓取 + 下载)
  -> Bot 上传结果文件
```

鉴权规则：

- 配置了 `WORKER_API_TOKEN`：请求必须带 `X-Worker-Token`
- 未配置：Worker 只接受本机请求（loopback）

## 项目结构（核心）

```text
go-crawler/
├── main.go
├── auth.go
├── downloader.go
├── scraper_navigator.go
├── scraper_extractor.go
├── scraper_cache.go
├── telegram_bot.go
├── telegram_handler_basic.go
├── telegram_handler_download.go
├── telegram_handler_status.go
├── telegram_handler_favorites.go
├── telegram_handler_monitor.go
├── telegram_worker.go
├── worker_server.go
├── worker_handlers.go
├── cache.go
├── config.go
├── config_validation.go
├── config_env.go
├── monitor.go
├── daemon.go
├── gobot.go
└── config.example.json
```

## 防休眠说明（macOS）

`gobot` 启动时：

- 若系统有 `caffeinate`，自动使用 `caffeinate -i`
- 若没有，自动降级直接启动服务（会给出提示）

## 故障排查

### Worker 连接失败

1. 检查 Worker 是否运行：`./gobot worker status`
2. 检查地址：`worker_addr` / `WORKER_ADDR`
3. 若配置了 token，确认 Bot/Worker 的 `WORKER_API_TOKEN` 一致
4. 看日志：`./gobot worker logs`

### Bot 无响应

1. `./gobot status`
2. `./gobot logs`
3. 确认 `allowed_user_ids`/`admin_user_ids` 配置正确

## 相关文档

- [USAGE_GUIDE.md](USAGE_GUIDE.md)
- [TELEGRAM_BOT.md](TELEGRAM_BOT.md)
- [SOURCE_CODE_GUIDE.md](SOURCE_CODE_GUIDE.md)
- [CONFIG_ENHANCEMENT.md](CONFIG_ENHANCEMENT.md)
- [MAC_SLEEP_SOLUTION.md](MAC_SLEEP_SOLUTION.md)
