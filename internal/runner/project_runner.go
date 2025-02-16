package runner

// TODO: 拆分 runner 基础功能和调度功能到不同文件

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/model"
	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type ProjectRunner interface {
	PrepareEnvironment(ctx context.Context, request *api.PrepareLocalEnvRequest) error
	RunTask(task *model.Task) model.TaskResult // 执行编译任务
	Cleanup(ctx context.Context) error         // 清理资源
}

// NewProjectRunner 根据传入的 containerImage 创建相应的 ProjectRunner
func NewScheduledProjectRunner(containerImage string, kvStoreClient store.KVStoreClient, workerCount int) (ProjectRunner, error) {
	if containerImage == "" {
		return NewTaskBufferedRunner(NewLocalProjectRunner(), kvStoreClient, workerCount)
	}
	return NewTaskBufferedRunner(NewContainerProjectRunner(containerImage), kvStoreClient, workerCount)
}

var log = logging.NewComponentLogger("runner")

type LocalProjectRunner struct {
	mountedRootDir  string
	mountedBuildDir string
	projectRootDir  string
}

func NewLocalProjectRunner() *LocalProjectRunner {
	return &LocalProjectRunner{}
}

func (l *LocalProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing local environment for project", "project", req.String())
	l.projectRootDir = req.Project.RootDir
	l.mountedBuildDir = utils.GetMountedBuildDir(req.Project.NinjaHost, req.Project.NinjaDir)
	mountedRootDir, err := mountNinjaProject(ctx, req)
	if err != nil {
		log.Errorw("Failed to mount ninja project", "req", req, "error", err)
		return err
	}
	l.mountedRootDir = mountedRootDir
	return nil
}

func mountNinjaProject(ctx context.Context, req *api.PrepareLocalEnvRequest) (string, error) {
	// 设置本地编译环境，例如 NFS 挂载
	project := req.GetProject()
	ninjaHost := project.GetNinjaHost()
	rootDir := project.GetRootDir()
	log.Debugw("Mounting ninja project", "ninjaHost", ninjaHost, "rootDir", rootDir)

	// 生成挂载点目录, 若目录不存在则递归创建
	// Generate mount point in user's home directory instead of /home/root
	mountedRootDir := utils.GenMountedRootDir(ninjaHost, rootDir)
	// Add timeout context
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Make directory with detailed error logging
	err := os.MkdirAll(mountedRootDir, 0755)
	if err != nil {
		log.Errorw("Failed to create directory",
			"mountedRootDir", mountedRootDir,
			"error", err,
			"currentUser", os.Getenv("USER"),
			"currentUID", os.Getuid())
		return "", fmt.Errorf("failed to create mount directory: %w", err)
	}

	log.Debugw("directory created successfully", "mountedRootDir", mountedRootDir)
	return mountedRootDir, utils.MountNFS(ctx, ninjaHost, rootDir, mountedRootDir)
}

func (l *LocalProjectRunner) RunTask(task *model.Task) model.TaskResult {
	log.Debugw("Executing local task", "task cmd", task.Command)

	task.Command = strings.Replace(task.Command, l.projectRootDir, l.mountedRootDir, -1)
	cmd := &utils.Command{
		Content: task.Command,
		WorkDir: l.mountedBuildDir,
		Env:     make(map[string]string),
	}

	cmdRes := utils.ExecCommand(context.Background(), cmd)

	status := "ok"
	var err error
	if cmdRes.ExitCode != 0 {
		status = "error"
		// 只在有错误时设置
		if cmdRes.Error != "" {
			err = errors.New(cmdRes.Error)
		}
	}

	res := model.TaskResult{
		CmdKey:   task.CmdKey,
		StdOut:   cmdRes.Stdout,
		StdErr:   cmdRes.Stderr,
		Err:      err,
		ExitCode: cmdRes.ExitCode,
		Status:   status,
	}

	log.Debugw("Task done.", "task", task, "result", res)
	return res
}

func (l *LocalProjectRunner) Cleanup(ctx context.Context) error {
	err := utils.UnmountNFS(ctx, l.mountedRootDir)
	if err != nil {
		log.Errorw("Failed to unmount nfs", "mountedRootDir", l.mountedRootDir, "error", err)
		return err
	}
	return nil
}

