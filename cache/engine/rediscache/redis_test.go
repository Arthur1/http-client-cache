package rediscache

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/Arthur1/http-client-cache/cache/key"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

type testKeyGenerator struct{}

func (g *testKeyGenerator) Key(_ *http.Request) (string, error) {
	return "test", nil
}

func TestNew(t *testing.T) {
	t.Parallel()
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		redisCli := redis.NewClient(&redis.Options{})
		e := New(redisCli)
		assert.IsType(t, &key.DefaultKeyGenerator{}, e.keyGenerator)
		assert.NotEmpty(t, e.redisCache)
	})

	t.Run("WithKeyGenerator", func(t *testing.T) {
		t.Parallel()
		redisCli := redis.NewClient(&redis.Options{})
		keyGenerator := &testKeyGenerator{}
		e := New(redisCli, WithKeyGenerator(keyGenerator))
		assert.Equal(t, keyGenerator, e.keyGenerator)
	})

	t.Run("WithLocalCache", func(t *testing.T) {
		t.Parallel()
		redisCli := redis.NewClient(&redis.Options{})
		localCache := cache.NewTinyLFU(10, time.Minute)
		New(redisCli, WithLocalCache(localCache))
	})
}

func TestCacheEngineKey(t *testing.T) {
	t.Parallel()
	redisCli := redis.NewClient(&redis.Options{})
	keyGenerator := &testKeyGenerator{}
	e := New(redisCli, WithKeyGenerator(keyGenerator))
	got, err := e.Key(nil)
	assert.NoError(t, err)
	assert.Equal(t, "test", got)
}

func TestCacheEngineGetAndSet(t *testing.T) {
	t.Parallel()
	t.Run("cache miss", func(t *testing.T) {
		t.Parallel()
		rs, err := miniredis.Run()
		if err != nil {
			t.Fatal(err)
		}
		redisCli := redis.NewClient(&redis.Options{Addr: rs.Addr(), DB: 0})
		e := New(redisCli)
		ctx := context.Background()
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)

		_, ok, err := e.Get(ctx, "key1", req)
		assert.False(t, ok)
		assert.NoError(t, err)
	})

	t.Run("set and cache hit", func(t *testing.T) {
		t.Parallel()
		rs, err := miniredis.Run()
		if err != nil {
			t.Fatal(err)
		}
		redisCli := redis.NewClient(&redis.Options{Addr: rs.Addr(), DB: 0})
		e := New(redisCli)
		ctx := context.Background()
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		serializedResMock := []byte("HTTP/1.1 200 OK\nContent-Length: 3\n\nOK\n")
		resMock, _ := http.ReadResponse(bufio.NewReader(bytes.NewReader(serializedResMock)), req)

		err = e.Set(ctx, "key1", resMock, time.Hour)
		assert.NoError(t, err)
		res, ok, err := e.Get(ctx, "key1", req)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		resb, err := ioutil.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK\n", string(resb))
	})
}
