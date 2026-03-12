# Instagram 爬虫项目 - 源码功能指南

本文档提供快速查找各源码文件功能的指南。每个文件都已添加详细的中文注释，说明其职责、核心概念和关键函数。

## 📋 文件功能速查表

| 文件 | 职责 | 关键函数 | 行数 |
|------|------|---------|------|
| **main.go** | CLI 参数解析与子命令分发 | `main()`, `handleLogin()`, `handleDownload()`, `handleBot()`, `handleWorker()` | ~220 |
| **auth.go** | 认证管理与会话持久化 | `LoadSession()`, `SaveSession()`, `SetCookies()`, `EnsureLoggedIn()`, `CreateBrowserContext()`, `CreateFastBrowserContext()` | ~230 |
| **login.go** | 手动登录流程 | `ManualLogin()` | ~70 |
| **scraper.go** | 页面爬取与媒体 URL 提取 | `NavigateToUser()`, `ScrollToLoadMore()`, `GetPostByIndex()`, `ExtractMediaURLs()`, `extractMediaFromJSON()` | ~480 |
| **downloader.go** | 文件下载与并发控制 | `CreateUserDirectory()`, `DownloadMedia()`, `DownloadPost()`, `downloadConcurrently()` | ~270 |
| **cache.go** | 三层缓存系统 | `LoadMediaCache()`, `LoadPostsCache()`, `LoadFilesCache()`, `GetDownloadHistory()` | ~370 |
| **config.go** | 配置管理 | `LoadConfig()`, `GetWorkerAddr()`, `GetWorkerBaseURL()` | ~66 |
| **bot.go** | Telegram Bot 实现 | `NewTelegramBot()`, `Start()`, `handleCommand()`, `handleCallback()`, `downloadPost()` | ~500+ |
| **worker.go** | Worker HTTP 服务 | `NewWorkerServer()`, `RunWorker()`, `handleDownload()`, `handleCheckUpdate()` | ~400+ |
| **daemon.go** | 守护进程管理 | `StartServiceDaemon()`, `StopServiceDaemon()`, `RestartServiceDaemon()`, `GetServiceRuntime()` | ~377 |
| **gobot.go** | 守护进程 CLI 工具 | `main()`, `printGobotUsage()`, `showLogs()` | ~118 |

## 🔍 按功能分类查找

### 登录与认证
- **auth.go** - 会话加载/保存、Cookie 注入、登录状态验证
- **login.go** - 手动登录流程（打开浏览器让用户登录）

### 爬取与下载
- **scraper.go** - 访问主页、滚动加载、定位帖子、提取媒体 URL
- **downloader.go** - 并发下载、文件管理、重试机制

### 缓存系统
- **cache.go** - 三层缓存（媒体/帖子/文件）、缓存过期管理、下载历史

### Bot 与 Worker
- **bot.go** - Telegram Bot 交互、权限检查、状态管理
- **worker.go** - HTTP 服务、浏览器复用、缓存优先

### 配置与管理
- **config.json** - 配置文件（Token、用户 ID、常用账户）
- **config.go** - 配置加载与验证
- **daemon.go** - 后台进程管理（启动/停止/重启）
- **gobot.go** - CLI 工具（命令行管理服务）

### 主入口
- **main.go** - CLI 参数解析、子命令分发

## 🚀 常见修改位置

### 添加新的 CLI 命令
**位置**: `main.go` 的 `main()` 函数
```go
switch command {
case "login":
    handleLogin()
case "download", "dl":
    handleDownload()
// 在这里添加新命令
}
```

### 修改爬取逻辑
**位置**: `scraper.go`
- `NavigateToUser()` - 访问主页的逻辑
- `ScrollToLoadMore()` - 滚动加载的逻辑
- `ExtractMediaURLs()` - GraphQL 调用和媒体提取

### 修改下载逻辑
**位置**: `downloader.go`
- `DownloadMedia()` - 单个文件下载
- `downloadConcurrently()` - 并发控制（修改 maxConcurrent 参数）

