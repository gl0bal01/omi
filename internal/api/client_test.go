package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gl0bal01/omi/internal/closeutil"
)

func TestClient_RetryOn5xxNonStream(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, time.Second)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", bytes.NewReader([]byte(`{}`)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte(`{}`))), nil
	}
	resp, err := c.do(req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer closeutil.Quiet(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls=%d, want 2 (single retry)", got)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(400)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, time.Second)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", strings.NewReader("{}"))
	resp, err := c.do(req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer closeutil.Quiet(resp.Body)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls=%d, want 1 (no retry)", got)
	}
}

func TestClient_RetryOn429NonStream(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, time.Second)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", strings.NewReader("{}"))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("{}")), nil
	}
	resp, err := c.do(req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer closeutil.Quiet(resp.Body)
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls=%d want 2", got)
	}
}

func TestClient_NoRetryOnStream(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, time.Second)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", strings.NewReader("{}"))
	resp, _ := c.do(req, true)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls=%d, want 1 (stream skips retry)", got)
	}
}

func TestClient_InjectsAPIKey(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("API-KEY")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient("secret-key", srv.URL, time.Second)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", strings.NewReader("{}"))
	resp, err := c.do(req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	_ = resp.Body.Close()
	if seen != "secret-key" {
		t.Fatalf("API-KEY=%q", seen)
	}
}

func TestNewClient_StreamHasNoGlobalTimeout(t *testing.T) {
	c := NewClient("k", "https://example.com", time.Second)
	if c.HTTP.Timeout != time.Second {
		t.Fatalf("HTTP timeout=%v want %v", c.HTTP.Timeout, time.Second)
	}
	if c.StreamHTTP.Timeout != 0 {
		t.Fatalf("StreamHTTP timeout=%v want 0", c.StreamHTTP.Timeout)
	}
}

func TestDebugRequest_RedactsAPIKey(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	var logs bytes.Buffer
	c := NewClient("super-secret-api-key", srv.URL, time.Second)
	c.SetDebug(true, &logs)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", strings.NewReader(`{"type":"PING"}`))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(`{"type":"PING"}`)), nil
	}
	resp, err := c.do(req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	_ = resp.Body.Close()

	seen = logs.String()
	if strings.Contains(seen, "super-secret-api-key") {
		t.Fatalf("debug logs leaked full API key: %q", seen)
	}
	if !strings.Contains(seen, "sup...-key") {
		t.Fatalf("debug logs missing masked key: %q", seen)
	}
	if !strings.Contains(seen, `preview="{\"type\":\"PING\"}"`) {
		t.Fatalf("debug logs missing body preview: %q", seen)
	}
}

func TestClient_RefusesCrossHostRedirect(t *testing.T) {
	var leaked atomic.Bool
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("API-KEY") != "" {
			leaked.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+"/collect", http.StatusFound)
	}))
	defer src.Close()

	c := NewClient("super-secret-api-key", src.URL, time.Second)
	req, _ := http.NewRequest(http.MethodGet, src.URL+"/start", nil)
	resp, err := c.do(req, false)
	if resp != nil {
		closeutil.Quiet(resp.Body)
	}
	if err == nil {
		t.Fatalf("expected cross-host redirect to be refused, got nil error")
	}
	if leaked.Load() {
		t.Fatalf("API-KEY header leaked to cross-host redirect target")
	}
}
