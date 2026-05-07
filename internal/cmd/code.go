package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/gl0bal01/omi/internal/models"
	"github.com/spf13/cobra"
)

const defaultCodeModel = "gpt-5.1-codex"

func newCodeCmd() *cobra.Command {
	var modelFlag string
	cmd := &cobra.Command{
		Use:           "code <prompt>",
		Short:         "Generate code via CODE_GENERATOR",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.Join(args, " ")

			cfg, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			modelInput := resolveCodeModel(modelFlag, cfg.CodeModel)
			modelID, _, err := models.Resolve(modelInput, models.CapCode)
			if err != nil {
				return &UsageError{Msg: err.Error()}
			}

			if runOpts.verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "omi: effective: endpoint=/api/features type=CODE_GENERATOR model=%s\n", modelID)
			}
			client := newAPIClient(apiKey)
			text, err := client.Code(cmd.Context(), modelID, prompt)
			if err != nil {
				return TranslateAPIError(err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), text)
			return err
		},
	}
	cmd.Flags().StringVar(&modelFlag, "code-model", "", "code model alias or id (default gpt-5.1-codex)")
	_ = cmd.RegisterFlagCompletionFunc("code-model", codeModelCompletion)
	return cmd
}

// codeModelCompletion lists code-capable aliases for `--code-model`.
func codeModelCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	out := []string{}
	for _, e := range models.Entries() {
		if e.Caps&models.CapCode == 0 {
			continue
		}
		if strings.HasPrefix(e.Alias, toComplete) {
			out = append(out, e.Alias)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func resolveCodeModel(flag, fromCfg string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("OMI_CODE_MODEL"); env != "" {
		return env
	}
	if fromCfg != "" {
		return fromCfg
	}
	return defaultCodeModel
}
