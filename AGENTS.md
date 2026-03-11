# Instagram 爬虫项目 (go-crawler)

这是一个用 Go 语言编写的 Instagram 内容下载工具，使用浏览器自动化技术实现登录、爬取和下载功能。

## 项目概览

**项目类型**: CLI 工具
**语言**: Go 1.24.0
**主要功能**: Instagram 用户帖子的自动化下载
**架构模式**: 单体应用，模块化函数设计

## 核心技术栈

- **chromedp** (v0.14.2): 浏览器自动化，用于登录和页面交互
- **goquery** (v1.11.0): HTML 解析和 DOM 查询
- **标准库**: net/http (下载), encoding/json (会话管理), sync (并发控制)

## 项目结构

```
go-crawler/
├── main.go              # 主入口，CLI 参数解析和流程编排
├── auth.go              # 认证管理：会话保存/加载、Cookie 设置、登录状态验证
├── login.go             # 手动登录流程：打开浏览器让用户登录
├── scraper.go           # 爬取逻辑：访问用户主页、定位帖子、提取媒体 URL
├── downloader.go        # 下载逻辑：并发下载、重试机制、文件管理
├── bot.go               # Telegram Bot 实现：消息处理、下载任务管理
├── config.go            # 配置管理：加载 config.json
├── setup_bot.go         # Bot 命令菜单设置
├── daemon.go            # 守护进程管理：启动/停止/重启逻辑
├── gobot.go             # 守护进程 CLI 工具入口
├── build.sh             # 编译脚本
├── go.mod               # 模块定义
├── .instagram_session.json  # 会话文件（Cookie 存储，不提交到 Git）
├── .gobot.pid           # 守护进程 PID 文件（不提交到 Git）
├── gobot.log            # 守护进程日志文件（不提交到 Git）
├── downloads/           # 下载目录（按用户名组织）
├── crawler              # 编译后的可执行文件（主程序）
└── gobot                # 编译后的可执行文件（守护进程管理工具）
```

## 核心文件详解

### 1. main.go (主入口)
- **职责**: CLI 参数解析、流程编排
- **命令**:
  - `./crawler login` - 手动登录
  - `./crawler <username> <post_index>` - 下载指定帖子
- **流程**: 启动浏览器 → 验证登录 → 访问用户主页 → 定位帖子 → 提取媒体 → 下载
- **关键函数**:
  - `main()`: 参数解析和流程控制

### 2. auth.go (认证管理)
- **职责**: 会话持久化、Cookie 管理、登录状态验证
- **关键常量**:
  - `sessionFile = ".instagram_session.json"` - 会话文件路径
- **核心类型**:
  ```go
  type Cookie struct {
      Name     string
      Value    string
      Domain   string
      Path     string
      Expires  float64
      HTTPOnly bool
      Secure   bool
  }
  ```
- **关键函数**:
  - `LoadSession()` - 从文件加载 Cookie
  - `SaveSession(cookies)` - 保存 Cookie 到文件
  - `SetCookies(ctx, cookies)` - 设置 Cookie 到浏览器
  - `EnsureLoggedIn(ctx)` - 验证登录状态，失败则提示重新登录
  - `CreateBrowserContext()` - 创建普通浏览器上下文（有头模式，用于登录）
  - `CreateFastBrowserContext()` - 创建快速浏览器上下文（无头模式，禁用图片）

### 3. login.go (登录流程)
- **职责**: 手动登录流程
- **关键函数**:
  - `ManualLogin()` - 打开浏览器，等待用户手动登录，保存 Cookie
- **流程**: 打开登录页 → 等待用户输入 → 获取 Cookie → 保存到文件

### 4. scraper.go (爬取逻辑)
- **职责**: 页面导航、帖子定位、媒体 URL 提取
- **核心类型**:
  ```go
  type MediaInfo struct {
      Type  string     // "image", "video", "carousel"
      URLs  []string   // 媒体 URL 列表
      Types []string   // 每个 URL 的类型（"image" 或 "video"）
  }
  ```
