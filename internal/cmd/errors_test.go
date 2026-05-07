package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/gl0bal01/omi/internal/api"
)

type fakeTimeoutErr struct{}

func (fakeTimeoutErr) Error() string   { return "fake timeout" }
func (fakeTimeoutErr) Timeout() bool   { return true }
func (fakeTimeoutErr) Temporary() bool { return false }

func TestTranslateAPIError_Table(t *testing.T) {
	cases := []struct {
		name     string
		in       error
		wantNil  bool
		wantKind string // "runtime", "passthrough"
		wantSubs []string
	}{
		{name: "nil", in: nil, wantNil: true},
		{
			name:     "401",
			in:       &api.Error{Status: 401, Message: "unauthorized"},
			wantKind: "runtime",
			wantSubs: []string{"invalid API key"},
		},
		{
			name:     "429",
			in:       &api.Error{Status: 429, Message: "slow down"},
			wantKind: "runtime",
			wantSubs: []string{"rate limited", "429"},
		},
		{
			name:     "500",
			in:       &api.Error{Status: 500, Message: "boom"},
			wantKind: "runtime",
			wantSubs: []string{"server error 500"},
		},
		{
			name:     "503",
			in:       &api.Error{Status: 503},
			wantKind: "runtime",
			wantSubs: []string{"temporarily unavailable", "503"},
		},
		{
			name:     "422",
			in:       &api.Error{Status: 422, Message: "bad prompt"},
			wantKind: "runtime",
			wantSubs: []string{"validation error", "bad prompt"},
		},
		{
			name:     "400 with message",
			in:       &api.Error{Status: 400, Message: "bad"},
			wantKind: "runtime",
			wantSubs: []string{"400", "bad"},
		},
		{
			name:     "400 with control chars sanitized",
			in:       &api.Error{Status: 400, Message: "bad\x1b[31mred"},
			wantKind: "runtime",
			wantSubs: []string{"400", "bad\\x1B[31mred"},
		},
		{
			name:     "deadline-exceeded",
			in:       context.DeadlineExceeded,
			wantKind: "runtime",
			wantSubs: []string{"timed out"},
		},
		{
			name:     "wrapped deadline",
			in:       fmt.Errorf("doing stuff: %w", context.DeadlineExceeded),
			wantKind: "runtime",
			wantSubs: []string{"timed out"},
		},
		{
			name:     "net.Error timeout",
			in:       fakeTimeoutErr{},
			wantKind: "runtime",
			wantSubs: []string{"timed out"},
		},
		{
			name:     "passthrough random",
			in:       errors.New("something else"),
			wantKind: "passthrough",
			wantSubs: []string{"something else"},
		},
		{
			name:     "url error",
			in:       &url.Error{Op: "Post", URL: "https://api.1min.ai", Err: errors.New("connection refused")},
			wantKind: "runtime",
			wantSubs: []string{"network error", "connection refused"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := TranslateAPIError(tc.in)
			if tc.wantNil {
				if out != nil {
					t.Fatalf("want nil, got %v", out)
				}
				return
			}
			if out == nil {
				t.Fatalf("want non-nil error, got nil")
			}
			switch tc.wantKind {
			case "runtime":
				var re *RuntimeError
				if !errors.As(out, &re) {
					t.Fatalf("want *RuntimeError, got %T (%v)", out, out)
				}
				if !strings.HasPrefix(re.Msg, "omi: ") {
					t.Errorf("RuntimeError msg should start with 'omi: ', got %q", re.Msg)
				}
			case "passthrough":
				if _, ok := out.(*RuntimeError); ok {
					t.Errorf("did not expect *RuntimeError, got %v", out)
				}
			}
			msg := out.Error()
			for _, s := range tc.wantSubs {
				if !strings.Contains(msg, s) {
					t.Errorf("msg=%q missing substring %q", msg, s)
				}
			}
		})
	}
}

func TestSanitizeForTerminal(t *testing.T) {
	got := SanitizeForTerminal("a\x00b\x1bc")
	if got != "a\\x00b\\x1Bc" {
		t.Fatalf("got=%q", got)
	}
}

func TestUsageError_SatisfiesError(t *testing.T) {
	var err error = &UsageError{Msg: "omi: bad flag"}
	if err.Error() != "omi: bad flag" {
		t.Errorf("Error()=%q", err.Error())
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("errors.As should detect *UsageError")
	}
}

func TestRuntimeError_SatisfiesError(t *testing.T) {
	var err error = &RuntimeError{Msg: "omi: boom"}
	if err.Error() != "omi: boom" {
		t.Errorf("Error()=%q", err.Error())
	}
	var re *RuntimeError
	if !errors.As(err, &re) {
		t.Errorf("errors.As should detect *RuntimeError")
	}
}

// Ensure fakeTimeoutErr satisfies net.Error.
var _ net.Error = fakeTimeoutErr{}
