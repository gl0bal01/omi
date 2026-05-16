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

func TestTranscribe_RequestShape(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotKey    string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.Header.Get("API-KEY")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"aiRecord":{"aiRecordDetail":{"resultObject":"hello world"}}}`)
	}))
	defer srv.Close()

	c := NewClient("testkey", srv.URL, 5*time.Second)
	text, err := c.Transcribe(context.Background(), "forevervoiceless/audio.mp3", "whisper-1")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method=%s", gotMethod)
	}
	if gotPath != "/api/features" {
		t.Errorf("path=%s", gotPath)
	}
	if gotKey != "testkey" {
		t.Errorf("API-KEY=%q", gotKey)
	}
	if gotBody["type"] != "SPEECH_TO_TEXT" {
		t.Errorf("type=%v", gotBody["type"])
	}
	if gotBody["model"] != "whisper-1" {
		t.Errorf("model=%v", gotBody["model"])
	}
	po, ok := gotBody["promptObject"].(map[string]any)
	if !ok {
		t.Fatalf("promptObject missing or wrong type: %T", gotBody["promptObject"])
	}
	if po["audioUrl"] != "forevervoiceless/audio.mp3" {
		t.Errorf("audioUrl=%v", po["audioUrl"])
	}
	if po["response_format"] != "text" {
		t.Errorf("response_format=%v", po["response_format"])
	}
	if text != "hello world" {
		t.Errorf("text=%q", text)
	}
}

func TestTranscribe_ResultArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"aiRecord":{"aiRecordDetail":{"resultObject":["array form"]}}}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	text, err := c.Transcribe(context.Background(), "p/a.mp3", "whisper-1")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "array form" {
		t.Errorf("text=%q", text)
	}
}

func TestTranscribe_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	_, err := c.Transcribe(context.Background(), "p", "m")
	if err == nil {
		t.Fatalf("want error")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != 401 {
		t.Fatalf("err=%v", err)
	}
}

func TestTranscribe_FallbackPromptObjectKeys(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		po, _ := body["promptObject"].(map[string]any)

		switch calls {
		case 1:
			if _, ok := po["audioUrl"]; !ok {
				t.Fatalf("call1 promptObject=%v, want audioUrl", po)
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "bad field")
		case 2:
			if _, ok := po["audio"]; !ok {
				t.Fatalf("call2 promptObject=%v, want audio", po)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":{"response":"ok via audio"}}`)
		default:
			t.Fatalf("unexpected extra call #%d; cascade should stop after 2 attempts", calls)
		}
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	got, err := c.Transcribe(context.Background(), "p/a.mp3", "whisper-1")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "ok via audio" {
		t.Fatalf("text=%q", got)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
}
