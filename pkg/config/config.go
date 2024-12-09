package config

import (
	"fmt"
	"os"

	"github.com/chanfun-ren/executor/pkg/logging"
)

const EXECUTOR_GROUP_SIZE = 3

// TODO: 更优雅的方式 <- 配置文件导入
const GRPC_PORT = 50051

func GetExecutorHome() string {
	username := os.Getenv("USER")
	if username == "" {
		logging.DefaultLogger().Infow("Failed to get USER environment variable", "USER", username)
		return "executor"
	}
	executorHome := fmt.Sprintf("/home/%s/", username)
	return executorHome
}
