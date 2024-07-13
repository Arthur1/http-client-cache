package httpclientcache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Arthur1/http-client-cache/cache/engine/rediscache"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestTransportWithRedisEngine(t *testing.T) {
	t.Parallel()
	var counter int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)
		fmt.Fprintln(w, "OK")
	}))
	defer ts.Close()

	rs, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	redisCli := redis.NewClient(&redis.Options{
		Addr: rs.Addr(),
		DB:   0,
	})

	transport := NewTransport(rediscache.New(redisCli))
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

	// access origin
	req1, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	res1, err := client.Do(req1)
	assert.NoError(t, err)
	resb1, err := io.ReadAll(res1.Body)
	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(resb1))
	assert.Equal(t, int64(1), counter)

	// fetch from cache
	req2, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	res2, err := client.Do(req2)
	assert.NoError(t, err)
	resb2, err := io.ReadAll(res2.Body)
	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(resb2))
	assert.Equal(t, int64(1), counter)

	// access origin because another url
	req3, _ := http.NewRequest(http.MethodGet, ts.URL+"/hoge", nil)
	res3, err := client.Do(req3)
	assert.NoError(t, err)
	resb3, err := io.ReadAll(res3.Body)
	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(resb3))
	assert.Equal(t, int64(2), counter)

	rs.FlushDB()

	// access origin because cache db has flushed
	req4, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	res4, err := client.Do(req4)
	assert.NoError(t, err)
	resb4, err := io.ReadAll(res4.Body)
	assert.NoError(t, err)
	assert.Equal(t, "OK\n", string(resb4))
	assert.Equal(t, int64(3), counter)
}
