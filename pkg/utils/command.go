package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

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

// ExecCommand 执行命令并返回结果
func ExecCommand(ctx context.Context, cmd *Command) CommandResult {
	log := logging.NewComponentLogger("internal")
	// log.Debugw("Executing command",
	// 	"command", cmd.Content,
	// 	"workDir", cmd.WorkDir,
	// 	"env", cmd.Env,
	// )

	// Prepare command
	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd.Content)
	execCmd.Dir = cmd.WorkDir

	for k, v := range cmd.Env {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Prepare stdout/stderr buffers
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	// Execute with timeout context
	err := execCmd.Start()
	if err != nil {
		return CommandResult{
			ExitCode: -1,
			Error:    fmt.Sprintf("failed to start command: %v", err),
		}
	}

	// Wait with timeout
	done := make(chan error)
	go func() {
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
			ExitCode: -1,
			Error:    "command timeout",
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
				result.ExitCode = -1
				result.Error = fmt.Sprintf("%v: %s", err, stderr.String())
			}
			log.Errorw("Command executed with error", "result", result)
		}

		// log.Debugw("Command executed", "result", result)

		return result
	}
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
