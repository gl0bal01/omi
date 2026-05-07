package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")

	s, err := NewAt(p)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	s.Set("research", "uuid-1")
	s.Set("work", "uuid-2")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := NewAt(p)
	if err != nil {
		t.Fatalf("NewAt 2: %v", err)
	}
	if got, ok := s2.Get("research"); !ok || got != "uuid-1" {
		t.Errorf("Get research = %q ok=%v", got, ok)
	}
	if got, ok := s2.Get("work"); !ok || got != "uuid-2" {
		t.Errorf("Get work = %q ok=%v", got, ok)
	}
	if len(s2.List()) != 2 {
		t.Errorf("List len=%d want 2", len(s2.List()))
	}
}

func TestStore_MissingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing.json")
	s, err := NewAt(p)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	if len(s.List()) != 0 {
		t.Errorf("expected empty store")
	}
	if _, ok := s.Get("anything"); ok {
		t.Errorf("Get on empty store should not find")
	}
}

func TestStore_DeleteMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	s, _ := NewAt(p)
	if s.Delete("nope") {
		t.Errorf("Delete missing should return false")
	}
	s.Set("a", "u")
	if !s.Delete("a") {
		t.Errorf("Delete existing should return true")
	}
	if _, ok := s.Get("a"); ok {
		t.Errorf("Get after delete should be false")
	}
}

func TestStore_Clear(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	s, _ := NewAt(p)
	s.Set("a", "1")
	s.Set("b", "2")
	s.Clear()
	if len(s.List()) != 0 {
		t.Errorf("Clear should empty map, got %d entries", len(s.List()))
	}
}

func TestStore_ListIsCopy(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	s, _ := NewAt(p)
	s.Set("a", "1")
	cp := s.List()
	cp["a"] = "mutated"
	if got, _ := s.Get("a"); got != "1" {
		t.Errorf("List should return a copy, not aliased map")
	}
}

func TestStore_SavePermissions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "omi", "sessions.json")
	s, err := NewAt(p)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	s.Set("work", "uuid-1")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Fatalf("dir mode=%o want 0700", mode)
	}

	fileInfo, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode=%o want 0600", mode)
	}
}
