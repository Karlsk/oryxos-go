package config

import "testing"

func TestRedactValue(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value any
		want  any
	}{
		{"exact_api_key", "API_KEY", "top-secret", "[REDACTED]"},
		{"exact_authorization", "authorization", "Bearer top-secret", "[REDACTED]"},
		{"exact_mcp_token", "mcp_token", "top-secret", "[REDACTED]"},
		{"exact_password", "password", "top-secret", "[REDACTED]"},
		{"exact_secret", "secret", "top-secret", "[REDACTED]"},
		{"exact_token", "token", "top-secret", "[REDACTED]"},
		{"contains_apikey", "serviceApiKeyValue", "top-secret", "[REDACTED]"},
		{"contains_credential", "credential_ref", "top-secret", "[REDACTED]"},
		{"contains_mcp_auth", "mcp_auth_header", "top-secret", "[REDACTED]"},
		{"contains_webhook", "webhook_endpoint", "top-secret", "[REDACTED]"},
		{"safe_key", "listen_address", "127.0.0.1:8080", "127.0.0.1:8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactValue(tc.key, tc.value); got != tc.want {
				t.Fatalf("RedactValue(%q, %#v) = %#v, want %#v", tc.key, tc.value, got, tc.want)
			}
		})
	}
}

func TestIsSensitiveKey(t *testing.T) {
	if !IsSensitiveKey([]string{"http"}, "webhook_url") {
		t.Fatal("IsSensitiveKey() = false, want true for webhook_url")
	}
	if IsSensitiveKey([]string{"http"}, "read_timeout") {
		t.Fatal("IsSensitiveKey() = true, want false for read_timeout")
	}
}

func TestSanitizeErrorString(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"sensitive_key_value", "api_key=top-secret", "[REDACTED]"},
		{"sensitive_colon_value", "password: top-secret", "[REDACTED]"},
		{"credential_url", "request to https://user:top-secret@example.invalid failed", "[REDACTED]"},
		{"bearer_credential", "upstream rejected Bearer top-secret", "[REDACTED]"},
		{"basic_credential", "upstream rejected Basic dXNlcjp0b3Atc2VjcmV0", "[REDACTED]"},
		{"webhook_path_credential", "request to https://hooks.example.invalid/a/very-secret-token failed", "[REDACTED]"},
		{"sensitive_query", "request to https://api.example.invalid/v1?access_token=top-secret failed", "[REDACTED]"},
		{"opaque_query_credential", "request to https://api.example.invalid/v1?sig=AbCdEf0123456789AbCdEf failed", "[REDACTED]"},
		{"opaque_path_credential", "request to https://api.example.invalid/v1/AbCdEf0123456789AbCdEf failed", "[REDACTED]"},
		{"safe_text", "config listen_address: invalid host:port", "config listen_address: invalid host:port"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeErrorString(tc.text); got != tc.want {
				t.Fatalf("SanitizeErrorString(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
