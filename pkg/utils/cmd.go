package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chanfun-ren/executor/pkg/config"

	"github.com/chanfun-ren/executor/pkg/status"
)

const (
	// KilledExitCode 是“os/exec”包在进程被终止时使用的特殊退出代码值。
	KilledExitCode = -1
	// NoExitCode 表示缺少退出代码值，通常是因为进程从未启动，或者由于错误而无法确定其实际退出代码。
	NoExitCode = -2
)

type Command struct {
	Content     string
	Env         []string // 每个元素的形式为“key=value”。
	Use_console bool
}

type Stdio struct {
	// Stdin 是已执行进程的可选标准输入源。
	Stdin io.Reader
	// Stdout 是已执行进程的可选 stdout 接收器。
	Stdout io.Writer
	// Stderr 是已执行进程的可选 stderr 接收器。
	Stderr io.Writer
}

type CommandResult struct {
	// 只有在命令无法启动，或者已经启动但没有完成的情况下，才会弹出错误信息。
	//
	// 如果该命令运行并返回一个非零的退出代码（比如1）。
	// 这被认为是一个成功的执行，这个错误将不会被填入。
	//
	// 在某些情况下，该命令可能由于与命令本身无关的问题而未能启动。例如，gRPC错误代码
	//
	// 如果对`exec.Cmd#Run'的调用返回-1，意味着命令被杀死或从未退出，那么这个字段应填入一个gRPC错误代码
	// 说明原因，比如DEADLINE_EXCEEDED（如果命令超时），UNAVAILABLE（如果有一个可以重试的瞬时错误），
	// 或RESOURCE_EXHAUSTED（如果该命令在执行过程中耗尽了内存）。
	Error error

	// 来自命令的标准输出。 即使出现错误，这也可能包含数据。
	Stdout []byte
	// 来自命令的标准错误。 即使出现错误，这也可能包含数据。
	Stderr []byte

	// ExitCode 是以下之一：
	// * 执行命令返回的退出代码
	// * -1 如果进程被杀死或没有退出
	// * -2 (NoExitCode) 如果因为返回而无法确定退出代码
	//   exec.ExitError 以外的错误。 这种情况通常意味着它无法启动。
	ExitCode int
}

func constructExecCommand(command *Command, workDir string, stdio *Stdio, designatedUser bool) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	// log.Debugw("constructExecCommand", "cmd_content", command.Content, "workDir", workDir)

	if stdio == nil {
		stdio = &Stdio{}
	}
	var cmd *exec.Cmd = exec.Command("/bin/bash", "-c", command.Content)
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if stdio.Stdout != nil {
		cmd.Stdout = stdio.Stdout
	}
	cmd.Stderr = &stderr
	if stdio.Stderr != nil {
		cmd.Stderr = stdio.Stderr
	}
	// 注意：在这里使用 StdinPipe() 而不是 cmd.Stdin，因为后一种方法会导致一个错误，
	// 如果进程不使用其标准输入，cmd.Wait() 可能会无限期挂起。https://go.dev/play/p/DpKaVrx8d8G
	if stdio.Stdin != nil {
		inp, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get stdin pipe: %s", err)
		}
		go func() {
			defer inp.Close()
			io.Copy(inp, stdio.Stdin)
		}()
	}

	// TODO: 指定用户
	// if designatedUser {
	// 	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// 	cmd.SysProcAttr.Credential = &syscall.Credential{
	// 		Uid: config.Config.User.Uid,
	// 		Gid: config.Config.User.Gid,
	// 	}
	// }

	/*for _, envVar := range command.Env {
		cmd.Env = append(cmd.Env, envVar)
	}*/
	cmd.Env = append(cmd.Env, command.Env...)
	return cmd, &stdout, &stderr, nil
}

// HandleError 任务重试三次，若都失败，则直接返回给scheduler，让Local_executor执行这条任务
func HandleError(fn func() error) (nBusy int, err error) {
	nBusy = 0
	for {
		err = fn()
		if err != nil && nBusy < 3 {
			nBusy++
			if strings.Contains(err.Error(), "go/apex-allowed-deps-error") {
				//让local_executor来执行 update-apex-allowed-deps.sh
				return
			} else {
				nBusy++
				time.Sleep(100 * time.Millisecond << uint(nBusy))
			}
		}
		return nBusy, err
	}
}

