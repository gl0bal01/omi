package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/stream"
)

func TestChat_HistorySettingsCompatibilityKeys(t *testing.T) {
	req := ChatRequest{
		Type:  "UNIFY_CHAT_WITH_AI",
		Model: "gpt-4o-mini",
		PromptObject: PromptObject{
			Prompt: "hi",
			Settings: &PromptSettings{
				HistorySettings: &PromptHistorySettings{
					IsMixed:           true,
					HistoryMixed:      true,
					HistoryMixedSnake: true,
				},
			},
		},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var shape map[string]any
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	prompt, ok := shape["promptObject"].(map[string]any)
	if !ok {
		t.Fatalf("promptObject type=%T", shape["promptObject"])
	}
	settings, ok := prompt["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings type=%T", prompt["settings"])
	}
	history, ok := settings["historySettings"].(map[string]any)
	if !ok {
		t.Fatalf("historySettings type=%T", settings["historySettings"])
	}
	if _, ok := history["isMixed"]; !ok {
		t.Fatalf("missing isMixed in %v", history)
	}
	if _, ok := history["historyMixed"]; !ok {
		t.Fatalf("missing historyMixed in %v", history)
	}
	if _, ok := history["history_mixed"]; !ok {
		t.Fatalf("missing history_mixed in %v", history)
	}
}

func TestChat_RequestShape(t *testing.T) {
	var (
		gotPath   string
		gotQuery  string
		gotKey    string
		gotBody   ChatRequest
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("isStreaming")
		gotKey = r.Header.Get("API-KEY")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: content\ndata: hello\n\nevent: done\n\n")
	}))
	defer srv.Close()

	c := NewClient("testkey", srv.URL, 5*time.Second)
	body, err := c.Chat(context.Background(), &ChatRequest{
		Type:         "UNIFY_CHAT_WITH_AI",
		Model:        "gpt-4o-mini",
		PromptObject: PromptObject{Prompt: "hi"},
	})
	if err != nil {
		t.Fatalf("Chat err: %v", err)
	}
	defer closeutil.Quiet(body)

	if gotMethod != http.MethodPost {
		t.Errorf("method=%s", gotMethod)
	}
	if gotPath != "/api/chat-with-ai" {
		t.Errorf("path=%s", gotPath)
	}
	if gotQuery != "true" {
		t.Errorf("isStreaming=%q", gotQuery)
	}
	if gotKey != "testkey" {
		t.Errorf("API-KEY=%q", gotKey)
	}
	if gotBody.Type != "UNIFY_CHAT_WITH_AI" {
		t.Errorf("type=%q", gotBody.Type)
	}
	if gotBody.Model != "gpt-4o-mini" {
		t.Errorf("model=%q", gotBody.Model)
	}
	if gotBody.PromptObject.Prompt != "hi" {
		t.Errorf("prompt=%q", gotBody.PromptObject.Prompt)
	}
}

func TestChat_StreamConsumption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: content\ndata: hello\n\nevent: done\n\n")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	body, err := c.Chat(context.Background(), &ChatRequest{
		Type:         "UNIFY_CHAT_WITH_AI",
		Model:        "m",
		PromptObject: PromptObject{Prompt: "p"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	defer closeutil.Quiet(body)

	ch := make(chan stream.Event, 8)
	errCh := make(chan error, 1)
	go func() { errCh <- stream.Parse(body, ch) }()

	var events []stream.Event
	for ev := range ch {
		events = append(events, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d, want 2", len(events))
	}
	if events[0].Type != "content" || events[0].Data != "hello" {
		t.Fatalf("ev0=%+v", events[0])
	}
	if events[1].Type != "done" {
		t.Fatalf("ev1.type=%q", events[1].Type)
	}
}

func TestChat_VisionRequest(t *testing.T) {
	var (
		gotPath  string
		gotQuery string
		gotBody  ChatRequest
		gotRaw   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("isStreaming")
		buf, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(buf, &gotBody)
		_ = json.Unmarshal(buf, &gotRaw)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: content\ndata: ok\n\nevent: done\n\n")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	body, err := c.Chat(context.Background(), &ChatRequest{
		Type:         "UNIFY_CHAT_WITH_AI",
		Model:        "gpt-4o",
		PromptObject: PromptObject{Prompt: "what is this?"},
		ImageList:    []string{"forevervoiceless/photo.jpg"},
	})
	if err != nil {
		t.Fatalf("Chat err: %v", err)
	}
	defer closeutil.Quiet(body)

	if gotPath != "/api/chat-with-ai" {
		t.Errorf("path=%s", gotPath)
	}
	if gotQuery != "true" {
		t.Errorf("isStreaming=%q", gotQuery)
	}
	if gotBody.Type != "UNIFY_CHAT_WITH_AI" {
		t.Errorf("type=%q", gotBody.Type)
	}
	if len(gotBody.ImageList) != 1 || gotBody.ImageList[0] != "forevervoiceless/photo.jpg" {
		t.Errorf("imageList=%v", gotBody.ImageList)
	}
	if _, hasFiles := gotRaw["files"]; hasFiles {
		t.Errorf("files key should be omitted, got %v", gotRaw["files"])
	}
}

func TestChat_DocRequest(t *testing.T) {
	var (
		gotPath string
		gotBody ChatRequest
		gotRaw  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(buf, &gotBody)
		_ = json.Unmarshal(buf, &gotRaw)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: content\ndata: ok\n\nevent: done\n\n")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	body, err := c.Chat(context.Background(), &ChatRequest{
		Type:         "UNIFY_CHAT_WITH_AI",
		Model:        "gpt-4o",
		PromptObject: PromptObject{Prompt: "summarize"},
		Files:        []string{"forevervoiceless/doc.pdf"},
	})
	if err != nil {
		t.Fatalf("Chat err: %v", err)
	}
	defer closeutil.Quiet(body)

	if gotPath != "/api/chat-with-ai" {
		t.Errorf("path=%s", gotPath)
	}
	if gotBody.Type != "UNIFY_CHAT_WITH_AI" {
		t.Errorf("type=%q", gotBody.Type)
	}
	if len(gotBody.Files) != 1 || gotBody.Files[0] != "forevervoiceless/doc.pdf" {
		t.Errorf("files=%v", gotBody.Files)
	}
	if _, hasImage := gotRaw["imageList"]; hasImage {
		t.Errorf("imageList key should be omitted, got %v", gotRaw["imageList"])
	}
}

func TestChat_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, "bad request")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	body, err := c.Chat(context.Background(), &ChatRequest{
		Type:         "UNIFY_CHAT_WITH_AI",
		Model:        "m",
		PromptObject: PromptObject{Prompt: "p"},
	})
	if body != nil {
		_ = body.Close()
	}
	if err == nil {
		t.Fatalf("want error")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err type=%T", err)
	}
	if apiErr.Status != 400 {
		t.Fatalf("status=%d", apiErr.Status)
	}
	if apiErr.Message != "bad request" {
		t.Fatalf("message=%q", apiErr.Message)
	}
}
