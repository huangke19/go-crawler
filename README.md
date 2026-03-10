# Instagram 爬虫 (go-crawler)

一个用 Go 语言编写的 Instagram 内容下载工具，支持自动登录和媒体下载功能。

## 功能特性

- ✅ 自动化浏览器登录（使用 Chrome/Chromium）
- ✅ 访问指定用户的 Instagram 主页
- ✅ 按序号获取用户的帖子
- ✅ 提取帖子中的图片和视频
- ✅ 自动下载媒体文件到本地
- ✅ 支持单个可执行文件分发

## 系统要求

- **操作系统**: macOS、Linux 或 Windows
- **Chrome/Chromium**: 需要安装 Chrome 或 Chromium 浏览器
- **网络**: 能够访问 Instagram 网站

## 安装

### 方式一：使用预编译的可执行文件

直接使用项目中的 `crawler` 文件（已编译，12MB）：

```bash
./crawler login      # 首次使用，进行登录
./crawler nike 5     # 下载 @nike 用户的第 5 条帖子
```

### 方式二：从源代码编译

需要安装 Go 1.24.0 或更高版本：

```bash
# 编译为当前系统的可执行文件
go build -o crawler

# 编译为 Linux 版本
go build -o crawler-linux -goos=linux -goarch=amd64

# 编译为 Windows 版本
go build -o crawler.exe -goos=windows -goarch=amd64
```

## 使用方法

### 第一步：登录

首次使用时，需要进行 Instagram 登录：

```bash
./crawler login
```

程序会：
1. 打开 Chrome 浏览器
2. 导航到 Instagram 登录页面
3. 等待你手动输入用户名和密码
4. 完成登录后自动保存会话信息到 `.instagram_session.json`

**注意**: 登录信息会保存在 `.instagram_session.json` 文件中，请妥善保管此文件。

### 第二步：下载内容

登录成功后，可以下载指定用户的帖子：

```bash
./crawler <目标用户名> <帖子序号>
```

**参数说明**:
- `<目标用户名>`: Instagram 用户名（不需要 @ 符号）
- `<帖子序号>`: 用户主页上的帖子序号，从 1 开始

**使用示例**:

```bash
# 下载 @nike 用户的第 5 条帖子
./crawler nike 5

# 下载 @instagram 用户的第 1 条帖子
./crawler instagram 1

# 下载 @nasa 用户的第 10 条帖子
./crawler nasa 10
```

### 输出结果

下载完成后，媒体文件会保存到：

```
downloads/<用户名>/post_<序号>/
```

例如：
```
downloads/nike/post_5/
├── image_1.jpg
├── image_2.jpg
└── video_1.mp4
```

## 工作流程

```
1. 启动程序
   ↓
2. 检查会话文件 (.instagram_session.json)
   ↓
3. 如果会话有效，跳过登录；否则需要重新登录
   ↓
4. 打开浏览器访问目标用户主页
   ↓
5. 定位并获取指定序号的帖子链接
   ↓
6. 进入帖子页面，提取所有媒体 URL
   ↓
7. 下载所有媒体文件到本地
   ↓
8. 完成
```

## 项目结构

```
go-crawler/
├── crawler              # 编译后的可执行文件
├── main.go             # 主程序入口
├── auth.go             # 认证相关函数
├── login.go            # 登录相关函数
├── scraper.go          # 网页爬取相关函数
├── downloader.go       # 下载相关函数
├── go.mod              # Go 模块定义
├── go.sum              # 依赖校验和
├── .gitignore          # Git 忽略文件
├── README.md           # 本文件
└── downloads/          # 下载的媒体文件存储目录
```

## 依赖库

- **chromedp**: 用于浏览器自动化和网页交互
- **goquery**: 用于 HTML 解析和 DOM 查询

## 常见问题

### Q: 程序找不到 Chrome 浏览器怎么办？

A: 确保已安装 Chrome 或 Chromium 浏览器。chromedp 会自动查找系统中的浏览器。

### Q: 登录失败怎么办？

A:
1. 检查网络连接
2. 确保 Instagram 网站可以访问
3. 删除 `.instagram_session.json` 文件，重新登录
4. 检查是否需要进行双因素认证

### Q: 下载速度很慢怎么办？

A: 这取决于网络速度和媒体文件大小。程序会逐个下载文件，无法加速。

### Q: 可以下载多个用户的内容吗？

A: 可以。登录一次后，可以多次运行程序下载不同用户的内容。

### Q: 如何更新程序？

A:
```bash
git pull origin master
go build -o crawler
```

## 注意事项

⚠️ **重要提示**:

1. **遵守法律**: 仅用于个人学习和研究，不得用于商业目的或违反 Instagram 服务条款
2. **隐私保护**: 尊重他人隐私，不要下载他人的私密内容
3. **账户安全**: 妥善保管 `.instagram_session.json` 文件，不要分享给他人
4. **速率限制**: Instagram 可能对频繁请求进行限制，请合理使用
5. **条款遵守**: 使用前请阅读 Instagram 的服务条款和使用政策

## 故障排除

### 程序无法启动

```bash
# 检查文件权限
chmod +x crawler

# 尝试直接运行
./crawler login
```

### 浏览器无法打开

```bash
# 确保 Chrome 已安装
which google-chrome  # Linux
which chromium       # Linux
# macOS 通常在: /Applications/Google Chrome.app
```

### 下载失败

1. 检查网络连接
2. 确认用户名和帖子序号正确
3. 检查 `downloads/` 目录是否有写入权限
4. 查看错误信息获取更多细节

## 许可证

本项目仅供学习和研究使用。

## 支持

如遇到问题，请检查：
1. 网络连接
2. Chrome 浏览器是否已安装
3. Instagram 账户是否正常
4. 会话文件是否有效
