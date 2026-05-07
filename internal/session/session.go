// Package session persists named conversation UUIDs to disk.
//
// File format: {"sessions": {"<name>": "<uuid>"}}
// Location: $XDG_CONFIG_HOME/omi/sessions.json or ~/.config/omi/sessions.json.
package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store is an in-memory map of session name → conversation UUID, with
// load/save against a JSON file. Callers must call Save() to persist mutations.
type Store struct {
	path     string
	Sessions map[string]string `json:"sessions"`
}

// Path returns the default sessions file path.
func Path() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "omi", "sessions.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omi", "sessions.json"), nil
}

// New loads the store from the default path. Missing file = empty store.
func New() (*Store, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return NewAt(p)
}

// NewAt loads the store from a given path. Used by tests.
func NewAt(path string) (*Store, error) {
	s := &Store{path: path, Sessions: map[string]string{}}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the CLI session store path or an explicit test path.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	tmp := struct {
		Sessions map[string]string `json:"sessions"`
	}{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return nil, err
	}
	if tmp.Sessions != nil {
		s.Sessions = tmp.Sessions
	}
	return s, nil
}

// Get returns the UUID for a name.
func (s *Store) Get(name string) (string, bool) {
	uuid, ok := s.Sessions[name]
	return uuid, ok
}

// Set assigns a UUID to a name in memory only. Caller must Save().
func (s *Store) Set(name, uuid string) {
	s.Sessions[name] = uuid
}

// Delete removes name from memory. Returns true if the key existed.
func (s *Store) Delete(name string) bool {
	if _, ok := s.Sessions[name]; !ok {
		return false
	}
	delete(s.Sessions, name)
	return true
}

// List returns a copy of the session map.
func (s *Store) List() map[string]string {
	out := make(map[string]string, len(s.Sessions))
	for k, v := range s.Sessions {
		out[k] = v
	}
	return out
}

// Clear empties the in-memory map. Caller must Save().
func (s *Store) Clear() {
	s.Sessions = map[string]string{}
}

// Save writes the in-memory state to disk with restrictive permissions.
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	wrap := struct {
		Sessions map[string]string `json:"sessions"`
	}{Sessions: s.Sessions}
	data, err := json.MarshalIndent(wrap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(s.path, 0o600)
}
