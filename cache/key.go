package cache

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
)

type KeyGenerator interface {
	Key(req *http.Request) (key string, err error)
}

type DefaultKeyGenerator struct {
	PartationKey string
}

func (g *DefaultKeyGenerator) Key(req *http.Request) (string, error) {
	var (
		body []byte
		err  error
	)
	if req.Body != nil {
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	h := fnv.New64a()
	h.Write([]byte(req.Method))
	h.Write([]byte(req.URL.String()))
	h.Write(body)
	return fmt.Sprintf("%s_%x", g.PartationKey, h.Sum64()), nil
}
