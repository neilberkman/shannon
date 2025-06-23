package artifacts

import (
	"reflect"
	"testing"

	"github.com/neilberkman/shannon/internal/models"
)

func TestExtractFromMessage(t *testing.T) {
	extractor := NewExtractor()

	tests := []struct {
		name     string
		message  models.Message
		expected []*Artifact
	}{
		{
			name: "single python code artifact",
			message: models.Message{
				ID:             123,
				ConversationID: 456,
				Sender:         "assistant",
				Text: `Here's a Python script to process your data:

<antArtifact identifier="data-processor" type="application/vnd.ant.code" language="python" title="Data Processing Script">
import pandas as pd
import numpy as np

def process_data(filename):
    df = pd.read_csv(filename)
    return df.describe()
</antArtifact>

This script loads a CSV file and returns basic statistics.`,
			},
			expected: []*Artifact{
				{
					ID:       "data-processor",
					Type:     TypeCode,
					Language: "python",
					Title:    "Data Processing Script",
					Content: `import pandas as pd
import numpy as np

def process_data(filename):
    df = pd.read_csv(filename)
    return df.describe()`,
					MessageID:      123,
					ConversationID: 456,
				},
			},
		},
		{
			name: "multiple artifacts in one message",
			message: models.Message{
				ID:             789,
				ConversationID: 101,
				Sender:         "assistant",
				Text: `I'll create both a README and the implementation:

<antArtifact identifier="readme" type="text/markdown" title="Project README">
# Data Processor

A simple tool for processing CSV files.

## Usage

Run the script with: python process.py data.csv
</antArtifact>

And here's the implementation:

<antArtifact identifier="impl" type="application/vnd.ant.code" language="python" title="process.py">
import sys
import pandas as pd

if __name__ == "__main__":
    process_data(sys.argv[1])
</antArtifact>`,
			},
			expected: []*Artifact{
				{
					ID:    "readme",
					Type:  TypeMarkdown,
					Title: "Project README",
					Content: `# Data Processor

A simple tool for processing CSV files.

## Usage

Run the script with: python process.py data.csv`,
					MessageID:      789,
					ConversationID: 101,
				},
				{
					ID:       "impl",
					Type:     TypeCode,
					Language: "python",
					Title:    "process.py",
					Content: `import sys
import pandas as pd

if __name__ == "__main__":
    process_data(sys.argv[1])`,
					MessageID:      789,
					ConversationID: 101,
				},
			},
		},
		{
			name: "SVG artifact",
			message: models.Message{
				ID:             111,
				ConversationID: 222,
				Sender:         "assistant",
				Text: `Here's a simple diagram:

<antArtifact identifier="diagram" type="image/svg+xml" title="System Architecture">
<svg width="200" height="100">
  <rect width="200" height="100" style="fill:rgb(0,0,255);stroke-width:3;stroke:rgb(0,0,0)" />
</svg>
</antArtifact>`,
			},
			expected: []*Artifact{
				{
					ID:    "diagram",
					Type:  TypeSVG,
					Title: "System Architecture",
					Content: `<svg width="200" height="100">
  <rect width="200" height="100" style="fill:rgb(0,0,255);stroke-width:3;stroke:rgb(0,0,0)" />
</svg>`,
					MessageID:      111,
					ConversationID: 222,
				},
			},
		},
		{
			name: "human message (no artifacts)",
			message: models.Message{
				ID:             333,
				ConversationID: 444,
				Sender:         "human",
				Text:           "Can you help me write a Python script?",
			},
			expected: nil,
		},
		{
			name: "assistant message without artifacts",
			message: models.Message{
				ID:             555,
				ConversationID: 666,
				Sender:         "assistant",
				Text:           "I'd be happy to help you write a Python script. What would you like it to do?",
			},
			expected: []*Artifact{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts, err := extractor.ExtractFromMessage(&tt.message)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(artifacts) != len(tt.expected) {
				t.Fatalf("expected %d artifacts, got %d", len(tt.expected), len(artifacts))
			}

			for i, artifact := range artifacts {
				expected := tt.expected[i]
				if !compareArtifacts(artifact, expected) {
					t.Errorf("artifact %d mismatch:\ngot:  %+v\nwant: %+v", i, artifact, expected)
				}
			}
		})
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		artifact Artifact
		expected string
	}{
		{
			name:     "python code",
			artifact: Artifact{Type: TypeCode, Language: "python"},
			expected: ".py",
		},
		{
			name:     "javascript code",
			artifact: Artifact{Type: TypeCode, Language: "javascript"},
			expected: ".js",
		},
		{
			name:     "markdown document",
			artifact: Artifact{Type: TypeMarkdown},
			expected: ".md",
		},
		{
			name:     "HTML document",
			artifact: Artifact{Type: TypeHTML},
			expected: ".html",
		},
		{
			name:     "SVG image",
			artifact: Artifact{Type: TypeSVG},
			expected: ".svg",
		},
		{
			name:     "React component",
			artifact: Artifact{Type: TypeReact},
			expected: ".jsx",
		},
		{
			name:     "Mermaid diagram",
			artifact: Artifact{Type: TypeMermaid},
			expected: ".mmd",
		},
		{
			name:     "unknown type",
			artifact: Artifact{Type: "unknown/type"},
			expected: ".txt",
		},
		{
			name:     "code with unknown language",
			artifact: Artifact{Type: TypeCode, Language: "unknown"},
			expected: ".txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.artifact.GetFileExtension()
			if got != tt.expected {
				t.Errorf("GetFileExtension() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetPreview(t *testing.T) {
	tests := []struct {
		name     string
		artifact Artifact
		maxLines int
		expected string
	}{
		{
			name: "short content",
			artifact: Artifact{
				Content: "Line 1\nLine 2\nLine 3",
			},
			maxLines: 5,
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name: "long content",
			artifact: Artifact{
				Content: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6",
			},
			maxLines: 3,
			expected: "Line 1\nLine 2\nLine 3\n... (3 more lines)",
		},
		{
			name: "exact match",
			artifact: Artifact{
				Content: "Line 1\nLine 2\nLine 3",
			},
			maxLines: 3,
			expected: "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.artifact.GetPreview(tt.maxLines)
			if got != tt.expected {
				t.Errorf("GetPreview() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseAttributes(t *testing.T) {
	extractor := NewExtractor()

	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "all attributes",
			input: `identifier="test-id" type="application/vnd.ant.code" language="python" title="Test Script"`,
			expected: map[string]string{
				"identifier": "test-id",
				"type":       "application/vnd.ant.code",
				"language":   "python",
				"title":      "Test Script",
			},
		},
		{
			name:  "minimal attributes",
			input: `identifier="simple" type="text/markdown"`,
			expected: map[string]string{
				"identifier": "simple",
				"type":       "text/markdown",
			},
		},
		{
			name:  "attributes with spaces",
			input: `identifier="my-id"   type="text/html"    title="My Document"`,
			expected: map[string]string{
				"identifier": "my-id",
				"type":       "text/html",
				"title":      "My Document",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractor.parseAttributes(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseAttributes() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Helper function to compare artifacts
func compareArtifacts(a, b *Artifact) bool {
	return a.ID == b.ID &&
		a.Type == b.Type &&
		a.Language == b.Language &&
		a.Title == b.Title &&
		a.Content == b.Content &&
		a.MessageID == b.MessageID &&
		a.ConversationID == b.ConversationID
}
