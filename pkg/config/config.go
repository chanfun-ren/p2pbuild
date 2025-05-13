package config

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"
)

// sharebuild daemon config
const GRPC_PORT = 50051 // TODO: 更优雅的方式 <- 配置文件导入

// ninja2 located host config, store server port
const STORE_PORT = 6379

func GetExecutorHome() string {
	username := os.Getenv("USER")
	if username == "" {
		logging.DefaultLogger().Infow("Failed to get USER environment variable", "USER", username)
		return "executor"
	}
	executorHome := fmt.Sprintf("/home/%s/", username)
	return executorHome
}

var POOLSIZE = runtime.NumCPU()

// var POOLSIZE = int(float64(runtime.NumCPU()) * 1.5)

// TODO: 合理配置任务队列大小
var QUEUESIZE = 1024 * 64

var CMDTTL = 10 * time.Minute
var TASKTTL = 5 * time.Minute

const ExecutorHome = "/home/executor"