- **关键函数**:
  - `NavigateToUser(ctx, username)` - 访问用户主页
  - `ScrollToLoadMore(ctx, targetIndex)` - 滚动加载更多帖子（每次加载 12 个）
  - `GetPostByIndex(ctx, index)` - 获取第 N 个帖子的 URL（从 1 开始）
  - `ExtractMediaURLs(ctx, postURL)` - 提取帖子中的所有媒体 URL
  - `extractShortcode(postURL)` - 从 URL 提取 shortcode
  - `extractMediaFromJSON(data)` - 从 GraphQL 响应中解析媒体信息
  - `extractImageURL(item)` - 提取图片 URL（优先 display_url）
- **API 调用**:
  - 使用 Instagram GraphQL API: `https://www.instagram.com/graphql/query`
  - doc_id: `8845758582119845`
  - 需要 X-CSRFToken 请求头（从 Cookie 中提取）
  - 响应格式: `data.xdt_shortcode_media` 或 `data.shortcode_media`

### 5. downloader.go (下载逻辑)
- **职责**: 文件下载、并发控制、重试机制
- **核心类型**:
  ```go
  type downloadTask struct {
      url      string
      savePath string
  }
  ```
- **关键函数**:
  - `CreateUserDirectory(username)` - 创建 `downloads/<username>/` 目录
  - `DownloadMedia(url, savePath, retries)` - 下载单个文件，支持重试
  - `DownloadPost(username, postIndex, mediaInfo)` - 下载帖子的所有媒体
  - `downloadConcurrently(tasks, maxConcurrent, retries)` - 并发下载（最多 5 个并发）
- **文件命名规则**:
  - 单图: `post_<index>.jpg`
  - 单视频: `post_<index>.mp4`
  - 轮播: `post_<index>_<seq>.jpg` 或 `.mp4`

### 6. bot.go (Telegram Bot)
- **职责**: Telegram Bot 实现、消息处理、下载任务管理
- **核心类型**:
  ```go
  type TelegramBot struct {
      bot              *tgbotapi.BotAPI
      allowedUsers     map[int64]bool
      favoriteAccounts []string
      userStates       map[int64]*UserState
      statesMutex      sync.RWMutex
  }

  type UserState struct {
      Step      string    // "waiting_account" 或 "waiting_index"
      Username  string
      Timestamp time.Time
  }
  ```
- **关键函数**:
  - `NewTelegramBot(token, allowedUserIDs, favoriteAccounts)` - 创建 Bot 实例
  - `Start()` - 启动 Bot，监听消息
  - `handleCommand(message)` - 处理命令（/start, /help, /download, /status）
  - `handleCallback(callback)` - 处理按钮点击
  - `downloadPost(username, postIndex)` - 执行下载任务
  - `sendFile(chatID, filePath)` - 上传文件到 Telegram

### 7. daemon.go (守护进程管理)
- **职责**: 后台进程管理、PID 管理、日志记录
- **关键常量**:
  - `pidFile = ".gobot.pid"` - PID 文件路径
  - `logFile = "gobot.log"` - 日志文件路径
- **关键函数**:
  - `StartDaemon()` - 启动后台进程（脱离终端）
  - `StopDaemon()` - 停止后台进程（SIGTERM → SIGKILL）
  - `RestartDaemon()` - 重启后台进程
  - `StatusDaemon()` - 查看进程状态和日志
  - `IsRunning()` - 检查进程是否运行
  - `IsProcessRunning(pid)` - 检查指定 PID 的进程
  - `ReadPID()` / `WritePID(pid)` - PID 文件操作

### 8. gobot.go (守护进程 CLI 工具)
- **职责**: 守护进程管理的命令行接口
- **命令**:
  - `gobot start` - 启动 Bot 后台服务
  - `gobot stop` - 停止 Bot 服务
  - `gobot restart` - 重启 Bot 服务
  - `gobot status` - 查看 Bot 运行状态
  - `gobot logs` - 查看完整日志
  - `gobot help` - 显示帮助信息

## 代码约定

### 命名规范
- **函数**: PascalCase（导出函数）
- **变量**: camelCase
- **常量**: camelCase 或 UPPER_CASE
- **类型**: PascalCase

### 错误处理
- 使用 `fmt.Errorf()` 包装错误信息
- 在关键步骤打印进度信息（`fmt.Printf`）
- 使用 `✓` 符号表示成功

