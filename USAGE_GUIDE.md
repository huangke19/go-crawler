# 使用指南

## 1. 本地 CLI 下载

```bash
./crawler login
./crawler download <username> <index>
./crawler dl <username> <index>
```

示例：

```bash
./crawler download nike 1
./crawler dl instagram 5
```

## 2. Telegram 远程下载

推荐双进程：

```bash
./gobot start
./gobot worker start
```

在 Telegram 中：

- `/download`
- `/dl <username> <index>`
- `/status`
- `/control`（管理员）

## 3. 三种交互方式

### 方式 A：完整命令

```text
/download nike 1
/dl instagram 5
```

### 方式 B：按钮 + 输入序号

1. 发送 `/download`
2. 点击账户按钮
3. 输入帖子序号

### 方式 C：回复输入

1. 发送 `/download`
2. 回复 `账户名 序号`（如 `nike 3`）

## 4. Worker 安全策略（重要）

- 配置了 `WORKER_API_TOKEN`：
  - Bot 请求会自动带 `X-Worker-Token`
  - Worker 会强制校验
- 未配置 token：
  - Worker 仅允许本机来源

因此：

- 单机个人使用：可不配 token
- 跨机器/暴露端口：必须配 token

## 5. 常用运维

```bash
./gobot status
./gobot worker status

./gobot logs
./gobot worker logs

./gobot restart
./gobot worker restart
```

## 6. 常见问题

### 下载卡住或失败

- 先检查 session：`./crawler login`
- 看 worker 日志：`./gobot worker logs`
- 检查网络与目标账户可访问性

### Worker 未授权

- 检查 Bot/Worker 的 `WORKER_API_TOKEN` 是否一致
- 若未配置 token，确认是本机调用

### 地址不一致

- 检查 `worker_addr` 或 `WORKER_ADDR` 是否与 Worker 监听一致
