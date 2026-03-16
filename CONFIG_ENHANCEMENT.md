# 配置管理增强说明

## 新增功能

### 1. 完整的配置验证

配置加载时会自动验证所有字段的合法性：

#### Telegram Bot Token 验证
- 不能为空
- 不能是占位符 `YOUR_BOT_TOKEN_HERE`
- 必须符合格式：`数字:字母数字` (例如：`123456789:ABCdef...`)

#### Worker 地址验证
- 格式必须为 `host:port`
- 端口号范围：1-65535
- 支持 `http://` 或 `https://` 前缀

#### 用户 ID 验证
- 所有用户 ID 必须为正整数
- 不允许零值或负数

#### 账户名验证
- 长度：1-30 个字符
- 只能包含：字母、数字、下划线、点号
- 不能以点号开头或结尾
- 符合 Instagram 账户名规则

#### 数值范围验证
- `monitor_interval_min`: 1-1440 分钟
- `monitor_compare_top_n`: 1-100
- `max_concurrent_downloads`: 1-100
- `posts_cache_expiry_hours`: 1-168 小时

### 2. 环境变量支持

现在可以通过环境变量覆盖配置文件中的设置（环境变量优先级更高）：

```bash
# Telegram Bot Token（必需）
export TELEGRAM_BOT_TOKEN="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"

# Worker 服务地址（可选）
export WORKER_ADDR="127.0.0.1:18080"

# 允许的用户 ID（可选，逗号分隔）
export ALLOWED_USER_IDS="123456789,987654321"

# 管理员用户 ID（可选，逗号分隔）
export ADMIN_USER_IDS="123456789"

# 监控间隔（分钟，可选）
export MONITOR_INTERVAL_MIN="30"

# 最大并发下载数（可选）
export MAX_CONCURRENT_DOWNLOADS="10"

# 帖子缓存过期时间（小时，可选）
export POSTS_CACHE_EXPIRY_HOURS="24"
```

### 3. 详细的错误提示

验证失败时会提供详细的错误信息：

```
配置验证失败，发现 3 个错误:
  1. 配置验证失败 [telegram_bot_token]: 不能为空，请在 config.json 中配置有效的 Bot Token
  2. 配置验证失败 [worker_addr=invalid]: 格式不正确，应为 'host:port' 格式（例如：127.0.0.1:18080）
  3. 配置验证失败 [allowed_user_ids=index 0: 0]: 用户 ID 必须为正整数
```

## 使用方式

### 方式 1：配置文件（推荐）

```json
{
  "telegram_bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
  "allowed_user_ids": [123456789],
  "admin_user_ids": [123456789],
  "favorite_accounts": ["nike", "instagram"],
  "worker_addr": "127.0.0.1:18080",
  "monitor_accounts": ["nasa"],
  "monitor_interval_min": 30,
  "monitor_compare_top_n": 10,
  "max_concurrent_downloads": 10,
  "posts_cache_expiry_hours": 24
}
```

### 方式 2：环境变量

适用于容器化部署或 CI/CD 环境：

```bash
# 设置环境变量
export TELEGRAM_BOT_TOKEN="your_token_here"
export WORKER_ADDR="0.0.0.0:18080"

# 启动服务
./crawler bot
```

### 方式 3：混合模式

配置文件 + 环境变量（环境变量会覆盖配置文件）：

```bash
# config.json 中配置基础设置
# 通过环境变量覆盖敏感信息
export TELEGRAM_BOT_TOKEN="production_token"

./crawler bot
```

## 迁移指南

### 从旧版本升级

旧版本的配置文件完全兼容，无需修改。新增的验证功能会在启动时自动检查配置的合法性。

如果配置验证失败，会看到详细的错误提示，根据提示修改配置即可。

### Docker 部署

```dockerfile
FROM golang:1.24

WORKDIR /app
COPY . .
RUN go build -o crawler

# 通过环境变量配置
ENV TELEGRAM_BOT_TOKEN="your_token"
ENV WORKER_ADDR="0.0.0.0:18080"

CMD ["./crawler", "bot"]
```

### Kubernetes 部署

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: crawler-config
type: Opaque
stringData:
  TELEGRAM_BOT_TOKEN: "your_token_here"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crawler-bot
spec:
  template:
    spec:
      containers:
      - name: bot
        image: crawler:latest
        env:
        - name: TELEGRAM_BOT_TOKEN
          valueFrom:
            secretKeyRef:
              name: crawler-config
              key: TELEGRAM_BOT_TOKEN
        - name: WORKER_ADDR
          value: "crawler-worker:18080"
```

## 最佳实践

### 1. 敏感信息管理

- ✅ 使用环境变量存储 Bot Token
- ✅ 不要将 Token 提交到 Git
- ✅ 使用 `.gitignore` 忽略 `config.json`

### 2. 配置验证

- ✅ 启动前会自动验证配置
- ✅ 验证失败会给出详细提示
- ✅ 修复配置后重新启动即可

### 3. 环境隔离

- ✅ 开发环境：使用配置文件
- ✅ 生产环境：使用环境变量
- ✅ CI/CD：使用环境变量 + Secret 管理

## 故障排查

### 问题 1：Bot Token 验证失败

```
配置验证失败 [telegram_bot_token]: 格式不正确
```

**解决方案**：
1. 检查 Token 格式是否为 `数字:字母数字`
2. 确认从 @BotFather 获取的 Token 完整复制
3. 检查是否有多余的空格或换行符

### 问题 2：Worker 地址验证失败

```
配置验证失败 [worker_addr=invalid]: 格式不正确
```

**解决方案**：
1. 确保格式为 `host:port`（例如：`127.0.0.1:18080`）
2. 端口号范围：1-65535
3. 可以省略 `http://` 前缀

### 问题 3：用户 ID 验证失败

```
配置验证失败 [allowed_user_ids]: 用户 ID 必须为正整数
```

**解决方案**：
1. 确保所有用户 ID 都是正整数
2. 不要使用 0 或负数
3. 从 Telegram 获取真实的用户 ID

## 技术细节

### 配置加载流程

1. 读取配置文件 `config.json`
2. 应用默认值
3. 用环境变量覆盖
4. 验证配置合法性
5. 应用到全局变量

### 验证规则

所有验证规则在 `config_validation.go` 中定义，包括：
- 格式验证（正则表达式）
- 范围验证（最小值/最大值）
- 逻辑验证（非空、非零）

### 环境变量解析

环境变量解析在 `config_env.go` 中实现，支持：
- 字符串类型（直接覆盖）
- 整数类型（自动转换）
- 数组类型（逗号分隔）

## 相关文件

- `config.go` - 配置结构定义和基础加载
- `config_validation.go` - 配置验证逻辑
- `config_env.go` - 环境变量支持
- `config_test.go` - 配置加载测试
- `config_validation_test.go` - 配置验证测试
