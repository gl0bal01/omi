package cmd

import (
	"testing"

	"github.com/gl0bal01/omi/internal/models"
)

func TestFilterEntries_All(t *testing.T) {
	in := []models.Entry{
		{Alias: "chat", Caps: 0},
		{Alias: "code", Caps: models.CapCode},
		{Alias: "vision", Caps: models.CapVision},
		{Alias: "both", Caps: models.CapCode | models.CapVision},
	}
	got := filterEntries(in, "all")
	if len(got) != len(in) {
		t.Fatalf("len=%d want %d", len(got), len(in))
	}
}

func TestFilterEntries_ChatExcludesCodeOnlyAndVisionOnly(t *testing.T) {
	in := []models.Entry{
		{Alias: "chat", Caps: 0},
		{Alias: "code", Caps: models.CapCode},
		{Alias: "vision", Caps: models.CapVision},
		{Alias: "both", Caps: models.CapCode | models.CapVision},
	}
	got := filterEntries(in, "chat")
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].Alias != "chat" || got[1].Alias != "both" {
		t.Fatalf("unexpected order/content: %+v", got)
	}
}

func TestCapsString(t *testing.T) {
	cases := []struct {
		name string
		in   models.Entry
		want string
	}{
		{name: "chat only", in: models.Entry{Caps: 0}, want: "chat"},
		{name: "code only", in: models.Entry{Caps: models.CapCode}, want: "code"},
		{name: "vision only", in: models.Entry{Caps: models.CapVision}, want: "vision"},
		{name: "chat+code+vision", in: models.Entry{Caps: models.CapCode | models.CapVision}, want: "chat,code,vision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := capsString(tc.in); got != tc.want {
				t.Fatalf("capsString()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestFindModel_ByAliasAndAPIID(t *testing.T) {
	e, aliases, ok := findModel("mini")
	if !ok {
		t.Fatalf("mini not found")
	}
	if e.Alias != "mini" {
		t.Fatalf("alias=%q", e.Alias)
	}
	if len(aliases) != 0 {
		t.Fatalf("mini should not have related aliases, got %v", aliases)
	}

	e2, aliases2, ok2 := findModel("gpt-4o-mini")
	if !ok2 {
		t.Fatalf("gpt-4o-mini not found")
	}
	if e2.APIID != "gpt-4o-mini" {
		t.Fatalf("apiId=%q", e2.APIID)
	}
	_ = aliases2
}

func TestNotesString(t *testing.T) {
	e := models.Entry{Caps: models.CapCode}
	if got := notesString(e); got != "code-only" {
		t.Fatalf("got=%q", got)
	}
}
