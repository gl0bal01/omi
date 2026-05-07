package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCode_RequestShape(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotKey    string
		gotBody   map[string]any
		gotQuery  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("isStreaming")
		gotKey = r.Header.Get("API-KEY")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"aiRecord":{"aiRecordDetail":{"resultObject":"package main\nfunc main(){}"}}}`)
	}))
	defer srv.Close()

	c := NewClient("testkey", srv.URL, 5*time.Second)
	text, err := c.Code(context.Background(), "gpt-5.1-codex", "fizzbuzz in go")
	if err != nil {
		t.Fatalf("Code: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method=%s", gotMethod)
	}
	if gotPath != "/api/features" {
		t.Errorf("path=%s", gotPath)
	}
	if gotQuery != "" {
		t.Errorf("isStreaming should be absent, got %q", gotQuery)
	}
	if gotKey != "testkey" {
		t.Errorf("API-KEY=%q", gotKey)
	}
	if gotBody["type"] != "CODE_GENERATOR" {
		t.Errorf("type=%v", gotBody["type"])
	}
	if gotBody["model"] != "gpt-5.1-codex" {
		t.Errorf("model=%v", gotBody["model"])
	}
	po, ok := gotBody["promptObject"].(map[string]any)
	if !ok {
		t.Fatalf("promptObject missing or wrong type: %T", gotBody["promptObject"])
	}
	if po["prompt"] != "fizzbuzz in go" {
		t.Errorf("prompt=%v", po["prompt"])
	}
	if text != "package main\nfunc main(){}" {
		t.Errorf("text=%q", text)
	}
}

func TestCode_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	_, err := c.Code(context.Background(), "m", "p")
	if err == nil {
		t.Fatalf("want error")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != 401 {
		t.Fatalf("err=%v", err)
	}
}
