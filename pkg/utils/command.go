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
	log.Infow("Executing command",
		"command", cmd.Content,
		"workDir", cmd.WorkDir,
		"env", cmd.Env,
	)

	// 构建命令
	execCmd := exec.CommandContext(ctx, "/bin/bash", "-c", cmd.Content)

	// 设置工作目录
	if cmd.WorkDir != "" {
		execCmd.Dir = cmd.WorkDir
	}

	// 设置环境变量
	if len(cmd.Env) > 0 {
		env := make([]string, 0, len(cmd.Env))
		for k, v := range cmd.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		execCmd.Env = append(execCmd.Env, env...)
	}

	// 准备标准输出和标准错误的buffer
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	// 执行命令
	err := execCmd.Run()

	// 准备返回结果
	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	// 处理错误和退出码
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Error = exitErr.Error()
		} else {
			result.ExitCode = -1
			result.Error = err.Error()
		}

		log.Errorw("Command execution failed",
			"command", cmd.Content,
			"exitCode", result.ExitCode,
			"error", result.Error,
			"stderr", result.Stderr,
		)
	} else {
		log.Infow("Command executed successfully",
			"command", cmd.Content,
			"stdout", result.Stdout,
		)
	}

	// 如果有标准错误输出，即使命令成功也记录下来
	if result.Stderr != "" && result.ExitCode == 0 {
		log.Warnw("Command completed with stderr",
			"command", cmd.Content,
			"stderr", result.Stderr,
		)
	}

	return result
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
