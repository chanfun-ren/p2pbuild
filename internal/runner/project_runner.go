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
	"sync/atomic"
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
	"golang.org/x/exp/rand"
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
	mountedRootDir  string // executor 挂载的项目主目录
	mountedBuildDir string
	projectRootDir  string // 客户端待编译项目主目录，同时也是 executor 的 workspace
	isLocalExecutor bool
}

func NewLocalProjectRunner() *LocalProjectRunner {
	return &LocalProjectRunner{
		isLocalExecutor: false,
	}
}

func (l *LocalProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing local environment for project", "project", req.String())
	l.projectRootDir = req.Project.RootDir

	if strings.HasPrefix(req.Project.NinjaHost, utils.GetOutboundIP().String()) {
		// local executor
		l.isLocalExecutor = true
		l.mountedRootDir = req.Project.RootDir
		l.mountedBuildDir = req.Project.NinjaDir
		return nil
	}

	// l.mountedBuildDir = utils.GetMountedBuildDir(req.Project.NinjaHost, req.Project.NinjaDir)
	// 使用和客户端完全相同的路径 mount.
	l.mountedBuildDir = req.Project.NinjaDir
	l.mountedRootDir = req.Project.RootDir
	err := mountNinjaProject(ctx, req, l.mountedRootDir)
	if err != nil {
		log.Errorw("Failed to mount ninja project", "req", req, "error", err)
		return err
	}
	return nil
}

func mountNinjaProject(ctx context.Context, req *api.PrepareLocalEnvRequest, mountedRootDir string) error {
	// 设置本地编译环境，例如 NFS 挂载
	project := req.GetProject()
	ninjaHost := project.GetNinjaHost()
	rootDir := project.GetRootDir()
	log.Debugw("Mounting ninja project", "ninjaHost", ninjaHost, "rootDir", rootDir)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 生成挂载点目录, 若目录不存在则递归创建
	// Make directory with detailed error logging
	err := os.MkdirAll(mountedRootDir, 0755)
	if err != nil {
		log.Errorw("Failed to create directory",
			"mountedRootDir", mountedRootDir,
			"error", err,
			"currentUser", os.Getenv("USER"),
			"currentUID", os.Getuid())
		return fmt.Errorf("failed to create mount directory: %w", err)
	}

	err = utils.MountNFS(ctx, ninjaHost, rootDir, mountedRootDir)
	if err != nil {
		log.Errorw("Failed to mount nfs", "mountedRootDir", mountedRootDir, "error", err)
		return err
	}

	// 本地生成一个和客户端项目主目录同名路径，将其映射到挂载路径
	// err = utils.MapWorkspace(mountedRootDir, rootDir)
	// if err != nil {
	// 	log.Debugw("Failed to map workspace", "mountedRootDir", mountedRootDir, "rootDir", rootDir, "error", err)
	// 	return err
	// }

	// log.Infow("Workspace created successfully", "mountedRootDir", mountedRootDir, "WorkspaceDir", rootDir)

	return nil
}

func umountNinjaProject(mountedRootDir, projectRootDir string) error {
	// Add timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := utils.UnmountNFS(ctx, mountedRootDir)
	if err != nil {
		log.Errorw("Failed to unmount nfs", "mountedRootDir", mountedRootDir, "error", err)
		return err
	}

	// err = utils.ClearWorkspace(projectRootDir)
	// if err != nil {
	// 	log.Errorw("Failed to clear workspace", "projectRootDir", projectRootDir, "error", err)
	// 	return err
	// }
	return nil
}

