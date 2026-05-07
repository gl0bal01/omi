package cmd

import "testing"

func TestResolveTranscribeModel_Default(t *testing.T) {
	t.Setenv("OMI_MODEL", "")
	got := resolveTranscribeModel("", "")
	if got != "qwen3-asr-flash" {
		t.Fatalf("default=%q", got)
	}
}

func TestResolveTranscribeModel_Precedence(t *testing.T) {
	t.Setenv("OMI_MODEL", "env-model")
	if got := resolveTranscribeModel("flag-model", "cfg-model"); got != "flag-model" {
		t.Fatalf("flag precedence=%q", got)
	}
	if got := resolveTranscribeModel("", "cfg-model"); got != "env-model" {
		t.Fatalf("env precedence=%q", got)
	}
	t.Setenv("OMI_MODEL", "")
	if got := resolveTranscribeModel("", "cfg-model"); got != "cfg-model" {
		t.Fatalf("cfg precedence=%q", got)
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
