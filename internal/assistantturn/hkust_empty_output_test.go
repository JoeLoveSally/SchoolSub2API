package assistantturn

import (
	"net/http"
	"testing"

	"ds2api/internal/sse"
)

func TestHKUSTThinkingOnlyIsBadGatewayAndNotRetryable(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "test-token")
	t.Setenv("HKUST_USE_API", "test-use-api")

	turn := BuildTurnFromCollected(sse.CollectResult{Thinking: "reasoning only"}, BuildOptions{})
	if turn.Error == nil {
		t.Fatal("expected reasoning-only error")
	}
	if turn.Error.Status != http.StatusBadGateway {
		t.Fatalf("expected 502 for HKUST reasoning-only output, got %d", turn.Error.Status)
	}
	if turn.Error.Code != "upstream_empty_output" {
		t.Fatalf("unexpected error code: %q", turn.Error.Code)
	}
	if ShouldRetryEmptyOutput(turn, 0, 1) {
		t.Fatal("reasoning-only output must not trigger synthetic retry")
	}
}

func TestPureEmptyOutputRemainsRetryable(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{}, BuildOptions{})
	if !ShouldRetryEmptyOutput(turn, 0, 1) {
		t.Fatal("genuinely empty output should remain retryable")
	}
}
