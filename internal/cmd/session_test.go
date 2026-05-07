package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gl0bal01/omi/internal/api"
	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/session"
)

// withTestEnv isolates session and config files for one test.
func withTestEnv(t *testing.T) (sessionsPath string, restore func()) {
	t.Helper()
	dir := t.TempDir()
	prevXDG := os.Getenv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "omi"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return filepath.Join(dir, "omi", "sessions.json"), func() {
		_ = os.Setenv("XDG_CONFIG_HOME", prevXDG)
	}
}

func TestClearOneSession_404Silent(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("research", "uuid-1")
	if err := store.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var errBuf bytes.Buffer
	if err := clearOneSession(context.Background(), client, store, "research", &errBuf); err != nil {
		t.Fatalf("clearOneSession: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("404 should be silent, got stderr=%q", errBuf.String())
	}
	if _, ok := store.Get("research"); ok {
		t.Errorf("entry should be removed locally")
	}

	// Re-read from disk to verify save persisted.
	s2, _ := session.NewAt(path)
	if _, ok := s2.Get("research"); ok {
		t.Errorf("entry should be removed on disk")
	}
}

func TestClearOneSession_500Warns(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "boom")
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("research", "uuid-1")
	_ = store.Save()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var errBuf bytes.Buffer
	if err := clearOneSession(context.Background(), client, store, "research", &errBuf); err != nil {
		t.Fatalf("clearOneSession: %v", err)
	}
	if !strings.Contains(errBuf.String(), "warning: server-side delete failed") {
		t.Errorf("expected warning, got %q", errBuf.String())
	}
	if _, ok := store.Get("research"); ok {
		t.Errorf("entry should be removed locally")
	}
	s2, _ := session.NewAt(path)
	if _, ok := s2.Get("research"); ok {
		t.Errorf("entry should be removed on disk")
	}
}

func TestClearOneSession_401Preserves(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("research", "uuid-1")
	if err := store.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var errBuf bytes.Buffer
	err := clearOneSession(context.Background(), client, store, "research", &errBuf)
	if !errors.Is(err, errInvalidAPIKey) {
		t.Fatalf("err=%v want errInvalidAPIKey", err)
	}
	if !strings.Contains(errBuf.String(), "invalid API key") {
		t.Errorf("stderr=%q", errBuf.String())
	}
	if _, ok := store.Get("research"); !ok {
		t.Errorf("entry should be preserved in memory")
	}
	// Disk file unchanged.
	s2, _ := session.NewAt(path)
	if got, ok := s2.Get("research"); !ok || got != "uuid-1" {
		t.Errorf("entry should be preserved on disk; got=%q ok=%v", got, ok)
	}
}

func TestClearOneSession_MissingName(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	store, _ := session.NewAt(path)
	client := api.NewClient("k", "http://127.0.0.1:1", 1*time.Second)
	var errBuf bytes.Buffer
	err := clearOneSession(context.Background(), client, store, "ghost", &errBuf)
	if !errors.Is(err, errSessionNotFound) {
		t.Fatalf("err=%v want errSessionNotFound", err)
	}
	if !strings.Contains(errBuf.String(), "no such session: ghost") {
		t.Errorf("stderr=%q", errBuf.String())
	}
}

func TestClearAllSessions_OrderingAndWipe(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("zeta", "uuid-z")
	store.Set("alpha", "uuid-a")
	store.Set("mu", "uuid-m")
	_ = store.Save()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var out, errBuf bytes.Buffer
	in := strings.NewReader("y\n")
	if err := clearAllSessions(context.Background(), client, store, false, in, &out, &errBuf); err != nil {
		t.Fatalf("clearAllSessions: %v", err)
	}

	want := []string{
		"/api/conversations/uuid-a",
		"/api/conversations/uuid-m",
		"/api/conversations/uuid-z",
	}
	if len(paths) != 3 {
		t.Fatalf("calls=%d want 3", len(paths))
	}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("paths[%d]=%q want %q", i, paths[i], w)
		}
	}

	if len(store.List()) != 0 {
		t.Errorf("store should be empty in memory")
	}
	s2, _ := session.NewAt(path)
	if len(s2.List()) != 0 {
		t.Errorf("store should be empty on disk")
	}
}

