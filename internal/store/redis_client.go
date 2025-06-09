package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/chanfun-ren/executor/internal/model"
	"github.com/chanfun-ren/executor/pkg/logging"
	redis "github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	script *redis.Script
}

var log = logging.DefaultLogger()

// NewRedisClient 初始化 Redis 客户端
func NewRedisClient(config KVStoreConfig) (KVStoreClient, error) {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	// options := &redis.Options{
	// 	Addr: addr,
	// 	// Password: config.Auth["password"].(string),
	// 	// DB:       config.Options["db"].(int),
	// }
	options := &redis.Options{
		Addr:         addr,
		DialTimeout:  10 * time.Second, // 建立新连接超时
		ReadTimeout:  10 * time.Second, // 单次读操作超时
		WriteTimeout: 10 * time.Second, // 单次写操作超时

		PoolSize:     100,             // 并发 goroutine 数量上限
		MinIdleConns: 4,               // 保留一定数量的闲置连接
		PoolTimeout:  5 * time.Second, // 适当增加等待超时，比如 3-5 秒

		// 定制底层 Dialer，用于启用 TCP keepalive
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return d.DialContext(ctx, network, addr)
		},
	}

	rdb := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Warnw("Failed to connect to Redis", "err", err)
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", addr, err)
	}

	log.Infow("Connected to Redis", "addr", addr)

	script := redis.NewScript(`
		-- 输入参数：
		-- 1. key
		-- 2. expected_status (期望的初始状态，比如 "unclaimed")
		-- 3. new_status (需要设置的新状态，比如 "claimed")
		-- 4. last_modifier (当前操作者的标识)
		-- 5. ttl (可选，新的状态设置后需要的 TTL，单位秒)
		-- 返回值：
		-- {
		--   code = 状态码,
		--   status = 最新的状态,
		--   last_modifier = 表示任务状态最后修改者
		-- }
		-- 状态码：
		-- -1: key 不存在
		--  0: 成功将状态从 expected_status 改为 new_status
		--  1: 失败，状态不匹配
		--  2: 失败，参数错误（例如 TTL 非法）

		-- 定义状态常量
		local UNCLAIMED = "unclaimed"
		local CLAIMED = "claimed"
		local DONE = "done"

		-- 状态校验函数
		local function isValidStatus(status)
			return status == UNCLAIMED or status == CLAIMED or status == DONE
		end

		-- 检查 key 是否存在
		local current_status = redis.call("HGET", KEYS[1], "status")
		if not current_status then
			return cjson.encode({code = -1, status = nil, last_modifier = nil}) -- key 不存在
		end

		-- redis.call("ECHO", "KEYS[1]: " .. KEYS[1])
		-- redis.call("ECHO", "Current status: " .. current_status)
		-- redis.call("ECHO", "Expected status: " .. ARGV[1])

		-- 校验状态是否有效
		if not isValidStatus(ARGV[1]) or not isValidStatus(ARGV[2]) then
			return cjson.encode({code = 2, status = current_status, last_modifier = nil}) -- 参数错误
		end

		-- 检查状态是否匹配
		if current_status == ARGV[1] then
			redis.call("HSET", KEYS[1], "status", ARGV[2])
			redis.call("HSET", KEYS[1], "last_modifier", ARGV[3]) -- 设置当前 last_modifier
			-- 设置 TTL可选
			if ARGV[4] and tonumber(ARGV[4]) > 0 then
				redis.call("EXPIRE", KEYS[1], tonumber(ARGV[4]))
			end
			return cjson.encode({code = 0, status = ARGV[2], last_modifier = ARGV[3]}) -- 成功
		else
			local modifier = redis.call("HGET", KEYS[1], "last_modifier") -- 获取当前 last_modifier
			return cjson.encode({code = 1, status = current_status, last_modifier = modifier}) -- 状态不匹配
		end
	`)
	return &RedisClient{client: rdb, script: script}, nil
}

func (r *RedisClient) FlushDB(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	result := r.client.HGet(ctx, key, field)
	if result.Err() != nil {
		// 如果出错，返回空字符串和错误
		return "", result.Err()
	}
	return result.Val(), nil
}

// HSet 实现 KVStoreClient 的 HSet 方法
func (r *RedisClient) HSet(ctx context.Context, key string, fields map[string]interface{}) error {
	return r.client.HSet(ctx, key, fields).Err()
}

// Expire 实现 KVStoreClient 的 Expire 方法
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// HSetWithTTL 实现 KVStoreClient 的 HSetWithTTL 方法
func (r *RedisClient) HSetWithTTL(ctx context.Context, key string, fields map[string]interface{}, ttl time.Duration) error {
	pipe := r.client.TxPipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// 使用 TaskStatus 类型
func (r *RedisClient) ClaimCmd(ctx context.Context, key string, expectedStatus model.TaskStatus, newStatus model.TaskStatus, lastModifier string, ttl time.Duration) (ClaimCmdResult, error) {
	// startTime := time.Now()
	// defer func() {
	// 	duration := time.Since(startTime)
	// 	log.Infow("ClaimCmd execution time", "key", key, "expectedStatus", expectedStatus, "newStatus", newStatus, "duration", duration)
	// }()
	// 确保状态值是有效的
	if expectedStatus < model.Unclaimed || expectedStatus > model.Done {
		return ClaimCmdResult{}, errors.New("invalid expected status")
	}
	if newStatus < model.Unclaimed || newStatus > model.Done {
		return ClaimCmdResult{}, errors.New("invalid new status")
	}

	ttlSeconds := int64(ttl.Seconds())
	args := []interface{}{expectedStatus.String(), newStatus.String(), lastModifier, ttlSeconds}

	rawResult, err := r.script.Run(ctx, r.client, []string{key}, args...).Result()
	if err != nil {
		return ClaimCmdResult{}, err
	}

	var result ClaimCmdResult
	if err := json.Unmarshal([]byte(rawResult.(string)), &result); err != nil {
		log.Errorw("ClaimCmd", "err", err)
		return ClaimCmdResult{}, errors.New("failed to parse Lua script response")
	}
	return result, nil
}
