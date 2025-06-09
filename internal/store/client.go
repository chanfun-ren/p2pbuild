package store

import (
	"context"
	"time"

	"github.com/chanfun-ren/executor/internal/model"
)

// ClaimCmdResult 用于解析 Lua 脚本返回值
// 对于 RedisClient 的 ClaimCmd 方法，返回值的含义如下：
// -- -1: key 不存在
// --  0: 成功将状态从 expected_status 改为 new_status
// --  1: 失败，状态不匹配
// --  2: 失败，参数错误（例如 TTL 非法）
type ClaimCmdResult struct {
	Code         int    `json:"code"`
	Status       string `json:"status"`
	LastModifier string `json:"last_modifier"`
}

type KVStoreConfig struct {
	Type    string                 // Store 类型，比如 "redis", "mysql" 等
	Host    string                 // 服务 IP 或域名
	Port    int                    // 服务端口
	Auth    map[string]interface{} // 认证信息，如用户名、密码
	Options map[string]interface{} // 额外配置选项
}

type KVStoreClient interface {
	FlushDB(ctx context.Context) error
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key string, fields map[string]interface{}) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	HSetWithTTL(ctx context.Context, key string, fields map[string]interface{}, ttl time.Duration) error
	ClaimCmd(ctx context.Context, key string, expectedStatus model.TaskStatus, newStatus model.TaskStatus, lastModifier string, ttl time.Duration) (ClaimCmdResult, error)
}
