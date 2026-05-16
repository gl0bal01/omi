package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gl0bal01/omi/internal/api"
)

// effectiveTimeout returns the configured --timeout, or api.DefaultTimeout when unset.
func effectiveTimeout() time.Duration {
	if runOpts != nil && runOpts.timeout > 0 {
		return runOpts.timeout
	}
	return api.DefaultTimeout
}

// UsageError indicates a user-input or flag-validation problem. Maps to exit 2.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// RuntimeError indicates an API or system failure. Maps to exit 1.
type RuntimeError struct{ Msg string }

func (e *RuntimeError) Error() string { return e.Msg }

// TranslateAPIError converts low-level errors (api.Error, net timeouts) into
// friendly *RuntimeError messages prefixed with "omi: ". Non-API errors pass
// through unchanged. nil → nil.
func TranslateAPIError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &RuntimeError{Msg: fmt.Sprintf("omi: request timed out after %s", effectiveTimeout())}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &RuntimeError{Msg: fmt.Sprintf("omi: request timed out after %s", effectiveTimeout())}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &RuntimeError{Msg: "omi: network error: " + SanitizeForTerminal(urlErr.Err.Error())}
	}
	var dnse *net.DNSError
	if errors.As(err, &dnse) {
		return &RuntimeError{Msg: "omi: network error: DNS resolution failed"}
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 401:
			return &RuntimeError{Msg: "omi: invalid API key"}
		case apiErr.Status == 429:
			return &RuntimeError{Msg: "omi: rate limited (429); retry shortly"}
		case apiErr.Status == 422:
			if apiErr.Message != "" {
				return &RuntimeError{Msg: fmt.Sprintf("omi: validation error: %s", SanitizeForTerminal(apiErr.Message))}
			}
			return &RuntimeError{Msg: "omi: validation error (422)"}
		case apiErr.Status == 502 || apiErr.Status == 503 || apiErr.Status == 504:
			return &RuntimeError{Msg: fmt.Sprintf("omi: upstream temporarily unavailable (%d); retry", apiErr.Status)}
		case apiErr.Status >= 500 && apiErr.Status < 600:
			return &RuntimeError{Msg: fmt.Sprintf("omi: server error %d", apiErr.Status)}
		case apiErr.Status >= 400 && apiErr.Status < 500:
			if apiErr.Message != "" {
				return &RuntimeError{Msg: fmt.Sprintf("omi: api error: %d: %s", apiErr.Status, SanitizeForTerminal(apiErr.Message))}
			}
			return &RuntimeError{Msg: fmt.Sprintf("omi: api error: %d", apiErr.Status)}
		}
	}

	return err
}

// SanitizeForTerminal replaces control bytes/runes with visible hex escapes so
// untrusted upstream messages cannot inject terminal control sequences.
func SanitizeForTerminal(s string) string {
	return sanitizeForTerminalWithAllowedControls(s, "")
}

func sanitizeForTerminalWithAllowedControls(s, allowed string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "\\x%02X", s[0])
			s = s[1:]
			continue
		}
		if strings.ContainsRune(allowed, r) {
			b.WriteRune(r)
			s = s[size:]
			continue
		}
		switch {
		case (r >= 0x00 && r <= 0x1F) || r == 0x7F || (r >= 0x80 && r <= 0x9F):
			if r <= 0xFF {
				fmt.Fprintf(&b, "\\x%02X", r)
			} else {
				fmt.Fprintf(&b, "\\u%04X", r)
			}
		default:
			b.WriteRune(r)
		}
		s = s[size:]
	}
	return b.String()
}
