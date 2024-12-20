package config

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"
)

// proxy config
const EXECUTOR_GROUP_SIZE = 3

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

var CMDTTL = 10 * time.Minute
var TASKTTL = 5 * time.Minute