### 修改 Bot 命令
**位置**: `bot.go` 的 `handleCommand()` 函数
```go
switch update.Message.Command() {
case "start":
    // 处理 /start 命令
case "download":
    // 处理 /download 命令
// 在这里添加新命令
}
```

### 修改 Worker 接口
**位置**: `worker.go` 的 `NewWorkerServer()` 函数
```go
mux.HandleFunc("/health", ws.handleHealth)
mux.HandleFunc("/download", ws.handleDownload)
// 在这里添加新接口
```

### 修改缓存策略
**位置**: `cache.go`
- `LoadMediaCache()` - 媒体缓存加载
- `LoadPostsCache()` - 帖子缓存加载
- `GetDownloadHistory()` - 下载历史查询

### 修改配置项
**位置**: `config.go` 的 `Config` 结构体
```go
type Config struct {
    TelegramBotToken string   `json:"telegram_bot_token"`
    AllowedUserIDs   []int64  `json:"allowed_user_ids"`
    // 在这里添加新配置项
}
```

## 📊 数据流向

### 下载流程
```
main.go (handleDownload)
  ↓
auth.go (EnsureLoggedIn)
  ↓
scraper.go (NavigateToUser → GetPostByIndex → ExtractMediaURLs)
  ↓
downloader.go (DownloadPost → downloadConcurrently)
  ↓
downloads/<username>/ (保存文件)
```

### Bot 流程
```
bot.go (Start → handleCommand/handleCallback)
  ↓
worker.go (HTTP POST /download)
  ↓
cache.go (检查缓存)
  ↓
scraper.go + downloader.go (实时抓取/下载)
  ↓
bot.go (sendFile → 上传到 Telegram)
```

### 守护进程流程
```
gobot.go (CLI 命令)
  ↓
daemon.go (StartServiceDaemon/StopServiceDaemon)
  ↓
main.go (crawler bot/worker)
  ↓
后台运行
```

## 🔑 关键概念

### 三层缓存
1. **文件缓存** (`files_cache.json`) - 已下载文件路径，秒回
2. **媒体缓存** (`media_cache.json`) - GraphQL 响应，避免重复调用
3. **帖子缓存** (`posts_cache.json`) - 用户主页帖子列表，1 小时过期

### 浏览器上下文
- **有头模式** (`CreateBrowserContext`) - 用于登录，显示 UI
- **无头模式** (`CreateFastBrowserContext`) - 用于爬取，不显示 UI，禁用图片

### 并发控制
- **信号量** - 使用 channel 限制并发数（最多 10 个）
- **WaitGroup** - 等待所有 goroutine 完成
- **RWMutex** - 保护缓存的并发读写

### 状态机
- **waiting_account** - 等待用户选择账户
- **waiting_index** - 等待用户选择帖子序号
- **过期时间** - 5 分钟

## 📝 注释规范

每个源文件都遵循以下注释规范：

1. **文件头注释** - 说明文件职责、核心概念、关键函数
2. **函数注释** - 说明函数功能、参数、返回值
3. **关键代码注释** - 解释复杂逻辑或重要决策

## 🛠️ 快速定位技巧

### 按功能查找
- 登录相关：`auth.go`, `login.go`
- 爬取相关：`scraper.go`
- 下载相关：`downloader.go`
- 缓存相关：`cache.go`
- Bot 相关：`bot.go`
- Worker 相关：`worker.go`

### 按关键词查找
- "GraphQL" → `scraper.go`
- "并发" → `downloader.go`
- "缓存" → `cache.go`
- "Telegram" → `bot.go`
- "HTTP" → `worker.go`
- "守护进程" → `daemon.go`

### 按错误类型查找
- 登录失败 → `auth.go`, `login.go`
- 爬取失败 → `scraper.go`
- 下载失败 → `downloader.go`
- 缓存问题 → `cache.go`
- Bot 无响应 → `bot.go`
- Worker 崩溃 → `worker.go`

## 📚 相关文档

- `CLAUDE.md` - 项目完整文档
- `config.example.json` - 配置文件示例
- `README.md` - 项目说明

---

**最后更新**: 2026-03-12
**项目版本**: 1.0.0
