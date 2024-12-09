package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
)

// 面向对象分析，ProxyServer 需要维护：

// - 发过来的项目 Project -> remote executor/peer 的映射。InitializeBuildEnv 时候设置, Execute 时使用，dispatch 命令到 remote peer.

// - redis_client, Execute 时候把命令通过 redis_client 存储到 redis_server

// - executor_client. SubmitAndExecute

const EXECUTOR_GROUP_SIZE = 3

var log = logging.DefaultLogger()

type SharebuildProxyService struct {
	api.UnimplementedShareBuildProxyServer
	projectToPeers sync.Map
	NetManager     *network.NetManager
}

func NewSharebuildProxyService(nm *network.NetManager) *SharebuildProxyService {
	return &SharebuildProxyService{
		NetManager: nm,
	}
}

func (s *SharebuildProxyService) InitializeBuildEnv(ctx context.Context, req *api.InitializeBuildEnvRequest) (*api.InitializeBuildEnvResponse, error) {
	// 1. peer discovery，选取一批（三个）活跃的 executor.
	active_peers := s.NetManager.PeerList()

	// 确定要取的 executor 数量
	count := EXECUTOR_GROUP_SIZE
	if len(active_peers) < EXECUTOR_GROUP_SIZE {
		count = len(active_peers)
	}
	if count == 0 {
		return &api.InitializeBuildEnvResponse{
			Status: "No active peers",
		}, nil
	}

	executors := make([]*api.Peer, count)
	for i, p := range active_peers {
		ip, port, _ := utils.MaddrToHostPort(p.Addresses[0])
		executors[i] = &api.Peer{
			Id:   p.ID,
			Ip:   ip,
			Port: port,
		}
	}

	// 2. 通知 remote executor 进行准备环境操作。-> RPC 调用
	// 根据 executors 的 ip, port 创建 grpc 连接，并且发起 PrepareLocalEnv RPC 调用
	// TODO: golang 协程 + 封装重用 grpc_client
	for _, executor := range executors {
		// 构建目标地址
		target := fmt.Sprintf("%s:%d", executor.Ip, executor.Port)
		log.Infow("Connecting to executor", "executor", executor.String(), "target", target)
		// TODO: do real connection
		// 创建 gRPC 连接
		// conn, err := grpc.Dial(target, grpc.WithInsecure()) // 根据需要替换成安全连接配置
		// if err != nil {
		// 	log.Errorw("Failed to connect to executor", "executor", executor.String(), "err", err)
		// 	continue // 跳过失败的 executor
		// }
		// defer conn.Close()

		// // 创建客户端并发起 RPC 调用
		// client := api.NewShareBuildExecutorClient(conn)
		// _, err = client.PrepareLocalEnv(context.Background(), &api.PrepareLocalEnvRequest{
		// 	Project:        req.Project,
		// 	ContainerImage: req.ContainerImage,
		// })
		// if err != nil {
		// 	log.Errorw("Failed to prepare environment for executor", "executor", executor.String(), "err", err)
		// 	continue
		// }

		log.Infow("Environment prepared successfully for executor", "executor", executor)
	}

	// 3. 返回对应的 remote executor 地址
	return &api.InitializeBuildEnvResponse{
		Status: "InitializeBuildEnv OK",
		Peers:  executors,
	}, nil
}
