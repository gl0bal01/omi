package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const defaultTranscribeModel = "qwen3-asr-flash"

var knownTranscribeModels = []string{
	"qwen3-asr-flash",
	"telephony",
	"telephony_short",
	"elevenlabs-speech-to-text",
	"gpt-4o-transcribe",
	"gpt-4o-transcribe-diarize",
	"whisper-1",
	"phone_call",
	"latest_short",
	"latest_long",
	"medical_conversation",
	"medical_dictation",
}

var transcribeAliases = map[string]string{
	"asr":                   "qwen3-asr-flash",
	"asr-fast":              "qwen3-asr-flash",
	"asr-telephony":         "telephony",
	"asr-telephony-short":   "telephony_short",
	"asr-openai":            "gpt-4o-transcribe",
	"asr-diarize":           "gpt-4o-transcribe-diarize",
	"asr-whisper":           "whisper-1",
	"asr-eleven":            "elevenlabs-speech-to-text",
	"asr-phone":             "phone_call",
	"asr-medical":           "medical_conversation",
	"asr-medical-dictation": "medical_dictation",
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

			cfg, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			resolved := resolveTranscribeModel(model, cfg.Model)
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
			rows := []row{
				{Alias: "asr", Model: "qwen3-asr-flash", Notes: "default balanced ASR"},
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

func resolveTranscribeModel(flag, fromCfg string) string {
	if flag != "" {
		return resolveTranscribeAlias(flag)
	}
	if env := os.Getenv("OMI_MODEL"); env != "" {
		return resolveTranscribeAlias(env)
	}
	if fromCfg != "" {
		return resolveTranscribeAlias(fromCfg)
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
