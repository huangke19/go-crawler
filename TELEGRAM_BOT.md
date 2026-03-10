# Telegram Bot 使用指南

## 快速开始

### 1. 创建 Telegram Bot

1. 在 Telegram 中找到 [@BotFather](https://t.me/botfather)
2. 发送 `/newbot` 创建新 bot
3. 按提示设置 bot 名称和用户名
4. 获取 Bot Token（格式：`123456789:ABCdefGHIjklMNOpqrsTUVwxyz`）

### 2. 获取你的 Telegram User ID

1. 在 Telegram 中找到 [@userinfobot](https://t.me/userinfobot)
2. 发送任意消息，bot 会返回你的 User ID（纯数字）

### 3. 配置 Bot

```bash
# 复制配置文件模板
cp config.example.json config.json

# 编辑配置文件
nano config.json
```

配置示例：
```json
{
  "telegram_bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
  "allowed_user_ids": [123456789, 987654321],
  "favorite_accounts": ["nike", "instagram", "natgeo", "nasa", "apple"]
}
```

参数说明：
- `telegram_bot_token`: 你的 Bot Token（必填）
- `allowed_user_ids`: 允许使用 bot 的用户 ID 列表（可选，留空则所有人都可使用）
- `favorite_accounts`: 常用账户列表（可选，用于快速选择按钮）

### 4. 启动 Bot

```bash
# 首次使用需要先登录 Instagram
./crawler login

# 启动 Telegram Bot
./crawler bot
```

## Bot 命令

在 Telegram 中向你的 bot 发送以下命令：

- `/start` - 开始使用
- `/help` - 查看帮助
- `/download [username] [index]` - 下载指定帖子（支持三种方式）
- `/dl [username] [index]` - download 的简写
- `/status` - 查看 bot 状态

## 使用示例

### 方式 1: 完整命令（最快）
```
/download nike 1
/dl instagram 5
```

### 方式 2: 快速选择按钮
```
用户: /download

Bot: 📥 选择下载方式:
     🔹 快速选择常用账户:
     [nike] [instagram] [natgeo]
     [nasa] [apple]

     🔹 或直接回复此消息，格式: <账户名> <帖子序号>
        示例: nike 1

用户: [点击 nike 按钮]

Bot: ✅ 已选择账户: @nike
     请输入帖子序号 (例如: 1, 5, 10):

用户: 5

Bot: ⏳ 正在下载 @nike 的第 5 个帖子...
```

### 方式 3: 直接回复输入
```
用户: /download

Bot: 📥 选择下载方式:
     ...

用户: [回复消息] nike 1

Bot: ⏳ 正在下载 @nike 的第 1 个帖子...
```

Bot 会自动：
1. 下载指定的 Instagram 帖子
2. 将图片/视频上传到 Telegram
3. 显示下载进度和结果

## 安全建议

1. **保护 Bot Token**：不要将 `config.json` 提交到 Git
2. **启用白名单**：配置 `allowed_user_ids` 限制访问
3. **定期更新**：保持依赖库最新版本

## 故障排除

### Bot 无法启动
- 检查 `config.json` 是否存在且格式正确
- 确认 Bot Token 有效
- 查看终端错误信息

### 下载失败
- 确保已运行 `./crawler login` 登录 Instagram
- 检查 `.instagram_session.json` 是否存在
- 确认用户名和帖子序号正确

### 无法接收消息
- 确认已在 Telegram 中向 bot 发送 `/start`
- 检查 User ID 是否在白名单中
- 查看终端日志

## 部署到服务器

```bash
# 编译
go build -o crawler

# 使用 screen 或 tmux 保持运行
screen -S instagram-bot
./crawler bot

# 分离会话：Ctrl+A, D
# 重新连接：screen -r instagram-bot
```

或使用 systemd 服务（推荐）：

```ini
# /etc/systemd/system/instagram-bot.service
[Unit]
Description=Instagram Telegram Bot
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/go-crawler
ExecStart=/path/to/go-crawler/crawler bot
Restart=always

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl enable instagram-bot
sudo systemctl start instagram-bot
sudo systemctl status instagram-bot
```
