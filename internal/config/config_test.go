package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setupTempXDG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	setupTempXDG(t)

	in := &Config{
		APIKey:    "sk-abcd1234",
		Model:     "mini",
		CodeModel: "codex",
		Web:       true,
		Mix:       false,
		NumSites:  5,
		MaxWords:  500,
	}
	if err := in.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if *out != *in {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", out, in)
	}
}

func TestSave_FilePerm0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits not meaningful on Windows")
	}
	dir := setupTempXDG(t)
	c := &Config{APIKey: "sk-test"}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	p := filepath.Join(dir, "omi", "config.json")
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("file perm = %o, want 0600", mode)
	}
}

func TestSave_DirPerm0700_OnFirstUse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory mode bits not meaningful on Windows")
	}
	dir := setupTempXDG(t)
	// Pre-condition: omi/ does not exist yet.
	omiDir := filepath.Join(dir, "omi")
	if _, err := os.Stat(omiDir); !os.IsNotExist(err) {
		t.Fatalf("expected omi/ to be absent before Save()")
	}

	c := &Config{APIKey: "sk-test"}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	st, err := os.Stat(omiDir)
	if err != nil {
		t.Fatalf("stat omi/: %v", err)
	}
	if !st.IsDir() {
		t.Fatalf("omi/ is not a directory")
	}
	if mode := st.Mode().Perm(); mode != 0o700 {
		t.Errorf("dir perm = %o, want 0700", mode)
	}
}

func TestLoad_MissingFile_ReturnsZero(t *testing.T) {
	setupTempXDG(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if *c != (Config{}) {
		t.Errorf("expected zero-value config, got %+v", c)
	}
}

func TestMaskAPIKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcdef", "***"},             // 6 chars: short
		{"abcdefg", "***"},            // 7 chars: below threshold, fully masked
		{"abcdefghij", "***"},         // 10 chars: below threshold
		{"sk-abc12345", "sk-...2345"}, // 11 chars: exactly threshold
		{"sk-abcd1234", "sk-...1234"},
	}
	for _, tc := range cases {
		got := MaskAPIKey(tc.in)
		if got != tc.want {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
