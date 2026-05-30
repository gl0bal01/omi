package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gl0bal01/omi/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// validConfigKeys is the closed set accepted by `omi config set|get|list`,
// in stable display order for `omi config list`.
var validConfigKeys = []string{"api_key", "model", "code_model", "web", "mix", "num_sites", "max_words"}

var validConfigKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(validConfigKeys))
	for _, k := range validConfigKeys {
		m[k] = struct{}{}
	}
	return m
}()

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "config",
		Short:         "Read or write CLI configuration",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigListCmd())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Long: "Set a config value.\n\n" +
			"For api_key, pass \"-\" as the value to read the key from stdin " +
			"(or a no-echo prompt on a terminal) so the secret never appears in " +
			"the process list or shell history:\n\n    omi config set api_key -",
		Args:          cobra.MaximumNArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return &UsageError{Msg: "omi: usage: omi config set <key> <value>"}
			}
			key, value := args[0], args[1]
			if !isValidConfigKey(key) {
				return &UsageError{Msg: fmt.Sprintf("omi: unknown config key '%s' (valid: %s)", key, strings.Join(validConfigKeys, ", "))}
			}

			// `omi config set api_key -` reads the secret from stdin instead of
			// argv, keeping it out of the process list and shell history.
			if key == "api_key" && value == "-" {
				secret, err := readSecretValue(cmd, "api_key")
				if err != nil {
					return &RuntimeError{Msg: "omi: read api_key: " + err.Error()}
				}
				if secret == "" {
					return &UsageError{Msg: "omi: empty api_key from stdin"}
				}
				value = secret
			}

			cfg, err := config.Load()
			if err != nil {
				return &RuntimeError{Msg: "omi: load config: " + err.Error()}
			}

			if err := setConfigField(cfg, key, value); err != nil {
				return &UsageError{Msg: err.Error()}
			}

			if err := cfg.Save(); err != nil {
				return &RuntimeError{Msg: "omi: save config: " + err.Error()}
			}
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:           "get <key>",
		Short:         "Print a config value",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &UsageError{Msg: "omi: usage: omi config get <key>"}
			}
			key := args[0]
			if !isValidConfigKey(key) {
				return &UsageError{Msg: fmt.Sprintf("omi: unknown config key '%s' (valid: %s)", key, strings.Join(validConfigKeys, ", "))}
			}

			cfg, err := config.Load()
			if err != nil {
				return &RuntimeError{Msg: "omi: load config: " + err.Error()}
			}

			out := cmd.OutOrStdout()
			value, isSet := getConfigField(cfg, key)
			if !isSet {
				return &RuntimeError{Msg: "omi: config key '" + key + "' is not set"}
			}

			if key == "api_key" && !reveal {
				value = config.MaskAPIKey(value)
			}
			_, err = fmt.Fprintln(out, value)
			return err
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show the raw api_key value")
	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List all config keys (api_key masked)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("omi: load config: %w", err)
			}
			out := cmd.OutOrStdout()
			for _, k := range validConfigKeys {
				v, _ := getConfigField(cfg, k)
				if k == "api_key" {
					v = config.MaskAPIKey(v)
				}
				_, _ = fmt.Fprintf(out, "%s = %s\n", k, v)
			}
			return nil
		},
	}
}

func isValidConfigKey(k string) bool {
	_, ok := validConfigKeySet[k]
	return ok
}

// getConfigField returns the stringified value plus whether the field is "set"
// (non-zero). Bools and ints are always treated as set so that `omi config get
// web` after `set web false` still prints `false` (not exit 1).
func getConfigField(c *config.Config, key string) (string, bool) {
	switch key {
	case "api_key":
		return c.APIKey, c.APIKey != ""
	case "model":
		return c.Model, c.Model != ""
	case "code_model":
		return c.CodeModel, c.CodeModel != ""
	case "web":
		return strconv.FormatBool(c.Web), true
	case "mix":
		return strconv.FormatBool(c.Mix), true
	case "num_sites":
		return strconv.Itoa(c.NumSites), true
	case "max_words":
		return strconv.Itoa(c.MaxWords), true
	}
	return "", false
}

func setConfigField(c *config.Config, key, value string) error {
	switch key {
	case "api_key":
		c.APIKey = value
	case "model":
		c.Model = value
	case "code_model":
		c.CodeModel = value
	case "web":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("omi: invalid bool for '%s': %s (use true|false|1|0|yes|no)", key, value)
		}
		c.Web = b
	case "mix":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("omi: invalid bool for '%s': %s (use true|false|1|0|yes|no)", key, value)
		}
		c.Mix = b
	case "num_sites":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("omi: invalid non-negative int for '%s': %s", key, value)
		}
		c.NumSites = n
	case "max_words":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("omi: invalid non-negative int for '%s': %s", key, value)
		}
		c.MaxWords = n
	}
	return nil
}

// readSecretValue reads a secret from stdin. On an interactive terminal it uses
// a no-echo prompt; otherwise it reads the first line of stdin (pipe/file).
func readSecretValue(cmd *cobra.Command, label string) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Enter %s: ", label)
		b, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("not a bool")
}
