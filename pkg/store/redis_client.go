package store

import (
	"context"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"
	redis "github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

var log = logging.DefaultLogger()

// NewRedisClient 初始化 Redis 客户端
func NewRedisClient(addr string, password string, db int) *RedisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.Warnw("Failed to connect to Redis", "err", err)
		return nil
	}
	log.Infof("Connected to Redis: %s\n", pong)

	return &RedisClient{client: rdb}
}

func (r *RedisClient) Set(key string, value string, ttl time.Duration) error {
	return r.client.Set(context.Background(), key, value, ttl).Err()
}

func (r *RedisClient) Get(key string) (string, error) {
	return r.client.Get(context.Background(), key).Result()
}

func (r *RedisClient) Delete(key string) error {
	return r.client.Del(context.Background(), key).Err()
}

func (r *RedisClient) Ping() (string, error) {
	return r.client.Ping(context.Background()).Result()
}
