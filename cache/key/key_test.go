package key

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultKeyGeneratorKey(t *testing.T) {
	t.Parallel()
	t.Run("If partition keys, HTTP methods, URLs, and bodies are all the same, same cache key", func(t *testing.T) {
		t.Parallel()
		g1 := &DefaultKeyGenerator{PartitionKey: ""}
		g2 := &DefaultKeyGenerator{PartitionKey: ""}
		req1, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		req2, _ := http.NewRequest(http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("A")))

		got1, err := g1.Key(req1)
		assert.NoError(t, err)
		got2, err := g1.Key(req1)
		assert.NoError(t, err)
		got3, err := g1.Key(req2)
		assert.NoError(t, err)
		got4, err := g1.Key(req2)
		assert.NoError(t, err)
		got5, err := g2.Key(req1)
		assert.NoError(t, err)

		assert.Equal(t, got1, got2)
		assert.Equal(t, got3, got4)
		assert.Equal(t, got1, got5)
	})

	t.Run("If different partition keys, different cache keys", func(t *testing.T) {
		t.Parallel()
		g1 := &DefaultKeyGenerator{PartitionKey: "client1"}
		g2 := &DefaultKeyGenerator{PartitionKey: "client2"}
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

		got1, err := g1.Key(req)
		assert.NoError(t, err)
		got2, err := g2.Key(req)
		assert.NoError(t, err)

		assert.NotEqual(t, got1, got2)
	})

	t.Run("If different HTTP methods, different cache keys", func(t *testing.T) {
		t.Parallel()
		g := &DefaultKeyGenerator{PartitionKey: ""}
		req1, _ := http.NewRequest(http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("A")))
		req2, _ := http.NewRequest(http.MethodPut, "http://example.com", io.NopCloser(strings.NewReader("A")))

		got1, err := g.Key(req1)
		assert.NoError(t, err)
		got2, err := g.Key(req2)
		assert.NoError(t, err)

		assert.NotEqual(t, got1, got2)
	})

	t.Run("If different URLs, different cache keys", func(t *testing.T) {
		t.Parallel()
		g := &DefaultKeyGenerator{PartitionKey: ""}
		req1, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		req2, _ := http.NewRequest(http.MethodGet, "http://example.com/hoge", nil)

		got1, err := g.Key(req1)
		assert.NoError(t, err)
		got2, err := g.Key(req2)
		assert.NoError(t, err)

		assert.NotEqual(t, got1, got2)
	})

	t.Run("If different bodies, different bodies", func(t *testing.T) {
		t.Parallel()
		g := &DefaultKeyGenerator{PartitionKey: ""}
		req1, _ := http.NewRequest(http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("A")))
		req2, _ := http.NewRequest(http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("B")))

		got1, err := g.Key(req1)
		assert.NoError(t, err)
		got2, err := g.Key(req2)
		assert.NoError(t, err)

		assert.NotEqual(t, got1, got2)
	})

	t.Run("Header differences do not affect key generation", func(t *testing.T) {
		t.Parallel()
		g := &DefaultKeyGenerator{PartitionKey: ""}
		req1, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		req1.Header.Set("Authorization", "Bearer 1")
		req2, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		req2.Header.Set("Authorization", "Bearer 2")

		got1, err := g.Key(req1)
		assert.NoError(t, err)
		got2, err := g.Key(req2)
		assert.NoError(t, err)

		assert.Equal(t, got1, got2)
	})

	t.Run("Request body is still readable after key generation", func(t *testing.T) {
		t.Parallel()
		g := &DefaultKeyGenerator{PartitionKey: ""}
		req, _ := http.NewRequest(http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("A")))
		_, err := g.Key(req)
		assert.NoError(t, err)

		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, "A", string(body))
	})
}
