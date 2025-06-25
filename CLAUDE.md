# Claude Development Guidelines for Shannon

This document contains project-specific guidelines for Claude when working on the Shannon codebase.

## CRITICAL: Unified Conversation View

**UPDATE: The conversation view has been unified in `cmd/tui/conversation.go`**
- Both browse.go and search.go now delegate all conversation display and interaction to the shared conversationView component
- This ensures consistent behavior whether users reach conversations via browse or search
- All artifact navigation, key handlers, and display logic is centralized
- Any changes to conversation viewing should be made in conversation.go only

## Project Overview

Shannon is a CLI tool for searching and browsing Claude conversation history. It uses SQLite with FTS5 (Full Text Search) for efficient searching and provides both CLI and TUI interfaces.

## Pre-Commit Checklist

Before committing any changes, ALWAYS run these commands:

```bash
# Run all tests
go test ./...

# Run linter
golangci-lint run

# Run formatter
go fmt ./...

# Verify the build
go build
```

## Code Quality Standards

1. **Error Handling**: Always check error returns, especially for:
   - Database operations
   - File I/O operations
   - HTTP requests
   - Command execution

2. **Resource Cleanup**: Always defer cleanup operations:
   ```go
   defer rows.Close()
   defer tx.Rollback() // Will be no-op if tx.Commit() succeeded
   ```

3. **Test Coverage**: When adding new features, include:
   - Unit tests for individual functions
   - Integration tests for database operations
   - Table-driven tests for multiple scenarios

## Search Implementation

The search functionality uses SQLite FTS5 with two indexes:
- `messages_fts`: Standard text search
- `messages_fts_code`: Optimized for code patterns

Key behaviors:
- Multi-word queries use implicit AND: "machine learning" → "machine AND learning"
- Boolean operators (AND/OR/NOT) are case-insensitive
- Use quotes for exact phrase matching: '"exact phrase"'

## Testing Guidelines

### CRITICAL: Test Data Must Be Synthetic

⚠️ **NEVER use real conversation data from any user's Claude history in tests** ⚠️

All test data MUST be completely synthetic and generic. Examples of acceptable test data:
- Programming questions: "How do I use Python for machine learning?"
- Generic project names: "Test Project Alpha", "Python Development"
- Standard placeholder names: "Alice", "Bob", "Charlie"
- Technical content: "Python is great for data science with pandas and numpy"

### Integration Test Pattern

```go
func setupTestDB(t *testing.T) (*Engine, func()) {
    // Create temporary database
    // Insert synthetic test data
    // Return engine and cleanup function
}
```

## Common Commands

```bash
# Run specific tests
go test ./internal/search -v

# Run tests with coverage
go test ./... -cover

# Build for current platform
go build

# Run the TUI
./shannon tui

# Search from CLI
./shannon search "search query"

# Import conversations
./shannon import /path/to/conversations.json

# Work with artifacts (via TUI)
# Artifacts are now integrated into the TUI conversation view
# Press 'a' to enter artifact mode when viewing a conversation with artifacts
```

## Release Process

The release process is automated using GoReleaser via a GitHub Actions workflow.

1.  Ensure the `CHANGELOG.md` is up-to-date with all relevant changes for the new release.
2.  Commit any final changes.
3.  Create a new version tag (e.g., `git tag v0.2.5`).
4.  Push the tag to the `main` branch (`git push origin main --tags`).

Pushing a new `v*` tag will automatically trigger the release workflow, which will:
- Run all tests and linters.
- Build the binaries for all supported platforms.
- Create a new GitHub Release with the artifacts.
- Publish packages to Homebrew, Scoop, etc. as configured in `.goreleaser.yml`.

## Architecture Notes

- Both CLI and TUI use the same `internal/search` engine
- Date formatting uses `github.com/dustin/go-humanize`
- Terminal rendering uses `github.com/charmbracelet/bubbletea`
- All SQL queries should use parameterized statements
- **Unified conversation view**: The `cmd/tui/conversation.go` component handles all conversation display and interaction for both browse and search modes

## Artifact Extraction

Shannon can extract and manage artifacts from Claude conversations. Artifacts are special content blocks (code, documents, etc.) that Claude generates.

### Supported Artifact Types
- Code (`application/vnd.ant.code`) - with language-specific syntax highlighting
- Markdown (`text/markdown`)
- HTML (`text/html`)
- SVG images (`image/svg+xml`)
- React components (`application/vnd.ant.react`)
- Mermaid diagrams (`application/vnd.ant.mermaid`)

### Implementation Details
- Artifacts are extracted on-demand, not during import
- The `internal/artifacts` package handles all extraction logic
- The `view` command shows artifacts inline by default
- Artifacts can be exported to files with appropriate extensions

## Performance Considerations

- The code FTS table is automatically selected for queries with code patterns
- Use LIMIT and OFFSET for pagination in large result sets
- Conversations are indexed by updated_at for efficient recent queries

## Security

- Never log or commit sensitive data
- Use parameterized SQL queries to prevent injection
- Sanitize file paths before operations
- Keep dependencies updated

## Platform-Specific Features

### Clipboard Support
- Full clipboard support on macOS and Windows via `golang.design/x/clipboard`
- Linux requires X11 development headers (libx11-dev) for clipboard support
- Build constraints prevent compilation errors on systems without required headers
- Graceful fallback with error messages when clipboard is unavailable

Remember: When in doubt, ask for clarification rather than making assumptions about user data or preferences.