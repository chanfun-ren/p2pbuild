package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/common"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"google.golang.org/grpc"
)

// 面向对象分析，ProxyServer 需要维护：

// - 发过来的项目 Project -> remote executor/peer 的映射。InitializeBuildEnv 时候设置, Execute 时使用，dispatch 命令到 remote peer.

// - redis_client, Execute 时候把命令通过 redis_client 存储到 redis_server

// - executor_client. SubmitAndExecute

var log = logging.NewComponentLogger("proxy")

type SharebuildProxyService struct {
	api.UnimplementedShareBuildProxyServer
	NetManager         *network.NetManager
	projectToExecutors sync.Map // map[string][]*api.Peer
	grpcClients        sync.Map // map[string]*grpc.ClientConn
	kvStoreClient      store.KVStoreClient
}

func NewSharebuildProxyService(nm *network.NetManager, kvCli store.KVStoreClient) *SharebuildProxyService {
	return &SharebuildProxyService{
		NetManager:    nm,
		kvStoreClient: kvCli,
	}
}

func (s *SharebuildProxyService) InitializeBuildEnv(ctx context.Context, req *api.InitializeBuildEnvRequest) (*api.InitializeBuildEnvResponse, error) {
	// Validate the "project" field and its subfields
	if req.Project == nil {
		return nil, fmt.Errorf("project is required")
	}

	if req.Project.NinjaHost == "" {
		return nil, fmt.Errorf("ninjaHost is required")
	}

	if req.Project.NinjaDir == "" {
		return nil, fmt.Errorf("ninjaDir is required")
	}

	if req.Project.RootDir == "" {
		return nil, fmt.Errorf("rootDir is required")
	}

	// TODO: add some validation here for the container image format if exists

	// 1. 从活跃的节点中选取一批作为待编译项目的 executor
	executors, err := s.pickActiveExecutors(int(req.WorkerNum))
	if err != nil {
		log.Errorw("failed to pick active executors", "err", err)
		return NewIBEResponse(api.RC_PROXY_INTERNAL_ERROR, "Failed to pick active executors", nil), nil
	}

	// 2. 通知每个 executor 准备环境
	ready_executors, err := s.prepareEnvironments(executors, req)
	if err != nil {
		log.Errorw("failed to prepare environments", "executors", executors, "err", err)
		return NewIBEResponse(api.RC_PROXY_INTERNAL_ERROR, "Failed to prepare environments", nil), nil
	}
	s.projectToExecutors.Store(common.GenProjectKey(req.Project), ready_executors)

	// 3. 返回成功信息
	return NewIBEResponse(api.RC_PROXY_OK, "Environment prepared successfully", ready_executors), nil
}

