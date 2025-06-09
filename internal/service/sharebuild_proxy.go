package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/common"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
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

// 获取 project 对应的 executors 的 grpc 连接，如果 projectkey 不存在就根据 executors 创建 conns
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
		target := fmt.Sprintf("%s:%d", executor.Ip, config.GlobalConfig.GrpcPort)
		log.Infow("Connecting to executor", "executor", executor.String(), "target", target)

		// 创建 gRPC 连接
		// conn, err := grpc.Dial(target, grpc.WithInsecure())
		conn, err := grpc.Dial(
			target,
			grpc.WithInsecure(),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:    30 * time.Hour, // 客户端心跳间隔（根据网络调整）
				Timeout: 5 * time.Hour,  // 心跳超时
			}),
			grpc.WithDefaultServiceConfig(`{
				"retryPolicy": {
					"maxAttempts": 3,
					"initialBackoff": "0.1s",
					"maxBackoff": "1s"
				}
			}`),
		)
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
	cmdContent := string(req.CmdContent)

	fields := map[string]interface{}{
		"status":  "unclaimed",
		"content": cmdContent,
	}
	err := s.kvStoreClient.HSetWithTTL(ctx, taskKey, fields, config.GlobalConfig.CmdTTL)
	if err != nil {
		log.Errorw("Failed to store command", "taskKey", taskKey, "fields", fields, "err", err)
		return NewFAEResponse(api.RC_PROXY_KVSTORE_FAILED, "Failed to store command"), nil
	}

	log.Debugw("Command stored in KV store", "taskKey", taskKey, "fields", fields)

	// 2. 获取 project 对应的 executors
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

	// 尝试执行任务，如果失败则在 local_executor 上重试:
	// 1. 将所有 executors 区分 local executor 和 remote executor
	// 2. 向所有 executors 下发 SubmitAndExecute 调用请求并收集结果
	// 3. 如果最终结果失败，再单独下发给 local executor 执行一次
	return s.tryExecuteWithRetry(ctx, req, executors, conns)
}

// 尝试执行任务，如果失败则在 local_executor 上重试:
// 1. 将所有 executors 区分 local executor 和 remote executor
// 2. 向所有 executors 下发 SubmitAndExecute 调用请求并收集结果
// 3. 如果最终结果失败，再单独下发给 local executor 执行一次
func (s *SharebuildProxyService) tryExecuteWithRetry(ctx context.Context, req *api.ForwardAndExecuteRequest,
	executors []*api.Peer, conns []*grpc.ClientConn) (*api.ForwardAndExecuteResponse, error) {

	var localExecutor *api.Peer
	var localExecutorConn *grpc.ClientConn

	for i, executor := range executors {
		if utils.IsLocalExecutor(executor.Ip) {
			localExecutor = executor
			localExecutorConn = conns[i]
			break
		}
	}

	if localExecutor == nil {
		log.Fatalw("No local executor found", "executors", executors)
		// return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, "No local executor found"), nil
	}

	if localExecutorConn == nil {
		log.Fatalw("No local executor connection found", "executors", executors)
		// return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, "No local executor connection found"), nil
	}

	// 用于并发执行和结果收集
	var wg sync.WaitGroup
	resultChan := make(chan *ExecutorResult, len(executors))
	successChan := make(chan *ExecutorResult, 1) // 用于快速获取成功结果
	doneChan := make(chan struct{})              // 用于指示所有worker完成

	// 启动所有executor的工作协程
	for i := range executors {
		wg.Add(1)
		go func(idx int, conn *grpc.ClientConn) {
			defer wg.Done()

			// 调用 SubmitAndExecute 方法
			client := api.NewShareBuildExecutorClient(conn)
			res, err := client.SubmitAndExecute(context.Background(), &api.SubmitAndExecuteRequest{
				Project: req.Project,
				CmdId:   req.CmdId,
			})

			// 处理执行结果
			if err != nil {
				log.Fatalw("Failed to forward command to executor", "executor", executors[idx].Ip,
					"err", err, "request", req.CmdId, "response", res)
				resultChan <- nil
				return
			}
			result := &ExecutorResult{
				Executor: executors[idx],
				Response: res,
			}
			// 所有结果都发送到resultChan
			resultChan <- result

			// 成功结果尝试发送到successChan (非阻塞)
			if res.Status.Code == api.RC_EXECUTOR_OK {
				select {
				case successChan <- result:
					// 成功发送
				default:
					// successChan已满，不应该出现此种情况(多个executor 执行成功)
				}
			} else {
				// 失败正常，只有一个 executor 可以成功抢占到命令
				// log.Warnw("Executor failed to execute task", "executor", executors[idx].Ip,
				// 	"code", res.Status.Code, "message", res.Status.Message)
			}
		}(i, conns[i])
	}

	// 启动goroutine等待所有worker完成并关闭通道
	go func() {
		wg.Wait()
		close(resultChan)
		close(doneChan)
	}()

	// 等待成功结果、上下文取消或所有worker完成
	var successResult *ExecutorResult
	select {
	case result := <-successChan:
		// 获取到成功结果
		successResult = result
	case <-ctx.Done():
		// 上下文被取消
		return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, "Context cancelled"), nil
	case <-doneChan:
		// 所有worker完成，检查resultChan中是否有成功结果
		for result := range resultChan {
			if result != nil && result.Response.Status.Code == api.RC_EXECUTOR_OK {
				successResult = result
				break
			}
		}
	}

	// 如果已经有成功结果，直接返回
	if successResult != nil {
		return NewFAEResponseWithExecutor(
			&api.Status{
				Code:    api.RC_PROXY_OK,
				Message: "Task executed successfully",
			},
			successResult.Executor, req.CmdId,
			successResult.Response.StdOut,
			successResult.Response.StdErr,
		), nil
	}

	// 这一批次全部失败，继续 local executor 重试
	log.Warnw("All executors failed this attempt, will retry in local executor", "cmdId", req.CmdId, "cmd", string(req.CmdContent))

	// 向 local executor 下发 SubmitAndExecute 调用请求
	localClient := api.NewShareBuildExecutorClient(localExecutorConn)
	localRes, localErr := localClient.SubmitAndExecute(context.Background(), &api.SubmitAndExecuteRequest{
		Project: req.Project,
		CmdId:   req.CmdId,
	})

	if localErr != nil {
		log.Fatalw("Failed to submit task to local executor", "cmdId", req.CmdId, "localExecutorIp", localExecutor.Ip, "error", localErr)
		return NewFAEResponse(api.RC_PROXY_INTERNAL_ERROR, "Failed to submit task to local executor when retrying"), nil
	}

	if localRes.Status.Code == api.RC_EXECUTOR_OK {
		log.Infow("Task successfully executed on local executor after remote failures", "cmdId", req.CmdId, "localExecutorIp", localExecutor.Ip)
		return NewFAEResponseWithExecutor(
			&api.Status{
				Code:    api.RC_PROXY_OK,
				Message: "Task executed successfully by local executor after remote failures",
			},
			localExecutor, // The local executor peer info
			req.CmdId,
			localRes.StdOut,
			localRes.StdErr,
		), nil
	}

	// 所有调用均都失败
	log.Errorw("Local executor also failed to execute task, all executors failed", "cmdId", req.CmdId, "localExecutorIp", localExecutor.Ip,
		"code", localRes.Status.Code, "message", localRes.Status.Message)

	return NewFAEResponse(
		api.RC_PROXY_ALL_EXECUTOR_FAILED,
		"Failed to execute task on all executors after retry in local executor",
	), nil
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
