package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsClaudeContextWindowExceeded(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "litellm_error_class",
			message: "litellm.ContextWindowExceededError: upstream rejected the request",
			want:    true,
		},
		{
			name:    "dynamic_context_limit",
			message: "This model's maximum context length is 12345 tokens. The prompt contains at least 12346 input tokens.",
			want:    true,
		},
		{
			name:    "different_dynamic_context_limit",
			message: "maximum context length is 200000 tokens; request exceeds the context window",
			want:    true,
		},
		{
			name:    "anthropic_retry_message",
			message: "prompt is too long",
			want:    true,
		},
		{
			name:    "ordinary_upstream_error",
			message: "upstream websocket disconnected",
			want:    false,
		},
		{
			name:    "output_limit_is_not_context_limit",
			message: "maximum output token limit exceeded",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isClaudeContextWindowExceeded(tc.message); got != tc.want {
				t.Fatalf("isClaudeContextWindowExceeded(%q)=%v want %v", tc.message, got, tc.want)
			}
		})
	}
}

func TestWriteClaudeErrorNormalizesContextOverflow(t *testing.T) {
	rec := httptest.NewRecorder()
	upstream := "ContextWindowExceededError: maximum context length is 12345 tokens; prompt has 12346"

	writeClaudeError(rec, http.StatusInternalServerError, upstream)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	errPayload, _ := payload["error"].(map[string]any)
	if got := errPayload["type"]; got != "invalid_request_error" {
		t.Fatalf("unexpected error type: %#v", got)
	}
	if got := errPayload["message"]; got != claudePromptTooLongMessage {
		t.Fatalf("unexpected error message: %#v", got)
	}
	if strings.Contains(rec.Body.String(), "12345") || strings.Contains(rec.Body.String(), "12346") {
		t.Fatalf("upstream context limit leaked into normalized response: %s", rec.Body.String())
	}
}

func TestClaudeStreamErrorNormalizesContextOverflow(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := &claudeStreamRuntime{w: rec}
	upstream := "litellm.BadRequestError: ContextWindowExceededError: maximum context length is 200000 tokens"

	runtime.sendError(upstream)

	frames := parseClaudeFrames(t, rec.Body.String())
	errFrames := findClaudeFrames(frames, "error")
	if len(errFrames) != 1 {
		t.Fatalf("expected one error frame, got %d body=%s", len(errFrames), rec.Body.String())
	}
	errPayload, _ := errFrames[0].Payload["error"].(map[string]any)
	if got := errPayload["type"]; got != "invalid_request_error" {
		t.Fatalf("unexpected stream error type: %#v body=%s", got, rec.Body.String())
	}
	if got := errPayload["message"]; got != claudePromptTooLongMessage {
		t.Fatalf("unexpected stream error message: %#v body=%s", got, rec.Body.String())
	}
	if got := errPayload["code"]; got != "invalid_request" {
		t.Fatalf("unexpected stream error code: %#v body=%s", got, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "200000") {
		t.Fatalf("upstream context limit leaked into normalized stream response: %s", rec.Body.String())
	}
}
