package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gl0bal01/omi/internal/closeutil"
)

const DefaultBaseURL = "https://api.1min.ai"
const DefaultTimeout = 60 * time.Second

// maxErrorBodyBytes caps the upstream error body we read so a hostile or
// runaway server cannot pump arbitrary bytes into log output.
const maxErrorBodyBytes = 64 * 1024

// maxResponseBodyBytes caps non-streaming success responses (code, transcribe).
const maxResponseBodyBytes = 16 * 1024 * 1024

// readErrorBody reads up to maxErrorBodyBytes from r and returns the bytes.
// Errors are intentionally swallowed — callers already have a status code.
func readErrorBody(r io.Reader) []byte {
	data, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return data
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("api error: status %d", e.Status)
	}
	return fmt.Sprintf("api error: status %d: %s", e.Status, e.Message)
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTP       *http.Client
	StreamHTTP *http.Client
	Debug      bool
	DebugOut   io.Writer
	Backoffs   []time.Duration
}

func NewClient(apiKey, baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		HTTP:       &http.Client{Timeout: timeout},
		StreamHTTP: &http.Client{},
		Debug:      false,
		DebugOut:   io.Discard,
		Backoffs:   []time.Duration{100 * time.Millisecond, 300 * time.Millisecond},
	}
}

// SetDebug toggles request/response debug logs with header/body redaction.
func (c *Client) SetDebug(enabled bool, out io.Writer) {
	c.Debug = enabled
	if out == nil {
		out = io.Discard
	}
	c.DebugOut = out
}

// do performs an HTTP request with API-KEY auth.
//
// Retry policy: a single 500ms-backoff retry applies ONLY to non-streaming
// requests, and only when the failure is a connection-level error or a 5xx
// response received before any body has been read. Streaming requests
// (stream=true) are NEVER retried — once the server starts writing SSE bytes,
// any subsequent error is surfaced verbatim. 4xx is never retried.
func (c *Client) do(req *http.Request, stream bool) (*http.Response, error) {
	req.Header.Set("API-KEY", c.APIKey)
	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	httpClient := c.HTTP
	if stream {
		httpClient = c.StreamHTTP
	}
	attempts := 1
	if !stream {
		attempts += len(c.Backoffs)
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptReq := req
		if attempt > 1 {
			attemptReq, err = cloneRequest(req)
			if err != nil {
				return resp, err
			}
		}

		start := time.Now()
		c.debugRequest(attemptReq, stream, attempt, attempts)
		resp, err = httpClient.Do(attemptReq)
		c.debugResponse(attemptReq, resp, err, stream, attempt, attempts, time.Since(start))

		if stream || !shouldRetry(resp, err) || attempt == attempts {
			return resp, err
		}
		wait := c.Backoffs[attempt-1]
		if resp != nil {
			if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
				wait = d
			}
			_ = resp.Body.Close()
		}
		select {
		case <-attemptReq.Context().Done():
			return resp, attemptReq.Context().Err()
		case <-time.After(wait):
		}
	}
	return resp, err
}

// parseRetryAfter parses the Retry-After header per RFC 7231: integer seconds
// or HTTP-date. Returns 0 on parse failure (caller falls back to default
// backoff). Caps at 30s so a hostile server cannot stall the client.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	const maxWait = 30 * time.Second
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		d := time.Duration(secs) * time.Second
		if d > maxWait {
			return maxWait
		}
		return d
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		if d > maxWait {
			return maxWait
		}
		return d
	}
	return 0
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600)) {
		return true
	}
	return false
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	r := req.Clone(req.Context())
	if req.Body == nil || req.GetBody == nil {
		r.Body = nil
		return r, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	r.Body = body
	return r, nil
}

func (c *Client) debugRequest(req *http.Request, stream bool, attempt, attempts int) {
	if !c.Debug {
		return
	}
	_, _ = fmt.Fprintf(c.DebugOut, "omi: debug: -> %s %s stream=%t attempt=%d/%d\n", req.Method, req.URL.String(), stream, attempt, attempts)
	_, _ = fmt.Fprintf(c.DebugOut, "omi: debug:    headers: %s\n", debugHeaders(req.Header))
	_, _ = fmt.Fprintf(c.DebugOut, "omi: debug:    body: %s\n", debugBodySummary(req))
}

func (c *Client) debugResponse(req *http.Request, resp *http.Response, err error, stream bool, attempt, attempts int, elapsed time.Duration) {
	if !c.Debug {
		return
	}
	if err != nil {
		_, _ = fmt.Fprintf(c.DebugOut, "omi: debug: <- %s %s error=%v elapsed=%s attempt=%d/%d retry=%t\n",
			req.Method, req.URL.String(), err, elapsed.Round(time.Millisecond), attempt, attempts, !stream && attempt < attempts)
		return
	}
	_, _ = fmt.Fprintf(c.DebugOut, "omi: debug: <- %s %s status=%d elapsed=%s attempt=%d/%d retry=%t\n",
		req.Method, req.URL.String(), resp.StatusCode, elapsed.Round(time.Millisecond), attempt, attempts, !stream && shouldRetry(resp, nil) && attempt < attempts)
}

func debugHeaders(h http.Header) string {
	if len(h) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.Join(h[k], ",")
		if textproto.CanonicalMIMEHeaderKey(k) == "Api-Key" {
			v = maskSecret(v)
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

func debugBodySummary(req *http.Request) string {
	if req == nil || req.Body == nil {
		return "none"
	}
	if req.GetBody == nil {
		return "present (non-replayable)"
	}
	body, err := req.GetBody()
	if err != nil {
		return "unavailable: " + err.Error()
	}
	defer closeutil.Quiet(body)
	data, err := io.ReadAll(body)
	if err != nil {
		return "unavailable: " + err.Error()
	}
	const max = 220
	preview := data
	if len(preview) > max {
		preview = preview[:max]
	}
	normalized := strings.Join(strings.Fields(string(preview)), " ")
	if len(data) > max {
		normalized += " ..."
	}
	return fmt.Sprintf("bytes=%d preview=%q", len(data), normalized)
}

func maskSecret(s string) string {
	if len(s) <= 7 {
		if s == "" {
			return ""
		}
		return "***"
	}
	var b bytes.Buffer
	b.WriteString(s[:3])
	b.WriteString("...")
	b.WriteString(s[len(s)-4:])
	return b.String()
}
