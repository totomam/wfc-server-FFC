package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// countingBody serves `{"secret":"aaaa...` forever-ish (size bytes) and counts what the handler read.
type countingBody struct {
	size, read int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.read >= b.size {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > b.size-b.read {
		n = b.size - b.read
	}
	for i := int64(0); i < n; i++ {
		if b.read+i < 11 {
			p[i] = `{"secret":"`[b.read+i]
		} else {
			p[i] = 'a'
		}
	}
	b.read += n
	return int(n), nil
}

// postCapHandlers lists every admin POST handler that goes through parsePost.
var postCapHandlers = map[string]http.HandlerFunc{
	"/api/ban":   HandleBan,
	"/api/unban": HandleUnban,
	"/api/kick":  HandleKick,
}

// NET-15: a body over maxPostBodySize gets 413 and the handler reads at most the cap (+1 byte to detect overflow).
func TestPostBodyCap(t *testing.T) {
	for path, h := range postCapHandlers {
		body := &countingBody{size: 16 << 20}
		req := httptest.NewRequest(http.MethodPost, path, body)
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: status %d, want 413", path, rec.Code)
		}
		if body.read > maxPostBodySize+1 {
			t.Errorf("%s: handler read %d bytes, cap is %d", path, body.read, maxPostBodySize)
		}
	}
}
