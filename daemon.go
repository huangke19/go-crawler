// ============================================================================
// daemon.go - 守护进程管理（Bot 和 Worker）
// ============================================================================
//
// 职责：
//   - 启动/停止/重启后台服务（Bot 和 Worker）
//   - PID 文件管理
//   - 日志文件管理
//   - 进程状态检查
//   - 优雅关闭（SIGTERM → SIGKILL）
//
// 核心概念：
//   - 守护进程：脱离终端运行的后台进程（使用 syscall.Setsid）
//   - PID 文件：记录进程 ID，用于后续管理
//   - 日志重定向：stdout/stderr 重定向到日志文件
//   - caffeinate：macOS 防休眠工具（仅在 macOS 上使用）
//
// 支持的服务：
//   - bot：Telegram Bot 服务
//   - worker：下载执行服务
//
// 关键函数：
//   - StartServiceDaemon()：启动后台服务
//   - StopServiceDaemon()：停止后台服务
//   - RestartServiceDaemon()：重启后台服务
//   - GetServiceRuntime()：获取服务运行状态
//   - PrintServiceStatus()：打印服务状态
//   - IsServiceRunning()：检查服务是否运行
//   - IsProcessRunning()：检查进程是否存在
//   - ReadServicePID() / WriteServicePID()：PID 文件操作
//   - ShowLastLogs()：显示最近的日志
//
// ============================================================================

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type serviceSpec struct {
	name    string
	label   string
	pidFile string
	logFile string
	args    []string
}

type ServiceRuntime struct {
	Service string
	Label   string
	Running bool
	PID     int
	LogPath string
	Detail  string
}

func resolveExecutablePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return executable, nil
	}
	return resolved, nil
}

// getWorkDir 获取守护进程工作目录（稳定路径，避免受当前执行目录影响）。
// 优先级：
// 1. 环境变量 GOBOT_WORKDIR
// 2. 可执行文件所在目录
// 3. 用户主目录 ~/.gobot
// 4. /tmp
func getWorkDir() string {
	if value := strings.TrimSpace(os.Getenv("GOBOT_WORKDIR")); value != "" {
		return value
	}
	if executable, err := resolveExecutablePath(); err == nil {
		return filepath.Dir(executable)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gobot")
	}
	return "/tmp"
}

func getServiceSpec(service string) (serviceSpec, error) {
	switch service {
	case "bot":
		return serviceSpec{
			name:    "bot",
			label:   "Bot",
			pidFile: ".gobot.pid",
			logFile: "gobot.log",
			args:    []string{"bot"},
		}, nil
	case "worker":
		return serviceSpec{
			name:    "worker",
			label:   "Worker",
			pidFile: ".goworker.pid",
			logFile: "goworker.log",
			args:    []string{"worker"},
		}, nil
	default:
		return serviceSpec{}, fmt.Errorf("不支持的服务类型: %s", service)
	}
}

func buildServiceCommand(spec serviceSpec, crawlerPath string) (*exec.Cmd, bool) {
	if _, err := exec.LookPath("caffeinate"); err == nil {
		args := append([]string{"-i", crawlerPath}, spec.args...)
		cmd := exec.Command("caffeinate", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		return cmd, true
	}

	cmd := exec.Command(crawlerPath, spec.args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, false
}

func StartServiceDaemon(service string) (string, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return "", err
	}

	// 检查是否已经运行（防止多实例）
	if IsServiceRunning(service) {
		pid, _ := ReadServicePID(service)
		return "", fmt.Errorf("%s 已在运行 (PID: %d)，请先停止", spec.label, pid)
	}

	executable, err := resolveExecutablePath()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	exePath := filepath.Join(filepath.Dir(executable), "crawler")
	if _, err := os.Stat(exePath); err != nil {
		return "", fmt.Errorf("未找到 crawler 可执行文件: %s，请先执行 ./build.sh", exePath)
	}
	workDir := getWorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("创建工作目录失败: %w", err)
	}

	logPath := filepath.Join(workDir, spec.logFile)
	logFd, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logFd.Close()

	cmd, usingCaffeinate := buildServiceCommand(spec, exePath)
	cmd.Stdout = logFd
	cmd.Stderr = logFd

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动进程失败: %w", err)
	}

	if err := WriteServicePID(service, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("保存 PID 失败: %w", err)
	}

	time.Sleep(500 * time.Millisecond)
	if !IsServiceRunning(service) {
		return "", fmt.Errorf("进程启动后立即退出，请查看日志: %s", logPath)
	}

	msg := fmt.Sprintf("%s 已启动 (PID: %d)\n日志文件: %s", spec.label, cmd.Process.Pid, logPath)
	if !usingCaffeinate {
		msg += "\n提示: 未检测到 caffeinate，系统休眠时服务可能暂停"
	}
	return msg, nil
}

