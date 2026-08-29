package config

import (
	"net/url"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var (
	sensitiveKeyValuePattern = regexp.MustCompile(`(?i)(?:["']?[a-z0-9_-]*(?:api[_-]?key|authorization|mcp[_-]?(?:auth|token)|password|secret|token|webhook[_-]?url|apikey|credential|mcp_auth|webhook)[a-z0-9_-]*["']?\s*(?:=|:)\s*)(?:[^\s,;]+)`) //nolint:lll
	credentialURLPattern     = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@`)
	authorizationPattern     = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`)
	urlPattern               = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s<>"']+`)
)

// RedactValue returns a complete redaction for values whose key is sensitive.
func RedactValue(key string, value any) any {
	if IsSensitiveKey(nil, key) {
		return redactedValue
	}
	return value
}

// IsSensitiveKey reports whether a field name or its containing path is sensitive.
func IsSensitiveKey(path []string, key string) bool {
	for _, candidate := range append(append([]string(nil), path...), key) {
		name := strings.ToLower(candidate)
		switch name {
		case "api_key", "authorization", "mcp_auth", "mcp_token", "password", "secret", "token", "webhook_url":
			return true
		}
		if strings.Contains(name, "apikey") || strings.Contains(name, "credential") || strings.Contains(name, "mcp_auth") || strings.Contains(name, "webhook") {
			return true
		}
	}
	return false
}

// SanitizeErrorString removes a complete sensitive value from a caller-visible error string.
func SanitizeErrorString(text string) string {
	if sensitiveKeyValuePattern.MatchString(text) || credentialURLPattern.MatchString(text) || authorizationPattern.MatchString(text) || containsCredentialURL(text) {
		return redactedValue
	}
	return text
}

func containsCredentialURL(text string) bool {
	for _, raw := range urlPattern.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ".,;:!?)]}")
		parsed, err := url.Parse(raw)
		if err != nil {
			return true
		}
		if parsed.User != nil {
			return true
		}
		// Query strings are an unsafe logging boundary: even neutral parameter names
		// commonly carry signatures or opaque access credentials.
		if parsed.RawQuery != "" {
			return true
		}
		host := strings.ToLower(parsed.Hostname())
		if strings.Contains(host, "hook") && parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
			return true
		}
		for _, part := range strings.Split(parsed.Path, "/") {
			if isSensitiveURLPart(part) || looksLikeOpaqueCredential(part) {
				return true
			}
		}
		if parsed.Fragment != "" {
			return true
		}
	}
	return false
}

func isSensitiveURLPart(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(value, "-", "_"), "%5f", "_"))
	for _, marker := range []string{"api_key", "apikey", "authorization", "callback", "credential", "hook", "mcp_auth", "password", "secret", "token", "webhook"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func looksLikeOpaqueCredential(value string) bool {
	if len(value) < 16 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z':
			hasLetter = true
		case character >= '0' && character <= '9':
			hasDigit = true
		case character == '-', character == '_', character == '.', character == '~', character == '+', character == '=':
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}
