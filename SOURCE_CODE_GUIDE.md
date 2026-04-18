# 源码功能指南

本文档用于快速定位“要改哪里”。

## 核心文件

| 文件 | 作用 |
|---|---|
| `main.go` | CLI 入口（login/download/bot/worker/check-update/setup-bot/ytdl） |
| `auth.go` | 登录态保存/加载、Cookie 注入、登录校验 |
| `scraper_navigator.go` | 主页导航、帖子定位、滚动加载 |
| `scraper_extractor.go` | GraphQL 媒体提取（支持 context 取消） |
| `scraper_cache.go` | 帖子缓存刷新与对比 |
| `downloader.go` | 并发下载与文件落盘 |
| `cache.go` | 三层缓存（媒体/帖子/文件）与历史查询 |
| `telegram_bot.go` | Bot 生命周期、权限检查、消息工具函数 |
| `telegram_handler_*.go` | Bot 命令与回调处理 |
| `telegram_worker.go` | Bot 调用 Worker HTTP（自动带鉴权头） |
| `worker_server.go` | Worker 生命周期、路由注册、浏览器复用 |
| `worker_handlers.go` | Worker 接口处理（下载/刷新/监控） |
| `monitor.go` | 定时监控与新帖推送 |
| `config.go` | 配置结构与默认值 |
| `config_env.go` | 环境变量覆盖 |
| `config_validation.go` | 配置合法性验证 |
| `daemon.go` | Bot/Worker 守护进程管理 |
| `gobot.go` | 守护管理 CLI |

## 按功能查找

### 新增/调整命令

- `main.go`

### 改下载逻辑

- 抓取：`scraper_navigator.go`, `scraper_extractor.go`
- 下载：`downloader.go`
- 缓存：`cache.go`

### 改 Telegram 交互

- 命令分发：`telegram_handler_basic.go`
- 下载交互：`telegram_handler_download.go`
- 状态控制：`telegram_handler_status.go`

### 改 Worker HTTP 接口

- 路由：`worker_server.go`
- 处理器：`worker_handlers.go`

## 当前接口

- `GET /health`
- `POST /download`
- `POST /download-url`
- `POST /check-update`
- `POST /monitor-check`
- `GET /metrics`

## Worker 鉴权规则

- 配置 `WORKER_API_TOKEN` 时，必须带 `X-Worker-Token`
- 未配置 token 时，仅允许 loopback 来源

## 关键流程

### Bot 下载流程

1. 用户发 `/download`
2. Bot 收集参数
3. Bot 调用 Worker
4. Worker 执行抓取下载
5. Bot 上传文件

### 外部链接下载流程

1. 用户发送 `/ytdl <url>` 或直接发送链接
2. Bot 调用 Worker `/download-url`
3. Worker 使用 `yt-dlp` 或回退逻辑下载
4. Bot 上传文件

### 缓存命中顺序

1. `files_cache`
2. `media_cache`
3. `posts_cache`
4. 实时抓取

## 历史命名变更

旧文档中的以下名称已拆分：

- `worker.go` -> `worker_server.go` + `worker_handlers.go`
- `bot.go` -> `telegram_bot.go` + `telegram_handler_*.go` + `telegram_worker.go`
- `scraper.go` -> `scraper_navigator.go` + `scraper_extractor.go` + `scraper_cache.go`
