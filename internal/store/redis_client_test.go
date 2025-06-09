package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chanfun-ren/executor/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, KVStoreClient) {
	mr := miniredis.RunT(t)
	config := KVStoreConfig{
		Type: "redis",
		Host: mr.Host(),
		Port: mr.Server().Addr().Port,
	}
	client, err := NewRedisClient(config)
	require.NoError(t, err)
	return mr, client
}

func TestBasicOperations(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	ctx := context.Background()

	t.Run("HSet and HGet", func(t *testing.T) {
		key := "test:hash1"
		fields := map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
		}

		err := client.HSet(ctx, key, fields)
		require.NoError(t, err)

		// Test HGet
		val, err := client.HGet(ctx, key, "field1")
		require.NoError(t, err)
		assert.Equal(t, "value1", val)

		// Test non-existent field
		_, err = client.HGet(ctx, key, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("HSetWithTTL", func(t *testing.T) {
		key := "test:hash2"
		fields := map[string]interface{}{
			"field1": "value1",
		}
		ttl := 1 * time.Second

		err := client.HSetWithTTL(ctx, key, fields, ttl)
		require.NoError(t, err)

		// Verify value exists
		val, err := client.HGet(ctx, key, "field1")
		require.NoError(t, err)
		assert.Equal(t, "value1", val)

		// Fast forward time in miniredis
		mr.FastForward(2 * time.Second)

		// Verify key has expired
		_, err = client.HGet(ctx, key, "field1")
		assert.Error(t, err)
	})
}

func TestClaimCmd(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	ctx := context.Background()

	t.Run("Basic state transition", func(t *testing.T) {
		key := "test:claim1"
		// Set initial state
		err := client.HSet(ctx, key, map[string]interface{}{
			"status": model.Unclaimed.String(),
		})
		require.NoError(t, err)

		result, err := client.ClaimCmd(ctx, key, model.Unclaimed, model.Claimed, "worker1", 10*time.Second)
		if err != nil {
			t.Logf("ClaimCmd error: %+v", err)
		}
		require.NoError(t, err)
		assert.Equal(t, 0, result.Code)
		assert.Equal(t, model.Claimed.String(), result.Status)
		assert.Equal(t, "worker1", result.LastModifier)
	})

	t.Run("Concurrent claims", func(t *testing.T) {
		key := "test:claim3"
		err := client.HSet(ctx, key, map[string]interface{}{
			"status": model.Unclaimed.String(),
		})
		require.NoError(t, err)

		numWorkers := 100
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go func() {
				defer wg.Done()
				result, err := client.ClaimCmd(ctx, key,
					model.Unclaimed,
					model.Claimed,
					"worker1",
					10*time.Second,
				)
				if err == nil && result.Code == 0 {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		assert.Equal(t, 1, successCount, "Only one worker should successfully claim the task")

		// Verify final state
		val, err := client.HGet(ctx, key, "status")
		require.NoError(t, err)
		assert.Equal(t, model.Claimed.String(), val)
	})

	t.Run("Complete state transition cycle", func(t *testing.T) {
		key := "test:claim4"
		transitions := []struct {
			from       model.TaskStatus
			to         model.TaskStatus
			modifier   string
			wantCode   int
			wantStatus string
		}{
			{model.Unclaimed, model.Claimed, "worker1", 0, model.Claimed.String()},
			{model.Claimed, model.Done, "worker1", 0, model.Done.String()},
			{model.Done, model.Claimed, "worker1", 0, model.Claimed.String()}, // Reset cycle
			{model.Claimed, model.Unclaimed, "worker1", 0, model.Unclaimed.String()},

			{model.Unclaimed, model.Claimed, "worker1", 0, model.Claimed.String()},
			{model.Claimed, model.Done, "worker1", 0, model.Done.String()},
			{model.Done, model.Claimed, "worker1", 0, model.Claimed.String()}, // Reset cycle
			{model.Claimed, model.Unclaimed, "worker1", 0, model.Unclaimed.String()},
		}

		// Set initial state
		err := client.HSet(ctx, key, map[string]interface{}{
			"status": model.Unclaimed.String(),
		})
		require.NoError(t, err)

		for i, tr := range transitions {
			result, err := client.ClaimCmd(ctx, key, tr.from, tr.to, tr.modifier, 10*time.Second)
			require.NoError(t, err)
			assert.Equal(t, tr.wantCode, result.Code, "transition %d failed", i)
			assert.Equal(t, tr.wantStatus, result.Status)
			assert.Equal(t, tr.modifier, result.LastModifier)
			t.Logf("transition %d: %+v", i, result)
		}
	})

	t.Run("Non-existent key", func(t *testing.T) {
		key := "test:claim5"
		result, err := client.ClaimCmd(ctx, key, model.Unclaimed, model.Claimed, "worker1", 10*time.Second)
		require.NoError(t, err)
		assert.Equal(t, -1, result.Code)
	})

	t.Run("Invalid status values", func(t *testing.T) {
		key := "test:claim6"
		_, err := client.ClaimCmd(ctx, key, 999, model.Claimed, "worker1", 10*time.Second)
		assert.Error(t, err)

		_, err = client.ClaimCmd(ctx, key, model.Unclaimed, 999, "worker1", 10*time.Second)
		assert.Error(t, err)
	})
}
