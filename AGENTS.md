# Instagram 爬虫项目 (go-crawler)

这是一个用 Go 编写的 Instagram 下载工具，支持 CLI 下载与 Telegram 远程控制。

## 项目概览

- 项目类型: CLI 工具
- 语言: Go 1.24.0
- 核心能力:
  - Instagram 登录与下载
  - Telegram Bot 交互
  - Bot/Worker 双进程控制（手机端按钮控制 worker）

## 主要架构

采用控制面/执行面分离：

- `crawler bot`：控制面。常驻接收 Telegram 消息、展示按钮、上传下载结果。
- `crawler worker`：执行面。提供本地 HTTP 接口，执行下载任务。
- `daemon.go` + `gobot`：统一守护管理 `bot` 与 `worker`。

## 项目结构（关键文件）

```text
go-crawler/
├── main.go                # crawler 命令入口（login/download/bot/worker/setup-bot）
├── worker.go              # worker HTTP 服务与下载任务执行
├── bot.go                 # Telegram 命令、按钮回调、worker 调用
├── daemon.go              # bot/worker 双服务守护管理
├── gobot.go               # gobot 管理命令（支持 worker 子命令）
├── config.go              # 配置加载与默认值（admin_user_ids、worker_addr）
├── setup_bot.go           # BotFather 命令菜单文本
├── downloader.go          # 下载实现
├── scraper.go             # 帖子定位与媒体提取
├── auth.go/login.go       # 登录与会话管理
├── config.example.json    # 配置示例
└── build.sh               # 编译脚本
```

## 配置项

`config.json` 新增：

- `admin_user_ids`: `/control` 管理员白名单
- `worker_addr`: worker 监听地址（默认 `127.0.0.1:18080`）

兼容策略：

- `admin_user_ids` 未配置时，默认回退 `allowed_user_ids`

## 命令

### crawler

- `./crawler login`
- `./crawler download <username> <index>`
- `./crawler bot`
- `./crawler worker`
- `./crawler setup-bot`

### gobot

- `./gobot start|stop|restart|status|logs`（默认管理 bot）
- `./gobot worker start|stop|restart|status|logs`

## Telegram 命令

- `/download`、`/dl`：下载交互
- `/status`：查看 bot/worker 状态
- `/control`：管理员控制 worker（启动/停止/重启/状态）

## 核心流程

1. 用户在 Telegram 发 `/download`
2. Bot 完成账户与序号交互
3. Bot 调用 worker `/download`
4. worker 执行浏览器抓取与下载
5. Bot 上传文件到 Telegram

## 运行建议

- 为了手机端始终可控，`bot` 需要常驻在线。
- 推荐使用 `launchd` 或 `gobot` 守护能力保证异常自动恢复。
- `worker` 可按需启停，建议通过 `/control` 或 `gobot worker ...` 管理。
