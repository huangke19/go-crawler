# Telegram Bot 使用指南

## 架构说明

当前采用双进程架构：

- `crawler bot`：常驻在线，接收 Telegram 消息、展示按钮、上传文件。
- `crawler worker`：执行实际下载任务。

`/control` 控制的是 `worker`，不是 `bot` 本体。

## 配置

```bash
cp config.example.json config.json
```

`config.json` 示例：

```json
{
  "telegram_bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
  "allowed_user_ids": [123456789],
  "admin_user_ids": [123456789],
  "favorite_accounts": ["nike", "instagram", "natgeo"],
  "worker_addr": "127.0.0.1:18080"
}
```

字段说明：

- `telegram_bot_token`：Bot Token（必填）
- `allowed_user_ids`：允许使用 Bot 的用户（空则所有人可用）
- `admin_user_ids`：可使用 `/control` 的管理员（空时默认回退到 `allowed_user_ids`）
- `favorite_accounts`：`/download` 快捷账户按钮
- `worker_addr`：worker 监听地址

## 启动方式

```bash
# 1) 首次登录 Instagram
./crawler login

# 2) 启动 bot（后台）
./gobot start

# 3) 启动 worker（后台）
./gobot worker start
```

## Telegram 命令

- `/start` - 开始使用
- `/help` - 查看帮助
- `/download` - 下载帖子（按钮交互）
- `/dl` - `/download` 简写
- `/status` - 查看 bot 与 worker 状态
- `/control` - 管理员控制 worker（启动/停止/重启/状态）

## 手机端控制流程

1. 管理员发送 `/control`
2. Bot 返回控制面板按钮：启动、停止、重启、状态
3. 点击按钮后，Bot 在原消息里返回结果

## 行为约束

- worker 未运行时，`/download` 会提示“请联系管理员启动 worker”。
- 重复点击启动/停止为幂等语义，不会因已运行/已停止报错。

## 常用运维命令

```bash
./gobot status
./gobot worker status
./gobot worker restart
./gobot worker stop
```
