package rediscache

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/Arthur1/http-client-cache/cache/engine"
	"github.com/Arthur1/http-client-cache/cache/key"
	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
)

type CacheEngine struct {
	redisCache   *cache.Cache
	keyGenerator key.KeyGenerator
}

var _ engine.CacheEngine = (*CacheEngine)(nil)

type RedisClient interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd
	SetXX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd
	SetNX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Option interface {
	apply(opts *options)
}

var (
	_ Option = keyGeneratorOption{}
	_ Option = localCacheOption{}
)

type options struct {
	keyGenerator key.KeyGenerator
	localCache   cache.LocalCache
}

type keyGeneratorOption struct {
	keyGenerator key.KeyGenerator
}

func (o keyGeneratorOption) apply(opts *options) {
	opts.keyGenerator = o.keyGenerator
}

func WithKeyGenerator(keyGenerator key.KeyGenerator) keyGeneratorOption {
	return keyGeneratorOption{keyGenerator}
}

type localCacheOption struct {
	localCache cache.LocalCache
}

func (o localCacheOption) apply(opts *options) {
	opts.localCache = o.localCache
}

func WithLocalCache(localCache cache.LocalCache) localCacheOption {
	return localCacheOption{localCache}
}

func New(redisCli RedisClient, opts ...Option) *CacheEngine {
	options := &options{
		keyGenerator: key.NewKeyGenerator(""),
		localCache:   nil,
	}
	for _, o := range opts {
		o.apply(options)
	}

	redisCache := cache.New(&cache.Options{
		Redis:      redisCli,
		LocalCache: options.localCache,
	})
	return &CacheEngine{
		redisCache:   redisCache,
		keyGenerator: options.keyGenerator,
	}
}

func (e *CacheEngine) Key(req *http.Request) (key string, err error) {
	return e.keyGenerator.Key(req)
}

func (e *CacheEngine) Get(ctx context.Context, key string, req *http.Request) (*http.Response, bool, error) {
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

func (e *CacheEngine) Set(ctx context.Context, key string, res *http.Response, ttl time.Duration) error {
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
