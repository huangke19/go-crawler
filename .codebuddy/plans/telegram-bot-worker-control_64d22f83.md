---
name: telegram-bot-worker-control
overview: 为现有 Telegram Bot 增加基于按钮的远程控制能力，实现对独立 worker 的启动、停止、重启与状态查询，同时保持 bot 常驻在线以支持手机端持续可控。
todos:
  - id: refactor-service-model
    content: 重构 daemon.go 支持 bot 与 worker 双服务管理
    status: completed
  - id: add-worker-entry
    content: 扩展 main.go 并新增 worker.go 运行与接口入口
    status: completed
    dependencies:
      - refactor-service-model
  - id: wire-bot-control
    content: 在 bot.go 增加 /control 按钮回调与管理员鉴权
    status: completed
    dependencies:
      - add-worker-entry
  - id: route-download-to-worker
    content: 将下载执行从 bot 本地迁移为 worker 调用
    status: completed
    dependencies:
      - wire-bot-control
  - id: align-cli-config
    content: 更新 gobot.go、config.go、config.example.json 保持命令配置一致
    status: completed
    dependencies:
      - route-download-to-worker
  - id: update-docs
    content: 更新 setup_bot.go、TELEGRAM_BOT.md、MAC_SLEEP_SOLUTION.md、AGENTS.md
    status: completed
    dependencies:
      - align-cli-config
---

## User Requirements

- 在 Telegram Bot 中新增一个“控制命令”，通过按钮控制爬虫服务，至少包含：启动、停止、重启。
- 控制入口来自手机端 Telegram，对话交互应直观，按钮点击后有明确结果反馈。
- Bot 需要长期在线作为控制入口；控制目标应为独立执行服务，而不是直接停掉 Bot 自身。
- 在 Bot 不在线时，手机消息不会触发本地动作；因此需要将 Bot 设计为常驻服务以保证可控性。

## Product Overview

- 产品保持“手机端远程控制 + 本机执行”的模式：Telegram Bot 负责接收命令与展示状态，爬虫执行能力由独立进程承载。
- 用户在 Telegram 中发送控制命令后，看到内联按钮菜单，点击即可执行启动、停止、重启、状态查询。
- 整体交互效果应为：单条消息内完成操作引导、处理中提示、成功或失败反馈，避免命令行式复杂输入。

## Core Features

- 控制面板命令：提供 `/control`，展示 worker 的启动、停止、重启、状态按钮。
- 权限分级：普通白名单用户可下载；管理员可执行控制操作。
- 幂等控制反馈：重复点击启动/停止不会报错，返回“已运行/已停止”等一致结果。
- 可恢复运行：Bot 常驻在线，worker 可独立启停，避免“停掉后无法手机端恢复”。

## Tech Stack Selection

- 语言与运行时：Go 1.24（沿用现有 CLI 架构）
- Bot 交互：`github.com/go-telegram-bot-api/telegram-bot-api/v5`（复用现有命令与回调机制）
- 进程管理：现有 `daemon.go` 的 PID/日志/信号管理能力（扩展为 bot/worker 双目标）
- 本地服务通信：Go 标准库 `net/http`（worker 本地控制与任务入口）

## Implementation Approach

采用“控制面与执行面分离”的策略：`crawler bot` 持续在线处理 Telegram 消息；`crawler worker` 独立承载下载执行。Bot 的按钮命令仅控制 worker 生命周期并查询状态，下载请求也经 worker 执行，避免 Bot 直接承担重任务导致可用性下降。
关键决策：复用现有 daemon 模式并参数化服务类型，减少新模式引入；保留现有命令兼容，逐步迁移下载入口到 worker。
性能与可靠性：worker 单次下载链路复杂度与现有一致（主要受网络与浏览器驱动影响）；通过进程隔离降低 Bot 阻塞风险，控制操作为 O(1) 本地进程操作，瓶颈在 chromedp 下载任务本身。

## Implementation Notes (Execution Details)

- 复用现有 `fmt/log` 输出风格、错误包装风格，避免引入新日志框架。
- 控制回调数据使用固定前缀（如 `ctl:worker:*`），降低解析分支复杂度与误触发概率。
- 下载热路径从 Bot 迁移到 worker 后，Bot 只负责状态与回传，避免长任务占用更新循环。
- 控制命令需幂等：已启动再启动、已停止再停止都返回成功语义，降低误操作成本。
- 保持向后兼容：`/download` 与现有白名单机制继续可用；未配置管理员时默认仅允许 `allowed_user_ids` 中用户控制或显式禁用控制命令。

## Architecture Design

- `main.go`：新增 `worker` 子命令，形成 `bot` 与 `worker` 双入口。
- `bot.go`：新增 `/control` 命令与控制按钮回调；下载请求改为调用 worker 接口。
- `daemon.go`：从单一 bot 守护扩展为可指定服务目标（bot/worker）的统一守护逻辑。
- `gobot.go`：CLI 扩展为可管理 worker 生命周期（与 Telegram 控制语义对齐）。
- `config.go`：新增管理员与 worker 连接配置，统一配置加载与默认值处理。

## Directory Structure Summary

本次改造基于现有单体 CLI，重点是“进程职责拆分 + 控制通道补齐”，不做无关重构。

- `/Users/huangke/Developer/go-crawler/main.go` [MODIFY]  
目的：扩展命令分发，新增 `worker` 运行入口。
要求：保持现有 `login/download/bot/setup-bot` 行为不变。

- `/Users/huangke/Developer/go-crawler/bot.go` [MODIFY]  
目的：新增 `/control` 与控制按钮流转、管理员鉴权。
要求：按钮回调支持启动/停止/重启/状态；下载执行改为通过 worker。

- `/Users/huangke/Developer/go-crawler/daemon.go` [MODIFY]  
目的：支持 bot/worker 双服务守护管理。
要求：独立 PID/日志文件、幂等启停、状态准确回报。

- `/Users/huangke/Developer/go-crawler/gobot.go` [MODIFY]  
目的：扩展 CLI 管理 worker（start/stop/restart/status）。
要求：兼容现有命令习惯并清晰提示服务对象。

- `/Users/huangke/Developer/go-crawler/config.go` [MODIFY]  
目的：新增管理员与 worker 相关配置字段。
要求：配置缺省安全、错误提示明确、兼容旧配置。

- `/Users/huangke/Developer/go-crawler/config.example.json` [MODIFY]  
目的：补充新配置示例。
要求：示例覆盖管理员、worker 地址/开关等关键项。

- `/Users/huangke/Developer/go-crawler/setup_bot.go` [MODIFY]  
目的：更新 BotFather 命令列表，加入控制命令。
要求：命令说明与实际实现一致。

- `/Users/huangke/Developer/go-crawler/TELEGRAM_BOT.md` [MODIFY]  
目的：更新控制命令、权限模型与手机端操作流程。
要求：明确 Bot 常驻与 worker 启停边界。

- `/Users/huangke/Developer/go-crawler/MAC_SLEEP_SOLUTION.md` [MODIFY]  
目的：补充 Bot 常驻、异常拉起与远程控制说明。
要求：文档与实际守护行为一致。

- `/Users/huangke/Developer/go-crawler/AGENTS.md` [MODIFY]  
目的：同步新增命令、文件职责与流程变更。
要求：按项目维护约定更新核心流程与关键函数。

- `/Users/huangke/Developer/go-crawler/worker.go` [NEW]  
目的：承载 worker 进程主逻辑与本地控制/任务接口。
要求：提供健康检查、任务执行入口、优雅退出与错误返回。