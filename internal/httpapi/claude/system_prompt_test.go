package claude

import (
	"strings"
	"testing"
)

func TestInjectClaudeToolPromptPreservesStructuredSystem(t *testing.T) {
	payload := map[string]any{
		"system": []any{
			map[string]any{"type": "text", "text": "You are Claude Code."},
			map[string]any{"type": "text", "text": "Working directory: /home/jianqiao/workspace/gclaw/gclaw-deepbrain-copy"},
		},
	}
	messages := []any{map[string]any{"role": "user", "content": "read the config"}}
	tools := []any{
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

	gotMessages := injectClaudeToolPrompt(payload, messages, tools)
	if len(gotMessages) != 1 {
		t.Fatalf("messages changed unexpectedly: %#v", gotMessages)
	}
	system, _ := payload["system"].(string)
	if !strings.Contains(system, "Working directory: /home/jianqiao/workspace/gclaw/gclaw-deepbrain-copy") {
		t.Fatalf("structured system cwd was lost: %q", system)
	}
	if !strings.Contains(system, "Tool: Read") || !strings.Contains(system, "<|DSML|tool_calls>") {
		t.Fatalf("tool instructions were not merged into system prompt: %q", system)
	}
}