### 并发模式
- 使用 `sync.WaitGroup` 等待所有 goroutine 完成
- 使用 channel 作为信号量控制并发数（`semaphore := make(chan struct{}, maxConcurrent)`）
- 使用 `atomic.AddInt32` 更新共享计数器

### 浏览器自动化
- 登录时使用有头模式（`headless: false`）
- 爬取时使用无头模式（`headless: true`）并禁用图片加载
- User-Agent: `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36`
- 使用 `chromedp.Sleep()` 等待页面加载（1-3 秒）

### HTTP 请求
- 使用标准库 `net/http`
- 设置 30 秒超时
- 模仿浏览器请求头（User-Agent, Referer, X-CSRFToken 等）
- 从会话文件加载 Cookie

## 关键流程

### 登录流程
1. 调用 `ManualLogin()`
2. 创建有头浏览器上下文
3. 导航到 `https://www.instagram.com/accounts/login/`
4. 等待用户手动输入账号密码
5. 获取浏览器 Cookie
6. 保存到 `.instagram_session.json`

### 下载流程
1. 解析命令行参数（用户名、帖子序号）
2. 创建快速浏览器上下文（无头模式）
3. 调用 `EnsureLoggedIn()` 验证登录状态
4. 调用 `NavigateToUser()` 访问用户主页
5. 调用 `GetPostByIndex()` 定位第 N 个帖子
   - 如果需要，调用 `ScrollToLoadMore()` 滚动加载
   - 解析 HTML，提取帖子链接
6. 调用 `ExtractMediaURLs()` 提取媒体 URL
   - 从 URL 提取 shortcode
   - 调用 Instagram GraphQL API
   - 解析 JSON 响应，提取媒体 URL
7. 调用 `DownloadPost()` 下载所有媒体
   - 创建用户目录
   - 并发下载（最多 5 个并发）
   - 支持重试（默认 1 次）

### Bot 后台服务流程
1. 用户执行 `gobot start`
2. `StartDaemon()` 启动新进程执行 `crawler bot`
3. 新进程脱离终端（创建新会话）
4. 输出重定向到 `gobot.log`
5. PID 保存到 `.gobot.pid`
6. 用户可以通过 `gobot status/stop/restart` 管理服务

### Bot 消息处理流程
1. Bot 监听 Telegram 消息
2. 检查用户权限（白名单）
3. 处理命令或普通消息
4. 对于 `/download` 命令：
   - 显示账户选择按钮
   - 用户选择账户后显示帖子序号按钮
   - 用户选择序号后执行下载
   - 下载完成后上传文件到 Telegram

## 常见开发任务

### 添加新功能
- **修改 CLI 参数**: 编辑 `main.go` 的参数解析逻辑
- **修改爬取逻辑**: 编辑 `scraper.go`
- **修改下载逻辑**: 编辑 `downloader.go`
- **修改认证逻辑**: 编辑 `auth.go`

### 调试技巧
- 在 `CreateFastBrowserContext()` 中设置 `headless: false` 查看浏览器行为
- 在关键步骤添加 `fmt.Printf()` 打印调试信息
- 检查 `.instagram_session.json` 文件确认 Cookie 是否有效
- 使用 `chromedp.Sleep()` 增加等待时间

### 编译和分发
```bash
# 编译当前系统
go build -o crawler

# 编译 Linux
GOOS=linux GOARCH=amd64 go build -o crawler-linux

# 编译 Windows
GOOS=windows GOARCH=amd64 go build -o crawler.exe

# 编译 macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o crawler-mac-arm64
```

### 测试
```bash
# 登录
./crawler login

# 下载测试
./crawler nike 1
./crawler instagram 5
```

## 注意事项

### 安全性
- `.instagram_session.json` 包含敏感信息，已添加到 `.gitignore`
- 文件权限设置为 `0600`（仅所有者可读写）
- 不要在代码中硬编码账号密码

### Instagram API
- 使用 GraphQL API（doc_id: `8845758582119845`）
- 需要有效的 Cookie 和 X-CSRFToken
- 响应格式可能变化，需要兼容 `xdt_shortcode_media` 和 `shortcode_media`
- 支持三种媒体类型：单图、单视频、轮播（多图/视频混合）

