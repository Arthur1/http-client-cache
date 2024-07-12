package cache

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
)

type RedisCacheEngine struct {
	redisCache   *cache.Cache
	keyGenerator KeyGenerator
}

var _ CacheEngine = (*RedisCacheEngine)(nil)

type RedisClient interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd
	SetXX(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.BoolCmd
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.BoolCmd

	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

func NewRedisCacheEngine(redisCli RedisClient) *RedisCacheEngine {
	redisCache := cache.New(&cache.Options{
		Redis: redisCli,
	})
	return &RedisCacheEngine{
		redisCache: redisCache,
	}
}

func (e *RedisCacheEngine) Key(req *http.Request) (key string, err error) {
	return e.keyGenerator.Key(req)
}

func (e *RedisCacheEngine) Get(ctx context.Context, key string, req *http.Request) (*http.Response, bool, error) {
	var resb []byte
	if err := e.redisCache.Get(ctx, key, &resb); err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			return nil, false, nil
		}
		return nil, false, err
	}
	res, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(resb)), req)
	if err != nil {
		return nil, false, err
	}
	return res, true, nil
}

func (e *RedisCacheEngine) Set(ctx context.Context, key string, res *http.Response, ttl time.Duration) error {
	resb, err := httputil.DumpResponse(res, true)
	if err != nil {
		return err
	}
	item := &cache.Item{
		Ctx:   ctx,
		Key:   key,
		Value: resb,
		TTL:   ttl,
	}
	return e.redisCache.Set(item)
}
