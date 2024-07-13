package httpclientcache

import (
	"bufio"
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Arthur1/http-client-cache/cache/engine"
)

type Transport struct {
	cacheEngine          engine.CacheEngine
	base                 http.RoundTripper
	cacheableStatusCodes map[int]struct{}
	logger               *slog.Logger
	expiration           time.Duration
}

var (
	defaultBase                 = http.DefaultTransport
	defaultLogger               = slog.Default()
	defaultCacheableStatusCodes = map[int]struct{}{http.StatusOK: {}}
	defaultExpiration           = 1 * time.Minute
)

type options struct {
	base                 http.RoundTripper
	cacheableStatusCodes map[int]struct{}
	logger               *slog.Logger
	expiration           time.Duration
}

type Option interface {
	apply(opts *options)
}

var (
	_ Option = baseOption{}
	_ Option = cacheableStatusCodesOption{}
	_ Option = loggerOption{}
	_ Option = expirationOption(0)
)

type baseOption struct {
	base http.RoundTripper
}

func (o baseOption) apply(opts *options) {
	opts.base = http.RoundTripper(o.base)
}

func WithBase(child http.RoundTripper) baseOption {
	return baseOption{child}
}

type cacheableStatusCodesOption []int

func (o cacheableStatusCodesOption) apply(opts *options) {
	opts.cacheableStatusCodes = map[int]struct{}{}
	for _, statusCode := range o {
		opts.cacheableStatusCodes[statusCode] = struct{}{}
	}
}

type loggerOption struct {
	logger *slog.Logger
}

func (o loggerOption) apply(opts *options) {
	opts.logger = o.logger
}

func WithLogger(logger *slog.Logger) loggerOption {
	return loggerOption{logger}
}

type expirationOption time.Duration

func (o expirationOption) apply(opts *options) {
	opts.expiration = time.Duration(o)
}

func WithExpiration(expiration time.Duration) expirationOption {
	return expirationOption(expiration)
}

func WithCacheableStatusCodes(statusCodes []int) cacheableStatusCodesOption {
	return cacheableStatusCodesOption(statusCodes)
}

func NewTransport(cacheEngine engine.CacheEngine, opts ...Option) http.RoundTripper {
	options := &options{
		base:                 defaultBase,
		logger:               defaultLogger,
		cacheableStatusCodes: defaultCacheableStatusCodes,
		expiration:           defaultExpiration,
	}
	for _, o := range opts {
		o.apply(options)
	}

	return &Transport{
		cacheEngine:          cacheEngine,
		base:                 options.base,
		logger:               options.logger,
		cacheableStatusCodes: options.cacheableStatusCodes,
		expiration:           options.expiration,
	}
}

var group singleflight.Group

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	key, err := t.cacheEngine.Key(req)
	if err != nil {
		t.logger.ErrorContext(ctx, "through http-client-cache because failed to generate cache key", slog.Any("error", err))
		return t.base.RoundTrip(req)
	}

	cachedRes, ok, err := t.cacheEngine.Get(ctx, key, req)
	if err != nil {
		t.logger.ErrorContext(ctx, "through http-client-cache because failed to get from cache", slog.Any("error", err))
		return t.base.RoundTrip(req)
	}
	if ok {
		// cache hit
		return cachedRes, nil
	}

	maybeResb, err, _ := group.Do(key, func() (any, error) {
		res, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		if _, ok := t.cacheableStatusCodes[res.StatusCode]; ok {
			if err := t.cacheEngine.Set(ctx, key, res, t.expiration); err != nil {
				t.logger.ErrorContext(ctx, "through http-client-cache because failed to set to cache", slog.Any("error", err))
			}
		}
		resb, err := httputil.DumpResponse(res, true)
		return resb, err
	})
	if err != nil {
		return nil, err
	}
	resb := maybeResb.([]byte)
	res, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(resb)), req)
	return res, err
}