### 性能优化
- 无头模式 + 禁用图片加载（减少带宽和加载时间）
- 并发下载（最多 5 个并发）
- 滚动加载优化（每次加载 12 个帖子）
- 减少等待时间（1 秒而非 3 秒）

### 限制
- Instagram 可能对频繁请求进行限制
- 需要手动登录（不支持自动化登录）
- 依赖 Chrome/Chromium 浏览器
- 仅支持公开账户或已关注的私密账户

## 依赖更新

```bash
# 更新所有依赖
go get -u ./...

# 更新特定依赖
go get -u github.com/chromedp/chromedp
go get -u github.com/PuerkitoBio/goquery

# 清理依赖
go mod tidy
```

## 故障排除

### 登录失败
- 删除 `.instagram_session.json` 重新登录
- 检查网络连接
- 确认 Instagram 账户正常

### 爬取失败
- 检查用户名是否正确
- 确认帖子序号在范围内
- 查看是否需要重新登录（Session 过期）

### 下载失败
- 检查网络连接
- 确认 `downloads/` 目录有写入权限
- 查看错误信息定位问题

### 浏览器无法启动
- 确认已安装 Chrome 或 Chromium
- macOS: `/Applications/Google Chrome.app`
- Linux: `which google-chrome` 或 `which chromium`

## 开发环境设置

```bash
# 克隆项目
git clone <repo-url>
cd go-crawler

# 安装依赖
go mod download

# 编译
./build.sh

# 首次使用：登录 Instagram
./crawler login

# 启动 Bot 后台服务
./gobot start

# 查看服务状态
./gobot status

# 停止服务
./gobot stop
```

## Mac 防休眠配置

Bot 已集成 `caffeinate` 命令，启动时自动防止 Mac 休眠：

```bash
./gobot start  # 自动使用 caffeinate 防止系统休眠
```

**工作原理：**
- 使用 `caffeinate -i` 防止系统空闲休眠
- 只要 Bot 运行，系统就不会休眠
- 停止 Bot 后，系统恢复正常休眠策略

详细配置见 `MAC_SLEEP_SOLUTION.md`。

## 守护进程管理

### 启动服务
```bash
./gobot start
```

输出：
```
✅ Bot 已启动 (PID: 12345)
📝 日志文件: /Users/huangke/Developer/go-crawler/gobot.log
```

### 查看状态
```bash
./gobot status
```

输出：
```
✅ Bot 正在运行
PID: 12345
日志文件: gobot.log

最近日志:
---
2026-03-10 16:14:00 Telegram Bot 已启动: @YourBot
✅ Bot 已启动，等待消息...
---
```

### 重启服务
```bash
./gobot restart
```

### 停止服务
```bash
./gobot stop
```

### 查看日志
```bash
tail -f gobot.log
```

## 技术细节

### 守护进程实现
- 使用 `syscall.Setsid` 创建新会话，脱离终端
- 使用 `caffeinate -i` 防止 Mac 休眠
- PID 文件用于进程管理和状态检查
- 使用 SIGTERM 优雅停止，SIGKILL 强制停止
- 日志重定向到 `gobot.log`

## 维护约定

**重要**: 每次对话完成功能开发后，必须自动更新本文件，无需用户提醒。

需要更新的内容：
- 新增的文件路径和职责说明
- 新增的函数、类型定义和接口设计
- 重要的架构决策和技术选型
- 修改的核心流程和业务逻辑
- 新增的依赖库和版本
- 新的配置项和环境变量
- 重要的 Bug 修复和性能优化

更新位置：
- 新文件 → 添加到"项目结构"和"核心文件详解"
- 新函数/类型 → 添加到对应文件的"关键函数"部分
- 架构变更 → 更新"项目概览"和相关流程图
- 新依赖 → 更新"核心技术栈"和"依赖更新"

## Git 工作流

- 主分支: `master`
- 默认分支（PR 目标）: `main`
- 当前状态: 有未提交的修改（auth.go, downloader.go, main.go, scraper.go）

## 最近提交

- `bf3f0d1` - 优化性能和用户体验
- `4c8e8f2` - 初始提交：Instagram 爬虫项目
