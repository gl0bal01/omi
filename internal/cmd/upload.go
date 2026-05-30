package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "upload <file>",
		Short:         "Upload a file to 1min.ai and print the asset path",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			_, apiKey, err := loadAPIKey()
			if err != nil {
				return err
			}

			client := newAPIClient(apiKey)
			assetPath, err := client.UploadAsset(cmd.Context(), path)
			if err != nil {
				return TranslateAPIError(err)
			}
			// Sanitize the upstream-controlled path before printing so a
			// hostile/compromised API response cannot inject terminal control
			// sequences into the user's TTY.
			_, err = fmt.Fprint(cmd.OutOrStdout(), SanitizeForTerminal(assetPath))
			return err
		},
	}
}
