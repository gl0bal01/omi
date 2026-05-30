package redact

import (
	"strings"
	"testing"
)

func TestAPIKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "one char", in: "x", want: "***"},
		{name: "six chars masked entirely", in: "abcdef", want: "***"},
		{name: "seven chars masked", in: "abcdefg", want: "***"},
		{name: "ten chars masked (below threshold)", in: "abcdefghij", want: "***"},
		{name: "eleven chars revealed", in: "abcdefghijk", want: "abc...hijk"},
		{name: "long key", in: "sk-live-1234567890abcdef", want: "sk-...cdef"},
		{name: "unicode key byte-indexed", in: "ＡＢＣ_efgh", want: "Ａ...efgh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := APIKey(tt.in)
			if got != tt.want {
				t.Fatalf("APIKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAPIKeyNeverLeaksMiddle(t *testing.T) {
	const secret = "MIDDLE-SECRET-PAYLOAD-XXXX"
	got := APIKey(secret)
	if got == secret {
		t.Fatalf("APIKey returned raw input")
	}
	if strings.Contains(got, "SECRET") {
		t.Fatalf("APIKey leaked middle segment: %q", got)
	}
}
