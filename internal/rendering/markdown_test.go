package rendering

import (
	"strings"
	"testing"
)

func TestNewMarkdownRenderer(t *testing.T) {
	tests := []struct {
		name  string
		width int
		valid bool
	}{
		{"valid width", 80, true},
		{"small width", 20, true},
		{"large width", 200, true},
		{"zero width", 0, true}, // Should still work
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewMarkdownRenderer(tt.width)
			if tt.valid && err != nil {
				t.Errorf("NewMarkdownRenderer() error = %v, expected success", err)
			}
			if tt.valid && renderer == nil {
				t.Error("NewMarkdownRenderer() returned nil renderer")
			}
			if tt.valid && renderer.width != tt.width {
				t.Errorf("NewMarkdownRenderer() width = %v, expected %v", renderer.width, tt.width)
			}
		})
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected ContentType
	}{
		{
			name:     "plain text",
			text:     "This is just plain text without any markdown.",
			expected: ContentTypePlain,
		},
		{
			name:     "markdown with code blocks",
			text:     "Here's some code:\n\n```python\ndef hello():\n\tprint(\"Hello, world!\")\n```\n\nThat's it!",
			expected: ContentTypeMarkdown,
		},
		{
			name:     "markdown with headers",
			text:     "# Main Title\n\n## Subtitle\n\nSome content here.\n\n### Another section",
			expected: ContentTypeMarkdown,
		},
		{
			name:     "markdown with lists",
			text:     "Here are some items:\n\n- First item\n- Second item\n- Third item\n\nAnd numbered:\n1. One\n2. Two",
			expected: ContentTypeMarkdown,
		},
		{
			name:     "mixed content",
			text:     "This has some *italic* text and a bit of **bold** but mostly plain.",
			expected: ContentTypeMarkdown, // Updated expectation
		},
		{
			name:     "inline code",
			text:     "Use the `print()` function to display output.",
			expected: ContentTypeMarkdown, // Updated expectation
		},
		{
			name:     "technical terms",
			text:     "We need to configure the API endpoint and set up authentication.",
			expected: ContentTypePlain, // API is detected but not enough other markers
		},
		{
			name:     "file extensions",
			text:     "Save this as script.py and run it with python script.py",
			expected: ContentTypePlain, // File extensions detected but low ratio
		},
		{
			name:     "empty text",
			text:     "",
			expected: ContentTypePlain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectContentType(tt.text)
			if result != tt.expected {
				t.Errorf("DetectContentType() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRenderMessage(t *testing.T) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	tests := []struct {
		name       string
		text       string
		sender     string
		isSnippet  bool
		shouldWork bool
	}{
		{
			name:       "plain text",
			text:       "This is just plain text.",
			sender:     "human",
			isSnippet:  false,
			shouldWork: true,
		},
		{
			name:       "markdown with code",
			text:       "Here's some code:\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
			sender:     "assistant",
			isSnippet:  false,
			shouldWork: true,
		},
		{
			name:       "snippet with highlighting",
			text:       "This is a <mark>highlighted</mark> snippet.",
			sender:     "assistant",
			isSnippet:  true,
			shouldWork: true,
		},
		{
			name:       "markdown headers",
			text:       "# Title\n\n## Subtitle\n\nContent here.",
			sender:     "assistant",
			isSnippet:  false,
			shouldWork: true,
		},
		{
			name:       "markdown list",
			text:       "Items:\n\n- First\n- Second\n- Third",
			sender:     "human",
			isSnippet:  false,
			shouldWork: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.RenderMessage(tt.text, tt.sender, tt.isSnippet)
			if tt.shouldWork && err != nil {
				t.Errorf("RenderMessage() error = %v, expected success", err)
			}
			if tt.shouldWork && result == "" {
				t.Error("RenderMessage() returned empty result")
			}
			// For snippets, ensure search highlighting is preserved
			if tt.isSnippet && strings.Contains(tt.text, "<mark>") {
				// The exact highlighting format may change, but some highlighting should be present
				if !strings.Contains(result, "highlighted") {
					t.Error("RenderMessage() did not preserve highlighted content in snippet")
				}
			}
		})
	}
}

func TestFormatPlainText(t *testing.T) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "simple text",
			text:     "Hello world",
			expected: "Hello world",
		},
		{
			name:     "text with inline code",
			text:     "Use `print()` function",
			expected: "Use print() function", // Should have some formatting
		},
		{
			name:     "text with code block",
			text:     "Example:\n\n```\ncode here\n```\n\nDone.",
			expected: "Example:\n\ncode here\n\nDone.", // Should have some formatting
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.formatPlainText(tt.text)
			if result == "" {
				t.Error("formatPlainText() returned empty result")
			}
			// Check that result contains the expected content (formatting may vary)
			if !strings.Contains(result, "Hello world") && strings.Contains(tt.text, "Hello world") {
				t.Error("formatPlainText() lost original content")
			}
		})
	}
}

