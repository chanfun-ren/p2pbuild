package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/common"
	"github.com/chanfun-ren/executor/internal/model"
	"github.com/chanfun-ren/executor/internal/runner"
	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/libp2p/go-libp2p/core/host"
)

type ShareBuildExecutorService struct {
	api.UnimplementedShareBuildExecutorServer
	ProjectRunners sync.Map // map[*api.Project]*runner.ProjectRunner
	Host           host.Host
}

func NewShareBuildExecutorService(h host.Host) *ShareBuildExecutorService {
	return &ShareBuildExecutorService{
		Host: h,
	}
}

func (s *ShareBuildExecutorService) getProjectRunner(project *api.Project) (runner.ProjectRunner, bool) {
	projectRunner, ok := s.ProjectRunners.Load(common.GenProjectKey(project))
	if ok {
		return projectRunner.(runner.ProjectRunner), true
	}
	return nil, false
}

func (s *ShareBuildExecutorService) PrepareLocalEnv(ctx context.Context, req *api.PrepareLocalEnvRequest) (*api.PrepareLocalEnvResponse, error) {
	log := logging.NewComponentLogger("executor")

	project := req.Project
	if project == nil {
		return NewPLEResponse(api.RC_EXECUTOR_INVALID_ARGUMENT, "invalid arguments: `project` is missing"), nil
	}
	redisConfig := store.KVStoreConfig{
		Type: "redis",
		Host: project.NinjaHost,
		Port: config.STORE_PORT,
	}
	redisCli, err := store.GetKVStoreFactory().CreateKVStoreClient(redisConfig)
	if err != nil {
		log.Errorw("failed to create KVStoreClient", "redisConfig", redisConfig, "err", err)
		return NewPLEResponse(api.RC_EXECUTOR_RESOURCE_CREATION_FAILED, "failed to create KVStoreClient"), nil
	}

	// 始终创建新的 ProjectRunner, 用户可能更新 containerImage
	ProjectRunner, err := runner.NewScheduledProjectRunner(req.ContainerImage, redisCli, config.POOLSIZE)
	if err != nil {
		log.Errorw("failed to create ProjectRunner", "containerImage", req.ContainerImage, "err", err)
		return NewPLEResponse(api.RC_EXECUTOR_RUNNER_CREATION_FAILED, "failed to create ProjectRunner"), nil
	}
	log.Debugw("new ProjectRunner for project", "project", project.String())

	// 保存到映射
	s.ProjectRunners.Store(common.GenProjectKey(project), ProjectRunner)

	// 准备环境
	if err := ProjectRunner.PrepareEnvironment(ctx, req); err != nil {
		log.Errorw("failed to prepare environment", "project", project.String(), "err", err)
		return NewPLEResponse(api.RC_EXECUTOR_ENV_PREPARE_FAILED, "failed to prepare environment"), nil
	}

	log.Infow("Environment initialized successfully", "project", project.String())
	return NewPLEResponse(api.RC_EXECUTOR_OK, "Environment initialized successfully"), nil

}

func (s *ShareBuildExecutorService) SubmitAndExecute(ctx context.Context, req *api.SubmitAndExecuteRequest) (*api.SubmitAndExecuteResponse, error) {
	log := logging.NewComponentLogger("executor")
	executorId := s.Host.ID().ShortString()

	projectRunner, ok := s.getProjectRunner(req.Project)
	if !ok {
		return NewSAEResponse(api.RC_EXECUTOR_RESOURCE_NOT_FOUND, "project runner not found", req.CmdId, "", ""), nil
	}
	taskRunner, ok := projectRunner.(*runner.TaskBufferedRunner)
	if !ok {
		return NewSAEResponse(api.RC_EXECUTOR_INVALID_ARGUMENT, "invalid project runner type", req.CmdId, "", ""), nil
	}

	cmdKey := common.GenCmdKey(req.Project, req.CmdId)
	log.Infow("SubmitAndExecute", "cmdKey", cmdKey)

	result, err := taskRunner.TryClaimTask(ctx, cmdKey, executorId)
	log.Debugw("TryClaimTask", "result", result, "err", err)
	if err != nil {
		return NewSAEResponse(api.RC_EXECUTOR_INTERNAL_ERROR, fmt.Sprintf("failed to claim task: %v", err), req.CmdId, "", ""), nil
	}

	// 根据状态码处理逻辑
	switch result.Code {
	case -1: // 任务不存在
		return NewSAEResponse(api.RC_EXECUTOR_RESOURCE_NOT_FOUND, "task not found", req.CmdId, "", ""), nil
	case 1: // 任务状态不匹配
		return NewSAEResponse(api.RC_EXECUTOR_TASK_ALREADY_CLAIMED, "task already claimed by others", req.CmdId, "", ""), nil
	case 2: // 参数错误
		return NewSAEResponse(api.RC_EXECUTOR_INVALID_ARGUMENT, "invalid arguments", req.CmdId, "", ""), nil
	case 0: // 成功 claimed
		// 执行命令，这里可以启动一个后台 goroutine 或者直接执行任务
		content, err := taskRunner.KVStore.HGet(ctx, cmdKey, "content")
		if err != nil {
			return NewSAEResponse(api.RC_EXECUTOR_INTERNAL_ERROR, fmt.Sprintf("failed to get command content: %v", err), req.CmdId, "", ""), nil
		}

		resultChan := make(chan model.TaskResult, 1)
		task := model.Task{
			CmdKey:     cmdKey,
			Command:    content,
			ResultChan: resultChan,
		}
		taskRes, err := taskRunner.SubmitAndWaitTaskRes(ctx, &task, executorId)
		if err != nil {
			// fallback
			taskRunner.UnclaimeTask(ctx, cmdKey, executorId)
			return NewSAEResponse(api.RC_EXECUTOR_TASK_FAILED, fmt.Sprintf("failed to execute task: %v", err), req.CmdId, "", ""), nil
		}

		// 任务执行成功：更新任务状态为完成
		luaRes, _ := taskRunner.TryFinishTask(ctx, task.CmdKey, executorId)
		if luaRes.Code != 0 {
			return NewSAEResponse(api.RC_EXECUTOR_INTERNAL_ERROR, "failed to finish task", req.CmdId, "", ""), nil
		}

		log.Debugw("task done", "taskRes", taskRes)
		return NewSAEResponse(api.RC_EXECUTOR_OK, "task executed successfully", req.CmdId, taskRes.StdOut, taskRes.StdErr), nil

	default: // 非预期结果
		return NewSAEResponse(api.RC_EXECUTOR_UNEXPECTED_RESULT, "unexpected result code", req.CmdId, "", ""), nil
	}
}

func NewSAEResponse(statusCode api.RC, statusMessage, id, stdOut, stdErr string) *api.SubmitAndExecuteResponse {
	return &api.SubmitAndExecuteResponse{
		Status: &api.Status{
			Code:    statusCode,
			Message: statusMessage,
		},
		Id:     id,
		StdOut: stdOut,
		StdErr: stdErr,
	}
}

func NewPLEResponse(statusCode api.RC, statusMessage string) *api.PrepareLocalEnvResponse {
	return &api.PrepareLocalEnvResponse{
		Status: &api.Status{
			Code:    statusCode,
			Message: statusMessage,
		},
	}
}
