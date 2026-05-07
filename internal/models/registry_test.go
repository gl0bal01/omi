package models

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// resetForTest replaces the loadOnce state so a test can reload registry
// from the embedded source or a custom path.
func resetForTest(t *testing.T) {
	t.Helper()
	loadOnce = sync.Once{}
	registry = nil
}

func TestResolve_Embedded_AliasHits(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)

	cases := []struct {
		alias string
		api   string
	}{
		{"mini", "gpt-4o-mini"},
		{"codex", "gpt-5.1-codex"},
		{"pixtral", "pixtral-12b"},
		{"sonar", "sonar-pro"},
	}
	for _, tc := range cases {
		var required Capability
		switch tc.alias {
		case "codex":
			required = CapCode
		case "pixtral":
			required = CapVision
		}
		got, _, err := Resolve(tc.alias, required)
		if err != nil {
			t.Errorf("Resolve(%q): unexpected err: %v", tc.alias, err)
		}
		if got != tc.api {
			t.Errorf("Resolve(%q) = %q, want %q", tc.alias, got, tc.api)
		}
	}
}

func TestResolve_RawPassthroughEmitsExactWarning(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	var buf bytes.Buffer
	stderr = &buf
	defer func() { stderr = os.Stderr }()

	got, defs, err := Resolve("custom-raw-id", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "custom-raw-id" {
		t.Errorf("model=%q", got)
	}
	if defs != (ModelDefaults{}) {
		t.Errorf("defs not empty: %+v", defs)
	}
	want := "omi: warning: 'custom-raw-id' is not a known alias; treating as raw model ID. Run 'omi models' to see known aliases.\n"
	if buf.String() != want {
		t.Errorf("stderr=%q\nwant %q", buf.String(), want)
	}
}

func TestResolve_KnownAPIID_NoWarning(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	var buf bytes.Buffer
	stderr = &buf
	defer func() { stderr = os.Stderr }()

	got, defs, err := Resolve("gpt-4o-mini", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "gpt-4o-mini" {
		t.Errorf("model=%q", got)
	}
	if defs != (ModelDefaults{}) {
		t.Errorf("defs not empty: %+v", defs)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected warning: %q", buf.String())
	}
}

func TestResolveQuiet_UnknownNoWarning(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	var buf bytes.Buffer
	stderr = &buf
	defer func() { stderr = os.Stderr }()

	got, defs, err := ResolveQuiet("custom-raw-id", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "custom-raw-id" {
		t.Errorf("model=%q", got)
	}
	if defs != (ModelDefaults{}) {
		t.Errorf("defs not empty: %+v", defs)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected warning: %q", buf.String())
	}
}

func TestResolve_CodeOnly_RejectedOnChat(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	_, _, err := Resolve("codex", 0)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "does not support chat") {
		t.Errorf("err=%v", err)
	}
}

func TestResolve_NonCode_RejectedAsCode(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	_, _, err := Resolve("mini", CapCode)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "does not support code") {
		t.Errorf("err=%v", err)
	}
}

func TestResolve_VisionOnly_RejectedOnChat(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	_, _, err := Resolve("pixtral", 0)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "vision-only") {
		t.Errorf("err=%v", err)
	}
}

func TestResolve_NonVision_RejectedAsVision(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	_, _, err := Resolve("mini", CapVision)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "does not support vision") {
		t.Errorf("err=%v", err)
	}
}

func TestResolve_EmptyInput(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	got, defs, err := Resolve("", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("got=%q", got)
	}
	if defs != (ModelDefaults{}) {
		t.Errorf("defs not empty: %+v", defs)
	}
}

func TestEmbedded_DefaultsParsed(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)
	all := All()
	sonar, ok := all["sonar"]
	if !ok {
		t.Fatalf("sonar entry missing")
	}
	if sonar.Defaults.WebSearch == nil || !*sonar.Defaults.WebSearch {
		t.Errorf("sonar.WebSearch=%v", sonar.Defaults.WebSearch)
	}
	if sonar.Defaults.NumOfSite == nil || *sonar.Defaults.NumOfSite != 5 {
		t.Errorf("sonar.NumOfSite=%v", sonar.Defaults.NumOfSite)
	}

	grokCode, ok := all["grok-code"]
	if !ok {
		t.Fatalf("grok-code entry missing")
	}
	if grokCode.Caps != CapCode {
		t.Errorf("grok-code.Caps=%d want %d", grokCode.Caps, CapCode)
	}

	codex, ok := all["codex"]
	if !ok {
		t.Fatalf("codex entry missing")
	}
	if codex.Caps != CapCode {
		t.Errorf("codex.Caps=%d want %d", codex.Caps, CapCode)
	}

	pixtral, ok := all["pixtral"]
	if !ok {
		t.Fatalf("pixtral entry missing")
	}
	if pixtral.Caps != CapVision {
		t.Errorf("pixtral.Caps=%d want %d", pixtral.Caps, CapVision)
	}
}

func TestLoadFromPath_OverrideReplaces(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)

	dir := t.TempDir()
	override := filepath.Join(dir, "models.json")
	body := `[{"alias":"otheralias","apiId":"other-api","caps":[]}]`
	if err := os.WriteFile(override, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := loadFromPath(override)
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	if _, ok := r["otheralias"]; !ok {
		t.Errorf("otheralias missing from override")
	}
	if _, ok := r["mini"]; ok {
		t.Errorf("mini should not be present in override-only set")
	}
}

func TestLoadFromPath_MalformedFallsBackToEmbedded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("override resolution uses HOME; os.UserHomeDir on Windows reads USERPROFILE instead")
	}
	resetForTest(t)
	defer resetForTest(t)

	dir := t.TempDir()
	override := filepath.Join(dir, "models.json")
	if err := os.WriteFile(override, []byte("not json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Point HOME at the temp dir so overridePath() resolves to our bad file.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)
	// The override resolves to <HOME>/.config/omi/models.json — write there.
	configDir := filepath.Join(dir, ".config", "omi")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "models.json"), []byte("{ broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	stderr = &buf
	defer func() { stderr = os.Stderr }()

	r, err := loadDefault()
	if err != nil {
		t.Fatalf("loadDefault: %v", err)
	}
	if _, ok := r["mini"]; !ok {
		t.Errorf("expected fallback to embedded (mini missing)")
	}
	if !strings.Contains(buf.String(), "failed to parse models.json override") {
		t.Errorf("stderr warning missing: %q", buf.String())
	}
}

func TestLoadFromPath_OverrideViaXDG(t *testing.T) {
	resetForTest(t)
	defer resetForTest(t)

	dir := t.TempDir()
	configDir := filepath.Join(dir, "omi")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `[{"alias":"override-only","apiId":"o","caps":[]}]`
	if err := os.WriteFile(filepath.Join(configDir, "models.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	r, err := loadDefault()
	if err != nil {
		t.Fatalf("loadDefault: %v", err)
	}
	if _, ok := r["override-only"]; !ok {
		t.Errorf("override-only missing")
	}
	if _, ok := r["mini"]; ok {
		t.Errorf("mini should not exist when override fully replaces embedded")
	}
}