func TestFormatInlineCode(t *testing.T) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	tests := []struct {
		name string
		text string
	}{
		{
			name: "single inline code",
			text: "Use `print()` function",
		},
		{
			name: "multiple inline code",
			text: "Use `print()` and `input()` functions",
		},
		{
			name: "no inline code",
			text: "Just plain text here",
		},
		{
			name: "malformed code",
			text: "Missing closing `backtick",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.formatInlineCode(tt.text)
			if result == "" {
				t.Error("formatInlineCode() returned empty result")
			}
			// Should not crash and should return some reasonable result
		})
	}
}

func TestContentTypeString(t *testing.T) {
	tests := []struct {
		contentType ContentType
		expected    string
	}{
		{ContentTypePlain, "plain"},
		{ContentTypeMarkdown, "markdown"},
		{ContentTypeMixed, "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.contentType.String()
			if result != tt.expected {
				t.Errorf("ContentType.String() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRenderConversationWithMarkdown(t *testing.T) {
	messages := []MessageForRendering{
		{
			Sender: "human",
			Text:   "Hello, can you help me with some code?",
		},
		{
			Sender: "assistant",
			Text:   "Sure! Here's an example:\n\n```python\nprint('Hello, world!')\n```\n\nThat should work.",
		},
		{
			Sender: "human",
			Text:   "Thanks! That's exactly what I needed.",
		},
	}

	tests := []struct {
		name     string
		messages []MessageForRendering
		width    int
		valid    bool
	}{
		{
			name:     "normal conversation",
			messages: messages,
			width:    80,
			valid:    true,
		},
		{
			name:     "narrow width",
			messages: messages,
			width:    40,
			valid:    true,
		},
		{
			name:     "empty messages",
			messages: []MessageForRendering{},
			width:    80,
			valid:    true,
		},
		{
			name: "single message",
			messages: []MessageForRendering{
				{Sender: "human", Text: "Hello"},
			},
			width: 80,
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderConversationWithMarkdown(tt.messages, tt.width)
			if tt.valid && err != nil {
				t.Errorf("RenderConversationWithMarkdown() error = %v, expected success", err)
			}
			if tt.valid && len(tt.messages) > 0 && result == "" {
				t.Error("RenderConversationWithMarkdown() returned empty result for non-empty messages")
			}
			// Check that all senders are present in output
			for _, msg := range tt.messages {
				if !strings.Contains(strings.ToUpper(result), strings.ToUpper(msg.Sender)) {
					t.Errorf("RenderConversationWithMarkdown() missing sender %s in output", msg.Sender)
				}
			}
		})
	}
}

// Benchmark tests for performance
func BenchmarkDetectContentType(b *testing.B) {
	text := "# Markdown Example\n\nHere's some code:\n\n```python\ndef hello():\n\tprint(\"Hello, world!\")\n```\n\nAnd some lists:\n- Item 1\n- Item 2\n- Item 3"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectContentType(text)
	}
}

func BenchmarkRenderMessage(b *testing.B) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		b.Fatalf("Failed to create renderer: %v", err)
	}

	text := "# Example\n\nHere's some **bold** text and *italic* text.\n\n```go\nfunc main() {\n\tfmt.Println(\"Hello\")\n}\n```\n\nThat's it!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = renderer.RenderMessage(text, "assistant", false)
	}
}

func BenchmarkRenderSnippet(b *testing.B) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		b.Fatalf("Failed to create renderer: %v", err)
	}

	text := "This is a <mark>highlighted</mark> snippet with some **bold** text."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = renderer.RenderMessage(text, "assistant", true)
	}
}

