package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"
)

// Command 表示要执行的命令
type Command struct {
	Content string            // 命令内容
	WorkDir string            // 工作目录
	Env     map[string]string // 环境变量
}

// CommandResult 表示命令执行的结果
type CommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// ExecCommand 执行命令并返回结果，失败时自动重试
func ExecCommand(ctx context.Context, cmd *Command) CommandResult {
	log := logging.NewComponentLogger("internal")
	if cmd.Content == "" {
		log.Fatalw("Command content is empty", "pid", os.Getpid())
		return CommandResult{
			ExitCode: -1,
			Error:    "command content is empty",
		}
	}

	// 重试参数
	maxRetries := 3
	retryDelay := 500 * time.Millisecond

	var lastResult CommandResult
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 记录重试日志
		if attempt > 0 {
			log.Infow("Retrying command execution", "attempt", attempt,
				"maxRetries", maxRetries, "command", cmd.Content, "pid", os.Getpid())

			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				return CommandResult{
					Stdout:   lastResult.Stdout,
					Stderr:   lastResult.Stderr,
					ExitCode: -2,
					Error:    "context cancelled during retry wait",
				}
			case <-time.After(retryDelay):
				// 等待重试延迟时间
			}

			// 指数退避: 每次重试增加等待时间
			retryDelay = retryDelay * 2
		}

		// Prepare command
		execCmd := exec.CommandContext(ctx, "sh", "-c", cmd.Content)
		execCmd.Dir = cmd.WorkDir

		// 设置环境变量
		execCmd.Env = os.Environ() // 先继承全局环境变量
		// for aosp: 手动添加 TOP="/home/lab2/android-12.0.0_r4" 环境变量
		// 对于 AOSP, project_root_dir 和 work_dir 为同一个目录
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("TOP=%s", cmd.WorkDir))
		for k, v := range cmd.Env {
			execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		// Prepare stdout/stderr buffers
		var stdout, stderr bytes.Buffer
		execCmd.Stdout = &stdout
		execCmd.Stderr = &stderr

		startTime := time.Now()
		// Execute with timeout context
		err := execCmd.Start()
		if err != nil {
			lastResult = CommandResult{
				ExitCode: -3,
				Error:    fmt.Sprintf("failed to start command: %v", err),
			}
			continue // 命令启动失败，尝试重试
		}

		// Wait with timeout
		done := make(chan error)
		go func() {
			// 等待命令执行完成
			done <- execCmd.Wait()
		}()

		// Wait for completion or timeout
		select {
		case <-ctx.Done():
			// Try to kill if context cancelled
			execCmd.Process.Kill()
			return CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: -4,
				Error:    "command timeout: " + ctx.Err().Error(),
			}
		case err := <-done:
			result := CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: 0,
			}

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					result.ExitCode = exitErr.ExitCode()
					result.Error = fmt.Sprintf("%s: %s", exitErr.Error(), stderr.String())
				} else {
					result.ExitCode = -5
					result.Error = fmt.Sprintf("%v: %s", err, stderr.String())
				}
				log.Warnw("Command execution failed", "attempt", attempt+1,
					"result", result, "pid", os.Getpid())

				// 保存结果并尝试重试
				lastResult = result
				continue
			}

			// 命令成功执行
			if attempt > 0 {
				log.Infow("Command succeeded after retries", "attempts", attempt+1)
			}

			execTime := time.Since(startTime)
			if execTime > 4*time.Minute {
				log.Warnw("Long running command", "duration", execTime, "command", cmd.Content)
			}

			return result
		}
	}

	// 达到最大重试次数
	// !TODO: debug 临时使用 Fatalw, 后续改为 warning
	log.Warnw("Command failed after maximum retries", "maxRetries", maxRetries,
		"lastExitCode", lastResult.ExitCode, "lastError", lastResult.Error, "command", cmd)

	return lastResult
}

// WithStdio 允许自定义标准输入输出
type StdioOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// ExecCommandWithStdio 执行命令并允许自定义标准输入输出
func ExecCommandWithStdio(ctx context.Context, cmd *Command, stdio *StdioOptions) CommandResult {
	execCmd := exec.CommandContext(ctx, "/bin/bash", "-c", cmd.Content)

	if cmd.WorkDir != "" {
		execCmd.Dir = cmd.WorkDir
	}

	if len(cmd.Env) > 0 {
		env := make([]string, 0, len(cmd.Env))
		for k, v := range cmd.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		execCmd.Env = append(execCmd.Env, env...)
	}

	// 设置标准输入输出
	if stdio != nil {
		if stdio.Stdin != nil {
			execCmd.Stdin = stdio.Stdin
		}
		if stdio.Stdout != nil {
			execCmd.Stdout = stdio.Stdout
		}
		if stdio.Stderr != nil {
			execCmd.Stderr = stdio.Stderr
		}
	}

	// 如果没有设置自定义输出，使用buffer捕获输出
	var stdout, stderr bytes.Buffer
	if execCmd.Stdout == nil {
		execCmd.Stdout = &stdout
	}
	if execCmd.Stderr == nil {
		execCmd.Stderr = &stderr
	}

	err := execCmd.Run()

	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		result.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result
}
