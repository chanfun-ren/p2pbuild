package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chanfun-ren/executor/internal/model"
)

type MySQLClient struct {
	db *sql.DB
}

func NewMySQLClient(config KVStoreConfig) (KVStoreClient, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		config.Auth["user"].(string),
		config.Auth["password"].(string),
		config.Host,
		config.Port,
		config.Options["database"].(string),
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping MySQL: %w", err)
	}

	return &MySQLClient{db: db}, nil
}

func (r *MySQLClient) FlushDB(ctx context.Context) error {
	return fmt.Errorf("FlushDB operation is not supported in MySQL")
}

func (r *MySQLClient) HGet(ctx context.Context, key, field string) (string, error) {
	return "", nil
}

// HSet 实现 KVStoreClient 的 HSet 方法
func (r *MySQLClient) HSet(ctx context.Context, key string, fields map[string]interface{}) error {
	return nil
}

// Expire 实现 KVStoreClient 的 Expire 方法
func (r *MySQLClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}

// HSetWithTTL 实现 KVStoreClient 的 HSetWithTTL 方法
func (r *MySQLClient) HSetWithTTL(ctx context.Context, key string, fields map[string]interface{}, ttl time.Duration) error {
	return nil
}

// 使用 TaskStatus 类型
func (r *MySQLClient) ClaimCmd(ctx context.Context, key string, expectedStatus model.TaskStatus, newStatus model.TaskStatus, lastModifier string, ttl time.Duration) (ClaimCmdResult, error) {
	return ClaimCmdResult{}, nil
}