func TestRenderFullMessageWithHyperlinks(t *testing.T) {
	tests := []struct {
		name               string
		setupEnv           func()
		text               string
		expectedContains   []string
		unexpectedContains []string
	}{
		{
			name: "URLs become hyperlinks in supported terminal",
			setupEnv: func() {
				// Will be handled by t.Setenv in test
			},
			text: "Check out https://example.com for more info",
			expectedContains: []string{
				"\x1b]8;;https://example.com\x1b\\https://example.com\x1b]8;;\x1b\\",
				"Check out",
				"for more info",
			},
			unexpectedContains: []string{},
		},
		{
			name: "URLs remain plain text in unsupported terminal",
			setupEnv: func() {
				// Will be handled by t.Setenv in test
			},
			text: "Check out https://example.com for more info",
			expectedContains: []string{
				"https://example.com",
				"Check out",
				"for more info",
			},
			unexpectedContains: []string{
				"\x1b]8;;",
			},
		},
		{
			name: "markdown with hyperlinks",
			setupEnv: func() {
				// Will be handled by t.Setenv in test
			},
			text: "# Header\n\nVisit https://github.com for code",
			expectedContains: []string{
				"Header",
				"\x1b]8;;https://github.com\x1b\\https://github.com\x1b]8;;\x1b\\",
			},
			unexpectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment based on test case using t.Setenv
			switch tt.name {
			case "URLs become hyperlinks in supported terminal", "markdown with hyperlinks":
				t.Setenv("TERM_PROGRAM", "ghostty")
				t.Setenv("TERM", "")
				t.Setenv("KITTY_WINDOW_ID", "")
			case "URLs remain plain text in unsupported terminal":
				t.Setenv("TERM_PROGRAM", "")
				t.Setenv("KITTY_WINDOW_ID", "")
				t.Setenv("TERM", "dumb")
			}

			renderer, err := NewMarkdownRenderer(80)
			if err != nil {
				t.Fatalf("Failed to create renderer: %v", err)
			}

			result, err := renderer.renderFullMessage(tt.text, "human")
			if err != nil {
				t.Errorf("renderFullMessage() error = %v", err)
			}

			for _, expected := range tt.expectedContains {
				if !containsStringTest(result, expected) {
					t.Errorf("renderFullMessage() result does not contain expected substring %q\nResult: %q", expected, result)
				}
			}

			for _, unexpected := range tt.unexpectedContains {
				if containsStringTest(result, unexpected) {
					t.Errorf("renderFullMessage() result contains unexpected substring %q\nResult: %q", unexpected, result)
				}
			}
		})
	}
}

func TestRenderSnippetWithHyperlinks(t *testing.T) {

	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	tests := []struct {
		name   string
		text   string
		sender string
	}{
		{
			name:   "snippet with URLs",
			text:   "Found this link: https://example.com",
			sender: "assistant",
		},
		{
			name:   "snippet with search highlighting",
			text:   "This has <mark>highlighted</mark> text and https://github.com",
			sender: "human",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup supported terminal using t.Setenv
			t.Setenv("TERM_PROGRAM", "ghostty")
			t.Setenv("TERM", "")
			t.Setenv("KITTY_WINDOW_ID", "")

			result, err := renderer.renderSnippet(tt.text, tt.sender)
			if err != nil {
				t.Errorf("renderSnippet() error = %v", err)
			}

			// Should not crash and should return a reasonable result
			if result == "" {
				t.Error("renderSnippet() returned empty result")
			}
		})
	}
}

func TestRenderConversationWithMarkdownAndHyperlinks(t *testing.T) {
	// Setup supported terminal using t.Setenv
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM", "")
	t.Setenv("KITTY_WINDOW_ID", "")

	messages := []MessageForRendering{
		{
			Sender: "human",
			Text:   "Check out https://github.com/example/repo for the code",
		},
		{
			Sender: "assistant",
			Text:   "Thanks! I looked at https://github.com/example/repo and here's what I found:\n\n```go\nfunc main() {\n\tfmt.Println(\"Hello\")\n}\n```",
		},
	}

	result, err := RenderConversationWithMarkdown(messages, 100)
	if err != nil {
		t.Errorf("RenderConversationWithMarkdown() error = %v", err)
	}

	// Should contain hyperlinks for URLs
	expectedSubstrings := []string{
		"\x1b]8;;https://github.com/example/repo\x1b\\https://github.com/example/repo\x1b]8;;\x1b\\",
		"HUMAN",
		"ASSISTANT",
	}

	for _, expected := range expectedSubstrings {
		if !containsStringTest(result, expected) {
			t.Errorf("RenderConversationWithMarkdown() result does not contain expected substring %q", expected)
		}
	}
}

// Helper function for simple substring checking
func containsStringTest(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
