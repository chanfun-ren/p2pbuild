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
		Port: config.GlobalConfig.StorePort,
	}
	log.Infow("Creating KVStoreClient for project", "project", project.String(), "redisConfig", redisConfig)

	redisCli, err := store.GetKVStoreFactory().CreateKVStoreClient(redisConfig)
	if err != nil {
		log.Errorw("failed to create KVStoreClient", "redisConfig", redisConfig, "err", err)
		return NewPLEResponse(api.RC_EXECUTOR_RESOURCE_CREATION_FAILED, "failed to create KVStoreClient"), nil
	}

	// 始终创建新的 ProjectRunner, 用户可能更新 containerImage
	ProjectRunner, err := runner.NewScheduledProjectRunner(req.ContainerImage, redisCli, config.GlobalConfig.PoolSize)
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
		return NewPLEResponse(api.RC_EXECUTOR_ENV_PREPARE_FAILED, "failed to prepare environment"), fmt.Errorf("failed to prepare environment: %v", err)
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

	TaskKey := common.GenTaskKey(req.Project, req.CmdId)

	// Submit TaskKey to queue and wait for result
	resultChan := make(chan model.TaskResult, 1)
	task := model.Task{
		TaskKey:    TaskKey,
		ResultChan: resultChan,
	}

	taskRes, err := taskRunner.SubmitAndWaitTaskRes(context.Background(), &task, executorId)
	if err != nil || taskRes.ExitCode != 0 {
		log.Warnw("failed to execute task", "task", task, "err", err, "taskRes", taskRes)
		return NewSAEResponse(api.RC_EXECUTOR_INTERNAL_ERROR, fmt.Sprintf("failed to execute task: %v", err), req.CmdId, taskRes.StdOut, taskRes.StdErr), nil
	}

	return NewSAEResponse(taskRes.StatusCode, taskRes.Status, req.CmdId, taskRes.StdOut, taskRes.StdErr), nil
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

func (s *ShareBuildExecutorService) CleanupLocalEnv(ctx context.Context, req *api.CleanupLocalEnvRequest) (*api.CleanupLocalEnvResponse, error) {
	projectRunner, ok := s.getProjectRunner(req.Project)
	if !ok {
		return NewCLEResponse(api.RC_EXECUTOR_RESOURCE_NOT_FOUND, "project runner not found"), nil
	}
	err := projectRunner.Cleanup(ctx)
	if err != nil {
		return NewCLEResponse(api.RC_EXECUTOR_INTERNAL_ERROR, fmt.Sprintf("failed to cleanup project runner: %v", err)), nil
	}
	s.ProjectRunners.Delete(common.GenProjectKey(req.Project))
	return NewCLEResponse(api.RC_EXECUTOR_OK, "project runner cleaned up"), nil
}

func NewCLEResponse(statusCode api.RC, statusMessage string) *api.CleanupLocalEnvResponse {
	return &api.CleanupLocalEnvResponse{
		Status: &api.Status{
			Code:    statusCode,
			Message: statusMessage,
		},
	}
}
