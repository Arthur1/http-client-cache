# http-client-cache

http-client-cache is a Go library for transparent HTTP client-side caching using Transport.

Under the standard configuration, the request is retrieved from cache if the request URL, method, and body match.

Only Redis is currently supported as a cache backend.

## Usage

```go
redisCli := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 0})
transport := httpclientcache.NewTransport(
	rediscache.New(redisCli),
	httpclientcache.WithExpiration(5*time.Minute),
)
client := &http.Client{Transport: transport}
```

## Example

```go
import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"time"

	httpclientcache "github.com/Arthur1/http-client-cache"
	"github.com/Arthur1/http-client-cache/cache/engine/rediscache"
	"github.com/Arthur1/http-client-cache/cache/key"
	"github.com/redis/go-redis/v9"
)

func main() {
	redisCli := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 0})
	transport := httpclientcache.NewTransport(
        // By changing the argument of NewKeyGenerator, the cache space can be separated by such as user.
		rediscache.New(redisCli, rediscache.WithKeyGenerator(key.NewKeyGenerator("**userid**"))),
		httpclientcache.WithExpiration(5*time.Minute),
	)
	client := &http.Client{Transport: transport}

	req1, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	res1, _ := client.Do(req1) // fetch from origin
	resd1, _ := httputil.DumpResponse(res1, false)
	slog.Info("res1", slog.String("response", string(resd1)))

	req2, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	res2, _ := client.Do(req2) // fetch from cache
	resd2, _ := httputil.DumpResponse(res2, false)
	slog.Info("res2", slog.String("response", string(resd2)))

	time.Sleep(5 * time.Minute)

	req3, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	res3, _ := client.Do(req3) // fetch from origin
	resd3, _ := httputil.DumpResponse(res3, false)
	slog.Info("res3", slog.String("response", string(resd3)))
}
```
