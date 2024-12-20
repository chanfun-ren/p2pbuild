package store

import (
	"fmt"
	"io"
	"sync"
)

type KVStoreFactory struct {
	clients map[string]KVStoreClient // 存储已创建的客户端实例
	mu      sync.Mutex               // 保证并发安全
}

// 全局唯一工厂实例
var factoryInstance *KVStoreFactory
var once sync.Once

// 获取工厂单例
func GetKVStoreFactory() *KVStoreFactory {
	once.Do(func() {
		factoryInstance = &KVStoreFactory{
			clients: make(map[string]KVStoreClient),
		}
	})
	return factoryInstance
}

func (f *KVStoreFactory) CreateKVStoreClient(config KVStoreConfig) (KVStoreClient, error) {
	// 用 IP:Port 作为唯一标识（可以根据需求调整标识规则）
	clientKey := fmt.Sprintf("%s:%s:%d", config.Type, config.Host, config.Port)

	// 并发安全地获取或创建实例
	f.mu.Lock()
	defer f.mu.Unlock()

	if client, exists := f.clients[clientKey]; exists {
		return client, nil // 直接复用已存在的实例
	}

	// 根据类型创建新的实例
	var client KVStoreClient
	var err error
	switch config.Type {
	case "redis":
		client, err = NewRedisClient(config)
	case "mysql":
		client, err = NewMySQLClient(config)
	default:
		return nil, fmt.Errorf("unsupported KVStore type: %s", config.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create KVStoreClient: %w", err)
	}

	// 缓存实例
	f.clients[clientKey] = client
	return client, nil
}

func (f *KVStoreFactory) CloseAllClients() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, client := range f.clients {
		if closer, ok := client.(io.Closer); ok {
			_ = closer.Close() // 关闭客户端（如果实现了 Close 方法）
		}
		delete(f.clients, key)
	}
}
