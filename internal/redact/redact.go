// Package redact centralizes secret-masking helpers so the same format is
// used for config display, doctor output, and debug-HTTP header redaction.
package redact

// APIKey returns a display-safe form of an API key. Empty → empty. Keys
// shorter than 7 characters are flattened to "***". Otherwise returns the
// first 3 characters, an ellipsis, and the last 4.
func APIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 7 {
		return "***"
	}
	return key[:3] + "..." + key[len(key)-4:]
}