func (l *LocalProjectRunner) RunTask(task *model.Task) model.TaskResult {
	// log.Debugw("Executing local task", "task cmd", task.Command)

	// map workspace 后原理上不再需要更换路径
	// task.Command = strings.Replace(task.Command, l.projectRootDir, l.mountedRootDir, -1)
	cmd := &utils.Command{
		Content: task.Command,
		WorkDir: l.mountedBuildDir,
		Env:     make(map[string]string),
	}

	timeout := 2 * time.Minute
	if l.isLocalExecutor {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmdRes := utils.ExecCommand(ctx, cmd)

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
		TaskKey:  task.TaskKey,
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
	if l.isLocalExecutor {
		return nil
	}
	return umountNinjaProject(l.mountedRootDir, l.projectRootDir)
}

// TODO: 本地 executor 统一本地执行用 localProjectRunner 的执行逻辑, 不用 container, 提高效率
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
	err := mountNinjaProject(ctx, req, c.mountedRootDir)
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
		"TaskKey", task.TaskKey,
		"command", task.Command,
		"mountedRootDir", c.mountedRootDir,
		"mountedBuildDir", c.mountedBuildDir,
	)

	// 创建 Docker 客户端
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return c.newErrorResult(task.TaskKey, "failed to create docker client", err)
	}
	defer cli.Close()

	// 创建容器
	containerID, err := c.createContainer(ctx, cli, task.Command)
	if err != nil {
		log.Errorw("Failed to create container", "TaskKey", task.TaskKey, "error", err)
		return c.newErrorResult(task.TaskKey, "failed to create container", err)
	}

	// 确保容器被清理
	defer func() {
		// 暂不清理，开发环境调试时保留容器
		// c.cleanupContainer(ctx, cli, containerID)
	}()

	// 启动容器
	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return c.newErrorResult(task.TaskKey, "failed to start container", err)
	}

	// 等待容器执行完成
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var status container.WaitResponse
	select {
	case err := <-errCh:
		return c.newErrorResult(task.TaskKey, "container wait failed", err)
	case status = <-statusCh:
		// 继续处理
	case <-ctx.Done():
		return c.newErrorResult(task.TaskKey, "container execution timeout", ctx.Err())
	}

	// 获取容器日志
	stdout, stderr, err := c.getContainerLogs(ctx, cli, containerID)
	if err != nil {
		return c.newErrorResult(task.TaskKey, "failed to get container logs", err)
	}

	res := model.TaskResult{
		TaskKey:  task.TaskKey,
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
			Target: c.mountedRootDir, // TODO: 变更为 projectRootDir
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

func (c *ContainerProjectRunner) newErrorResult(TaskKey string, msg string, err error) model.TaskResult {
	return model.TaskResult{
		TaskKey:  TaskKey,
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
	baseRunner         ProjectRunner
	LocalQueue         chan model.Task     // 本地命令任务队列
	RemoteQueue        chan model.Task     // 非本地（其他远程节点）命令任务队列
	stopChan           chan struct{}       // 停止所有 worker 的信号
	KVStore            store.KVStoreClient // 存储客户端
	wg                 sync.WaitGroup      // 用于管理协程的退出
	workerCount        int                 // worker 数量
	localIPv4          string              // 本机 IP 地址
	ReceivedtaskCount  int                 // 接收到的总的任务数量
	PreemptedTaskCount atomic.Uint64       // 抢占成功的任务数量
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
		baseRunner:         baseRunner,
		LocalQueue:         make(chan model.Task, config.GlobalConfig.QueueSize),
		RemoteQueue:        make(chan model.Task, config.GlobalConfig.QueueSize),
		stopChan:           make(chan struct{}),
		KVStore:            kvStore,
		workerCount:        workerCount,
		localIPv4:          utils.GetOutboundIP().String(),
		ReceivedtaskCount:  0,
		PreemptedTaskCount: atomic.Uint64{},
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
	return r.baseRunner.RunTask(task)
}

func (r *TaskBufferedRunner) Cleanup(ctx context.Context) error {
	err := r.baseRunner.Cleanup(ctx)
	r.KVStore.FlushDB(ctx)
	log.Infow("ProjectRunner free...", "ReceivedtaskCount", r.ReceivedtaskCount, "PreemptedTaskCount", r.PreemptedTaskCount.Load())
	r.Stop()
	return err
}

// 启动 Worker
func (r *TaskBufferedRunner) startWorkers() {
	for i := 0; i < r.workerCount; i++ {
		r.wg.Add(1)
		go r.worker_busy_wait(fmt.Sprintf("worker-%d", i+1))
	}
}

// TODO: 修复下面的 worker 逻辑，偶尔会导致 server 卡死并且 CPU 利用率不饱和
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
			// 优先执行本地任务 (非阻塞尝试)
			var task model.Task
			var ok bool
			select {
			case task = <-r.LocalQueue:
				ok = true
			default:
				// 本地任务为空，进入下一步判断或等待
				ok = false
			}

			// 如果本地队列没有任务，则尝试阻塞地等待 本地队列、远程队列 或 停止信号
			if !ok {
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
				select {
				// 再次检查 LocalQueue，因为在上次检查后可能有新任务到达，维持优先级
				case task = <-r.LocalQueue:
					ok = true
					// 如果 LocalQueue 仍然没有，等待 RemoteQueue
				case task = <-r.RemoteQueue:
					ok = true
					// 同时等待停止信号
				case <-r.stopChan:
					fmt.Printf("[%s] Stopping worker\n", workerID)
					return // 收到停止信号，直接返回
				}
			}

			// 注意：如果上面的阻塞 select 因为 stopChan 而返回，ok 会是 false，不会进入这里
			if ok {
				// Try claim task
				result, err := r.TryClaimTask(context.Background(), task.TaskKey, workerID)
				if err != nil {
					task.ResultChan <- model.TaskResult{
						TaskKey: task.TaskKey,
						Status:  "claim_error",
						Err:     err,
					}
					continue // 处理下一个循环
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
					r.PreemptedTaskCount.Add(1)
					// 从 redis 获取具体命令内容
					content, err := r.KVStore.HGet(context.Background(), task.TaskKey, "content")
					if err != nil {
						task.ResultChan <- model.TaskResult{
							TaskKey: task.TaskKey,
							Status:  "error",
							Err:     fmt.Errorf("failed to get command content: %v", err),
						}
						continue // 处理下一个循环
					}

					// 执行命令
					task.Command = content
					taskRes := r.baseRunner.RunTask(&task)
					taskRes.StatusCode = api.RC_EXECUTOR_OK
					// 任务执行成功：更新任务状态为完成.
					if taskRes.ExitCode == 0 {
						r.TryFinishTask(context.Background(), task.TaskKey, workerID)
						log.Infow("Task done.", "taskKey", task.TaskKey, "workerID", workerID)
					} else { // fallback
						log.Warnw("Task failed, unclaiming...", "taskKey", task.TaskKey, "workerID", workerID)
						r.UnclaimTask(context.Background(), task.TaskKey, workerID)
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
			// 如果 ok 是 false (只有在 stopChan 触发时才可能)，循环会在顶部检测到 stopChan 并退出

		} // end outer select
	} // end for
}

// Worker 逻辑
func (r *TaskBufferedRunner) worker_busy_wait(workerID string) {
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
			case task = <-r.RemoteQueue:
				ok = true
			default:
				ok = false
				// default:
				// 	// 本地任务为空，再尝试远程任务
				// 	select {
				// 	case task = <-r.RemoteQueue:
				// 		ok = true
				// 	default:
				// 		ok = false
				// 	}
			}

			if !ok {
				// 当前无任务，避免 busy-wait
				// time.Sleep(50 * time.Millisecond)
				// continue
				time.Sleep(1 * time.Second)
				log.Debugw("No task to process...", "workerID", workerID, "queueTasksNum", len(r.LocalQueue)+len(r.RemoteQueue))
				continue
			}

			// Try claim task
			result, err := r.TryClaimTask(context.Background(), task.TaskKey, workerID)
			if err != nil {
				task.ResultChan <- model.TaskResult{
					TaskKey: task.TaskKey,
					Status:  "error",
					Err:     err,
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
				log.Errorw("Task not found", "taskKey", task.TaskKey, "workerID", workerID)
			case 1: // 任务状态不匹配
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_TASK_ALREADY_CLAIMED,
					Message:    "task already claimed by others",
				}
				log.Warnw("Task already claimed by others", "taskKey", task.TaskKey, "workerID", workerID)
			case 2: // 参数错误
				task.ResultChan <- model.TaskResult{
					StatusCode: api.RC_EXECUTOR_INVALID_ARGUMENT,
					Message:    "invalid arguments",
				}
				log.Errorw("Invalid arguments for task", "taskKey", task.TaskKey, "workerID", workerID)
			case 0: // 成功 claimed
				log.Infow("Claim task successfully", "taskKey", task.TaskKey, "workerID", workerID)
				r.PreemptedTaskCount.Add(1)
				// 从 redis 获取具体命令内容
				content, err := r.KVStore.HGet(context.Background(), task.TaskKey, "content")
				if err != nil {
					task.ResultChan <- model.TaskResult{
						TaskKey: task.TaskKey,
						Status:  "error",
						Err:     fmt.Errorf("failed to get command content: %v", err),
					}
					continue
				}

				if content == "" {
					log.Fatalw("Task content is empty", "taskKey", task.TaskKey, "workerID", workerID)
				}

				// 执行命令
				task.Command = content
				taskRes := r.baseRunner.RunTask(&task)
				taskRes.StatusCode = api.RC_EXECUTOR_OK
				// 任务执行成功：更新任务状态为完成.
				if taskRes.ExitCode == 0 {
					r.TryFinishTask(context.Background(), task.TaskKey, workerID)
					log.Infow("Task done.", "taskKey", task.TaskKey, "workerID", workerID)
				} else { // fallback
					log.Errorw("Task failed, unclaiming...", "taskRes", taskRes, "task", task, "workerID", workerID)
					r.UnclaimTask(context.Background(), task.TaskKey, workerID)
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

func (r *TaskBufferedRunner) TryClaimTask(ctx context.Context, TaskKey, operator string) (store.ClaimCmdResult, error) {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		result, err := r.KVStore.ClaimCmd(ctx, TaskKey, model.Unclaimed, model.Claimed, operator, config.GlobalConfig.TaskTTL)
		if err == nil {
			return result, nil
		}
		log.Warnw("Retrying claim task", "attempt", i+1, "err", err)
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return store.ClaimCmdResult{}, fmt.Errorf("failed to claim task after %d retries", maxRetries)
}

func (r *TaskBufferedRunner) UnclaimTask(ctx context.Context, TaskKey, operator string) (store.ClaimCmdResult, error) {
	return r.KVStore.ClaimCmd(ctx, TaskKey, model.Claimed, model.Unclaimed, operator, config.GlobalConfig.TaskTTL)
}

func (r *TaskBufferedRunner) TryFinishTask(ctx context.Context, TaskKey, operator string) (store.ClaimCmdResult, error) {
	return r.KVStore.ClaimCmd(ctx, TaskKey, model.Claimed, model.Done, operator, config.GlobalConfig.TaskTTL)
}

// 处理具体任务
func (r *TaskBufferedRunner) SubmitAndWaitTaskRes(ctx context.Context, task *model.Task, operator string) (model.TaskResult, error) {
	// 根据 TaskKey 判断是否本机任务
	isLocal := strings.HasPrefix(task.TaskKey, r.localIPv4)
	var queue chan model.Task
	if isLocal {
		queue = r.LocalQueue
	} else {
		queue = r.RemoteQueue
	}

	// 提交任务并等待结果
	select {
	case queue <- *task:
		// r.ReceivedtaskCount++
		select {
		case result := <-task.ResultChan:
			return result, result.Err
		case <-ctx.Done():
			log.Errorw("context canceled in SubmitAndWaitTaskRes", "err", ctx.Err())
			return model.TaskResult{}, ctx.Err()
		}
	default:
		return model.TaskResult{}, errors.New("task queue is full")
	}
}