type ContainerProjectRunner struct {
	containerImage  string
	mountedRootDir  string
	mountedBuildDir string
	projectRootDir  string
}

func NewContainerProjectRunner(image string) *ContainerProjectRunner {
	// Strip docker:// prefix if present
	imageAddr := strings.TrimPrefix(image, "docker://")
	return &ContainerProjectRunner{
		containerImage: imageAddr,
	}
}

func (c *ContainerProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing container environment for project", "project", req.String())
	c.mountedRootDir = utils.GenMountedRootDir(req.Project.NinjaHost, req.Project.RootDir)
	c.mountedBuildDir = utils.GetMountedBuildDir(req.Project.NinjaHost, req.Project.NinjaDir)
	c.projectRootDir = req.Project.RootDir
	// 1. 挂载项目
	_, err := mountNinjaProject(ctx, req)
	if err != nil {
		log.Errorw("Failed to mount project", "req", req, "error", err)
		return err
	}

	// 2. 后台拉取镜像
	go func(image string) {
		bgCtx := context.Background() // 独立上下文，不会受请求的 ctx 限制
		if err := utils.PullImageIfNecessary(bgCtx, image); err != nil {
			log.Errorw("Failed to pull image in background", "image", image, "error", err)
		} else {
			log.Infow("Image is ready in background", "image", image)
		}
	}(c.containerImage)

	// 3. 提前返回，拉取任务在后台进行
	return nil

}

func (c *ContainerProjectRunner) RunTask(task *model.Task) model.TaskResult {
	// 使用传入的上下文或创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	task.Command = strings.Replace(task.Command, c.projectRootDir, c.mountedRootDir, -1)
	log.Infow("Starting container task execution",
		"cmdKey", task.CmdKey,
		"command", task.Command,
		"mountedRootDir", c.mountedRootDir,
		"mountedBuildDir", c.mountedBuildDir,
	)

	// 创建 Docker 客户端
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return c.newErrorResult(task.CmdKey, "failed to create docker client", err)
	}
	defer cli.Close()

	// 创建容器
	containerID, err := c.createContainer(ctx, cli, task.Command)
	if err != nil {
		log.Errorw("Failed to create container", "cmdKey", task.CmdKey, "error", err)
		return c.newErrorResult(task.CmdKey, "failed to create container", err)
	}

	// 确保容器被清理
	defer func() {
		// 暂不清理，开发环境调试时保留容器
		// c.cleanupContainer(ctx, cli, containerID)
	}()

	// 启动容器
	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return c.newErrorResult(task.CmdKey, "failed to start container", err)
	}

	// 等待容器执行完成
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var status container.WaitResponse
	select {
	case err := <-errCh:
		return c.newErrorResult(task.CmdKey, "container wait failed", err)
	case status = <-statusCh:
		// 继续处理
	case <-ctx.Done():
		return c.newErrorResult(task.CmdKey, "container execution timeout", ctx.Err())
	}

	// 获取容器日志
	stdout, stderr, err := c.getContainerLogs(ctx, cli, containerID)
	if err != nil {
		return c.newErrorResult(task.CmdKey, "failed to get container logs", err)
	}

	res := model.TaskResult{
		CmdKey:   task.CmdKey,
		StdOut:   stdout,
		StdErr:   stderr,
		Err:      nil,
		ExitCode: int(status.StatusCode),
		Status:   c.getStatus(status.StatusCode),
	}

	log.Debugw("Container Task done.", "task", task, "result", res)
	return res
}

// 辅助方法
func (c *ContainerProjectRunner) createContainer(ctx context.Context, cli *client.Client, cmd string) (string, error) {
	// 获取当前用户的 UID 和 GID
	currentUID := os.Getuid()
	currentGID := os.Getgid()

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      c.containerImage,
		Cmd:        []string{"/bin/bash", "-c", cmd},
		WorkingDir: c.mountedBuildDir,
		Tty:        false,
		User:       fmt.Sprintf("%d:%d", currentUID, currentGID), // 使用宿主机的用户权限
	}, &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: c.mountedRootDir,
			Target: c.mountedRootDir,
		}},
		Resources: container.Resources{
			Memory:    2 * 1024 * 1024 * 1024, // 2GB
			CPUQuota:  100000,
			CPUPeriod: 100000,
		},
	}, nil, nil, "")

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

