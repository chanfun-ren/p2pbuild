package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"google.golang.org/grpc"
)

// 面向对象分析，ProxyServer 需要维护：

// - 发过来的项目 Project -> remote executor/peer 的映射。InitializeBuildEnv 时候设置, Execute 时使用，dispatch 命令到 remote peer.

// - redis_client, Execute 时候把命令通过 redis_client 存储到 redis_server

// - executor_client. SubmitAndExecute

var log = logging.DefaultLogger()

type SharebuildProxyService struct {
	api.UnimplementedShareBuildProxyServer
	NetManager         *network.NetManager
	projectToExecutors sync.Map // map[*api.Project][]*api.Peer
	grpcClients        sync.Map // map[*api.Project][]*grpc.ClientConn
}

func NewSharebuildProxyService(nm *network.NetManager) *SharebuildProxyService {
	return &SharebuildProxyService{
		NetManager: nm,
	}
}

func (s *SharebuildProxyService) InitializeBuildEnv(ctx context.Context, req *api.InitializeBuildEnvRequest) (*api.InitializeBuildEnvResponse, error) {
	// 1. 从活跃的节点中选取一批作为待编译项目的 executor
	executors, err := s.pickActiveExecutors()
	if err != nil {
		return nil, err
	}
	s.projectToExecutors.Store(req.Project, executors)

	// 2. 通知每个 executor 准备环境
	ready_executors, err := s.prepareEnvironments(executors, req)
	if err != nil {
		return nil, err
	}

	// 3. 返回成功信息
	return &api.InitializeBuildEnvResponse{
		Status: "InitializeBuildEnv OK",
		Peers:  ready_executors,
	}, nil
}

// 获取活跃的 executors
func (s *SharebuildProxyService) pickActiveExecutors() ([]*api.Peer, error) {
	activePeers := s.NetManager.PeerList()

	// 确定要取的 executor 数量
	count := config.EXECUTOR_GROUP_SIZE
	if len(activePeers) < config.EXECUTOR_GROUP_SIZE {
		count = len(activePeers)
	}
	if count == 0 {
		return nil, fmt.Errorf("no active peers")
	}

	// 创建 executors 列表
	executors := make([]*api.Peer, count)
	for i, p := range activePeers[:count] {
		ip, port, _ := utils.MaddrToHostPort(p.Addresses[0])
		executors[i] = &api.Peer{
			Id:   p.ID,
			Ip:   ip,
			Port: port,
		}
	}
	return executors, nil
}

// 获取或创建 gRPC 客户端连接
func (s *SharebuildProxyService) getOrCreateGrpcConnsForProject(project *api.Project, executors []*api.Peer) ([]*grpc.ClientConn, error) {
	// 尝试从缓存中获取连接
	if conns, ok := s.grpcClients.Load(project); ok {
		return conns.([]*grpc.ClientConn), nil
	}

	// 如果没有缓存，创建新的连接
	conns := make([]*grpc.ClientConn, len(executors))
	for i, executor := range executors {
		// 构建目标地址
		target := fmt.Sprintf("%s:%d", executor.Ip, config.GRPC_PORT)
		log.Infow("Connecting to executor", "executor", executor.String(), "target", target)

		// 创建 gRPC 连接
		conn, err := grpc.Dial(target, grpc.WithInsecure()) // 根据需要替换成安全连接配置
		if err != nil {
			log.Errorw("Failed to connect to executor", "executor", executor.String(), "err", err)
			return nil, err
		}
		conns[i] = conn
	}

	// 将连接缓存
	s.grpcClients.Store(project, conns)
	return conns, nil
}

// 准备环境
func (s *SharebuildProxyService) prepareEnvironments(executors []*api.Peer, req *api.InitializeBuildEnvRequest) ([]*api.Peer, error) {
	// 获取或创建 gRPC 连接
	conns, err := s.getOrCreateGrpcConnsForProject(req.Project, executors)
	if err != nil {
		return []*api.Peer{}, err
	}

	var ready_executors []*api.Peer

	// 遍历每个连接并执行 RPC 调用
	for i, conn := range conns {
		client := api.NewShareBuildExecutorClient(conn)

		_, err := client.PrepareLocalEnv(context.Background(), &api.PrepareLocalEnvRequest{
			Project:        req.Project,
			ContainerImage: req.ContainerImage,
		})
		if err != nil {
			log.Errorw("Failed to prepare environment for executor", "executor", executors[i].String(), "err", err)
			continue
		}
		ready_executors = append(ready_executors, executors[i])

		log.Infow("Environment prepared successfully for executor", "executor", executors[i])
	}
	return ready_executors, nil
}
