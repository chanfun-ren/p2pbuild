package config

import (
	"fmt"
	"os"

	"github.com/chanfun-ren/executor/pkg/logging"
)

// proxy config
const EXECUTOR_GROUP_SIZE = 3

// sharebuild daemon config
const GRPC_PORT = 50051 // TODO: 更优雅的方式 <- 配置文件导入

func GetExecutorHome() string {
	username := os.Getenv("USER")
	if username == "" {
		logging.DefaultLogger().Infow("Failed to get USER environment variable", "USER", username)
		return "executor"
	}
	executorHome := fmt.Sprintf("/home/%s/", username)
	return executorHome
}

// redis config
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// DefaultRedisConfig 返回默认的 Redis 配置
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:     "localhost:6379", // 默认地址
		Password: "",               // 默认无密码
		DB:       0,                // 默认数据库
	}
}
