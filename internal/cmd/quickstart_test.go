package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuickstartCommand_Output(t *testing.T) {
	root := newRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"quickstart"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	for _, s := range []string{
		"omi quickstart",
		"General chat",
		"Code generation",
		"3-model consensus (omi client-side feature)",
		"./bin/omi consensus \"Should we ship this?\"",
		"Sessions and mixed history",
		"./bin/omi -s research \"Remember this context\"",
		"./bin/omi --mixed \"Compare this with prior context\"",
		"./bin/omi -s research --mixed \"Continue with mixed history\"",
		"./bin/omi session list",
		"Transcription",
		"doctor --live",
	} {
		if !strings.Contains(got, s) {
			t.Fatalf("missing %q in output: %s", s, got)
		}
	}
}
