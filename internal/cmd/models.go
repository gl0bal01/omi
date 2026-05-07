package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/gl0bal01/omi/internal/models"
	"github.com/spf13/cobra"
)

// newModelsCmd lists registry entries and supports metadata views.
func newModelsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "models [chat|code|vision|all]",
		Short: "List model aliases with CAPS, DEFAULTS, and NOTES",
		Long: `List known model aliases and their metadata.

Output columns:
  ALIAS      user-facing shortcut
  APIID      raw upstream model id sent to 1min.ai
  CAPS       endpoint capability tags (chat/code/vision)
  DEFAULTS   per-model defaults (webSearch, numOfSite, maxWord, conversationType)
  NOTES      quick behavior hints

Filter (optional):
  chat   chat-capable models (default)
  code   code-capable models
  vision vision-capable models
  all    all aliases`,
		Example: `  omi models
  omi models code
  omi models vision
  omi models all
  omi models --json
  omi models explain sonar
  omi models explain gpt-4o-mini --json`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := "chat"
			if len(args) == 1 {
				filter = args[0]
			}
			switch filter {
			case "chat", "code", "vision", "all":
			default:
				return &UsageError{Msg: fmt.Sprintf("omi: invalid filter '%s' (use: chat, code, vision, all)", filter)}
			}

			entries := filterEntries(models.Entries(), filter)
			if asJSON {
				out := make([]map[string]any, 0, len(entries))
				for _, e := range entries {
					out = append(out, map[string]any{
						"alias":    e.Alias,
						"apiId":    e.APIID,
						"caps":     capList(e),
						"defaults": defaultsMap(e.Defaults),
						"notes":    noteList(e),
					})
				}
				return printJSON(cmd.OutOrStdout(), map[string]any{
					"filter":  filter,
					"count":   len(out),
					"entries": out,
				})
			}

			if filter == "chat" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "# showing chat-capable aliases (use 'omi models code', 'omi models vision', or 'omi models all' for other views)")
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ALIAS\tAPIID\tCAPS\tDEFAULTS\tNOTES")
			for _, e := range entries {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Alias, e.APIID, capsString(e), defaultsString(e.Defaults), notesString(e))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.AddCommand(newModelsExplainCmd())
	return cmd
}

func newModelsExplainCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:           "explain <alias-or-apiId>",
		Short:         "Show full details for one model alias or API ID",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := args[0]
			entry, aliases, ok := findModel(q)
			if !ok {
				return &UsageError{Msg: fmt.Sprintf("omi: model '%s' not found in registry", q)}
			}

			if asJSON {
				return printJSON(cmd.OutOrStdout(), map[string]any{
					"query":          q,
					"alias":          entry.Alias,
					"apiId":          entry.APIID,
					"caps":           capList(entry),
					"defaults":       defaultsMap(entry.Defaults),
					"notes":          noteList(entry),
					"relatedAliases": aliases,
				})
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Alias: %s\n", entry.Alias)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "APIID: %s\n", entry.APIID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "CAPS: %s\n", capsString(entry))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "DEFAULTS: %s\n", defaultsString(entry.Defaults))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "NOTES: %s\n", notesString(entry))
			if len(aliases) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Related aliases: %s\n", strings.Join(aliases, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func filterEntries(in []models.Entry, filter string) []models.Entry {
	out := make([]models.Entry, 0, len(in))
	for _, e := range in {
		switch filter {
		case "chat":
			if e.Caps == models.CapCode || e.Caps == models.CapVision {
				continue
			}
		case "code":
			if e.Caps&models.CapCode == 0 {
				continue
			}
		case "vision":
			if e.Caps&models.CapVision == 0 {
				continue
			}
		case "all":
			// no filter
		}
		out = append(out, e)
	}
	return out
}

func capList(e models.Entry) []string {
	parts := make([]string, 0, 3)
	if e.Caps != models.CapCode && e.Caps != models.CapVision {
		parts = append(parts, "chat")
	}
	if e.Caps&models.CapCode != 0 {
		parts = append(parts, "code")
	}
	if e.Caps&models.CapVision != 0 {
		parts = append(parts, "vision")
	}
	return parts
}

func capsString(e models.Entry) string {
	parts := capList(e)
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func defaultsString(d models.ModelDefaults) string {
	var parts []string
	if d.WebSearch != nil {
		parts = append(parts, "webSearch="+strconv.FormatBool(*d.WebSearch))
	}
	if d.NumOfSite != nil {
		parts = append(parts, "numOfSite="+strconv.Itoa(*d.NumOfSite))
	}
	if d.MaxWord != nil {
		parts = append(parts, "maxWord="+strconv.Itoa(*d.MaxWord))
	}
	if d.ConversationType != "" {
		parts = append(parts, "conversationType="+d.ConversationType)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func defaultsMap(d models.ModelDefaults) map[string]any {
	out := map[string]any{}
	if d.WebSearch != nil {
		out["webSearch"] = *d.WebSearch
	}
	if d.NumOfSite != nil {
		out["numOfSite"] = *d.NumOfSite
	}
	if d.MaxWord != nil {
		out["maxWord"] = *d.MaxWord
	}
	if d.ConversationType != "" {
		out["conversationType"] = d.ConversationType
	}
	return out
}

func noteList(e models.Entry) []string {
	out := []string{}
	if e.Caps == models.CapCode {
		out = append(out, "code-only")
	}
	if e.Caps == models.CapVision {
		out = append(out, "vision-only")
	}
	if e.Defaults.WebSearch != nil && *e.Defaults.WebSearch {
		out = append(out, "web-search default")
	}
	if e.Defaults.ConversationType == "code" {
		out = append(out, "code-first")
	}
	if len(out) == 0 {
		out = append(out, "general")
	}
	return out
}

func notesString(e models.Entry) string {
	return strings.Join(noteList(e), ";")
}

func findModel(query string) (models.Entry, []string, bool) {
	entries := models.Entries()
	for _, e := range entries {
		if e.Alias == query {
			return e, aliasesForAPIID(entries, e.APIID, e.Alias), true
		}
	}
	for _, e := range entries {
		if e.APIID == query {
			return e, aliasesForAPIID(entries, e.APIID, e.Alias), true
		}
	}
	return models.Entry{}, nil, false
}

func aliasesForAPIID(entries []models.Entry, apiID, exclude string) []string {
	out := []string{}
	for _, e := range entries {
		if e.APIID == apiID && e.Alias != exclude {
			out = append(out, e.Alias)
		}
	}
	sort.Strings(out)
	return out
}
