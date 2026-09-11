package shared

import (
	"net/http"
	"os"
	"strings"
)

func ShouldWriteUpstreamEmptyOutputError(text, thinking string) bool {
	return strings.TrimSpace(text) == ""
}

func UpstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string) {
	_ = text
	if contentFilter {
		return http.StatusBadRequest, "Upstream content filtered the response and returned no output.", "content_filter"
	}
	if thinking != "" {
		if hkustUpstreamConfiguredForEmptyOutput() {
			return http.StatusBadGateway, "Upstream returned reasoning without visible output or a valid tool call.", "upstream_empty_output"
		}
		return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned reasoning without visible output.", "upstream_empty_output"
	}
	return http.StatusServiceUnavailable, "Upstream service is unavailable and returned no output.", "upstream_unavailable"
}

func hkustUpstreamConfiguredForEmptyOutput() bool {
	return strings.TrimSpace(os.Getenv("HKUST_TOKEN")) != "" ||
		strings.TrimSpace(os.Getenv("HKUST_USE_API")) != "" ||
		strings.TrimSpace(os.Getenv("HKUST_WS_URL")) != ""
}

func WriteUpstreamEmptyOutputError(w http.ResponseWriter, text, thinking string, contentFilter bool) bool {
	if !ShouldWriteUpstreamEmptyOutputError(text, thinking) {
		return false
	}
	status, message, code := UpstreamEmptyOutputDetail(contentFilter, text, thinking)
	WriteOpenAIErrorWithCode(w, status, message, code)
	return true
}
