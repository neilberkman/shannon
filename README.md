# Shannon

A powerful CLI tool for searching through your exported Claude conversation history.

Named after Claude Shannon, the father of information theory, this tool helps you search, browse, and analyze your Claude.ai conversations with advanced full-text search capabilities.

> **Note**: This is an unofficial tool not affiliated with Anthropic.

## Features

- 🔍 **Full-text search** with SQLite FTS5
- 🌳 **Conversation branching** support (when available in exports)
- 💻 **Cross-platform** - Works on macOS, Linux, and Windows
- 🚀 **Fast** - Single Go binary with embedded database
- 🎨 **Multiple interfaces** - CLI for scripting, TUI for interactive use
- 🎯 **Interactive TUI** - Browse and search with keyboard navigation

## Installation

### Homebrew (macOS/Linux)

```bash
brew install yourusername/shannon/shannon
```

### Install Script (macOS/Linux)

```bash
curl -sSL https://raw.githubusercontent.com/yourusername/shannon/main/install.sh | bash
```

### From Source

```bash
go install github.com/yourusername/shannon@latest
```

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/yourusername/shannon/releases).

- **macOS**: `shannon_x.x.x_darwin_amd64.tar.gz` (Intel) or `shannon_x.x.x_darwin_arm64.tar.gz` (Apple Silicon)
- **Linux**: `shannon_x.x.x_linux_amd64.tar.gz` or `shannon_x.x.x_linux_arm64.tar.gz`
- **Windows**: `shannon_x.x.x_windows_amd64.zip` or `shannon_x.x.x_windows_arm64.zip`

### Scoop (Windows)

```bash
scoop bucket add shannon https://github.com/yourusername/scoop-shannon
scoop install shannon
```

## Usage

### Import Claude Export

First, export your Claude conversations from the Claude.ai interface, then:

```bash
shannon import path/to/conversations.json
```

### Search

Basic search:
```bash
shannon search "machine learning"
```

Advanced search with filters:
```bash
# Search only human messages
shannon search "python code" --sender human

# Search within date range
shannon search "bug" --start-date 2024-01-01 --end-date 2024-12-31

# Search within specific conversation
shannon search "function" --conversation 123

# Show context around search results
shannon search "error" --context --context-lines 3
```

### List Conversations

```bash
# List all conversations
shannon list

# List with filtering and sorting
shannon list --search "python" --limit 20 --sort messages

# Output just IDs for piping
shannon list --format json --quiet | jq -r '.conversations[].id'
```

### Recent Conversations

```bash
# Show conversations from last 7 days
shannon recent

# Show conversations from last 30 days
shannon recent --days 30

# Get just IDs for piping
shannon recent --format id | xargs -I {} shannon export {}
```

### Export Conversations

```bash
# Export single conversation (stdout by default)
shannon export 123

# Export as JSON
shannon export 123 --format json

# Export to file
shannon export 123 -o conversation.md

# Export multiple conversations to directory
shannon export 123 456 789 -d exports/

# Pipe to other tools
shannon export 123 | less
shannon export 123 --format json | jq '.messages[] | select(.sender == "human")'

# Read IDs from stdin
shannon search "bug" --format json | jq -r '.results[].conversation_id' | shannon export -
```

### Edit Conversations

```bash
# Open conversation in default editor
shannon edit 123

# Open with specific editor
shannon edit 123 --editor vim

# Open as JSON
shannon edit 123 --format json
```

### View Conversation

```bash
# View full conversation
shannon view 123

# View with branch information
shannon view 123 --branches
```

### Statistics

```bash
shannon stats
```

### Interactive TUI Mode

```bash
# Launch TUI with search
shannon tui "machine learning"

# Launch TUI in browse mode
shannon tui
```

TUI Keyboard Shortcuts:
- **Browse Mode**:
  - `↑/↓`: Navigate conversations
  - `Enter`: View conversation
  - `/`: Search
  - `q`: Quit
  
- **Search Results**:
  - `↑/↓`: Navigate results
  - `Enter`: View message details
  - `v`: View full conversation
  - `Esc`: Back to list
  - `q`: Quit

- **Conversation View**:
  - `↑/↓`: Scroll
  - `Esc`: Back
  - `q`: Quit

## Search Syntax

- **Phrase search**: `"exact phrase"`
- **Wildcard**: `test*`
- **Boolean**: `machine AND learning`
- **Exclusion**: `python -javascript`

## Unix Pipeline Integration

ClaudeSearch is designed to work well with Unix pipelines:

```bash
# Export search results as JSON and process with jq
shannon search "error" --format json | jq '.results[] | .conversation_name'

# Export as CSV for analysis
shannon search "python" --format csv | cut -d, -f1,4 | sort | uniq

# List conversations as JSON and filter
shannon list --format json | jq '.conversations[] | select(.message_count > 100)'

# Quiet mode for cleaner output
shannon search "bug" --quiet | grep -E "^\d+"

# Export conversation and process
shannon export 123 --quiet | grep "TODO"

# Pipeline from search to export
shannon search "python" --format json --quiet | \
  jq -r '.results[].conversation_id' | \
  sort -u | \
  head -5 | \
  xargs -I {} shannon export {} -o "python_{}.md"

# Recent conversations pipeline
shannon recent --format id | \
  while read id; do
    echo "=== Conversation $id ==="
    shannon export $id | head -20
  done
```

## Configuration

Configuration file is stored in platform-specific locations:
- Linux: `~/.config/shannon/config.yaml`
- macOS: `~/Library/Application Support/shannon/config.yaml`
- Windows: `%APPDATA%\shannon\config.yaml`

Database is stored in:
- Linux: `~/.local/share/shannon/claude-search.db`
- macOS: `~/Library/Application Support/shannon/claude-search.db`
- Windows: `%LOCALAPPDATA%\shannon\claude-search.db`

## Development

### Requirements

- Go 1.21+
- SQLite3 (embedded, no external dependency needed)

### Building

```bash
go build -o shannon
```

### Testing

```bash
go test ./...
```

### Linting

```bash
go fmt ./...
go vet ./...
golangci-lint run ./...
```

## License

MIT