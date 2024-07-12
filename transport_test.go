package httpclientcache

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mock_cache "github.com/Arthur1/http-client-cache/cache/mock"
	"github.com/Arthur1/http-client-cache/internal/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func assertTransport(t *testing.T, maybeTranport http.RoundTripper) *Transport {
	t.Helper()
	assert.IsType(t, &Transport{}, maybeTranport)
	transport := maybeTranport.(*Transport)
	return transport
}

type myChildTransport struct{}

func (t *myChildTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestNewTransport(t *testing.T) {
	t.Parallel()
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		cacheEngine := &mock_cache.MockCacheEngine{}
		transport := assertTransport(t, NewTransport(cacheEngine))
		assert.Equal(t, cacheEngine, transport.cacheEngine)
		assert.Equal(t, defaultChild, transport.child)
		testutil.NoDiff(t, defaultCacheableStatusCodes, transport.cacheableStatusCodes, nil)
		assert.Equal(t, defaultLogger, transport.logger)
		assert.Equal(t, defaultExpiration, transport.expiration)
	})

	t.Run("WithChild", func(t *testing.T) {
		t.Parallel()
		child := &myChildTransport{}
		transport := assertTransport(t, NewTransport(nil, WithChild(child)))
		assert.Equal(t, child, transport.child)
	})

	t.Run("WithCacheableStatusCodes", func(t *testing.T) {
		t.Parallel()
		statusCodes := []int{http.StatusOK, http.StatusBadRequest}
		transport := assertTransport(t, NewTransport(nil, WithCacheableStatusCodes(statusCodes)))
		want := map[int]struct{}{
			http.StatusOK:         {},
			http.StatusBadRequest: {},
		}
		testutil.NoDiff(t, want, transport.cacheableStatusCodes, nil)
	})

	t.Run("WithLogger", func(t *testing.T) {
		t.Parallel()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		transport := assertTransport(t, NewTransport(nil, WithLogger(logger)))
		assert.Equal(t, logger, transport.logger)
	})

	t.Run("WithTransport", func(t *testing.T) {
		t.Parallel()
		expiration := 5 * time.Hour
		transport := assertTransport(t, NewTransport(nil, WithExpiration(expiration)))
		assert.Equal(t, expiration, transport.expiration)
	})
}

func TestTransportRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("If cacheable status code and cache miss, retrieve response from origin and set to cache", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil)
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, nil).Times(1)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).Times(1)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := client.Do(req)
		assert.NoError(t, err)
		resb, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK\n", string(resb))
		assert.Equal(t, int64(1), counter)
	})

	t.Run("Avoid cache stampede", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			time.Sleep(100 * time.Millisecond)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(testutil.NewConcurrentTestReporter(t))
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil).AnyTimes()
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, nil).AnyTimes()
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).AnyTimes()

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
				res, err := client.Do(req)
				assert.NoError(t, err)
				resb, err := io.ReadAll(res.Body)
				assert.NoError(t, err)
				assert.Equal(t, "OK\n", string(resb))
				assert.Equal(t, int64(1), counter)
			}()
		}
		wg.Wait()

		assert.Equal(t, int64(1), counter)
	})

	t.Run("If uncacheable status code and cache miss, retrieve response from origin and do not set to cache", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintln(w, "BadGateway")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil)
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, nil).Times(1)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).Times(0)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := client.Do(req)
		assert.NoError(t, err)
		resb, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "BadGateway\n", string(resb))
		assert.Equal(t, int64(1), counter)
	})

	t.Run("If cache hit, retrieve response from cache", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil).Times(1)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).Times(0)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		resMock, _ := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte("HTTP/1.1 200 OK\nContent-Length: 3\n\nOK"))), req)
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(resMock, true, nil).Times(1)

		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, resMock, res)
		assert.Equal(t, int64(0), counter)
	})

	t.Run("If cache key error is occurred, retrieve response from origin and do not set to cache", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", fmt.Errorf("error"))
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, nil).Times(0)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).Times(0)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := client.Do(req)
		assert.NoError(t, err)
		resb, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK\n", string(resb))
		assert.Equal(t, int64(1), counter)
	})

	t.Run("If cache get error is occurred, retrieve response from origin and do not set to cache", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil)
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, fmt.Errorf("error")).Times(1)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(nil).Times(0)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := client.Do(req)
		assert.NoError(t, err)
		resb, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK\n", string(resb))
		assert.Equal(t, int64(1), counter)
	})

	t.Run("If cache set error is occurred, retrieve response from origin", func(t *testing.T) {
		var counter int64
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counter, 1)
			fmt.Fprintln(w, "OK")
		}))
		defer ts.Close()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cacheEngineMock := mock_cache.NewMockCacheEngine(ctrl)
		cacheEngineMock.EXPECT().Key(gomock.Any()).Return("", nil)
		cacheEngineMock.EXPECT().Get(gomock.Any(), "", gomock.Any()).Return(nil, false, nil).Times(1)
		cacheEngineMock.EXPECT().Set(gomock.Any(), "", gomock.Any(), time.Minute).Return(fmt.Errorf("error")).Times(1)

		transport := NewTransport(cacheEngineMock)
		client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

		req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
		res, err := client.Do(req)
		assert.NoError(t, err)
		resb, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK\n", string(resb))
		assert.Equal(t, int64(1), counter)
	})
}
