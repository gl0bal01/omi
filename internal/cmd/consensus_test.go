package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gl0bal01/omi/internal/api"
)

func TestParseConsensusModels(t *testing.T) {
	got, err := parseConsensusModels(" mini, claude,mini,gemini-pro ")
	if err != nil {
		t.Fatalf("parseConsensusModels: %v", err)
	}
	want := []string{"mini", "claude", "gemini-pro"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}

	if _, err := parseConsensusModels("mini"); err == nil {
		t.Fatalf("want error for one model")
	}
}

func TestRunConsensus_QueriesPanelThenSynthesizes(t *testing.T) {
	var (
		mu          sync.Mutex
		gotModels   []string
		synthPrompt string
		synthModel  = "gpt-5.5"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat-with-ai" {
			http.NotFound(w, r)
			return
		}
		var body api.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		mu.Lock()
		gotModels = append(gotModels, body.Model)
		if body.Model == synthModel {
			synthPrompt = body.PromptObject.Prompt
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: content\n")
		_, _ = io.WriteString(w, fmt.Sprintf("data: {\"content\":\"answer from %s\"}\n\n", body.Model))
		_, _ = io.WriteString(w, "event: done\n\n")
	}))
	defer srv.Close()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	result, err := runConsensus(context.Background(), client, []string{"mini", "claude", "gemini-pro"}, "best", "ship it?")
	if err != nil {
		t.Fatalf("runConsensus: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), gotModels...)
	prompt := synthPrompt
	mu.Unlock()
	sort.Strings(got)
	wantModels := []string{"claude-sonnet-5", "gemini-3.1-pro-preview", "gpt-4o-mini", "gpt-5.5"}
	if fmt.Sprint(got) != fmt.Sprint(wantModels) {
		t.Fatalf("models=%v want=%v", got, wantModels)
	}
	if len(result.Answers) != 3 {
		t.Fatalf("answers=%d want 3", len(result.Answers))
	}
	if result.Answers[0].Model != "mini" || result.Answers[1].Model != "claude" || result.Answers[2].Model != "gemini-pro" {
		t.Fatalf("answer order not preserved: %+v", result.Answers)
	}
	for _, want := range []string{"ship it?", "[mini / gpt-4o-mini]", "[claude / claude-sonnet-5]", "Recommendation:"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("synth prompt missing %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(result.Synth.Answer, "answer from gpt-5.5") {
		t.Fatalf("synth=%q", result.Synth.Answer)
	}
}

func TestBuildConsensusPromptSections(t *testing.T) {
	prompt := buildConsensusPrompt("q?", []consensusAnswer{
		{Model: "a", APIID: "api-a", Answer: "one"},
		{Model: "b", APIID: "api-b", Answer: "two"},
	})
	for _, s := range []string{"Original question:", "Model answers:", "Consensus:", "Disagreements:", "Recommendation:"} {
		if !strings.Contains(prompt, s) {
			t.Fatalf("missing %q in %s", s, prompt)
		}
	}
}
