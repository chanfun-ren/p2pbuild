package runner

// TODO: 拆分 runner 基础功能和调度功能

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/model"
	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
)

type ProjectRunner interface {
	PrepareEnvironment(ctx context.Context, request *api.PrepareLocalEnvRequest) error
	RunTask(task model.Task) model.TaskResult // 执行编译任务
	Cleanup() error                           // 清理资源
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
	workDir string
}

func NewLocalProjectRunner() *LocalProjectRunner {
	return &LocalProjectRunner{}
}

func (l *LocalProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing local environment for project", "project", req.String())
	l.workDir = req.Project.NinjaDir
	return mountNinjaProject(ctx, req)
}

func mountNinjaProject(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	// 设置本地编译环境，例如 NFS 挂载
	project := req.GetProject()
	ninjaHost := project.GetNinjaHost()
	rootDir := strings.TrimSuffix(project.GetRootDir(), "/") //把结尾的斜杠去掉，方便后面字符串替换

	log.Debugw("Mounting ninja project", "ninjaHost", ninjaHost)
	// 生成挂载点目录, 若目录不存在则递归创建
	mountedRootDir := utils.GenMountedRootDir(ninjaHost, rootDir)
	if err := os.MkdirAll(mountedRootDir, os.ModePerm); err != nil {
		log.Fatalw("fail to create dir", "mountedRootDir", mountedRootDir, "err", err)
	}
	log.Debugw("directory created successfully", "mountedRootDir", mountedRootDir)

	// 执行挂载 NFS 操作
	return utils.MountNFS(ctx, ninjaHost, rootDir, mountedRootDir)
}

func (l *LocalProjectRunner) RunTask(task model.Task) model.TaskResult {
	log.Debugw("Executing local task", "task cmd", task.Command)

	cmd := &utils.Command{
		Content: task.Command,
		WorkDir: l.workDir,
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

func (l *LocalProjectRunner) Cleanup() error {
	fmt.Println("Cleaning up local resources")
	return nil
}

type ContainerProjectRunner struct {
	containerImage string
	containerID    string
}

func NewContainerProjectRunner(image string) *ContainerProjectRunner {
	return &ContainerProjectRunner{
		containerImage: image,
	}
}

func (c *ContainerProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing container environment for project", "project", req.String())

	// 1. 挂载项目
	err := mountNinjaProject(ctx, req)
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

	// 3. 提前返回，拉取任务在后��进行
	return nil

}

func (c *ContainerProjectRunner) RunTask(task model.Task) model.TaskResult {
	log.Debugw("Executing local task", "cmdContent", task.Command)
	// 容器内执行任务
	return model.TaskResult{}
}

func (c *ContainerProjectRunner) Cleanup() error {
	fmt.Printf("Stopping and cleaning up container: %s\n", c.containerID)
	return nil
}

// 基础 Runner 之上装饰调度功能，只不过这里的调度是抢占式
type TaskBufferedRunner struct {
	baseRunner  ProjectRunner
	LocalQueue  chan model.Task     // 缓冲队列，存储任务
	stopChan    chan struct{}       // 停止所有 worker 的信号
	KVStore     store.KVStoreClient // 存储客户端
	wg          sync.WaitGroup      // 用于管理协程的退出
	workerCount int                 // worker 数量
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
		LocalQueue:  make(chan model.Task, workerCount*2),
		stopChan:    make(chan struct{}),
		KVStore:     kvStore,
		workerCount: workerCount,
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

func (r *TaskBufferedRunner) RunTask(task model.Task) model.TaskResult {
	// r.submitCommand(common.GenCmdKey(req.Project, req.CmdId))
	return r.baseRunner.RunTask(task)
}

func (r *TaskBufferedRunner) Cleanup() error {
	return r.baseRunner.Cleanup()
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
	for {
		select {
		case <-r.stopChan:
			fmt.Printf("[%s] Stopping worker\n", workerID)
			return
		case <-r.stopChan:
			return
		case task := <-r.LocalQueue:
			// 执行任务
			taskRes := r.baseRunner.RunTask(task)
			// 发送结果
			task.ResultChan <- taskRes
			close(task.ResultChan)
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
	// 提交任务并等待结果
	select {
	case r.LocalQueue <- *task:
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
