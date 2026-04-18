# Telegram Bot 使用指南

## 架构

- `crawler bot`: 控制面，常驻接收 Telegram 命令与按钮回调
- `crawler worker`: 执行面，处理下载/刷新/监控检测

Bot 与 Worker 通过本机 HTTP 通信，典型接口：

- `POST /download`
- `POST /check-update`
- `POST /monitor-check`
- `GET /health`

## 鉴权与访问策略

Worker 默认安全策略：

- 配置 `worker_api_token` 或 `WORKER_API_TOKEN` 时：
  - 请求必须携带 `X-Worker-Token`
- 未配置 token 时：
  - 仅允许本机来源（loopback）访问

这意味着个人单机使用时可以不配 token；跨机器或暴露端口时建议必须配置。

## 配置示例

```json
{
  "telegram_bot_token": "123456789:ABCdef...",
  "allowed_user_ids": [123456789],
  "admin_user_ids": [123456789],
  "favorite_accounts": ["nike", "instagram", "natgeo"],
  "worker_addr": "127.0.0.1:18080",
  "worker_api_token": "replace_with_strong_token"
}
```

## 启动

```bash
# 首次登录
./crawler login

# 启动 bot
./gobot start

# 启动 worker
./gobot worker start
```

## Telegram 命令

- `/download`: 下载交互
- `/status`: 查看 bot/worker 状态；管理员会看到 worker 控制按钮
- `/favorites`: 管理常用账号
- `/monitor`: 监控相关操作
- `/ytdl <url>`: 下载 YouTube / X 视频
- 直接发送 YouTube / X 链接：自动识别并下载

## 常用运维命令

```bash
./gobot status
./gobot worker status

./gobot restart
./gobot worker restart

./gobot logs
./gobot worker logs
```

## 常见问题

### 1) `/download` 报“下载服务未启动”

- 检查 `./gobot worker status`
- 如果未启动：`./gobot worker start`

### 1.1) 管理员如何远程控制 worker

- 发送 `/status`
- 点击消息里的 `启动`、`停止`、`重启`、`状态` 按钮
- 当前没有单独的 `/control` 命令

### 2) Worker 接口返回未授权

- 检查 Bot 与 Worker 的 `WORKER_API_TOKEN` 是否一致
- 若未配置 token，确认 Bot 与 Worker 在同一台机器上

### 3) Worker 地址错误

- 检查 `worker_addr` 或 `WORKER_ADDR`
- 建议优先用 `127.0.0.1:18080`