func (c *ContainerProjectRunner) getContainerLogs(ctx context.Context, cli *client.Client, containerID string) (string, string, error) {
	logs, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", "", err
	}
	defer logs.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, logs)
	if err != nil {
		return "", "", err
	}

	return stdout.String(), stderr.String(), nil
}

func (c *ContainerProjectRunner) cleanupContainer(ctx context.Context, cli *client.Client, containerID string) {
	timeout := 20
	err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		log.Warnw("Failed to stop container", "containerID", containerID, "error", err)
	}

	err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: true,
	})
	if err != nil {
		log.Warnw("Failed to remove container", "containerID", containerID, "error", err)
	}
}

func (c *ContainerProjectRunner) newErrorResult(cmdKey string, msg string, err error) model.TaskResult {
	return model.TaskResult{
		CmdKey:   cmdKey,
		StdOut:   "",
		StdErr:   fmt.Sprintf("%s: %v", msg, err),
		Err:      err,
		ExitCode: 1,
		Status:   "error",
	}
}

func (c *ContainerProjectRunner) getStatus(exitCode int64) string {
	if exitCode == 0 {
		return "ok"
	}
	return "error"
}

func (c *ContainerProjectRunner) Cleanup(ctx context.Context) error {
	// 进行 prepareEnvironment 的反向操作
	// 1. 清理挂载的目录
	err := utils.UnmountNFS(ctx, c.mountedRootDir)
	if err != nil {
		log.Errorw("Failed to unmount nfs", "mountedRootDir", c.mountedRootDir, "error", err)
		return err
	}

	// 2. 清理镜像容器(开发环境保留)
	// utils.RemoveImageAndContainers(context.Background(), c.containerImage)
	return nil
}

// 基础 Runner 之上装饰调度功能，只不过这里的调度是抢占式
type TaskBufferedRunner struct {
	baseRunner  ProjectRunner
	LocalQueue  chan model.Task     // 本地命令任务队列
	RemoteQueue chan model.Task     // 非本地（其他远程节点）命令任务队列
	stopChan    chan struct{}       // 停止所有 worker 的信号
	KVStore     store.KVStoreClient // 存储客户端
	wg          sync.WaitGroup      // 用于管理协程的退出
	workerCount int                 // worker 数量
	localIPv4   string              // 本机 IP 地址
}

// 初始化时自动启动 worker pool
func NewTaskBufferedRunner(baseRunner ProjectRunner, kvStore store.KVStoreClient, workerCount int) (*TaskBufferedRunner, error) {
	if kvStore == nil {
		return nil, errors.New("kvStore cannot be nil")
	}
	if workerCount <= 0 {
		return nil, errors.New("workerCount must be greater than zero")
	}

	// 初始化 TaskBufferedRunner
	taskBufferedRunner := &TaskBufferedRunner{
		baseRunner:  baseRunner,
		LocalQueue:  make(chan model.Task, config.QUEUESIZE),
		RemoteQueue: make(chan model.Task, config.QUEUESIZE),
		stopChan:    make(chan struct{}),
		KVStore:     kvStore,
		workerCount: workerCount,
		localIPv4:   utils.GetOutboundIP().String(),
	}

	// 启动所有 worker 协程
	go taskBufferedRunner.startWorkers()

	log.Infow("TaskBufferedRunner created successfully.", "workerCount", workerCount)

	return taskBufferedRunner, nil
}

func (r *TaskBufferedRunner) Stop() {
	close(r.stopChan)
	r.wg.Wait() // 等待所有 worker 停止
}

// 包装基础方法
func (r *TaskBufferedRunner) PrepareEnvironment(ctx context.Context, request *api.PrepareLocalEnvRequest) error {
	return r.baseRunner.PrepareEnvironment(ctx, request)
}

func (r *TaskBufferedRunner) RunTask(task *model.Task) model.TaskResult {
	// r.submitCommand(common.GenCmdKey(req.Project, req.CmdId))
	return r.baseRunner.RunTask(task)
}

func (r *TaskBufferedRunner) Cleanup(ctx context.Context) error {
	return r.baseRunner.Cleanup(ctx)
}

// 启动 Worker
func (r *TaskBufferedRunner) startWorkers() {
	for i := 0; i < r.workerCount; i++ {
		r.wg.Add(1)
		go r.worker(fmt.Sprintf("worker-%d", i+1))
	}
}

