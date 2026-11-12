package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisCache struct {
	rdb *redis.Client
}

// NewRedis 将 go-redis 客户端包装为 Cache。
func NewRedis(rdb *redis.Client) Cache { return &redisCache{rdb: rdb} }

func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := r.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	return v, err
}

func (r *redisCache) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return r.rdb.Set(ctx, key, val, ttl).Err()
}

var setNXScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then return 0 end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return 1
`)

func (r *redisCache) SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return r.rdb.SetNX(ctx, key, val, 0).Result()
	}
	n, err := setNXScript.Run(ctx, r.rdb, []string{key}, val, ttl.Milliseconds()).Int64()
	return n == 1, err
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.rdb.Del(ctx, keys...).Err()
}

var incrScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
return n
`)

func (r *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return incrScript.Run(ctx, r.rdb, []string{key}, ttl.Milliseconds()).Int64()
}

func (r *redisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.rdb.PExpire(ctx, key, ttl).Err()
}

func (r *redisCache) ScanKeys(ctx context.Context, prefix string, limit int64) ([]string, error) {
	var (
		cursor uint64
		out    []string
	)
	for {
		keys, next, err := r.rdb.Scan(ctx, cursor, prefix+"*", 500).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
		cursor = next
		if cursor == 0 || int64(len(out)) >= limit {
			break
		}
	}
	if int64(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *redisCache) Close() error { return r.rdb.Close() }
