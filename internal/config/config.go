package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gl0bal01/omi/internal/closeutil"
	"github.com/gl0bal01/omi/internal/redact"
)

// maxConfigBytes bounds the config file read so a malformed or hostile file
// cannot exhaust memory during JSON parsing. 1 MiB is far beyond any real
// config.
const maxConfigBytes = 1 << 20

type Config struct {
	APIKey    string `json:"api_key,omitempty"`
	Model     string `json:"model,omitempty"`
	CodeModel string `json:"code_model,omitempty"`
	Web       bool   `json:"web,omitempty"`
	Mix       bool   `json:"mix,omitempty"`
	NumSites  int    `json:"num_sites,omitempty"`
	MaxWords  int    `json:"max_words,omitempty"`
}

func Path() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "omi", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omi", "config.json"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) // #nosec G304 -- path is the XDG/user config file chosen by the CLI runtime.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	defer closeutil.Quiet(f)
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("omi: config file %s exceeds %d bytes", p, maxConfigBytes)
	}
	c := &Config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	return c, nil
}

// MaskAPIKey returns a display-safe form of an API key. See redact.APIKey.
func MaskAPIKey(key string) string {
	return redact.APIKey(key)
}

func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ") // #nosec G117 -- api_key persistence is explicit `omi config set api_key`; file mode is forced to 0600.
	if err != nil {
		return err
	}
	return atomicWrite0600(dir, p, data)
}

// atomicWrite0600 writes data to a fresh 0600 temp file in dir, then atomically
// renames it over dest. This avoids the truncate-then-chmod window of
// os.WriteFile and never writes through a pre-planted symlink at dest (rename
// replaces the symlink itself, not its target).
func atomicWrite0600(dir, dest string, data []byte) error {
	f, err := os.CreateTemp(dir, ".omi-*.tmp") // CreateTemp uses O_EXCL and 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op once renamed
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
