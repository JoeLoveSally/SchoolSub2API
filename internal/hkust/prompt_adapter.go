package hkust

import "strings"

const webChatTranscriptPreamble = "Continue the serialized conversation below as the assistant. Follow SYSTEM instructions and treat TOOL RESULT blocks as observations. Return only the next assistant response. Do not repeat transcript labels or tool results.\n\n"

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
	"<|Tool|>",
	"<|end▁of▁sentence|>",
	"<|end▁of▁toolresults|>",
	"<|end▁of▁instructions|>",
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
	replacer := strings.NewReplacer(
		"<|begin▁of▁sentence|>", "",
		"<|System|>", "\n[SYSTEM]\n",
		"<|end▁of▁instructions|>", "\n[/SYSTEM]\n",
		"<|User|>", "\n[USER]\n",
		"<|Assistant|>", "\n[ASSISTANT]\n",
		"<|Tool|>", "\n[TOOL RESULT]\n",
		"<|end▁of▁toolresults|>", "\n[/TOOL RESULT]\n",
		"<|end▁of▁sentence|>", "\n",
	)
	return webChatTranscriptPreamble + strings.TrimSpace(replacer.Replace(prompt))
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
// replaying DS2API's internal transcript protocol, content from the first hard
// boundary onward is discarded instead of being exposed as assistant text.
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
	keep := longestSuffixMatchingAnyPrefix(data, hardOutputProtocolBoundaries)
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
	first := -1
	for _, marker := range hardOutputProtocolBoundaries {
		if idx := strings.Index(data, marker); idx >= 0 && (first < 0 || idx < first) {
			first = idx
		}
	}
	return first
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
