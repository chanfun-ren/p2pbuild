package store_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/chanfun-ren/executor/internal/model"
	"github.com/chanfun-ren/executor/internal/store"
)

var kvStoreClient store.KVStoreClient

func init() {
	factory := store.GetKVStoreFactory()

	redisConfig := store.KVStoreConfig{
		Type: "redis",
		Host: "127.0.0.1",
		Port: 6379,
		Auth: map[string]interface{}{
			"password": "",
		},
		Options: map[string]interface{}{
			"db": 0,
		},
	}
	var err error
	kvStoreClient, err = factory.CreateKVStoreClient(redisConfig)
	if err != nil {
		log.Fatalf("Failed to create KVStoreClient: %v", err)
	}
}

func TestHSet(t *testing.T) {
	ctx := context.Background()
	key := "task:1"
	fields := map[string]interface{}{
		"status":  "unclaimed",
		"content": "echo 'hello world'",
	}

	// 测试 HSet
	err := kvStoreClient.HSet(ctx, key, fields)
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}
	t.Log("HSet success.")

	// 测试 HSetWithTTL
	ttl := 10 * time.Second
	err = kvStoreClient.HSetWithTTL(ctx, key, fields, ttl)
	if err != nil {
		t.Fatalf("HSetWithTTL failed: %v", err)
	}
	t.Log("HSetWithTTL success.")
}

func TestClaimCmd(t *testing.T) {
	ctx := context.Background()
	key := "task:2"
	fields := map[string]interface{}{
		"status":  "unclaimed",
		"content": "echo 'hello world'",
	}

	// 测试 HSet
	err := kvStoreClient.HSet(ctx, key, fields)
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}
	t.Log("HSet success.")

	expectedStatus := model.Unclaimed
	newStatus := model.Claimed
	lastModifier := "executor_30"
	ttl := 1000 * time.Second

	// 执行 ClaimCmd
	result, err := kvStoreClient.ClaimCmd(ctx, key, expectedStatus, newStatus, lastModifier, ttl)
	if err != nil {
		t.Fatalf("ClaimCmd failed: %v", err)
	}
	t.Logf("ClaimCmd result: Code: %d, Status: %s, Last Modifier: %s", result.Code, result.Status, result.LastModifier)

	// 尝试再次 Claim 相同的任务
	result, err = kvStoreClient.ClaimCmd(ctx, key, expectedStatus, newStatus, lastModifier, ttl)
	if err != nil {
		t.Fatalf("ClaimCmd failedx: %v", err)
	}
	t.Logf("After second call: Code: %d, Status: %s, Last Modifier: %s", result.Code, result.Status, result.LastModifier)

	// 标记完成
	result, err = kvStoreClient.ClaimCmd(ctx, key, model.Claimed, model.Done, lastModifier, ttl)
	if err != nil {
		t.Fatalf("ClaimCmdToDone failed: %v", err)
	}
	t.Logf("ClaimCmd result: Code: %d, Status: %s, Last Modifier: %s", result.Code, result.Status, result.LastModifier)

	// 尝试再次 Done 相同的任务
	result, err = kvStoreClient.ClaimCmd(ctx, key, model.Claimed, model.Done, lastModifier, ttl)
	if err != nil {
		t.Fatalf("ClaimCmdToDone failed: %v", err)
	}
	t.Logf("After second Done call: Code: %d, Status: %s, Last Modifier: %s", result.Code, result.Status, result.LastModifier)

}
