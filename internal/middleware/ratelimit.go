package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"cronApi/internal/errcode"
	"cronApi/internal/response"
)

// RateLimitStore 限流计数存储抽象，便于切换 Redis / 内存实现。
type RateLimitStore interface {
	// Incr 计数 +1，并在首次时设置过期时间；返回当前计数。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// RateLimit 基于固定窗口的限流中间件。
// keyFn 为 nil 时默认按「客户端 IP + 路由」限流；小程序场景也可改为按 openid 限流。
func RateLimit(store RateLimitStore, limit int, window time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	if keyFn == nil {
		keyFn = func(c *gin.Context) string {
			return "rl:" + c.FullPath() + ":" + c.ClientIP()
		}
	}
	return func(c *gin.Context) {
		key := keyFn(c)
		cnt, err := store.Incr(c.Request.Context(), key, window)
		if err != nil {
			// 限流组件故障时放行，避免影响主流程（fail-open）
			c.Next()
			return
		}
		if cnt > int64(limit) {
			c.Header("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			response.Abort(c, errcode.TooManyRequest)
			return
		}
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", max64(0, int64(limit)-cnt)))
		c.Next()
	}
}

// -------- Redis 实现 --------
type redisRL struct {
	rdb *redis.Client
}

// NewRedisRateLimitStore 基于 Redis 的分布式限流计数。
func NewRedisRateLimitStore(rdb *redis.Client) RateLimitStore { return &redisRL{rdb: rdb} }

func (r *redisRL) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := r.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// -------- 内存实现（单实例调试用） --------
type memRL struct {
	m sync.Map
}

type memEntry struct {
	count int64
	until time.Time
}

// NewMemoryRateLimitStore 单进程限流计数，重启即清空。
func NewMemoryRateLimitStore() RateLimitStore { return &memRL{} }

func (m *memRL) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	now := time.Now()
	v, _ := m.m.LoadOrStore(key, &memEntry{until: now.Add(ttl)})
	e := v.(*memEntry)
	if now.After(e.until) {
		e.count = 0
		e.until = now.Add(ttl)
	}
	e.count++
	return e.count, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
