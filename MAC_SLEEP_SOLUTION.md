# Mac 防休眠与常驻建议

## 目标

- `bot` 常驻在线（手机端随时可控）
- `worker` 按需启停（执行下载任务）

## 推荐方式

```bash
./gobot start
./gobot worker start
```

## 当前行为（与代码一致）

`gobot` 启动服务时：

- 若系统有 `caffeinate`：自动使用 `caffeinate -i`
- 若无 `caffeinate`：自动降级为直接启动 `crawler`

因此：

- 有 `caffeinate` 时可防止空闲休眠
- 无 `caffeinate` 时也不会启动失败，但系统休眠可能暂停服务

## 建议

- 日常仅常驻 `bot`，`worker` 用 Telegram `/status` 中的控制按钮或 `gobot worker ...` 管理
- 需要长期无人值守时，优先确保系统有 `caffeinate`

## launchd 方案

如果希望 bot 随系统启动，可使用：

```bash
./gobot launchd install
./gobot launchd status
./gobot launchd uninstall
```

## 检查命令

```bash
./gobot status
./gobot worker status
ps aux | grep caffeinate | grep -v grep
```
