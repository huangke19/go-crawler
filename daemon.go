package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	pidFile = ".gobot.pid"
	logFile = "gobot.log"
)

// getWorkDir 获取工作目录（优先使用当前目录，如果没有权限则使用用户主目录）
func getWorkDir() string {
	// 优先使用当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	// 回退到用户主目录
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gobot")
	}
	// 最后回退到 /tmp
	return "/tmp"
}

// StartDaemon 启动守护进程
func StartDaemon() error {
	// 检查是否已经在运行
	if IsRunning() {
		pid, _ := ReadPID()
		return fmt.Errorf("Bot 已经在运行中 (PID: %d)", pid)
	}

	// 获取当前可执行文件路径
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 获取 crawler 可执行文件路径（与 gobot 在同一目录）
	exePath := filepath.Join(filepath.Dir(executable), "crawler")

	// 获取工作目录
	workDir := getWorkDir()

	// 如果工作目录不存在则创建
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("创建工作目录失败: %w", err)
	}

	// 创建日志文件
	logPath := filepath.Join(workDir, logFile)
	logFd, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logFd.Close()

	// 启动子进程（执行 crawler bot）
	// 使用 caffeinate 防止 Mac 休眠
	cmd := exec.Command("caffeinate", "-i", exePath, "bot")
	cmd.Stdout = logFd
	cmd.Stderr = logFd
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // 创建新会话，脱离终端
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	// 保存 PID
	if err := WritePID(cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("保存 PID 失败: %w", err)
	}

	// 等待一下确认进程启动成功
	time.Sleep(500 * time.Millisecond)
	if !IsRunning() {
		return fmt.Errorf("进程启动后立即退出，请查看日志: %s", logPath)
	}

	fmt.Printf("✅ Bot 已启动 (PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("📝 日志文件: %s\n", logPath)
	return nil
}

// StopDaemon 停止守护进程
func StopDaemon() error {
	if !IsRunning() {
		return fmt.Errorf("Bot 未运行")
	}

	pid, err := ReadPID()
	if err != nil {
		return err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("查找进程失败: %w", err)
	}

	// 发送 SIGTERM 信号
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("停止进程失败: %w", err)
	}

	// 等待进程退出（最多 5 秒）
	for i := 0; i < 50; i++ {
		if !IsProcessRunning(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 如果还在运行，强制杀死
	if IsProcessRunning(pid) {
		process.Signal(syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond)
	}

	// 删除 PID 文件
	RemovePID()

	fmt.Printf("✅ Bot 已停止 (PID: %d)\n", pid)
	return nil
}

// RestartDaemon 重启守护进程
func RestartDaemon() error {
	if IsRunning() {
		fmt.Println("正在停止 Bot...")
		if err := StopDaemon(); err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
	}

	fmt.Println("正在启动 Bot...")
	return StartDaemon()
}

// StatusDaemon 查看守护进程状态
func StatusDaemon() error {
	if !IsRunning() {
		fmt.Println("❌ Bot 未运行")
		return nil
	}

	pid, err := ReadPID()
	if err != nil {
		return err
	}

	// 获取进程信息
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("❌ Bot 未运行 (PID 文件存在但进程不存在: %d)\n", pid)
		RemovePID()
		return nil
	}

	// 检查进程是否真的存在
	if !IsProcessRunning(pid) {
		fmt.Printf("❌ Bot 未运行 (PID 文件存在但进程不存在: %d)\n", pid)
		RemovePID()
		return nil
	}

	fmt.Printf("✅ Bot 正在运行\n")
	fmt.Printf("PID: %d\n", pid)

	// 显示日志文件路径
	logPath := filepath.Join(getWorkDir(), logFile)
	if _, err := os.Stat(logPath); err == nil {
		fmt.Printf("日志文件: %s\n", logPath)

		// 显示最后几行日志
		fmt.Println("\n最近日志:")
		fmt.Println("---")
		if err := ShowLastLogs(logPath, 10); err == nil {
			fmt.Println("---")
		}
	}

	_ = process // 避免未使用变量警告
	return nil
}

// IsRunning 检查守护进程是否在运行
func IsRunning() bool {
	pid, err := ReadPID()
	if err != nil {
		return false
	}
	return IsProcessRunning(pid)
}

// IsProcessRunning 检查指定 PID 的进程是否在运行
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// 发送信号 0 检查进程是否存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ReadPID 读取 PID 文件
func ReadPID() (int, error) {
	pidPath := filepath.Join(getWorkDir(), pidFile)
	data, err := ioutil.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("读取 PID 文件失败: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("解析 PID 失败: %w", err)
	}

	return pid, nil
}

// WritePID 写入 PID 文件
func WritePID(pid int) error {
	pidPath := filepath.Join(getWorkDir(), pidFile)
	data := []byte(strconv.Itoa(pid))
	if err := ioutil.WriteFile(pidPath, data, 0644); err != nil {
		return fmt.Errorf("写入 PID 文件失败: %w", err)
	}
	return nil
}

// RemovePID 删除 PID 文件
func RemovePID() {
	pidPath := filepath.Join(getWorkDir(), pidFile)
	os.Remove(pidPath)
}

// ShowLastLogs 显示日志文件的最后 N 行
func ShowLastLogs(logPath string, lines int) error {
	data, err := ioutil.ReadFile(logPath)
	if err != nil {
		return err
	}

	// 简单实现：按行分割，取最后 N 行
	content := string(data)
	allLines := []string{}
	currentLine := ""

	for _, ch := range content {
		if ch == '\n' {
			allLines = append(allLines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(ch)
		}
	}
	if currentLine != "" {
		allLines = append(allLines, currentLine)
	}

	// 取最后 N 行
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}

	for i := start; i < len(allLines); i++ {
		fmt.Println(allLines[i])
	}

	return nil
}
