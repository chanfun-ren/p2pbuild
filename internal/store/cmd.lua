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