package main

import (
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

// getWorkDir 获取工作目录（优先使用当前目录，如果没有权限则使用用户主目录）
func getWorkDir() string {
	if cwd, err := os.Getwd(); err == nil {
		return cwd
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

func buildServiceCommand(spec serviceSpec, crawlerPath string) *exec.Cmd {
	args := append([]string{"-i", crawlerPath}, spec.args...)
	cmd := exec.Command("caffeinate", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

func StartServiceDaemon(service string) (string, error) {
	spec, err := getServiceSpec(service)
	if err != nil {
		return "", err
	}

	if IsServiceRunning(service) {
		pid, _ := ReadServicePID(service)
		return fmt.Sprintf("%s 已在运行 (PID: %d)", spec.label, pid), nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	exePath := filepath.Join(filepath.Dir(executable), "crawler")
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

	cmd := buildServiceCommand(spec, exePath)
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

	return fmt.Sprintf("%s 已启动 (PID: %d)\n日志文件: %s", spec.label, cmd.Process.Pid, logPath), nil
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
	if err != nil {
		runtime.Running = false
		runtime.Detail = "PID 文件不存在或不可读"
		return runtime, nil
	}

	runtime.PID = pid
	if !IsProcessRunning(pid) {
		RemoveServicePID(service)
		runtime.Running = false
		runtime.Detail = fmt.Sprintf("PID 文件存在但进程不存在: %d", pid)
		return runtime, nil
	}

	runtime.Running = true
	runtime.Detail = fmt.Sprintf("正在运行 (PID: %d)", pid)
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
	data, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}

	allLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}

	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	for i := start; i < len(allLines); i++ {
		fmt.Println(allLines[i])
	}
	return nil
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
