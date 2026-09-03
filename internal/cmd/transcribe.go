package cmd

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const defaultTranscribeModel = "qwen3-asr-flash"

// transcribeEntry is the single source of truth for a speech-to-text model:
// a primary alias, optional extra aliases that map to the same Model, the
// raw upstream Model id, and short Notes shown by `omi transcribe models`.
type transcribeEntry struct {
	Alias   string
	Aliases []string
	Model   string
	Notes   string
}

// transcribeEntries drives `omi transcribe`, `omi transcribe models`, alias
// resolution, and shell completion. Add a new speech model here and every
// surface picks it up.
var transcribeEntries = []transcribeEntry{
	{Alias: "asr", Aliases: []string{"asr-fast"}, Model: "qwen3-asr-flash", Notes: "default balanced ASR"},
	{Alias: "asr-telephony", Model: "telephony", Notes: "call-center audio"},
	{Alias: "asr-telephony-short", Model: "telephony_short", Notes: "short telephony clips"},
	{Alias: "asr-openai", Model: "gpt-4o-transcribe", Notes: "OpenAI ASR"},
	{Alias: "asr-diarize", Model: "gpt-4o-transcribe-diarize", Notes: "speaker diarization"},
	{Alias: "asr-whisper", Model: "whisper-1", Notes: "legacy compatible"},
	{Alias: "asr-eleven", Model: "elevenlabs-speech-to-text", Notes: "ElevenLabs ASR"},
	{Alias: "asr-phone", Model: "phone_call", Notes: "phone-call optimized"},
	{Alias: "asr-medical", Model: "medical_conversation", Notes: "medical conversation"},
	{Alias: "asr-medical-dictation", Model: "medical_dictation", Notes: "medical dictation"},
}

// rawTranscribeModels lists upstream model ids that have no shorter alias.
var rawTranscribeModels = []string{"latest_short", "latest_long"}

var (
	knownTranscribeModels = buildKnownTranscribeModels()
	transcribeAliases     = buildTranscribeAliases()
)

func buildKnownTranscribeModels() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(transcribeEntries)+len(rawTranscribeModels))
	for _, e := range transcribeEntries {
		if _, ok := seen[e.Model]; ok {
			continue
		}
		seen[e.Model] = struct{}{}
		out = append(out, e.Model)
	}
	for _, m := range rawTranscribeModels {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func buildTranscribeAliases() map[string]string {
	out := make(map[string]string, len(transcribeEntries)*2)
	for _, e := range transcribeEntries {
		out[e.Alias] = e.Model
		for _, a := range e.Aliases {
			out[a] = e.Model
		}
	}
	return out
}

func newTranscribeCmd() *cobra.Command {
	var model string
	cmd := &cobra.Command{
		Use:           "transcribe <audio>",
		Short:         "Transcribe an audio file via SPEECH_TO_TEXT",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			_, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			resolved := resolveTranscribeModel(model)
			if runOpts.verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "omi: effective: endpoint=/api/features type=SPEECH_TO_TEXT model=%s\n", resolved)
			}
			client := newAPIClient(apiKey)
			assetPath, err := client.UploadAsset(cmd.Context(), path)
			if err != nil {
				return TranslateAPIError(err)
			}
			text, err := client.Transcribe(cmd.Context(), assetPath, resolved)
			if err != nil {
				return TranslateAPIError(err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), text)
			return nil
		},
	}
	cmd.Flags().StringVarP(&model, "model", "m", "", "speech-to-text model (default qwen3-asr-flash)")
	_ = cmd.RegisterFlagCompletionFunc("model", transcribeModelCompletion)
	cmd.AddCommand(newTranscribeModelsCmd())
	return cmd
}

func newTranscribeModelsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:           "models",
		Short:         "List speech-to-text models and aliases",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			type row struct {
				Alias string `json:"alias"`
				Model string `json:"model"`
				Notes string `json:"notes"`
			}
			rows := make([]row, 0, len(transcribeEntries))
			for _, e := range transcribeEntries {
				rows = append(rows, row{Alias: e.Alias, Model: e.Model, Notes: e.Notes})
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), map[string]any{
					"default":  defaultTranscribeModel,
					"aliases":  rows,
					"models":   knownTranscribeModels,
					"endpoint": "/api/features",
					"type":     "SPEECH_TO_TEXT",
				})
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "default: %s\n", defaultTranscribeModel)
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ALIAS\tMODEL\tNOTES")
			for _, r := range rows {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Alias, r.Model, r.Notes)
			}
			_ = tw.Flush()
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Raw model IDs:")
			for _, m := range knownTranscribeModels {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", m)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func transcribeModelCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	out := make([]string, 0, len(knownTranscribeModels)+len(transcribeAliases))
	seen := map[string]struct{}{}
	for alias := range transcribeAliases {
		if strings.HasPrefix(alias, toComplete) {
			out = append(out, alias)
			seen[alias] = struct{}{}
		}
	}
	for _, m := range knownTranscribeModels {
		if strings.HasPrefix(m, toComplete) {
			if _, ok := seen[m]; ok {
				continue
			}
			out = append(out, m)
			seen[m] = struct{}{}
		}
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}

// resolveTranscribeModel picks the speech model from the -m flag only.
// OMI_MODEL and config.model are chat models; feeding them to SPEECH_TO_TEXT
// fails upstream, so they are deliberately not consulted here.
func resolveTranscribeModel(flag string) string {
	if flag != "" {
		return resolveTranscribeAlias(flag)
	}
	return defaultTranscribeModel
}

func resolveTranscribeAlias(input string) string {
	if input == "" {
		return input
	}
	if m, ok := transcribeAliases[input]; ok {
		return m
	}
	return input
}
