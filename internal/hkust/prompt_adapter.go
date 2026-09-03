package hkust

import "strings"

const (
	webChatTranscriptPreamble = "Continue the reconstructed conversation below as the assistant. The prose that introduces system instructions, user messages, prior assistant messages, and tool results is scaffolding only. Never reproduce those scaffolding phrases in the response. Treat tool results as observations, preserve any DSML tool-call syntax exactly when a tool call is required, and otherwise return only the next assistant response.\n\n"
	nextAssistantCue          = "Now produce only the next assistant response."
)

var deepSeekPromptMarkers = []string{
	"<|begin▁of▁sentence|>",
	"<|System|>",
	"<|User|>",
	"<|Assistant|>",
	"<|Tool|>",
	"<|end▁of▁sentence|>",
	"<|end▁of▁toolresults|>",
	"<|end▁of▁instructions|>",
}

var hardOutputProtocolBoundaries = []string{
	"<|begin▁of▁sentence|>",
	"<|System|>",
	"<|User|>",
	"<|Assistant|>",
	"<|Tool|>",
	"<|end▁of▁sentence|>",
	"<|end▁of▁toolresults|>",
	"<|end▁of▁instructions|>",
}

// softOutputReplayBoundaries cover both the first-generation bracket labels
// and the natural-language scaffolding used by adaptPromptForWebChat. They are
// only treated as boundaries at the start of an output line, which avoids
// truncating ordinary prose that happens to mention one of these phrases.
var softOutputReplayBoundaries = []string{
	"[SYSTEM]",
	"[/SYSTEM]",
	"[USER]",
	"[ASSISTANT]",
	"[TOOL RESULT]",
	"[/TOOL RESULT]",
	"System instructions follow:",
	"End of system instructions.",
	"User message follows:",
	"Prior assistant message follows:",
	"Result from the previously requested tool follows:",
	"End of tool result.",
	nextAssistantCue,
}

// adaptPromptForWebChat converts DS2API's DeepSeek chat-template markers into a
// plain transcript. The HKUST endpoint accepts one ordinary web-chat message,
// so sending raw model-control tokens there effectively double-templates the
// request and can cause the model to replay tool results or role markers.
// DSML tool-call syntax is intentionally left untouched.
func adaptPromptForWebChat(prompt string) string {
	if !containsDeepSeekPromptMarker(prompt) {
		return prompt
	}

	trimmed := strings.TrimSpace(prompt)
	endsWithAssistantCue := strings.HasSuffix(trimmed, "<|Assistant|>")
	if endsWithAssistantCue {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "<|Assistant|>"))
	}

	replacer := strings.NewReplacer(
		"<|begin▁of▁sentence|>", "",
		"<|System|>", "\nSystem instructions follow:\n",
		"<|end▁of▁instructions|>", "\nEnd of system instructions.\n",
		"<|User|>", "\nUser message follows:\n",
		"<|Assistant|>", "\nPrior assistant message follows:\n",
		"<|Tool|>", "\nResult from the previously requested tool follows:\n",
		"<|end▁of▁toolresults|>", "\nEnd of tool result.\n",
		"<|end▁of▁sentence|>", "\n",
	)
	adapted := strings.TrimSpace(replacer.Replace(trimmed))
	if endsWithAssistantCue {
		adapted += "\n\n" + nextAssistantCue
	}
	return webChatTranscriptPreamble + adapted
}

func containsDeepSeekPromptMarker(prompt string) bool {
	for _, marker := range deepSeekPromptMarkers {
		if strings.Contains(prompt, marker) {
			return true
		}
	}
	return false
}

// protocolBoundaryFilter is a defensive output guard. If the web model starts
// replaying DS2API's internal transcript protocol or the web-chat scaffolding,
// content from the first boundary onward is discarded instead of being exposed
// as assistant text.
type protocolBoundaryFilter struct {
	pending string
	stopped bool
}

func (f *protocolBoundaryFilter) Feed(chunk string) string {
	if f.stopped || chunk == "" {
		return ""
	}
	data := f.pending + chunk
	f.pending = ""
	if idx := firstProtocolBoundary(data); idx >= 0 {
		f.stopped = true
		return data[:idx]
	}
	keep := longestSuffixMatchingAnyPrefix(data, outputBoundaryPrefixes())
	if keep > 0 {
		f.pending = data[len(data)-keep:]
		data = data[:len(data)-keep]
	}
	return data
}

func (f *protocolBoundaryFilter) Flush() string {
	if f.stopped || f.pending == "" {
		f.pending = ""
		return ""
	}
	out := f.pending
	f.pending = ""
	return out
}

func firstProtocolBoundary(data string) int {
	first := firstSubstring(data, hardOutputProtocolBoundaries)
	if soft := firstLineBoundarySubstring(data, softOutputReplayBoundaries); soft >= 0 && (first < 0 || soft < first) {
		first = soft
	}
	return first
}

func firstSubstring(data string, markers []string) int {
	first := -1
	for _, marker := range markers {
		if idx := strings.Index(data, marker); idx >= 0 && (first < 0 || idx < first) {
			first = idx
		}
	}
	return first
}

func firstLineBoundarySubstring(data string, markers []string) int {
	first := -1
	for _, marker := range markers {
		start := 0
		for start < len(data) {
			rel := strings.Index(data[start:], marker)
			if rel < 0 {
				break
			}
			idx := start + rel
			if isOutputLineBoundary(data, idx) {
				if first < 0 || idx < first {
					first = idx
				}
				break
			}
			start = idx + 1
		}
	}
	return first
}

func isOutputLineBoundary(data string, idx int) bool {
	if idx <= 0 {
		return true
	}
	for i := idx - 1; i >= 0; i-- {
		switch data[i] {
		case ' ', '\t', '\r':
			continue
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}

func outputBoundaryPrefixes() []string {
	out := make([]string, 0, len(hardOutputProtocolBoundaries)+len(softOutputReplayBoundaries))
	out = append(out, hardOutputProtocolBoundaries...)
	out = append(out, softOutputReplayBoundaries...)
	return out
}

func longestSuffixMatchingAnyPrefix(data string, markers []string) int {
	best := 0
	for _, marker := range markers {
		max := len(marker) - 1
		if len(data) < max {
			max = len(data)
		}
		for n := max; n > best; n-- {
			if strings.HasSuffix(data, marker[:n]) {
				best = n
				break
			}
		}
	}
	return best
}
