// Package api provides the shared HTTP response contract.
package api

import "net/url"

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPage         = 10000
	maxPageSize     = 100
)

// PageResult is the canonical data payload for a paginated Result.
type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

func parsePagination(values url.Values) (int, int, map[string]any) {
	page, details := parsePaginationValue(values, "page", defaultPage, maxPage)
	if details != nil {
		return 0, 0, details
	}
	pageSize, details := parsePaginationValue(values, "page_size", defaultPageSize, maxPageSize)
	if details != nil {
		return 0, 0, details
	}
	return page, pageSize, nil
}

func parsePaginationValue(values url.Values, field string, defaultValue, maximum int) (int, map[string]any) {
	raw, present := values[field]
	if !present {
		return defaultValue, nil
	}
	if len(raw) != 1 {
		return 0, paginationDetails(field, "duplicate")
	}
	value := raw[0]
	if value == "" || !isUnsignedDecimal(value) {
		return 0, paginationDetails(field, "invalid_format")
	}

	parsed := uint64(0)
	for i := 0; i < len(value); i++ {
		digit := uint64(value[i] - '0')
		if parsed > (^uint64(0)-digit)/10 {
			return 0, paginationDetails(field, "out_of_range")
		}
		parsed = parsed*10 + digit
	}
	// #nosec G115 -- maximum is a small positive contract bound before conversion.
	if parsed == 0 || parsed > uint64(maximum) {
		return 0, paginationDetails(field, "out_of_range")
	}
	return int(parsed), nil
}

func isUnsignedDecimal(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func paginationDetails(field, rule string) map[string]any {
	return map[string]any{"field": field, "rule": rule}
}

func validPage[T any](page PageResult[T]) bool {
	return page.Page >= 1 && page.Page <= maxPage &&
		page.PageSize >= 1 && page.PageSize <= maxPageSize &&
		page.Total >= 0
}

func totalPages(total int64, pageSize int) int64 {
	pages := total / int64(pageSize)
	if total%int64(pageSize) != 0 {
		pages++
	}
	return pages
}
