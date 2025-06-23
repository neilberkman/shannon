package artifacts

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neilberkman/shannon/internal/models"
)

// Artifact types that Claude supports
const (
	TypeCode     = "application/vnd.ant.code"
	TypeMarkdown = "text/markdown"
	TypeHTML     = "text/html"
	TypeSVG      = "image/svg+xml"
	TypeReact    = "application/vnd.ant.react"
	TypeMermaid  = "application/vnd.ant.mermaid"
)

// Artifact represents an extracted Claude artifact
type Artifact struct {
	ID             string
	Type           string
	Language       string // For code artifacts
	Title          string
	Content        string
	MessageID      int64
	ConversationID int64
}

// Extractor handles extracting artifacts from Claude messages
type Extractor struct {
	// ArtifactRegex matches artifact tags and content
	ArtifactRegex *regexp.Regexp
	// AttrRegex extracts attributes from the opening tag
	AttrRegex *regexp.Regexp
}

// NewExtractor creates a new artifact extractor
func NewExtractor() *Extractor {
	return &Extractor{
		// Matches <antArtifact...>content</antArtifact>
		ArtifactRegex: regexp.MustCompile(`(?s)<antArtifact\s+([^>]+)>(.*?)</antArtifact>`),
		// Matches individual attributes like identifier="value"
		AttrRegex: regexp.MustCompile(`(\w+)="([^"]+)"`),
	}
}

// ExtractFromMessage extracts all artifacts from a single message
func (e *Extractor) ExtractFromMessage(msg *models.Message) ([]*Artifact, error) {
	if msg.Sender != "assistant" {
		return nil, nil // Only assistant messages contain artifacts
	}

	var artifacts []*Artifact

	matches := e.ArtifactRegex.FindAllStringSubmatch(msg.Text, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		attrs := e.parseAttributes(match[1])
		content := strings.TrimSpace(match[2])

		artifact := &Artifact{
			ID:             attrs["identifier"],
			Type:           attrs["type"],
			Language:       attrs["language"],
			Title:          attrs["title"],
			Content:        content,
			MessageID:      msg.ID,
			ConversationID: msg.ConversationID,
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// ExtractFromConversation extracts all artifacts from a conversation
func (e *Extractor) ExtractFromConversation(conv *models.Conversation, messages []models.Message) ([]*Artifact, error) {
	var allArtifacts []*Artifact

	for i := range messages {
		artifacts, err := e.ExtractFromMessage(&messages[i])
		if err != nil {
			return nil, fmt.Errorf("failed to extract from message %d: %w", messages[i].ID, err)
		}
		allArtifacts = append(allArtifacts, artifacts...)
	}

	return allArtifacts, nil
}

// parseAttributes extracts key-value pairs from artifact tag attributes
func (e *Extractor) parseAttributes(attrString string) map[string]string {
	attrs := make(map[string]string)

	matches := e.AttrRegex.FindAllStringSubmatch(attrString, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			attrs[match[1]] = match[2]
		}
	}

	return attrs
}

// GetPreview returns a preview of the artifact content
func (a *Artifact) GetPreview(maxLines int) string {
	lines := strings.Split(a.Content, "\n")

	if len(lines) <= maxLines {
		return a.Content
	}

	preview := strings.Join(lines[:maxLines], "\n")
	return fmt.Sprintf("%s\n... (%d more lines)", preview, len(lines)-maxLines)
}

// GetFileExtension returns appropriate file extension based on artifact type/language
func (a *Artifact) GetFileExtension() string {
	switch a.Type {
	case TypeCode:
		return getLanguageExtension(a.Language)
	case TypeMarkdown:
		return ".md"
	case TypeHTML:
		return ".html"
	case TypeSVG:
		return ".svg"
	case TypeReact:
		return ".jsx"
	case TypeMermaid:
		return ".mmd"
	default:
		return ".txt"
	}
}

// getLanguageExtension maps programming languages to file extensions
func getLanguageExtension(language string) string {
	extensions := map[string]string{
		"python":     ".py",
		"javascript": ".js",
		"typescript": ".ts",
		"java":       ".java",
		"go":         ".go",
		"rust":       ".rs",
		"cpp":        ".cpp",
		"c":          ".c",
		"csharp":     ".cs",
		"ruby":       ".rb",
		"php":        ".php",
		"swift":      ".swift",
		"kotlin":     ".kt",
		"scala":      ".scala",
		"r":          ".r",
		"sql":        ".sql",
		"bash":       ".sh",
		"html":       ".html",
		"css":        ".css",
		"json":       ".json",
		"yaml":       ".yaml",
		"xml":        ".xml",
		"dockerfile": ".dockerfile",
		"makefile":   ".makefile",
	}

	if ext, ok := extensions[strings.ToLower(language)]; ok {
		return ext
	}
	return ".txt"
}

// GetTypeName returns a human-readable name for the artifact type
func (a *Artifact) GetTypeName() string {
	switch a.Type {
	case TypeCode:
		if a.Language != "" {
			return fmt.Sprintf("%s code", a.Language)
		}
		return "code"
	case TypeMarkdown:
		return "markdown"
	case TypeHTML:
		return "HTML"
	case TypeSVG:
		return "SVG image"
	case TypeReact:
		return "React component"
	case TypeMermaid:
		return "Mermaid diagram"
	default:
		return "document"
	}
}
