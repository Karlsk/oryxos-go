# OryxOS API Convention

All API responses use one outer envelope, real HTTP status codes, stable lower-case `snake_case` codes, and the correlation ID from `X-Request-ID`.

```go
type Result[T any] struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Data      *T             `json:"data,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
```

`code`, `message`, and `request_id` are always present. Successful responses have non-nil `data`; errors omit `data`. The HTTP status expresses transport semantics and the stable code is the machine-readable application classification. Clients must not branch on the message text.

Use the exact `X-Request-ID` header. A valid incoming ID is reused; otherwise middleware creates one. The response header, `request_id` body field, and access-log correlation value must match byte-for-byte.

Public JSON uses `snake_case`. Public timestamps are RFC3339 UTC strings, formatted with `value.UTC().Format(time.RFC3339)`. Do not expose local-time offsets or Unix timestamps.

Pagination is one-based. Omitted values default to `page=1` and `page_size=20`; bounds are page `1..10000` and page size `1..100`. `PageResult.Items` is never nil: an empty page serializes as `"items": []`, never `null` or an omitted key. `total_pages` is calculated from nonnegative total and page size without overflow.

Error `details` are optional and safe only when they contain exactly two string keys:

```text
field: [a-z][a-z0-9_]{0,63}
rule:  required | invalid_format | out_of_range | duplicate | too_large
```

Do not return raw request bodies, headers, query values, provider/database errors, file paths, stack traces, credentials, tokens, webhook URLs, or expanded configuration. Invalid descriptors, page values, or details use the single `500/internal/internal server error` fallback without data or details.
