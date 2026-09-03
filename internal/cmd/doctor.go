package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/gl0bal01/omi/internal/config"
	"github.com/gl0bal01/omi/internal/models"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name   string
	Status string
	Detail string
}

func newDoctorCmd() *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Check config, model compatibility, and API readiness",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return &RuntimeError{Msg: "omi: load config: " + err.Error()}
			}

			checks := make([]doctorCheck, 0, 8)
			failures := 0

			apiKey := runOpts.apiKey
			keySource := "flag --api-key"
			if apiKey == "" {
				apiKey = os.Getenv("OMI_API_KEY")
				keySource = "env OMI_API_KEY"
			}
			if apiKey == "" {
				apiKey = cfg.APIKey
				keySource = "config api_key"
			}
			if apiKey == "" {
				checks = append(checks, doctorCheck{"api_key", "FAIL", "missing (set OMI_API_KEY or omi config set api_key <KEY>)"})
				failures++
			} else {
				checks = append(checks, doctorCheck{"api_key", "PASS", fmt.Sprintf("present from %s (%s)", keySource, config.MaskAPIKey(apiKey))})
			}

			chatInput := resolveModelInput("", cfg.Model)
			chatModel, chatDetail, chatOK := validateModelFor("chat", chatInput, 0)
			checks = append(checks, doctorCheck{"chat_model", status(chatOK), chatDetail})
			if !chatOK {
				failures++
			}

			codeInput := resolveCodeModel("", cfg.CodeModel)
			codeModel, codeDetail, codeOK := validateModelFor("code", codeInput, models.CapCode)
			checks = append(checks, doctorCheck{"code_model", status(codeOK), codeDetail})
			if !codeOK {
				failures++
			}

			transcribeInput := resolveTranscribeModel("")
			transcribeResolved := resolveTranscribeAlias(transcribeInput)
			if isKnownTranscribeModel(transcribeResolved) {
				checks = append(checks, doctorCheck{"transcribe_model", "PASS", fmt.Sprintf("%s -> %s", transcribeInput, transcribeResolved)})
			} else {
				checks = append(checks, doctorCheck{"transcribe_model", "WARN", fmt.Sprintf("%s -> %s (raw passthrough)", transcribeInput, transcribeResolved)})
			}

			checks = append(checks, doctorCheck{"endpoint_chat", "PASS", fmt.Sprintf("/api/chat-with-ai uses model=%s", chatModel)})
			checks = append(checks, doctorCheck{"endpoint_code", "PASS", fmt.Sprintf("/api/features type=CODE_GENERATOR uses model=%s", codeModel)})
			checks = append(checks, doctorCheck{"endpoint_transcribe", "PASS", fmt.Sprintf("/api/features type=SPEECH_TO_TEXT uses model=%s", transcribeResolved)})

			if live && apiKey != "" {
				ok, detail := runLivePing(cmd.Context(), apiKey)
				checks = append(checks, doctorCheck{"live_api", status(ok), detail})
				if !ok {
					failures++
				}
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
			for _, c := range checks {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Status, c.Detail)
			}
			_ = tw.Flush()

			if failures > 0 {
				return &RuntimeError{Msg: fmt.Sprintf("omi: doctor found %d failing check(s)", failures)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "perform a lightweight live API connectivity check")
	return cmd
}

func validateModelFor(kind, input string, required models.Capability) (resolved, detail string, ok bool) {
	resolved, _, err := models.ResolveQuiet(input, required)
	if err != nil {
		return "", err.Error(), false
	}
	known := modelKnownKind(input)
	if input == "" {
		known = "default"
	}
	detail = fmt.Sprintf("%s -> %s (%s)", input, resolved, known)
	return resolved, detail, true
}

func modelKnownKind(input string) string {
	entries := models.Entries()
	for _, e := range entries {
		if e.Alias == input {
			return "alias"
		}
	}
	for _, e := range entries {
		if e.APIID == input {
			return "known apiId"
		}
	}
	return "raw"
}

func isKnownTranscribeModel(model string) bool {
	for _, m := range knownTranscribeModels {
		if m == model {
			return true
		}
	}
	return false
}

func runLivePing(ctx context.Context, apiKey string) (bool, string) {
	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := newAPIClient(apiKey)
	code, err := client.Ping(pingCtx)
	if err != nil {
		return false, "connectivity failed: " + err.Error()
	}
	if code >= 200 && code < 300 {
		return true, fmt.Sprintf("reachable (%d)", code)
	}
	return false, fmt.Sprintf("unexpected status %d", code)
}

func status(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