// 获取活跃的 executors
func (s *SharebuildProxyService) pickActiveExecutors(workerNum int) ([]*api.Peer, error) {
	activePeers := s.NetManager.PeerList()

	// 确定要取的 executor 数量
	count := workerNum
	if len(activePeers) < count {
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
func (s *SharebuildProxyService) getOrCreateGrpcConnsForProject(projectKey string, executors []*api.Peer) ([]*grpc.ClientConn, error) {
	// 尝试从缓存中获取连接
	if conns, ok := s.grpcClients.Load(projectKey); ok {
		return conns.([]*grpc.ClientConn), nil
	}

	// 如果没有缓存，创建新的连接
	conns := make([]*grpc.ClientConn, len(executors))
	for i, executor := range executors {
		// 构建目标地址
		target := fmt.Sprintf("%s:%d", executor.Ip, config.GRPC_PORT)
		log.Infow("Connecting to executor", "executor", executor.String(), "target", target)

		// 创建 gRPC 连接
		conn, err := grpc.Dial(target, grpc.WithInsecure())
		if err != nil {
			log.Errorw("Failed to connect to executor", "executor", executor.String(), "err", err)
			return nil, err
		}
		conns[i] = conn
	}

	// 将连接缓存
	s.grpcClients.Store(projectKey, conns)
	return conns, nil
}

// 准备环境
func (s *SharebuildProxyService) prepareEnvironments(executors []*api.Peer, req *api.InitializeBuildEnvRequest) ([]*api.Peer, error) {
	// 获取或创建 gRPC 连接
	conns, err := s.getOrCreateGrpcConnsForProject(common.GenProjectKey(req.Project), executors)
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

		// log.Infow("Environment prepared successfully for executor", "executor", executors[i])
	}
	// if no ready executor, return err
	if len(ready_executors) == 0 {
		return ready_executors, fmt.Errorf("no ready executor")
	}
	return ready_executors, nil
}

type ExecutorResult struct {
	Executor *api.Peer
	Response *api.SubmitAndExecuteResponse
}

func (s *SharebuildProxyService) ForwardAndExecute(ctx context.Context, req *api.ForwardAndExecuteRequest) (*api.ForwardAndExecuteResponse, error) {
	// Key: project:<cmd_id>
	// Fields:
	// - status: "unclaimed", "claimed", "done"
	// - content: Actual command content
	// 1. 将命令存储到公共存储组件
	taskKey := common.GenTaskKey(req.Project, req.CmdId)
	cmdContent := req.CmdContent

	fields := map[string]interface{}{
		"status":  "unclaimed",
		"content": cmdContent,
	}
	err := s.kvStoreClient.HSetWithTTL(ctx, taskKey, fields, config.CMDTTL)
	if err != nil {
		log.Errorw("Failed to store command", "taskKey", taskKey, "fields", fields, "err", err)
		return NewFAEResponse(api.RC_PROXY_KVSTORE_FAILED, "Failed to store command"), nil
	}

	log.Debugw("Command stored in KV store", "taskKey", taskKey, "fields", fields)

	// 2. 获取 project 对应的 executors
	// log.Debugw("ForwardAndExecute projectToExecutors", "projectToExecutors", utils.MapToString(&s.projectToExecutors))
	res, ok := s.projectToExecutors.Load(common.GenProjectKey(req.Project))
	if !ok {
		return NewFAEResponse(api.RC_PROXY_NO_AVAILABLE_EXECUTOR, "No executor found for project"), nil
	}
	executors, ok := res.([]*api.Peer)
	if !ok {
		return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, fmt.Sprintf("Invalid type for executors: expected []*api.Peer, got %T", executors)), nil
	}

	// 3. 发起 SubmitAndExecute 调用
	// 获取或创建 gRPC 连接
	conns, err := s.getOrCreateGrpcConnsForProject(common.GenProjectKey(req.Project), executors)
	if err != nil {
		return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, fmt.Sprintf("Failed to create gRPC connections: %v", err)), nil
	}

	// 用于并发执行的 wait group 和结果通道
	var wg sync.WaitGroup
	resultChan := make(chan *ExecutorResult, len(executors))

	// 并发地向每个 executor 发起请求
	for i, conn := range conns {
		wg.Add(1)
		go func(i int, conn *grpc.ClientConn) {
			defer wg.Done()

			// local executor 随机 2-3 ms 延迟(diff: 2.39ms, 2.46ms), 保证 Redis 争抢任务公平
			// if strings.HasPrefix(conn.Target(), utils.GetOutboundIP().String()) {
			// 	time.Sleep(time.Duration(rand.Intn(1000)+9000) * time.Microsecond)
			// }
			client := api.NewShareBuildExecutorClient(conn)

			// 调用 SubmitAndExecute 方法
			res, err := client.SubmitAndExecute(context.Background(), &api.SubmitAndExecuteRequest{
				Project: req.Project,
				CmdId:   req.CmdId,
			})

			// 根据结果返回状态到 resultChan
			if err != nil {
				log.Errorw("Failed to foward command on executor", "executor", executors[i].String(), "err", err)
				resultChan <- nil // proxy foward 内部错误
			} else {
				// log.Infow("Successfully FowardedAndExecute command to executor", "executor", executors[i].String())
				resultChan <- &ExecutorResult{
					Executor: executors[i], // 返回 foward 成功的 executor
					Response: res,          // 返回实际的执行结果
				}
			}
		}(i, conn)
	}

	// 等待所有的 goroutines 完成
	wg.Wait()
	close(resultChan)

	var successfulExecutor *api.Peer
	var finalStatus *api.Status
	var stdOut, stdErr string
	var allFailed bool = true

	// 处理所有 executor 的执行结果
	for result := range resultChan {
		if result.Response.Status.Code == api.RC_EXECUTOR_OK {
			successfulExecutor = result.Executor
			stdOut = result.Response.StdOut
			stdErr = result.Response.StdErr
			finalStatus = &api.Status{
				Code:    api.RC_PROXY_OK,
				Message: "Task executed successfully",
			}
			allFailed = false
			break
		}
	}

	if allFailed {
		// 所有 executor 执行失败，返回失败状态
		finalStatus = &api.Status{
			Code:    api.RC_PROXY_ALL_EXECUTOR_FAILED,
			Message: "Failed to execute task on all executors",
		}
		log.Errorw("Failed to execute task on all executors", "executors", executors)
	}

	log.Infow("Cmd done", "CmdId", req.CmdId, "executor", successfulExecutor.Ip)
	return NewFAEResponseWithExecutor(finalStatus, successfulExecutor, req.CmdId, stdOut, stdErr), nil

}

