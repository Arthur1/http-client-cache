package cache

import (
	"context"
	"net/http"
	"time"
)

type CacheEngine interface {
	Key(req *http.Request) (key string, err error)
	Get(ctx context.Context, key string, req *http.Request) (res *http.Response, ok bool, err error)
	Set(ctx context.Context, key string, res *http.Response, expiration time.Duration) error
}
