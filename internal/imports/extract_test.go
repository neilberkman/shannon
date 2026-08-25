package imports

import (
	"strings"
	"testing"

	"github.com/neilberkman/shannon/internal/models"
)

// blocksFrom is a helper for building content blocks from raw JSON fragments,
// matching the shapes Claude's export actually emits.
func TestSearchTextIndexesThinkingBlocks(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text: "Short visible answer.",
		Content: []models.ClaudeMessageContent{
			{Type: "thinking", Thinking: "The 83(b) deadline is 30 days from grant."},
			{Type: "text", Text: "Short visible answer."},
		},
	}

	got := SearchText(msg)
	if !strings.Contains(got, "83(b)") {
		t.Errorf("thinking block not indexed; got %q", got)
	}
	if !strings.Contains(got, "Short visible answer.") {
		t.Errorf("top-level text missing; got %q", got)
	}
	if strings.Count(got, "Short visible answer.") != 1 {
		t.Errorf("duplicate text block should be deduped; got %q", got)
	}
}

func TestSearchTextIndexesNestedToolResults(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text: "",
		Content: []models.ClaudeMessageContent{
			{
				Type:    "tool_use",
				Name:    "conversation_search",
				Input:   []byte(`{"query":"83(b) election"}`),
				Content: nil,
			},
			{
				Type:    "tool_result",
				Name:    "conversation_search",
				Content: []byte(`[{"type":"text","text":"Mirala NSO exercise, transfer date Feb 27"}]`),
			},
		},
	}

	got := SearchText(msg)
	for _, want := range []string{"conversation_search", "83(b) election", "Mirala NSO exercise"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in search text; got %q", want, got)
		}
	}
}

func TestSearchTextIndexesKnowledgeBlocks(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Content: []models.ClaudeMessageContent{
			{
				Type: "tool_result",
				Name: "web_search",
				Content: []byte(`[{"type":"knowledge",
					"title":"IRS Form 15620",
					"url":"https://irs.gov/pub/irs-pdf/f15620.pdf",
					"text":"Section 83(b) election form"}]`),
			},
		},
	}

	got := SearchText(msg)
	for _, want := range []string{"IRS Form 15620", "irs.gov", "Section 83(b) election form"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in search text; got %q", want, got)
		}
	}
}

func TestSearchTextKeepsEveryTextBlock(t *testing.T) {
	// The previous implementation stopped at the first text block, losing
	// everything the model said after a tool call.
	msg := &models.ClaudeChatMessage{
		Content: []models.ClaudeMessageContent{
			{Type: "text", Text: "First part before the tool call."},
			{Type: "tool_use", Name: "search"},
			{Type: "text", Text: "Second part after the tool call."},
		},
	}

	got := SearchText(msg)
	if !strings.Contains(got, "First part") || !strings.Contains(got, "Second part") {
		t.Errorf("both text blocks should be indexed; got %q", got)
	}
}

func TestSearchTextIndexesAttachments(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text: "see attached",
		Attachments: []models.ClaudeAttachment{
			{FileName: "grant.pdf", ExtractedContent: "Participation Threshold $49,028.16"},
		},
	}

	got := SearchText(msg)
	for _, want := range []string{"grant.pdf", "Participation Threshold"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in search text; got %q", want, got)
		}
	}
}

func TestSearchTextToleratesUnexpectedShapes(t *testing.T) {
	// Shape drift in a single block must not lose the rest of the message.
	msg := &models.ClaudeChatMessage{
		Content: []models.ClaudeMessageContent{
			{Type: "tool_result", Content: []byte(`"a bare string, not a block list"`)},
			{Type: "tool_use", Input: []byte(`["not","an","object"]`)},
			{Type: "text", Text: "still indexed"},
		},
	}

	got := SearchText(msg)
	if !strings.Contains(got, "still indexed") {
		t.Errorf("unexpected block shapes should not drop later content; got %q", got)
	}
}

func TestDisplayTextExcludesThinkingAndToolOutput(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text: "The deadline is September 11.",
		Content: []models.ClaudeMessageContent{
			{Type: "thinking", Thinking: "internal reasoning that should stay hidden"},
			{Type: "text", Text: "The deadline is September 11."},
			{Type: "tool_result", Content: []byte(`[{"type":"knowledge","text":"raw web page body"}]`)},
		},
	}

	got := DisplayText(msg)
	if got != "The deadline is September 11." {
		t.Errorf("display text should be the visible prose only; got %q", got)
	}
}

func TestDisplayTextFallsBackToTextBlocks(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text: "",
		Content: []models.ClaudeMessageContent{
			{Type: "thinking", Thinking: "hidden"},
			{Type: "text", Text: "first"},
			{Type: "text", Text: "second"},
		},
	}

	got := DisplayText(msg)
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("fallback should join all text blocks; got %q", got)
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("thinking must not reach display text; got %q", got)
	}
}

func TestSearchTextIndexesUploadedFileNames(t *testing.T) {
	// A message whose author uploaded documents without typing anything has
	// no text of any kind; the file names are its only searchable trace.
	msg := &models.ClaudeChatMessage{
		Text: "",
		Content: []models.ClaudeMessageContent{
			{Type: "text", Text: ""},
		},
		Files: []models.ClaudeFile{
			{FileUUID: "eb6fad8a", FileName: "Grant Agreement - Orion Nebula Group II - Xuku LLC.pdf"},
			{FileUUID: "d1aa35b2", FileName: "LLC_Agreement_-_Orion_Nebula_Group_II-FINAL.pdf"},
		},
	}

	got := SearchText(msg)
	for _, want := range []string{"Grant Agreement", "Orion Nebula Group II", "Xuku LLC", "LLC_Agreement"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in search text; got %q", want, got)
		}
	}
}

func TestDisplayTextIgnoresUploadedFileNames(t *testing.T) {
	msg := &models.ClaudeChatMessage{
		Text:  "here you go",
		Files: []models.ClaudeFile{{FileName: "secret-plan.pdf"}},
	}

	if got := DisplayText(msg); got != "here you go" {
		t.Errorf("file names must not enter display text; got %q", got)
	}
}
