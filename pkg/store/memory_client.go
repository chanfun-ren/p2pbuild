package store

import (
	"fmt"
	"sync"
	"time"
)

type MemoryStoreClient struct {
	data map[string]struct {
		value string
		ttl   time.Time
	}
	sync.RWMutex
}

func NewMemoryStoreClient() *MemoryStoreClient {
	return &MemoryStoreClient{
		data: make(map[string]struct {
			value string
			ttl   time.Time
		}),
	}
}

func (m *MemoryStoreClient) Set(key string, value string, ttl time.Duration) error {
	m.Lock()
	defer m.Unlock()
	m.data[key] = struct {
		value string
		ttl   time.Time
	}{value: value, ttl: time.Now().Add(ttl)}
	return nil
}

func (m *MemoryStoreClient) Get(key string) (string, error) {
	m.RLock()
	defer m.RUnlock()
	entry, exists := m.data[key]
	if !exists || time.Now().After(entry.ttl) {
		return "", fmt.Errorf("key not found or expired")
	}
	return entry.value, nil
}

func (m *MemoryStoreClient) Delete(key string) error {
	m.Lock()
	defer m.Unlock()
	delete(m.data, key)
	return nil
}
