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
	if !strings.Contains(got, "Result from the previously requested tool follows:\ncontext_window: 65536\nEnd of tool result.") {
		t.Fatalf("tool result was not preserved as natural transcript: %q", got)
	}
	if strings.Contains(got, "[TOOL RESULT]") || strings.Contains(got, "[SYSTEM]") {
		t.Fatalf("legacy bracket transcript labels must not be emitted: %q", got)
	}
	if !strings.Contains(got, "<|DSML|tool_calls>") || !strings.Contains(got, "src/config/config.yaml") {
		t.Fatalf("DSML tool history was not preserved: %q", got)
	}
	if !strings.HasSuffix(got, nextAssistantCue) {
		t.Fatalf("expected final assistant cue, got %q", got)
	}
	if !strings.HasPrefix(got, webChatTranscriptPreamble) {
		t.Fatalf("missing web-chat transcript preamble: %q", got)
	}
}

func TestAdaptPromptForWebChatPreservesBashCommandWhitespace(t *testing.T) {
	const command = "git checkout HEAD -- src/config/config.yaml"
	prompt := "<|begin▁of▁sentence|><|User|>restore the file" +
		"<|Assistant|><|DSML|tool_calls>\n<|DSML|invoke name=\"Bash\">\n" +
		"<|DSML|parameter name=\"command\"><![CDATA[" + command + "]]></|DSML|parameter>\n" +
		"</|DSML|invoke>\n</|DSML|tool_calls><|end▁of▁sentence|><|Assistant|>"

	got := adaptPromptForWebChat(prompt)
	if !strings.Contains(got, "<![CDATA["+command+"]]>") {
		t.Fatalf("bash command whitespace changed during HKUST prompt adaptation: %q", got)
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

func TestProtocolBoundaryFilterStopsLegacyToolResultReplayAcrossChunks(t *testing.T) {
	filter := protocolBoundaryFilter{}
	if got := filter.Feed("normal assistant text\n[TOOL RES"); got != "normal assistant text\n" {
		t.Fatalf("first chunk = %q", got)
	}
	if got := filter.Feed("ULT]\n34: face_session_reset_enabled: true"); got != "" {
		t.Fatalf("second chunk = %q, want empty", got)
	}
	if !filter.stopped {
		t.Fatal("expected legacy transcript replay to stop output")
	}
}

func TestProtocolBoundaryFilterStopsNaturalScaffoldingReplay(t *testing.T) {
	filter := protocolBoundaryFilter{}
	if got := filter.Feed("answer\nResult from the previously requested tool follows:\nsecret"); got != "answer\n" {
		t.Fatalf("filtered output = %q", got)
	}
	if !filter.stopped {
		t.Fatal("expected natural transcript scaffolding to stop output")
	}
}

func TestProtocolBoundaryFilterAllowsScaffoldingPhraseInsideNormalSentence(t *testing.T) {
	filter := protocolBoundaryFilter{}
	const text = "The phrase Result from the previously requested tool follows: is documentation text."
	if got := filter.Feed(text); got != text {
		t.Fatalf("ordinary inline prose was filtered: %q", got)
	}
	if filter.stopped {
		t.Fatal("inline scaffolding phrase must not be treated as replay")
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
