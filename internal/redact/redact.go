// Package redact centralizes secret-masking helpers so the same format is
// used for config display, doctor output, and debug-HTTP header redaction.
package redact

// APIKey returns a display-safe form of an API key. Empty → empty. Keys
// shorter than 11 bytes are flattened to "***" so the revealed prefix(3) and
// suffix(4) can never cover or overlap the whole secret — at least 4 bytes
// always stay hidden in the middle. Otherwise returns the first 3 bytes, an
// ellipsis, and the last 4.
func APIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 11 {
		return "***"
	}
	return key[:3] + "..." + key[len(key)-4:]
}