func NewIBEResponse(code api.RC, message string, peers []*api.Peer) *api.InitializeBuildEnvResponse {
	return &api.InitializeBuildEnvResponse{
		Status: &api.Status{
			Code:    code,
			Message: message,
		},
		Peers: peers,
	}
}

func NewFAEResponse(code api.RC, message string) *api.ForwardAndExecuteResponse {
	return &api.ForwardAndExecuteResponse{
		Status: &api.Status{
			Code:    code,
			Message: message,
		},
	}
}

func NewFAEResponseWithExecutor(status *api.Status, executor *api.Peer, cmdId, stdOut, stdErr string) *api.ForwardAndExecuteResponse {
	return &api.ForwardAndExecuteResponse{
		Status:   status,
		Executor: executor,
		Id:       cmdId,
		StdOut:   stdOut,
		StdErr:   stdErr,
	}
}

func (s *SharebuildProxyService) ClearBuildEnv(ctx context.Context, req *api.ClearBuildEnvRequest) (*api.ClearBuildEnvResponse, error) {
	// 1. 获取 project 对应的 executors
	res, ok := s.projectToExecutors.Load(common.GenProjectKey(req.Project))
	if !ok {
		return NewCBEResponse(api.RC_PROXY_NO_AVAILABLE_EXECUTOR, "No executor found for project"), nil
	}
	executors, ok := res.([]*api.Peer)
	if !ok {
		return NewCBEResponse(api.RC_PROXY_INTERNAL_ERROR, fmt.Sprintf("Invalid type for executors: expected []*api.Peer, got %T", executors)), nil
	}

	// 2. 通知每个 executor 清理环境
	conns, err := s.getOrCreateGrpcConnsForProject(common.GenProjectKey(req.Project), executors)
	if err != nil {
		return NewCBEResponse(api.RC_PROXY_INTERNAL_ERROR, fmt.Sprintf("Failed to create gRPC connections: %v", err)), nil
	}

	for i, conn := range conns {
		client := api.NewShareBuildExecutorClient(conn)
		_, err := client.CleanupLocalEnv(context.Background(), &api.CleanupLocalEnvRequest{
			Project: req.Project,
		})
		if err != nil {
			// TODO: 需要重试
			log.Errorw("Failed to clear environment on executor", "executor", executors[i].String(), "err", err)
			continue
		}
		log.Infow("Successfully cleared environment on executor", "executor", executors[i].String())
	}
	// 3. 删除 project 对应的 executors
	s.projectToExecutors.Delete(common.GenProjectKey(req.Project))
	s.grpcClients.Delete(common.GenProjectKey(req.Project))
	return NewCBEResponse(api.RC_PROXY_OK, "Environment cleared successfully"), nil
}

func NewCBEResponse(code api.RC, message string) *api.ClearBuildEnvResponse {
	return &api.ClearBuildEnvResponse{
		Status: &api.Status{
			Code:    code,
			Message: message,
		},
	}
}
