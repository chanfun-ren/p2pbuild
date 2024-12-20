package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

var script = redis.NewScript(`
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

-- 检查 key 是否存在
local current_status = redis.call("HGET", KEYS[1], "status")
if not current_status then
    return cjson.encode({code = -1, status = nil, last_modifier = nil}) -- key 不存在
end

-- 检查状态是否匹配
if current_status == ARGV[1] then
    redis.call("HSET", KEYS[1], "status", ARGV[2])
    redis.call("HSET", KEYS[1], "last_modifier", ARGV[3]) -- 设置最后修改者
    -- 设置 TTL（可选）
    if ARGV[4] and tonumber(ARGV[4]) > 0 then
        redis.call("EXPIRE", KEYS[1], tonumber(ARGV[4]))
    elseif ARGV[4] and tonumber(ARGV[4]) <= 0 then
        return cjson.encode({code = 2, status = current_status, last_modifier = nil}) -- 参数错误
    end
    return cjson.encode({code = 0, status = ARGV[2], last_modifier = ARGV[3]}) -- 成功
else
    local current_modifier = redis.call("HGET", KEYS[1], "last_modifier") -- 获取最后修改者
    return cjson.encode({code = 1, status = current_status, last_modifier = current_modifier}) -- 状态不匹配
end

`)

type LuaRes struct {
	Code         int    `json:"code"`
	Status       string `json:"status"`
	LastModifier string `json:"last_modifier"`
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Update to your Redis address
	})

	// 初始化任务数据
	key := "task:1"
	initialData := map[string]interface{}{
		"status":  "unclaimed",
		"content": "echo 'Hello World!'",
	}
	err := rdb.HSet(ctx, key, initialData).Err()
	if err != nil {
		log.Fatalf("Failed to set initial data: %v", err)
	}

	// 调用 Lua 脚本
	executorID := "executor-1"
	ttl := 30 // TTL in seconds
	result, err := script.Run(ctx, rdb, []string{key}, "unclaimed", "claimed", executorID, ttl).Result()
	if err != nil {
		log.Fatalf("Error executing Lua script: %v", err)
	}

	// 解析 JSON 返回值
	var res LuaRes
	if err := json.Unmarshal([]byte(result.(string)), &res); err != nil {
		log.Fatalf("Failed to parse Lua script result: %v", err)
	}

	// 打印结果
	fmt.Printf("Lua script result:\n")
	fmt.Printf("Code: %d\n", res.Code)
	fmt.Printf("Status: %s\n", res.Status)
	fmt.Printf("LastModifier: %s\n", res.LastModifier)

	// 检查是否设置了 TTL
	ttlRemaining, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		log.Fatalf("Failed to get TTL: %v", err)
	}
	fmt.Printf("TTL remaining: %s\n", ttlRemaining)

	// 验证其他情况
	// 再次调用脚本，模拟任务已被占用的情况
	newExecutorID := "executor-2"
	result, err = script.Run(ctx, rdb, []string{key}, "unclaimed", "claimed", newExecutorID, ttl).Result()
	if err != nil {
		log.Fatalf("Error executing Lua script: %v", err)
	}

	if err := json.Unmarshal([]byte(result.(string)), &res); err != nil {
		log.Fatalf("Failed to parse Lua script result: %v", err)
	}
	println()
	fmt.Printf("After second call:\n")
	fmt.Printf("Code: %d\n", res.Code)
	fmt.Printf("Status: %s\n", res.Status)
	fmt.Printf("LastModifier: %s\n", res.LastModifier)

	// 清理测试数据
	// _ = rdb.Del(ctx, key).Err()
}
