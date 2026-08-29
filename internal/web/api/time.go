package api

import "time"

// FormatTimeUTC returns a public RFC 3339 timestamp normalized to UTC.
func FormatTimeUTC(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}
