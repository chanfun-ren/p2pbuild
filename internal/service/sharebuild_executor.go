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
	redisConfig := store.KVStoreConfig{
		Type: "redis",
		Host: project.NinjaHost,
		Port: config.STORE_PORT,
	}
	redisCli, err := store.GetKVStoreFactory().CreateKVStoreClient(redisConfig)
	if err != nil {
		log.Errorw("failed to create KVStoreClient", "redisConfig", redisConfig, "err", err)
		return nil, fmt.Errorf("failed to create KVStoreClient: %v", err)
	}

	// 始终创建新的 ProjectRunner, 用户可能更新 containerImage
	ProjectRunner, err := runner.NewScheduledProjectRunner(req.ContainerImage, redisCli, config.POOLSIZE)
	if err != nil {
		return nil, fmt.Errorf("failed to create ProjectRunner: %v", err)
	}
	log.Debugw("new ProjectRunner for project", "project", project.String())

	// 保存到映射
	s.ProjectRunners.Store(common.GenProjectKey(project), ProjectRunner)

	// 准备环境
	if err := ProjectRunner.PrepareEnvironment(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to prepare environment: %v", err)
	}

	return &api.PrepareLocalEnvResponse{
		Status: "Environment initialized successfully",
	}, nil
}

func (s *ShareBuildExecutorService) SubmitAndExecute(ctx context.Context, req *api.SubmitAndExecuteRequest) (*api.SubmitAndExecuteResponse, error) {
	log := logging.NewComponentLogger("executor")
	projectRunner, ok := s.getProjectRunner(req.Project)
	if !ok {
		return nil, fmt.Errorf("project runner not found")
	}
	taskRunner, ok := projectRunner.(*runner.TaskBufferedRunner)
	if !ok {
		return nil, fmt.Errorf("invalid project runner type")
	}

	cmdKey := common.GenCmdKey(req.Project, req.CmdId)
	log.Infow("SubmitAndExecute", "cmdKey", cmdKey)
	executorId := s.Host.ID().ShortString()

	result, err := taskRunner.TryClaimTask(ctx, cmdKey, executorId)
	log.Debugw("TryClaimTask", "result", result, "err", err)
	if err != nil {
		return nil, fmt.Errorf("failed to claim command: %w", err)
	}

	// 根据状态码处理逻辑
	switch result.Code {
	case -1: // 任务不存在
		return &api.SubmitAndExecuteResponse{
			Status: "task not found",
		}, nil

	case 0: // 成功 claimed
		// 执行命令，这里可以启动一个后台 goroutine 或者直接执行任务
		content, err := taskRunner.KVStore.HGet(ctx, cmdKey, "content")
		if err != nil {
			return nil, fmt.Errorf("failed to get command content: %w", err)
		}
		resultChan := make(chan model.TaskResult, 1)
		task := model.Task{
			CmdKey:     cmdKey,
			Command:    content,
			ResultChan: resultChan,
		}
		taskRes, err := taskRunner.SubmitAndWaitTaskRes(ctx, &task, executorId)
		if err != nil {
			return nil, fmt.Errorf("failed to execute task: %w", err)
		}
		log.Debugw("task done", "taskRes", taskRes)

		return &api.SubmitAndExecuteResponse{
			Id:     req.CmdId,
			Status: "task done ok",
			StdOut: taskRes.StdOut,
			StdErr: taskRes.StdErr,
		}, nil

	case 1: // 任务状态不匹配
		return &api.SubmitAndExecuteResponse{
			Id:     req.CmdId,
			Status: "task already claimed by others",
		}, nil
	case 2: // 参数错误
		return &api.SubmitAndExecuteResponse{
			Id:     req.CmdId,
			Status: "invalid arguments",
		}, nil
	default:
		return &api.SubmitAndExecuteResponse{
			Id:     req.CmdId,
			Status: "unexpected result code",
		}, nil
	}
}

// enum Status {
//     SUCCESS = 0;
//     NOT_FOUND = 1;
//     ALREADY_CLAIMED = 2;
//     ALREADY_DONE = 3;
//     FAILED = 4;
//     QUEUE_FULL = 5;
//   }