func Run(ctx context.Context, command *Command, workDir string, stdio *Stdio, designatedUser bool) (int, *CommandResult) {
	var cmd *exec.Cmd
	var stdoutBuf, stderrBuf *bytes.Buffer

	nBusy, err := HandleError(func() error {
		// 每次尝试都创建一个新命令，因为命令只能运行一次。
		var err error
		cmd, stdoutBuf, stderrBuf, err = constructExecCommand(command, workDir, stdio, designatedUser)
		if err != nil {
			return err
		}
		err = cmd.Run()
		if err != nil {
			log.Warnw("cmd.Run() fail", "cmd", cmd, "err", err, "cmd.Stderr", cmd.Stderr)
		}
		return err
	})

	if err != nil {
		// fmt.Printf("err:\033[1;38;40m%v\033[0m\n", err)
		log.Warnw("err", err)
	}

	exitCode, err := ExitCode(ctx, cmd, err)

	if stdoutBuf.Len() > 0 {
		log.Infow("cmd", cmd, "stdout", stdoutBuf)
	}
	if stderrBuf.Len() > 0 {
		log.Infow("cmd", cmd, "stderr", stderrBuf)
	}

	res := &CommandResult{
		ExitCode: exitCode,
		Error:    err,
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
	}

	return nBusy, res
}

func ExitCode(ctx context.Context, cmd *exec.Cmd, err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	// exec.Error 仅在 exec.LookPath 无法将文件归类为可执行文件时返回。
	// 这可能是“not found”错误或权限错误，但我们只是将其报告为“not found”。
	if notFoundErr, ok := err.(*exec.Error); ok {
		return NoExitCode, status.NotFoundError(notFoundErr.Error())
	}

	// 如果我们由于任何其他原因未能获得进程的退出代码，
	// 则可能是客户端可以重试的暂时性错误，因此暂时返回 UNAVAILABLE。
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return NoExitCode, status.UnavailableError(err.Error())
	}
	processState := exitErr.ProcessState
	if processState == nil {
		return NoExitCode, status.UnavailableError(err.Error())
	}

	exitCode := processState.ExitCode()
	if exitCode == KilledExitCode { // KilledExitCode = -1
		if dl, ok := ctx.Deadline(); ok && time.Now().After(dl) {
			return exitCode, status.DeadlineExceededErrorf("Command timed out: %s", err.Error())
		}
		// 如果命令没有超时，可能是因为 OOM（Out Of Memory） 被内核 kill 掉了。
		return exitCode, status.ResourceExhaustedErrorf("Command was killed: %s", err.Error())
	}

	return exitCode, nil
}

func RunACmd(ctx context.Context, content string, workDir string) *CommandResult {
	cmd := &Command{Content: content}
	var stdout, stderr bytes.Buffer
	stdio := &Stdio{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_, cmdRes := Run(ctx, cmd, workDir, stdio, false)
	log.Debugw("Run cmd done", "Cmd", cmd, "cmdRes", cmdRes)
	return cmdRes
}

func GenMountedRootDir(ninjaHost string, projRootDir string) string {
	return config.GetExecutorHome() + ninjaHost + projRootDir
}

func MountNFS(ctx context.Context, nfsServerHost, nfsRemotePath, localMountPoint string) error {
	// 拼接 mount 命令并执行
	format := "mount -t nfs -o async %s:%s %s"
	mountCmd := fmt.Sprintf(format, nfsServerHost, nfsRemotePath, localMountPoint)
	getwd, err := os.Getwd()
	if err != nil {
		log.Fatalw("fail to get current working directory", "err", err)
	}

	cmdRes := RunACmd(ctx, mountCmd, getwd)
	if cmdRes.ExitCode == 0 {
		return nil
	}
	return fmt.Errorf("mount nfs failed: %s", cmdRes.Stderr)
}
