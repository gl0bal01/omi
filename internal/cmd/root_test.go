package cmd

import (
	"testing"

	"github.com/gl0bal01/omi/internal/models"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func TestResolveSettings_SonarAutoEnablesWeb(t *testing.T) {
	defs := models.ModelDefaults{
		WebSearch: boolPtr(true),
		NumOfSite: intPtr(5),
	}
	d := resolveSettings(&chatFlags{}, defs)
	if !d.WebSearch {
		t.Errorf("webSearch should be true")
	}
	if d.NumOfSite != 5 {
		t.Errorf("numOfSite=%d want 5", d.NumOfSite)
	}
	if !d.Notice {
		t.Errorf("notice should be set")
	}
}

func TestResolveSettings_NoWebSuppressesAuto(t *testing.T) {
	defs := models.ModelDefaults{
		WebSearch: boolPtr(true),
		NumOfSite: intPtr(5),
	}
	d := resolveSettings(&chatFlags{noWeb: true}, defs)
	if d.WebSearch {
		t.Errorf("webSearch should be false")
	}
	if d.Notice {
		t.Errorf("notice should be false under --no-web")
	}
}

func TestResolveSettings_ExplicitNumSitesOverride(t *testing.T) {
	defs := models.ModelDefaults{
		WebSearch: boolPtr(true),
		NumOfSite: intPtr(5),
	}
	d := resolveSettings(&chatFlags{numSites: 10, numSitesSet: true}, defs)
	if !d.WebSearch {
		t.Errorf("webSearch should be true")
	}
	if d.NumOfSite != 10 {
		t.Errorf("numOfSite=%d want 10", d.NumOfSite)
	}
}

func TestResolveSettings_ExplicitWebNoDoubleNotice(t *testing.T) {
	defs := models.ModelDefaults{
		WebSearch: boolPtr(true),
	}
	d := resolveSettings(&chatFlags{web: true}, defs)
	if d.Notice {
		t.Errorf("notice should be false when -w explicit")
	}
}

func TestResolveSettings_MaxWordPrecedence(t *testing.T) {
	defs := models.ModelDefaults{
		MaxWord: intPtr(500),
	}
	d := resolveSettings(&chatFlags{}, defs)
	if d.MaxWord != 500 {
		t.Errorf("maxWord=%d want 500", d.MaxWord)
	}
	d = resolveSettings(&chatFlags{maxWords: 200, maxWordsSet: true}, defs)
	if d.MaxWord != 200 {
		t.Errorf("maxWord=%d want 200", d.MaxWord)
	}
}

func TestResolveSettings_NoDefaults(t *testing.T) {
	d := resolveSettings(&chatFlags{}, models.ModelDefaults{})
	if d.WebSearch {
		t.Errorf("webSearch should be false")
	}
	if d.NumOfSite != 0 {
		t.Errorf("numOfSite=%d", d.NumOfSite)
	}
	if d.MaxWord != 0 {
		t.Errorf("maxWord=%d", d.MaxWord)
	}
	if d.Notice {
		t.Errorf("notice should be false")
	}
}

func TestResolveModelInputWithTask(t *testing.T) {
	t.Setenv("OMI_MODEL", "")
	got, err := resolveModelInputWithTask("", "", "vision")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "qwen3-vl-plus" {
		t.Fatalf("got=%q want qwen3-vl-plus", got)
	}

	got, err = resolveModelInputWithTask("mini", "", "vision")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "mini" {
		t.Fatalf("flag should win, got=%q", got)
	}

	_, err = resolveModelInputWithTask("", "", "transcribe")
	if err == nil {
		t.Fatalf("want error for transcribe task on chat command")
	}
}
