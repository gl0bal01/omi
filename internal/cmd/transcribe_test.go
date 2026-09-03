package cmd

import "testing"

func TestResolveTranscribeModel_IgnoresChatModel(t *testing.T) {
	// OMI_MODEL / config.model are chat models and must not leak into speech.
	t.Setenv("OMI_MODEL", "claude")
	if got := resolveTranscribeModel(""); got != "qwen3-asr-flash" {
		t.Fatalf("default=%q", got)
	}
	if got := resolveTranscribeModel("flag-model"); got != "flag-model" {
		t.Fatalf("flag precedence=%q", got)
	}
}

func TestResolveTranscribeAlias(t *testing.T) {
	if got := resolveTranscribeAlias("asr"); got != "qwen3-asr-flash" {
		t.Fatalf("asr alias=%q", got)
	}
	if got := resolveTranscribeAlias("asr-diarize"); got != "gpt-4o-transcribe-diarize" {
		t.Fatalf("asr-diarize alias=%q", got)
	}
	if got := resolveTranscribeAlias("qwen3-asr-flash"); got != "qwen3-asr-flash" {
		t.Fatalf("raw model changed=%q", got)
	}
}
