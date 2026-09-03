// Package models defines the alias registry and capability model.
//
// Capability semantics: every model is implicitly chat-capable UNLESS its
// capability set is exclusively CapCode (code-only) or exclusively CapVision
// (vision-only). There is no positive CapChat flag — chat eligibility is
// inferred from the absence of a code-only / vision-only restriction.
//
// Override file: ~/.config/omi/models.json (XDG_CONFIG_HOME aware) FULLY
// REPLACES the embedded set when present and parseable. No merge semantics.
// On parse failure the loader falls back to the embedded registry and emits
// a single stderr warning.
package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Capability is a bitmask describing what endpoints a model supports.
// CapChat is intentionally omitted; see package doc.
type Capability uint

const (
	CapCode   Capability = 1 << 0
	CapVision Capability = 1 << 1
)

// ModelDefaults mirrors the Python plugin's MODEL_DEFAULTS table. Pointer
// types distinguish "absent" from "zero".
type ModelDefaults struct {
	WebSearch        *bool
	NumOfSite        *int
	MaxWord          *int
	ConversationType string
}

// Entry is a single registry record.
type Entry struct {
	Alias    string
	APIID    string
	Caps     Capability
	Defaults ModelDefaults
}

// rawEntry is the on-disk JSON shape.
type rawEntry struct {
	Alias    string      `json:"alias"`
	APIID    string      `json:"apiId"`
	Caps     []string    `json:"caps"`
	Defaults *rawDefault `json:"defaults,omitempty"`
}

type rawDefault struct {
	WebSearch        *bool  `json:"webSearch,omitempty"`
	NumOfSite        *int   `json:"numOfSite,omitempty"`
	MaxWord          *int   `json:"maxWord,omitempty"`
	ConversationType string `json:"conversationType,omitempty"`
}

//go:embed models.json
var embeddedJSON []byte

var (
	loadOnce sync.Once
	registry map[string]Entry
	// stderr is overridable in tests.
	stderr io.Writer = os.Stderr
)

// All returns a snapshot of the loaded registry keyed by alias.
func All() map[string]Entry {
	ensureLoaded()
	out := make(map[string]Entry, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// Entries returns a defensive slice copy of every loaded registry entry,
// sorted by alias. Triggers the same one-shot load as Resolve so override
// file behavior is honored consistently across the registry surface.
func Entries() []Entry {
	ensureLoaded()
	out := make([]Entry, 0, len(registry))
	for _, v := range registry {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// Resolve looks up input as an alias and validates capability.
//
// Empty input → ("", ModelDefaults{}, nil) (caller falls back to default).
// Registry-known API IDs are accepted as explicit IDs without warning.
// Unknown alias/ID → emits the exact stderr warning and returns (input, empty, nil).
// Known alias → checks capability and returns (apiId, defaults, err).
func Resolve(input string, required Capability) (string, ModelDefaults, error) {
	return resolveInternal(input, required, true)
}

// ResolveQuiet behaves like Resolve but suppresses unknown-alias warnings.
func ResolveQuiet(input string, required Capability) (string, ModelDefaults, error) {
	return resolveInternal(input, required, false)
}

func resolveInternal(input string, required Capability, warnUnknown bool) (string, ModelDefaults, error) {
	if input == "" {
		return "", ModelDefaults{}, nil
	}
	ensureLoaded()
	entry, ok := registry[input]
	if !ok {
		if isKnownAPIID(input) {
			return input, ModelDefaults{}, nil
		}
		if warnUnknown {
			_, _ = fmt.Fprintf(stderr, "omi: warning: '%s' is not a known alias; treating as raw model ID. Run 'omi models' to see known aliases.\n", input)
		}
		return input, ModelDefaults{}, nil
	}

	switch required {
	case CapCode:
		if entry.Caps&CapCode == 0 {
			return "", ModelDefaults{}, fmt.Errorf("omi: model '%s' does not support code generation (use 'omi models code')", input)
		}
	case CapVision:
		// Chat models on chat-with-ai are multimodal; only code-only entries
		// cannot take an image.
		if entry.Caps == CapCode {
			return "", ModelDefaults{}, fmt.Errorf("omi: model '%s' does not support vision", input)
		}
	case 0:
		// Chat path: reject code-only or vision-only entries.
		if entry.Caps == CapCode {
			return "", ModelDefaults{}, fmt.Errorf("omi: model '%s' does not support chat (use one of: %s)", input, chatEligibleAliases())
		}
		if entry.Caps == CapVision {
			return "", ModelDefaults{}, fmt.Errorf("omi: model '%s' does not support chat (vision-only)", input)
		}
	}

	return entry.APIID, entry.Defaults, nil
}

func isKnownAPIID(input string) bool {
	for _, e := range registry {
		if e.APIID == input {
			return true
		}
	}
	return false
}

// chatEligibleAliases returns a sorted comma list of aliases that are chat-eligible.
func chatEligibleAliases() string {
	var out []string
	for alias, entry := range registry {
		if entry.Caps == CapCode || entry.Caps == CapVision {
			continue
		}
		out = append(out, alias)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func ensureLoaded() {
	loadOnce.Do(func() {
		var err error
		registry, err = loadDefault()
		if err != nil {
			registry = map[string]Entry{}
		}
	})
}

// loadDefault resolves the override path then falls back to embedded JSON.
func loadDefault() (map[string]Entry, error) {
	override, err := overridePath()
	if err == nil && override != "" {
		if _, statErr := os.Stat(override); statErr == nil {
			r, perr := loadFromPath(override)
			if perr == nil {
				// The override FULLY replaces the embedded registry and can
				// silently re-route a trusted alias to a different model ID.
				// Surface it so an unexpected remapping is visible.
				_, _ = fmt.Fprintf(stderr, "omi: note: using models.json override (%d entries) from %s\n", len(r), override)
				return r, nil
			}
			_, _ = fmt.Fprintf(stderr, "omi: warning: failed to parse models.json override: %v; falling back to embedded registry\n", perr)
		}
	}
	return parseRegistry(embeddedJSON)
}

// maxRegistryBytes bounds the override file read so a malformed or hostile
// models.json cannot exhaust memory during JSON parsing.
const maxRegistryBytes = 4 << 20

// loadFromPath is exported within-package for tests.
func loadFromPath(path string) (map[string]Entry, error) {
	f, err := os.Open(path) // #nosec G304 -- path is the documented XDG models override or an explicit test path.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxRegistryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRegistryBytes {
		return nil, fmt.Errorf("omi: models override %s exceeds %d bytes", path, maxRegistryBytes)
	}
	return parseRegistry(data)
}

func parseRegistry(data []byte) (map[string]Entry, error) {
	var raw []rawEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]Entry, len(raw))
	for _, r := range raw {
		entry := Entry{
			Alias: r.Alias,
			APIID: r.APIID,
		}
		for _, c := range r.Caps {
			switch c {
			case "code":
				entry.Caps |= CapCode
			case "vision":
				entry.Caps |= CapVision
			}
		}
		if r.Defaults != nil {
			entry.Defaults = ModelDefaults{
				WebSearch:        r.Defaults.WebSearch,
				NumOfSite:        r.Defaults.NumOfSite,
				MaxWord:          r.Defaults.MaxWord,
				ConversationType: r.Defaults.ConversationType,
			}
		}
		out[r.Alias] = entry
	}
	return out, nil
}

func overridePath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "omi", "models.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omi", "models.json"), nil
}
