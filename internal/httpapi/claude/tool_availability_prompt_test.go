package claude

import (
	"strings"
	"testing"
)

func TestBuildClaudeToolPromptAdvertisesExactIdentifiers(t *testing.T) {
	tools := []any{
		map[string]any{
			"name":        "Bash",
			"description": "Run a shell command",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
			},
		},
		map[string]any{
			"name":        "Read",
			"description": "Read a file",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string"},
				},
			},
		},
	}

	prompt := buildClaudeToolPrompt(tools)
	if !strings.Contains(prompt, "AVAILABLE TOOL IDENTIFIERS (exact and case-sensitive): Bash, Read") {
		t.Fatalf("expected exact tool identifier list, got %q", prompt)
	}
	if !strings.Contains(prompt, "Never invent, rename, alias, or translate a tool identifier") {
		t.Fatalf("expected unavailable-tool guard, got %q", prompt)
	}
	if strings.Contains(prompt, "Tool: Grep") {
		t.Fatalf("prompt must not advertise an undeclared Grep tool: %q", prompt)
	}
}