// 提交命令到队列
// func (r *TaskBufferedRunner) submitCommand(cmdKey string) {
// 	r.localQueue <- cmdKey
// }

// Worker 逻辑
func (r *TaskBufferedRunner) worker(workerID string) {
	defer r.wg.Done()
	// Worker handles full task lifecycle
	for {
		select {
		case <-r.stopChan:
			fmt.Printf("[%s] Stopping worker\n", workerID)
			return
		default:
			// 优先执行本地任务
			var task model.Task
			var ok bool
			select {
			case task = <-r.LocalQueue:
				ok = true
			default:
				// 本地任务为空，再尝试远程任务
				select {
				case task = <-r.RemoteQueue:
					ok = true
				default:
					ok = false
				}
			}

			if !ok {
				// 当前无任务，避免 busy-wait
				time.Sleep(50 * time.Millisecond)
				continue
			}

			// Try claim task
			result, err := r.TryClaimTask(context.Background(), task.CmdKey, workerID)
			if err != nil {
				task.ResultChan <- model.TaskResult{
					CmdKey: task.CmdKey,
					Status: "error",
					Err:    err,
				}
				continue
			}
			// Handle claim result
			switch result.Code {
			case -1: // 任务不存在
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_RESOURCE_NOT_FOUND,
					Message:    "task not found",
				}
			case 1: // 任务状态不匹配
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_TASK_ALREADY_CLAIMED,
					Message:    "task already claimed by others",
				}
			case 2: // 参数错误
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_INVALID_ARGUMENT,
					Message:    "invalid arguments",
				}
			case 0: // 成功 claimed
				// 从 redis 获取具体命令内容
				content, err := r.KVStore.HGet(context.Background(), task.CmdKey, "content")
				if err != nil {
					task.ResultChan <- model.TaskResult{
						CmdKey: task.CmdKey,
						Status: "error",
						Err:    fmt.Errorf("failed to get command content: %v", err),
					}
					continue
				}

				// 执行命令
				task.Command = content
				taskRes := r.baseRunner.RunTask(&task)
				taskRes.StatusCode = api.RC_EXECUTOR_OK
				// 任务执行成功：更新任务状态为完成.
				if taskRes.ExitCode == 0 {
					r.TryFinishTask(context.Background(), task.CmdKey, workerID)
				} else { // fallback
					r.UnclaimeTask(context.Background(), task.CmdKey, workerID)
				}
				task.ResultChan <- taskRes
			default:
				log.Errorf("[%s] Unexpected result code: %d\n", workerID, result.Code)
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_UNEXPECTED_RESULT,
					Message:    "unexpected result code",
				}
			}

		}
	}
}

func (r *TaskBufferedRunner) TryClaimTask(ctx context.Context, cmdKey, operator string) (store.ClaimCmdResult, error) {
	return r.KVStore.ClaimCmd(ctx, cmdKey, model.Unclaimed, model.Claimed, operator, config.TASKTTL)
}

func (r *TaskBufferedRunner) UnclaimeTask(ctx context.Context, cmdKey, operator string) (store.ClaimCmdResult, error) {
	return r.KVStore.ClaimCmd(ctx, cmdKey, model.Claimed, model.Unclaimed, operator, config.TASKTTL)
}

func (r *TaskBufferedRunner) TryFinishTask(ctx context.Context, cmdKey, operator string) (store.ClaimCmdResult, error) {
	return r.KVStore.ClaimCmd(ctx, cmdKey, model.Claimed, model.Done, operator, config.TASKTTL)
}

// 处理具体任务
func (r *TaskBufferedRunner) SubmitAndWaitTaskRes(ctx context.Context, task *model.Task, operator string) (model.TaskResult, error) {
	// 根据 cmdKey 判断是否本机任务
	isLocal := strings.HasPrefix(task.CmdKey, r.localIPv4)
	var queue chan model.Task
	if isLocal {
		queue = r.LocalQueue
	} else {
		queue = r.RemoteQueue
	}

	// 提交任务并等待结果
	select {
	case queue <- *task:
		select {
		case result := <-task.ResultChan:
			return result, result.Err
		case <-ctx.Done():
			return model.TaskResult{}, ctx.Err()
		}
	default:
		return model.TaskResult{}, errors.New("task queue is full")
	}
}
