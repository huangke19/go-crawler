package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

const (
	dailyMonitorCheckHour    = 10
	dailyMonitorCheckMinute  = 0
	scheduledCheckTimeout    = 30 * time.Minute
	scheduledOutputTrimLimit = 4000
)

// nextScheduledDailyCheck 返回下一次本地时间固定时刻的执行时间。
func nextScheduledDailyCheck(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// startDailyCheckScheduler 启动每天 10:00 的全量检查任务。
// 每次触发时热加载 config.json 中的 monitor_accounts，并顺序执行 `crawler cu <username>`。
func (ws *WorkerServer) startDailyCheckScheduler() {
	log.Printf("定时任务启动：每天 %02d:%02d 自动检查 monitor_accounts", dailyMonitorCheckHour, dailyMonitorCheckMinute)

	go func() {
		for {
			nextRun := nextScheduledDailyCheck(time.Now(), dailyMonitorCheckHour, dailyMonitorCheckMinute)
			wait := time.Until(nextRun)
			log.Printf("定时任务：下一次执行时间 %s（%.0f 分钟后）", nextRun.Format("2006-01-02 15:04:05"), wait.Minutes())

			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
				ws.runScheduledMonitorChecks()
			case <-ws.stopCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				log.Println("定时任务：收到停止信号，退出调度 goroutine")
				return
			}
		}
	}()
}

func (ws *WorkerServer) runScheduledMonitorChecks() {
	accounts, _, _, err := ws.loadMonitorConfigSnapshot()
	if err != nil {
		log.Printf("定时任务：加载 monitor_accounts 失败: %v", err)
		return
	}
	if len(accounts) == 0 {
		log.Println("定时任务：monitor_accounts 为空，跳过本次执行")
		return
	}

	log.Printf("定时任务：开始执行每日检查，共 %d 个账户", len(accounts))
	startedAt := time.Now()

	successCount := 0
	for _, username := range accounts {
		select {
		case <-ws.stopCh:
			log.Println("定时任务：收到停止信号，中止剩余账户检查")
			return
		default:
		}

		if err := ws.runScheduledCheckUpdate(username); err != nil {
			log.Printf("定时任务：@%s 检查失败: %v", username, err)
			continue
		}
		successCount++
	}

	log.Printf(
		"定时任务：每日检查完成，成功 %d/%d，耗时 %s",
		successCount,
		len(accounts),
		time.Since(startedAt).Round(time.Second),
	)
}

func (ws *WorkerServer) runScheduledCheckUpdate(username string) error {
	executable, err := resolveExecutablePath()
	if err != nil {
		return fmt.Errorf("解析 crawler 路径失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), scheduledCheckTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		select {
		case <-ws.stopCh:
			cancel()
		case <-done:
		}
	}()
	defer close(done)

	cmd := exec.CommandContext(ctx, executable, "cu", username)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	startedAt := time.Now()
	log.Printf("定时任务：开始执行 ./crawler cu %s", username)

	err = cmd.Run()
	duration := time.Since(startedAt).Round(time.Second)
	trimmedOutput := trimScheduledCommandOutput(output.String())

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("执行超时（%s）输出: %s", scheduledCheckTimeout, trimmedOutput)
	}
	if ctx.Err() == context.Canceled {
		return fmt.Errorf("执行已取消，输出: %s", trimmedOutput)
	}
	if err != nil {
		return fmt.Errorf("命令执行失败（耗时 %s）输出: %s", duration, trimmedOutput)
	}

	if trimmedOutput != "" {
		log.Printf("定时任务：@%s 检查完成，耗时 %s，输出:\n%s", username, duration, trimmedOutput)
	} else {
		log.Printf("定时任务：@%s 检查完成，耗时 %s", username, duration)
	}

	return nil
}

func trimScheduledCommandOutput(output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		return ""
	}
	if len(text) <= scheduledOutputTrimLimit {
		return text
	}
	return text[:scheduledOutputTrimLimit] + "\n...<truncated>"
}
