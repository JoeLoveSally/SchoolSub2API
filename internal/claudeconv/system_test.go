package claudeconv

import "testing"

func TestNormalizeSystemContentString(t *testing.T) {
	got := NormalizeSystemContent("  cwd=/work/repo  ")
	if got != "cwd=/work/repo" {
		t.Fatalf("NormalizeSystemContent() = %q", got)
	}
}

func TestNormalizeSystemContentBlocks(t *testing.T) {
	got := NormalizeSystemContent([]any{
		map[string]any{"type": "text", "text": "You are Claude Code.", "cache_control": map[string]any{"type": "ephemeral"}},
		map[string]any{"type": "text", "text": "Working directory: /home/jianqiao/workspace/repo"},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64"}},
	})
	want := "You are Claude Code.\n\nWorking directory: /home/jianqiao/workspace/repo"
	if got != want {
		t.Fatalf("NormalizeSystemContent() = %q, want %q", got, want)
	}
}
