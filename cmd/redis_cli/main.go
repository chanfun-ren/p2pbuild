package main

import (
	"fmt" // 假设 redis 包路径

	"github.com/chanfun-ren/executor/pkg/store" // 假设 RedisClient 实现位于此路径

	"github.com/chanfun-ren/executor/pkg/config" // 假设配置位于此路径
)

func main() {
	// 加载默认 Redis 配置
	redisConfig := config.DefaultRedisConfig()

	// 初始化 Redis 客户端
	redisClient := store.NewRedisClient(redisConfig.Addr, redisConfig.Password, redisConfig.DB)

	// 示例：测试连接
	pong, err := redisClient.Ping()

	if err != nil {
		fmt.Printf("Failed to connect to Redis: %v\n", err)
		return
	}
	fmt.Printf("Connected to Redis: %s\n", pong)
}
