package imports

import (
	"strings"

	"github.com/neilberkman/shannon/internal/models"
)

// maxBlockDepth bounds recursion through nested tool_result blocks. Real
// exports nest one level; the bound only guards against pathological input.
const maxBlockDepth = 4

// DisplayText returns the prose a reader should see for a message: the
// top-level text if present, otherwise the concatenated "text" blocks. It
// deliberately excludes extended thinking and tool results so that viewing a
// conversation shows what was actually said, not everything the model read.
func DisplayText(msg *models.ClaudeChatMessage) string {
	if strings.TrimSpace(msg.Text) != "" {
		return msg.Text
	}

	var b textCollector
	for i := range msg.Content {
		if msg.Content[i].Type == "text" {
			b.add(msg.Content[i].Text)
		}
	}
	return b.String()
}

// SearchText returns everything in a message worth indexing. Claude's export
// scatters prose across several block types, and for tool-using conversations
// the bulk of the content — search results, files read, extended thinking —
// never appears in the top-level "text" field at all. Indexing only that field
// silently hides those conversations from search.
func SearchText(msg *models.ClaudeChatMessage) string {
	var b textCollector
	b.add(msg.Text)

	for i := range msg.Content {
		collectBlock(&b, &msg.Content[i], 0)
	}

	for _, att := range msg.Attachments {
		b.add(att.FileName)
		b.add(att.ExtractedContent)
	}

	return b.String()
}

// collectBlock appends every searchable string in a content block.
func collectBlock(b *textCollector, block *models.ClaudeMessageContent, depth int) {
	if depth > maxBlockDepth {
		return
	}

	switch block.Type {
	case "thinking":
		b.add(block.Thinking)
	case "tool_use":
		b.add(block.Name)
		collectJSONStrings(b, block.ToolInput(), depth)
	case "tool_result":
		b.add(block.Name)
	case "knowledge":
		// A web page or document the model read: title, source, and body.
		b.add(block.Title)
		b.add(block.URL)
	case "local_resource":
		b.add(block.Name)
		b.add(block.FilePath)
	}

	// Every block type may carry a Text field; "knowledge" and "text" blocks
	// always do, and unknown future types likely will too.
	b.add(block.Text)

	for _, nested := range block.NestedContent() {
		collectBlock(b, &nested, depth+1)
	}
}

// collectJSONStrings walks a decoded JSON value and appends its string leaves.
// Tool inputs hold search queries and prompts that are worth finding later.
func collectJSONStrings(b *textCollector, v any, depth int) {
	if depth > maxBlockDepth {
		return
	}

	switch val := v.(type) {
	case string:
		b.add(val)
	case []any:
		for _, item := range val {
			collectJSONStrings(b, item, depth+1)
		}
	case map[string]any:
		for _, item := range val {
			collectJSONStrings(b, item, depth+1)
		}
	}
}

// textCollector accumulates text fragments, dropping empties and exact
// repeats. Repeats are common: a message's top-level Text usually duplicates
// its first text block verbatim.
type textCollector struct {
	parts []string
	seen  map[string]struct{}
}

func (b *textCollector) add(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	if b.seen == nil {
		b.seen = make(map[string]struct{})
	}
	if _, dup := b.seen[s]; dup {
		return
	}
	b.seen[s] = struct{}{}
	b.parts = append(b.parts, s)
}

func (b *textCollector) String() string {
	return strings.Join(b.parts, "\n\n")
}
