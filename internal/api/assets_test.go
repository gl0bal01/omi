package api

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadAsset_Multipart(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "hello.png")
	fixtureBytes := []byte("PNG-FIXTURE-BYTES-\x00\x01\x02")
	if err := os.WriteFile(fixturePath, fixtureBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var (
		gotMethod      string
		gotPath        string
		gotKey         string
		gotContentType string
		gotFieldName   string
		gotFileName    string
		gotFileBytes   []byte
		gotBoundary    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.Header.Get("API-KEY")
		gotContentType = r.Header.Get("Content-Type")

		mediaType, params, err := mime.ParseMediaType(gotContentType)
		if err != nil || mediaType != "multipart/form-data" {
			t.Errorf("content-type=%q parse err=%v media=%q", gotContentType, err, mediaType)
		}
		gotBoundary = params["boundary"]

		mr := multipart.NewReader(r.Body, gotBoundary)
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			gotFieldName = part.FormName()
			gotFileName = part.FileName()
			b, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read part body: %v", err)
			}
			gotFileBytes = b
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"fileContent":{"path":"forevervoiceless/abc.png"}}`)
	}))
	defer srv.Close()

	c := NewClient("testkey", srv.URL, 5*time.Second)
	got, err := c.UploadAsset(context.Background(), fixturePath)
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method=%s", gotMethod)
	}
	if gotPath != "/api/assets" {
		t.Errorf("path=%s", gotPath)
	}
	if gotKey != "testkey" {
		t.Errorf("API-KEY=%q", gotKey)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Errorf("content-type=%q", gotContentType)
	}
	if gotBoundary == "" {
		t.Errorf("boundary empty")
	}
	if gotFieldName != "asset" {
		t.Errorf("field name=%q want asset", gotFieldName)
	}
	if gotFileName != "hello.png" {
		t.Errorf("file name=%q", gotFileName)
	}
	if string(gotFileBytes) != string(fixtureBytes) {
		t.Errorf("file bytes mismatch: got %q", gotFileBytes)
	}
	if got != "forevervoiceless/abc.png" {
		t.Errorf("assetPath=%q", got)
	}
}

func TestUploadAsset_FallbackAssetPath(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(fixturePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"assetPath":"users/123/file.txt"}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	got, err := c.UploadAsset(context.Background(), fixturePath)
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if got != "users/123/file.txt" {
		t.Errorf("assetPath=%q", got)
	}
}

func TestUploadAsset_MissingPath(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(fixturePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"unrelated":true}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	_, err := c.UploadAsset(context.Background(), fixturePath)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "missing asset path") {
		t.Errorf("err=%v", err)
	}
}

func TestUploadAsset_FallbackFieldNameToFile(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(fixturePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("content-type parse: %v media=%q", err, mediaType)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		seen = append(seen, part.FormName())
		_, _ = io.Copy(io.Discard, part)

		if len(seen) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad field"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"path":"users/123/file.txt"}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	got, err := c.UploadAsset(context.Background(), fixturePath)
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if got != "users/123/file.txt" {
		t.Fatalf("assetPath=%q", got)
	}
	if len(seen) != 2 || seen[0] != "asset" || seen[1] != "file" {
		t.Fatalf("field sequence=%v, want [asset file]", seen)
	}
}

func TestUploadAsset_ResultPath(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(fixturePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"path":"users/123/result.txt"}}`)
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second)
	got, err := c.UploadAsset(context.Background(), fixturePath)
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if got != "users/123/result.txt" {
		t.Fatalf("assetPath=%q", got)
	}
}
