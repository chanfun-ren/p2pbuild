package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Update to your Redis address
	})

	// Example key and executor ID
	cmdID := "123"
	key := fmt.Sprintf("cmd:%s", cmdID)

	// Simulate a command in Redis
	rdb.HSet(ctx, key, "status", "unclaimed", "content", "echo 'hello world'")
}
