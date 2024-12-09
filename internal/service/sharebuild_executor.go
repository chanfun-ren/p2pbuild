package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/runner"
	"github.com/chanfun-ren/executor/pkg/logging"
)

type ShareBuildExecutorService struct {
	api.UnimplementedShareBuildExecutorServer
	ProjectRunners sync.Map // map[*api.Project]*runner.ProjectRunner

}

func NewShareBuildExecutorService() *ShareBuildExecutorService {
	return &ShareBuildExecutorService{}
}

func (s *ShareBuildExecutorService) getProjectRunner(project *api.Project) (runner.ProjectRunner, bool) {
	projectRunner, ok := s.ProjectRunners.Load(project)
	if ok {
		return projectRunner.(runner.ProjectRunner), true
	}
	return nil, false
}

func createProjectRunner(containerImage string) (runner.ProjectRunner, error) {
	if containerImage != "" {
		return runner.NewContainerProjectRunner(containerImage), nil
	}
	return runner.NewLocalProjectRunner(), nil
}

func (s *ShareBuildExecutorService) PrepareLocalEnv(ctx context.Context, req *api.PrepareLocalEnvRequest) (*api.PrepareLocalEnvResponse, error) {
	log := logging.FromContext(ctx)

	// 始终创建新的 ProjectRunner, 用户可能更新 containerImage
	project := req.Project
	ProjectRunner, err := createProjectRunner(req.ContainerImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create ProjectRunner: %v", err)
	}
	log.Infow("new ProjectRunner for project", "project", project.String())

	// 保存到映射
	s.ProjectRunners.Store(project, ProjectRunner)

	// 准备环境
	if err := ProjectRunner.PrepareEnvironment(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to prepare environment: %v", err)
	}

	return &api.PrepareLocalEnvResponse{
		Status: "Environment initialized successfully",
	}, nil
}