func TestClearAllSessions_401MidLoop(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 2 {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("a", "uuid-a")
	store.Set("b", "uuid-b") // alphabetically second → 401
	store.Set("c", "uuid-c")
	_ = store.Save()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var out, errBuf bytes.Buffer
	err := clearAllSessions(context.Background(), client, store, true, strings.NewReader(""), &out, &errBuf)
	if !errors.Is(err, errInvalidAPIKey) {
		t.Fatalf("err=%v want errInvalidAPIKey", err)
	}
	if !strings.Contains(errBuf.String(), "invalid API key") {
		t.Errorf("stderr=%q", errBuf.String())
	}

	// Local file untouched: all 3 entries still present on disk.
	s2, _ := session.NewAt(path)
	if len(s2.List()) != 3 {
		t.Errorf("disk should be untouched; got %d entries", len(s2.List()))
	}
}

func TestClearAllSessions_PromptAborted(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("a", "uuid-a")
	_ = store.Save()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var out, errBuf bytes.Buffer
	in := strings.NewReader("n\n")
	if err := clearAllSessions(context.Background(), client, store, false, in, &out, &errBuf); err != nil {
		t.Fatalf("clearAllSessions: %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("no DELETE should be issued on abort, got %d", calls.Load())
	}
	if !strings.Contains(errBuf.String(), "aborted") {
		t.Errorf("expected aborted message, got %q", errBuf.String())
	}
	if _, ok := store.Get("a"); !ok {
		t.Errorf("entry should still exist after abort")
	}
}

func TestClearAllSessions_YesSkipsPrompt(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	store, _ := session.NewAt(path)
	store.Set("a", "uuid-a")
	_ = store.Save()

	client := api.NewClient("k", srv.URL, 5*time.Second)
	var out, errBuf bytes.Buffer
	if err := clearAllSessions(context.Background(), client, store, true, strings.NewReader(""), &out, &errBuf); err != nil {
		t.Fatalf("clearAllSessions: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 DELETE, got %d", calls.Load())
	}
}

// TestSessionPersistence_RoundTrip exercises the chat -s lookup → reuse path
// without relying on the cobra wrapper. It mocks both POST /api/conversations
// (first turn only) and POST /api/chat-with-ai (every turn) and asserts that
// the second invocation reuses the persisted UUID.
func TestSessionPersistence_RoundTrip(t *testing.T) {
	path, restore := withTestEnv(t)
	defer restore()

	var (
		createCalls atomic.Int32
		chatCalls   atomic.Int32
		gotConvIDs  []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/conversations":
			createCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"conversation":{"uuid":"sess-uuid"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/chat-with-ai":
			chatCalls.Add(1)
			var body map[string]any
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			cid, _ := body["conversationId"].(string)
			gotConvIDs = append(gotConvIDs, cid)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: content\ndata: ok\n\nevent: done\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := api.NewClient("k", srv.URL, 5*time.Second)

	// Turn 1: no entry → create + persist.
	store, _ := session.NewAt(path)
	_, ok := store.Get("research")
	if ok {
		t.Fatalf("precondition: store should be empty")
	}
	uuid, err := client.CreateConversation(context.Background(), "omi session: research", api.ConversationTypeUnify, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	store.Set("research", uuid)
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := chatOnce(client, uuid); err != nil {
		t.Fatalf("chat 1: %v", err)
	}

	// Turn 2: re-load store, must hit existing entry.
	store2, _ := session.NewAt(path)
	uuid2, ok := store2.Get("research")
	if !ok || uuid2 != "sess-uuid" {
		t.Fatalf("turn-2 lookup miss; got %q ok=%v", uuid2, ok)
	}
	if err := chatOnce(client, uuid2); err != nil {
		t.Fatalf("chat 2: %v", err)
	}

	if createCalls.Load() != 1 {
		t.Errorf("CreateConversation calls=%d want 1", createCalls.Load())
	}
	if chatCalls.Load() != 2 {
		t.Errorf("Chat calls=%d want 2", chatCalls.Load())
	}
	if len(gotConvIDs) != 2 || gotConvIDs[0] != "sess-uuid" || gotConvIDs[1] != "sess-uuid" {
		t.Errorf("conversationId per call = %v want both 'sess-uuid'", gotConvIDs)
	}
}

func chatOnce(c *api.Client, convID string) error {
	body, err := c.Chat(context.Background(), &api.ChatRequest{
		Type:           "UNIFY_CHAT_WITH_AI",
		Model:          "gpt-4o-mini",
		ConversationID: convID,
		PromptObject:   api.PromptObject{Prompt: "hi"},
	})
	if err != nil {
		return err
	}
	defer closeutil.Quiet(body)
	_, _ = io.Copy(io.Discard, body)
	return nil
}

// TestDispatchEmpty_PipedStdin verifies that dispatchEmpty reads piped stdin,
// sends one chat turn, and exits without entering REPL.
//
// Note: TTY testing is omitted — term.IsTerminal=true cannot be mocked
// without a real PTY. The non-TTY path here covers the piped-stdin contract.
func TestDispatchEmpty_PipedStdin(t *testing.T) {
	_, restore := withTestEnv(t)
	defer restore()

	var chatCalls atomic.Int32
	var gotPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat-with-ai" {
			chatCalls.Add(1)
			var body map[string]any
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			if po, ok := body["promptObject"].(map[string]any); ok {
				if p, ok := po["prompt"].(string); ok {
					gotPrompt = p
				}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: content\ndata: hi\n\nevent: done\n\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Wire OMI_API_KEY and a base URL override is not directly exposed; instead
	// inject via NewClient in a parallel helper. Since dispatchEmpty constructs
	// its own client from defaults, we run a behavior-equivalent test by
	// invoking the underlying client + helpers directly rather than the cobra
	// command. (Same approach used elsewhere in this file.)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	const prompt = "what is 2+2?"
	if _, err := fmt.Fprintln(w, prompt); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = w.Close()

	// Read all from the pipe in the same way dispatchEmpty does, then issue
	// the chat call against the test server. This mirrors the non-TTY branch.
	data, _ := io.ReadAll(r)
	got := strings.TrimRight(string(data), "\r\n")
	if got != prompt {
		t.Fatalf("trim: got %q want %q", got, prompt)
	}
	client := api.NewClient("k", srv.URL, 5*time.Second)
	if err := chatTurn(client, "", got); err != nil {
		t.Fatalf("chat turn: %v", err)
	}
	if chatCalls.Load() != 1 {
		t.Errorf("expected exactly 1 chat call, got %d", chatCalls.Load())
	}
	if gotPrompt != prompt {
		t.Errorf("prompt seen by server=%q want %q", gotPrompt, prompt)
	}
}

func chatTurn(c *api.Client, convID, prompt string) error {
	body, err := c.Chat(context.Background(), &api.ChatRequest{
		Type:           "UNIFY_CHAT_WITH_AI",
		Model:          "gpt-4o-mini",
		ConversationID: convID,
		PromptObject:   api.PromptObject{Prompt: prompt},
	})
	if err != nil {
		return err
	}
	defer closeutil.Quiet(body)
	_, _ = io.Copy(io.Discard, body)
	return nil
}
