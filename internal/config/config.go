package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

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
	data, err := os.ReadFile(p) // #nosec G304 -- path is the XDG/user config file chosen by the CLI runtime.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	return c, nil
}

// MaskAPIKey returns a redacted form of the API key suitable for display.
// Empty input returns empty; keys shorter than 7 chars return "***".
// Otherwise returns the first 3 chars + "..." + last 4 chars.
func MaskAPIKey(key string) string {
	if len(key) < 7 {
		if key == "" {
			return ""
		}
		return "***"
	}
	return key[:3] + "..." + key[len(key)-4:]
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
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(p, 0o600)
}
