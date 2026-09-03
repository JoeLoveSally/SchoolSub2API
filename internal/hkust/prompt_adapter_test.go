package hkust

import (
	"strings"
	"testing"
)

func TestAdaptPromptForWebChatLeavesPlainPromptAlone(t *testing.T) {
	const prompt = "hello"
	if got := adaptPromptForWebChat(prompt); got != prompt {
		t.Fatalf("adaptPromptForWebChat() = %q, want %q", got, prompt)
	}
}

func TestAdaptPromptForWebChatRemovesDeepSeekMarkersAndPreservesDSML(t *testing.T) {
	prompt := "<|begin▁of▁sentence|>" +
		"<|System|>Working directory: /home/jianqiao/workspace/repo<|end▁of▁instructions|>" +
		"<|User|>read config" +
		"<|Assistant|><|DSML|tool_calls>\n<|DSML|invoke name=\"Read\">\n<|DSML|parameter name=\"file_path\"><![CDATA[src/config/config.yaml]]></|DSML|parameter>\n</|DSML|invoke>\n</|DSML|tool_calls><|end▁of▁sentence|>" +
		"<|Tool|>context_window: 65536<|end▁of▁toolresults|>" +
		"<|Assistant|>"

	got := adaptPromptForWebChat(prompt)
	for _, marker := range deepSeekPromptMarkers {
		if strings.Contains(got, marker) {
			t.Fatalf("adapted prompt still contains internal marker %q: %q", marker, got)
		}
	}
	if !strings.Contains(got, "Working directory: /home/jianqiao/workspace/repo") {
		t.Fatalf("system content was lost: %q", got)
	}
	if !strings.Contains(got, "[TOOL RESULT]\ncontext_window: 65536\n[/TOOL RESULT]") {
		t.Fatalf("tool result was not preserved as plain transcript: %q", got)
	}
	if !strings.Contains(got, "<|DSML|tool_calls>") || !strings.Contains(got, "src/config/config.yaml") {
		t.Fatalf("DSML tool history was not preserved: %q", got)
	}
	if !strings.HasPrefix(got, webChatTranscriptPreamble) {
		t.Fatalf("missing web-chat transcript preamble: %q", got)
	}
}

func TestProtocolBoundaryFilterStopsReplayAcrossChunks(t *testing.T) {
	filter := protocolBoundaryFilter{}
	if got := filter.Feed("valid answer<|end▁of"); got != "valid answer" {
		t.Fatalf("first chunk = %q", got)
	}
	if got := filter.Feed("▁sentence|><|Tool|>replayed tool output"); got != "" {
		t.Fatalf("second chunk = %q, want empty", got)
	}
	if !filter.stopped {
		t.Fatal("expected protocol filter to stop after internal boundary")
	}
	if got := filter.Feed("more replay"); got != "" {
		t.Fatalf("content after stop = %q", got)
	}
	if got := filter.Flush(); got != "" {
		t.Fatalf("flush after stop = %q", got)
	}
}

func TestProtocolBoundaryFilterDoesNotBlockDSML(t *testing.T) {
	filter := protocolBoundaryFilter{}
	const dsml = "<|DSML|tool_calls><|DSML|invoke name=\"Read\"></|DSML|invoke></|DSML|tool_calls>"
	if got := filter.Feed(dsml); got != dsml {
		t.Fatalf("DSML changed: %q", got)
	}
	if filter.stopped {
		t.Fatal("DSML must not be treated as an internal transcript boundary")
	}
}
