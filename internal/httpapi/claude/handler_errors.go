package claude

import (
	"net/http"
	"strings"
)

const claudePromptTooLongMessage = "prompt is too long"

func writeClaudeError(w http.ResponseWriter, status int, message string) {
	if isClaudeContextWindowExceeded(message) {
		status = http.StatusBadRequest
		message = claudePromptTooLongMessage
	}

	code := "invalid_request"
	switch status {
	case http.StatusUnauthorized:
		code = "authentication_failed"
	case http.StatusTooManyRequests:
		code = "rate_limit_exceeded"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusInternalServerError:
		code = "internal_error"
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": message,
			"code":    code,
			"param":   nil,
		},
	})
}

func isClaudeContextWindowExceeded(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, claudePromptTooLongMessage) {
		return true
	}

	compact := strings.NewReplacer(
		" ", "",
		"_", "",
		"-", "",
		".", "",
		":", "",
	).Replace(lower)
	if strings.Contains(compact, "contextwindowexceeded") || strings.Contains(compact, "contextlengthexceeded") {
		return true
	}

	hasContextKind := strings.Contains(lower, "context window") ||
		strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context") ||
		strings.Contains(lower, "max context")
	if !hasContextKind {
		return false
	}

	return strings.Contains(lower, "exceed") ||
		strings.Contains(lower, "too long") ||
		strings.Contains(lower, "maximum") ||
		strings.Contains(lower, "max ") ||
		strings.Contains(lower, "limit")
}
