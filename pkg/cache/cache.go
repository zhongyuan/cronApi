// Package cache 定义 KV 缓存抽象，并提供 Redis / 内存两种实现。
// 会话、登录态、限流计数都依赖它，方便单测时替换。
package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotFound 键不存在。
var ErrNotFound = errors.New("cache: key not found")

// Cache 最小 KV 接口（参考 go-redis 语义）。
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, keys ...string) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	// ScanKeys 按前缀列出键，供会话管理/审计使用。
	ScanKeys(ctx context.Context, prefix string, limit int64) ([]string, error)
	Close() error
}

// -------- 内存实现 --------

type memItem struct {
	val   string
	until time.Time // 零值表示永不过期
}

type memCache struct {
	mu   sync.RWMutex
	data map[string]memItem
}

// NewMemory 返回进程内缓存，用于本地开发与单测。
func NewMemory() Cache {
	c := &memCache{data: make(map[string]memItem)}
	go c.janitor()
	return c
}

func (m *memCache) janitor() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		m.mu.Lock()
		for k, v := range m.data {
			if !v.until.IsZero() && now.After(v.until) {
				delete(m.data, k)
			}
		}
		m.mu.Unlock()
	}
}

func (m *memCache) get(key string) (memItem, bool) {
	m.mu.RLock()
	it, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return memItem{}, false
	}
	if !it.until.IsZero() && time.Now().After(it.until) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return memItem{}, false
	}
	return it, true
}

func (m *memCache) Get(_ context.Context, key string) (string, error) {
	if it, ok := m.get(key); ok {
		return it.val, nil
	}
	return "", ErrNotFound
}

func (m *memCache) Set(_ context.Context, key, val string, ttl time.Duration) error {
	it := memItem{val: val}
	if ttl > 0 {
		it.until = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.data[key] = it
	m.mu.Unlock()
	return nil
}

func (m *memCache) SetNX(_ context.Context, key, val string, ttl time.Duration) (bool, error) {
	if _, ok := m.get(key); ok {
		return false, nil
	}
	it := memItem{val: val}
	if ttl > 0 {
		it.until = time.Now().Add(ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return false, nil
	}
	m.data[key] = it
	return true, nil
}

func (m *memCache) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	for _, k := range keys {
		delete(m.data, k)
	}
	m.mu.Unlock()
	return nil
}

func (m *memCache) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.data[key]
	if !ok || (!it.until.IsZero() && time.Now().After(it.until)) {
		it = memItem{val: "0"}
		if ttl > 0 {
			it.until = time.Now().Add(ttl)
		}
	}
	var n int64
	for _, ch := range it.val {
		if ch < '0' || ch > '9' {
			n = 0
			break
		}
		n = n*10 + int64(ch-'0')
	}
	n++
	it.val = itoa(n)
	m.data[key] = it
	return n, nil
}

func (m *memCache) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.data[key]
	if !ok {
		return nil
	}
	it.until = time.Now().Add(ttl)
	m.data[key] = it
	return nil
}

func (m *memCache) ScanKeys(_ context.Context, prefix string, limit int64) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.data))
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k)
			if int64(len(out)) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memCache) Close() error { return nil }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
