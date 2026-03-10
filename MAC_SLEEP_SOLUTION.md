# Mac 防休眠解决方案

## 问题
Mac mini 休眠会导致后台服务中断。

## 解决方案

### 方案 1：使用 caffeinate（已集成）

Bot 已经集成了 `caffeinate` 命令，启动时会自动防止系统休眠。

```bash
./gobot start  # 自动使用 caffeinate 防止休眠
```

**工作原理：**
- 使用 `caffeinate -i` 参数防止系统空闲休眠
- 只要 Bot 进程运行，系统就不会休眠
- 停止 Bot 后，系统恢复正常休眠策略

**优点：**
- 无需修改系统设置
- 只在 Bot 运行时防止休眠
- 停止 Bot 后自动恢复

### 方案 2：修改系统电源设置

如果你希望 Mac mini 永不休眠：

```bash
# 查看当前电源设置
pmset -g

# 禁用显示器休眠（保持系统运行）
sudo pmset displaysleep 0

# 禁用系统休眠
sudo pmset sleep 0

# 禁用硬盘休眠
sudo pmset disksleep 0
```

**恢复默认设置：**
```bash
sudo pmset displaysleep 10
sudo pmset sleep 10
sudo pmset disksleep 10
```

### 方案 3：使用 launchd 自动重启

创建 launchd 配置，系统唤醒后自动重启 Bot：

创建 `~/Library/LaunchAgents/com.instagram.gobot.plist`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.instagram.gobot</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/huangke/Developer/go-crawler/gobot</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/huangke/Developer/go-crawler/gobot.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/huangke/Developer/go-crawler/gobot.log</string>
</dict>
</plist>
```

加载配置：
```bash
launchctl load ~/Library/LaunchAgents/com.instagram.gobot.plist
```

卸载配置：
```bash
launchctl unload ~/Library/LaunchAgents/com.instagram.gobot.plist
```

### 方案 4：远程唤醒（Wake on LAN）

如果 Mac mini 已经休眠，可以通过网络唤醒：

1. 在 Mac mini 上启用 Wake on LAN：
   ```
   系统设置 → 节能 → 启用"网络唤醒"
   ```

2. 使用手机或其他设备发送 Wake on LAN 包唤醒 Mac mini

3. 推荐 App：
   - iOS: Mocha WOL
   - Android: Wake On Lan

## 推荐方案

**推荐使用方案 1（已集成）**，因为：
- 无需修改系统设置
- 自动管理，无需手动干预
- 停止 Bot 后系统恢复正常

如果你希望 Mac mini 完全不休眠，可以结合方案 2。

## 验证

检查 caffeinate 是否在运行：

```bash
ps aux | grep caffeinate | grep -v grep
```

输出示例：
```
huangke  76052  0.0  0.0  caffeinate -i /Users/huangke/Developer/go-crawler/crawler bot
```

## 注意事项

1. **电费考虑**：防止休眠会增加电力消耗
2. **散热**：确保 Mac mini 通风良好
3. **远程访问**：考虑配置 SSH 或 VNC 以便远程管理
4. **备份**：定期备份重要数据

## 其他建议

### 配置远程访问

启用 SSH（远程登录）：
```
系统设置 → 共享 → 远程登录
```

这样即使不在家也可以通过 SSH 管理 Bot：
```bash
ssh username@your-mac-mini-ip
cd /Users/huangke/Developer/go-crawler
./gobot status
```

### 配置静态 IP

在路由器中为 Mac mini 分配静态 IP，方便远程访问。

### 使用 Tailscale 或 ZeroTier

如果不想配置端口转发，可以使用 Tailscale 或 ZeroTier 创建虚拟局域网，随时随地访问 Mac mini。
