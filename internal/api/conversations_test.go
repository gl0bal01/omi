package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateConversation_RequestShape(t *testing.T) {
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
		_, _ = io.WriteString(w, `{"conversation":{"uuid":"abc-123"}}`)
	}))
	defer srv.Close()

	c := NewClient("testkey", srv.URL, 5*time.Second)
	uuid, err := c.CreateConversation(context.Background(), "omi session: research", ConversationTypeUnify, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if uuid != "abc-123" {
		t.Errorf("uuid=%q want abc-123", uuid)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method=%s", gotMethod)
	}
	if gotPath != "/api/conversations" {
		t.Errorf("path=%s", gotPath)
	}
	if gotKey != "testkey" {
		t.Errorf("API-KEY=%q", gotKey)
	}
	if gotBody["title"] != "omi session: research" {
		t.Errorf("title=%v", gotBody["title"])
	}
	if gotBody["type"] != ConversationTypeUnify {
		t.Errorf("type=%v", gotBody["type"])
	}
	if gotBody["model"] != "gpt-4o-mini" {
		t.Errorf("model=%v", gotBody["model"])
	}
}

func TestCreateConversation_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, "bad")
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	_, err := c.CreateConversation(context.Background(), "t", ConversationTypeUnify, "m")
	if err == nil {
		t.Fatalf("want error")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != 400 {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateConversation_FallbackTypeOnValidationFailure(t *testing.T) {
	var calls int32
	var seenTypes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var gotBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		seenTypes = append(seenTypes, gotBody["type"].(string))
		if len(seenTypes) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"validation failed"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"conversation":{"uuid":"abc-fallback"}}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	uuid, err := c.CreateConversation(context.Background(), "t", ConversationTypeChat, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if uuid != "abc-fallback" {
		t.Fatalf("uuid=%q", uuid)
	}
	if len(seenTypes) != 2 || seenTypes[0] != ConversationTypeChat || seenTypes[1] != ConversationTypeUnify {
		t.Fatalf("seenTypes=%v", seenTypes)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls=%d", got)
	}
}

func TestDeleteConversation_StatusMatrix(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantNil    bool
		wantStatus int
	}{
		{"200 ok", 200, true, 0},
		{"204 no content", 204, true, 0},
		{"404 already deleted", 404, true, 0},
		{"401 unauthorized", 401, false, 401},
		{"500 server error", 500, false, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			var gotMethod, gotPath, gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotKey = r.Header.Get("API-KEY")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			c := NewClient("testkey", srv.URL, 5*time.Second)
			err := c.DeleteConversation(context.Background(), "uuid-xyz")

			if tc.wantNil {
				if err != nil {
					t.Fatalf("err=%v want nil", err)
				}
			} else {
				if err == nil {
					t.Fatalf("want error")
				}
				apiErr, ok := err.(*Error)
				if !ok {
					t.Fatalf("err type=%T want *Error", err)
				}
				if apiErr.Status != tc.wantStatus {
					t.Errorf("status=%d want %d", apiErr.Status, tc.wantStatus)
				}
			}

			if got := calls.Load(); got != 1 {
				t.Errorf("calls=%d want 1 (no retry)", got)
			}
			if gotMethod != http.MethodDelete {
				t.Errorf("method=%s", gotMethod)
			}
			if !strings.HasSuffix(gotPath, "/api/conversations/uuid-xyz") {
				t.Errorf("path=%s", gotPath)
			}
			if gotKey != "testkey" {
				t.Errorf("API-KEY=%q", gotKey)
			}
		})
	}
}

func TestDeleteConversation_PathExact(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	if err := c.DeleteConversation(context.Background(), "deadbeef"); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if gotPath != "/api/conversations/deadbeef" {
		t.Errorf("path=%q", gotPath)
	}
}
