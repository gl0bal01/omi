package cmd

import (
	"os"

	"github.com/gl0bal01/omi/internal/config"
)

// loadAPIKey resolves the API key in priority order:
//  1. --api-key flag
//  2. OMI_API_KEY env var
//  3. config file
//
// Empty result → UsageError so the caller surfaces the standard
// "API key not set" message.
func loadAPIKey() (*config.Config, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", &RuntimeError{Msg: "omi: load config: " + err.Error()}
	}
	apiKey := runOpts.apiKey
	if apiKey == "" {
		apiKey = os.Getenv("OMI_API_KEY")
	}
	if apiKey == "" {
		apiKey = cfg.APIKey
	}
	if apiKey == "" {
		return cfg, "", &UsageError{Msg: "API key not set. Pass --api-key, set OMI_API_KEY, or run 'omi config set api_key <KEY>'."}
	}
	return cfg, apiKey, nil
}
