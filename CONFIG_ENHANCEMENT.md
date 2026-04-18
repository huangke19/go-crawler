# 配置增强说明

## 已支持的配置能力

- 配置文件 + 环境变量覆盖
- 完整字段验证（token、地址、用户 ID、账户名、数值范围）
- 默认值回填
- 兼容旧配置（`admin_user_ids` 为空时回退 `allowed_user_ids`）

## 新增安全相关配置

### `worker_api_token`

- `config.json` 字段：`worker_api_token`
- 环境变量：`WORKER_API_TOKEN`

用途：

- Bot 调 Worker 时自动携带 `X-Worker-Token`
- Worker 校验该 header

未配置时行为：

- Worker 接口仅允许本机来源访问（loopback）

## 环境变量清单

```bash
export TELEGRAM_BOT_TOKEN="123456789:ABC..."
export WORKER_ADDR="127.0.0.1:18080"
export WORKER_API_TOKEN="replace_with_strong_token"
export ALLOWED_USER_IDS="123456789,987654321"
export ADMIN_USER_IDS="123456789"
export MONITOR_INTERVAL_MIN="30"
export MAX_CONCURRENT_DOWNLOADS="10"
export POSTS_CACHE_EXPIRY_HOURS="24"
```

## 推荐实践

- 单机个人使用：`WORKER_API_TOKEN` 可选
- 跨机器/公网访问：必须配置 `WORKER_API_TOKEN`
- 生产环境优先使用环境变量注入敏感信息

## 常见错误

### Worker 地址格式错误

- 正确格式：`host:port`（如 `127.0.0.1:18080`）

### Worker 未授权

- Bot 与 Worker 的 `WORKER_API_TOKEN` 不一致
- 或未配置 token 且请求来源不是本机

## 相关文件

- `config.go`
- `config_env.go`
- `config_validation.go`
- `config.example.json`

## 运行与控制补充

- `worker_addr` / `WORKER_API_TOKEN` 主要用于 Bot 调 Worker
- 管理员在 Telegram 中通过 `/status` 消息里的按钮控制 worker
- `gobot worker ...` 适合本机运维；`gobot launchd ...` 适合让 bot 开机常驻
