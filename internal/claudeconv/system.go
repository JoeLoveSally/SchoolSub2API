package claudeconv

import (
	"fmt"
	"strings"
)

// NormalizeSystemContent converts Anthropic's top-level system field into plain
// text while preserving the text blocks Claude Code commonly sends. Claude Code
// may send system as either a string or an array of content blocks.
func NormalizeSystemContent(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", block["type"])))
			if typ != "text" {
				continue
			}
			text, _ := block["text"].(string)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	default:
		return ""
	}
}
