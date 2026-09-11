package shared

import (
	"os"
	"strings"
)

const EmptyOutputRetrySuffix = "Previous reply had no visible output. Please regenerate the visible final answer or tool call now."

func EmptyOutputRetryEnabled() bool {
	// The HKUST web-chat transport creates a fresh upstream conversation for
	// each completion and already returns a complete model turn. Synthetic
	// follow-up retries are both misleading for diagnostics and expensive when
	// the model has already spent a long time producing reasoning-only output.
	if strings.TrimSpace(os.Getenv("HKUST_TOKEN")) != "" ||
		strings.TrimSpace(os.Getenv("HKUST_USE_API")) != "" ||
		strings.TrimSpace(os.Getenv("HKUST_WS_URL")) != "" {
		return false
	}
	return true
}

func EmptyOutputRetryMaxAttempts() int {
	return 1
}

func ClonePayloadWithEmptyOutputRetryPrompt(payload map[string]any) map[string]any {
	return ClonePayloadForEmptyOutputRetry(payload, 0)
}

// ClonePayloadForEmptyOutputRetry creates a retry payload with the suffix
// appended and, if parentMessageID > 0, sets parent_message_id so the
// retry is submitted as a proper follow-up turn in the same DeepSeek
// session rather than a disconnected root message.
func ClonePayloadForEmptyOutputRetry(payload map[string]any, parentMessageID int) map[string]any {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	original, _ := payload["prompt"].(string)
	clone["prompt"] = AppendEmptyOutputRetrySuffix(original)
	if parentMessageID > 0 {
		clone["parent_message_id"] = parentMessageID
	}
	return clone
}

func AppendEmptyOutputRetrySuffix(prompt string) string {
	prompt = strings.TrimRight(prompt, "\r\n\t ")
	if prompt == "" {
		return EmptyOutputRetrySuffix
	}
	return prompt + "\n\n" + EmptyOutputRetrySuffix
}

func UsagePromptWithEmptyOutputRetry(originalPrompt string, retryAttempts int) string {
	if retryAttempts <= 0 {
		return originalPrompt
	}
	parts := make([]string, 0, retryAttempts+1)
	parts = append(parts, originalPrompt)
	next := originalPrompt
	for i := 0; i < retryAttempts; i++ {
		next = AppendEmptyOutputRetrySuffix(next)
		parts = append(parts, next)
	}
	return strings.Join(parts, "\n")
}