func StopServiceDaemon(service string) (string, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return "", err
	}

	if !IsServiceRunning(service) {
		return fmt.Sprintf("%s 已停止", spec.label), nil
	}

	pid, err := ReadServicePID(service)
	if err != nil {
		return "", err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return "", fmt.Errorf("查找进程失败: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return "", fmt.Errorf("停止进程失败: %w", err)
	}

	for i := 0; i < 50; i++ {
		if !IsProcessRunning(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if IsProcessRunning(pid) {
		_ = process.Signal(syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond)
	}

	RemoveServicePID(service)
	return fmt.Sprintf("%s 已停止 (PID: %d)", spec.label, pid), nil
}

func RestartServiceDaemon(service string) (string, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, 2)
	if IsServiceRunning(service) {
		stopMsg, err := StopServiceDaemon(service)
		if err != nil {
			return "", err
		}
		parts = append(parts, stopMsg)
		time.Sleep(1 * time.Second)
	} else {
		parts = append(parts, fmt.Sprintf("%s 当前未运行，直接启动", spec.label))
	}

	startMsg, err := StartServiceDaemon(service)
	if err != nil {
		return "", err
	}
	parts = append(parts, startMsg)
	return strings.Join(parts, "\n"), nil
}

func GetServiceRuntime(service string) (*ServiceRuntime, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(getWorkDir(), spec.logFile)
	runtime := &ServiceRuntime{
		Service: spec.name,
		Label:   spec.label,
		LogPath: logPath,
	}

	pid, err := ReadServicePID(service)
	if err == nil {
		runtime.PID = pid
		if IsProcessRunning(pid) {
			runtime.Running = true
			runtime.Detail = fmt.Sprintf("正在运行 (PID: %d，gobot 管理)", pid)
			return runtime, nil
		}

		// PID 文件已陈旧，先清理再尝试通过进程探测兜底识别。
		RemoveServicePID(service)
	}

	// 兜底：即使不是 gobot 启动，也尽量识别真实运行状态。
	detectedPID, detectErr := DetectServiceProcessPID(service)
	if detectErr == nil && detectedPID > 0 {
		runtime.Running = true
		runtime.PID = detectedPID
		runtime.Detail = fmt.Sprintf("正在运行 (PID: %d，外部启动)", detectedPID)
		return runtime, nil
	}

	runtime.Running = false
	if err != nil {
		runtime.Detail = "PID 文件不存在或不可读"
		return runtime, nil
	}
	runtime.Detail = fmt.Sprintf("PID 文件存在但进程不存在: %d", pid)
	return runtime, nil
}

func PrintServiceStatus(service string) error {
	runtime, err := GetServiceRuntime(service)
	if err != nil {
		return err
	}

	if !runtime.Running {
		fmt.Printf("❌ %s 未运行\n", runtime.Label)
		if runtime.Detail != "" {
			fmt.Printf("详情: %s\n", runtime.Detail)
		}
		return nil
	}

	fmt.Printf("✅ %s 正在运行\n", runtime.Label)
	fmt.Printf("PID: %d\n", runtime.PID)

	if _, err := os.Stat(runtime.LogPath); err == nil {
		fmt.Printf("日志文件: %s\n", runtime.LogPath)
		fmt.Println("\n最近日志:")
		fmt.Println("---")
		if err := ShowLastLogs(runtime.LogPath, 10); err == nil {
			fmt.Println("---")
		}
	}

	return nil
}

func IsServiceRunning(service string) bool {
	pid, err := ReadServicePID(service)
	if err != nil {
		return false
	}
	return IsProcessRunning(pid)
}

func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func commandLooksLikeService(cmdline, service string) bool {
	tokens := strings.Fields(cmdline)
	if len(tokens) == 0 {
		return false
	}

	hasServiceArg := false
	for _, token := range tokens {
		if token == service {
			hasServiceArg = true
			break
		}
	}
	if !hasServiceArg {
		return false
	}

	for _, token := range tokens {
		base := filepath.Base(token)
		if base == "crawler" || base == "go-crawler" {
			return true
		}
	}
	return false
}

// DetectServiceProcessPID 从系统进程中探测服务进程 PID。
// 仅用于 status 兜底展示，不依赖 gobot 的 PID 文件。
func DetectServiceProcessPID(service string) (int, error) {
	cmd := exec.Command("ps", "ax", "-o", "pid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("读取进程列表失败: %w", err)
	}

	var detectedPID int
	lines := bytes.Split(out, []byte{'\n'})
	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}

		cmdline := strings.TrimSpace(strings.Join(fields[1:], " "))
		if !commandLooksLikeService(cmdline, service) {
			continue
		}

		if !IsProcessRunning(pid) {
			continue
		}

		// 取较新的进程（PID 更大）作为当前活跃实例。
		if pid > detectedPID {
			detectedPID = pid
		}
	}

	return detectedPID, nil
}

func ReadServicePID(service string) (int, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return 0, err
	}

	pidPath := filepath.Join(getWorkDir(), spec.pidFile)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("读取 PID 文件失败: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("解析 PID 失败: %w", err)
	}

	return pid, nil
}

func WriteServicePID(service string, pid int) error {
	spec, err := getServiceSpec(service)
	if err != nil {
		return err
	}

	pidPath := filepath.Join(getWorkDir(), spec.pidFile)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("写入 PID 文件失败: %w", err)
	}
	return nil
}

func RemoveServicePID(service string) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return
	}
	pidPath := filepath.Join(getWorkDir(), spec.pidFile)
	_ = os.Remove(pidPath)
}

func ShowLastLogs(logPath string, lines int) error {
	cmd := exec.Command("tail", "-n", strconv.Itoa(lines), logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// 兼容旧接口（默认管理 bot）
func StartDaemon() error {
	msg, err := StartServiceDaemon("bot")
	if err != nil {
		return err
	}
	fmt.Printf("✅ %s\n", msg)
	return nil
}

func StopDaemon() error {
	msg, err := StopServiceDaemon("bot")
	if err != nil {
		return err
	}
	fmt.Printf("✅ %s\n", msg)
	return nil
}

func RestartDaemon() error {
	msg, err := RestartServiceDaemon("bot")
	if err != nil {
		return err
	}
	fmt.Printf("✅ %s\n", msg)
	return nil
}

func StatusDaemon() error {
	return PrintServiceStatus("bot")
}

func IsRunning() bool {
	return IsServiceRunning("bot")
}

func ReadPID() (int, error) {
	return ReadServicePID("bot")
}

func WritePID(pid int) error {
	return WriteServicePID("bot", pid)
}

func RemovePID() {
	RemoveServicePID("bot")
}
