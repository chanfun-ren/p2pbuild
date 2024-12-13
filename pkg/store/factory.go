package store

import "github.com/chanfun-ren/executor/pkg/config"

func NewStoreClient(storeType string) KVStoreClient {
	switch storeType {
	case "redis":
		redisConfig := config.DefaultRedisConfig()
		return NewRedisClient(redisConfig.Addr, redisConfig.Password, redisConfig.DB)
	case "memory":
		return &MemoryStoreClient{}
	default:
		panic("unsupported store type")
	}
}
