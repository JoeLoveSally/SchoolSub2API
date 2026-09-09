package main

import "testing"

func TestParseTargets(t *testing.T) {
	got, err := parseTargets("70000, 96000,128000")
	if err != nil {
		t.Fatalf("parseTargets failed: %v", err)
	}
	want := []int{70000, 96000, 128000}
	if len(got) != len(want) {
		t.Fatalf("unexpected target count: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target[%d]=%d want %d", i, got[i], want[i])
		}
	}
}

func TestParseTargetsRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "0", "70000,nope", "-1"} {
		if _, err := parseTargets(input); err == nil {
			t.Fatalf("parseTargets(%q) unexpectedly succeeded", input)
		}
	}
}

func TestContextOverflowExtractsDynamicLimit(t *testing.T) {
	message := "litellm.BadRequestError: ContextWindowExceededError: This model's maximum context length is 65,535 tokens."
	limit, ok := contextOverflow(message)
	if !ok {
		t.Fatal("expected context overflow to be detected")
	}
	if limit != 65535 {
		t.Fatalf("unexpected limit: got %d want %d", limit, 65535)
	}
}

func TestContextOverflowDoesNotDependOnSpecificLimit(t *testing.T) {
	message := "maximum context length is 262144 tokens; request exceeds the context window"
	limit, ok := contextOverflow(message)
	if !ok {
		t.Fatal("expected context overflow to be detected")
	}
	if limit != 262144 {
		t.Fatalf("unexpected limit: got %d want %d", limit, 262144)
	}
}

func TestContextOverflowRecognizesPromptTooLongWithoutLimit(t *testing.T) {
	limit, ok := contextOverflow("prompt is too long")
	if !ok {
		t.Fatal("expected prompt-too-long error to be detected")
	}
	if limit != 0 {
		t.Fatalf("unexpected parsed limit: %d", limit)
	}
}

func TestContextOverflowIgnoresOrdinaryErrors(t *testing.T) {
	if limit, ok := contextOverflow("upstream websocket disconnected"); ok || limit != 0 {
		t.Fatalf("unexpected overflow detection: ok=%v limit=%d", ok, limit)
	}
}

func TestDefaultModelsSkipGPTUnlessRequested(t *testing.T) {
	withoutGPT := defaultModels(false)
	for _, model := range withoutGPT {
		if model.name == "GPT-5.6" {
			t.Fatal("GPT-5.6 should be opt-in")
		}
	}

	withGPT := defaultModels(true)
	found := false
	for _, model := range withGPT {
		if model.name == "GPT-5.6" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("GPT-5.6 should be included when requested")
	}
}
