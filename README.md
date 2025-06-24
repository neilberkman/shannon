# Shannon

A powerful CLI tool for searching through your exported Claude conversation history.

This tool helps you search, browse, and analyze your Claude.ai conversations with advanced full-text search capabilities, rich markdown rendering, and both CLI and TUI interfaces.

> **Note**: This is an unofficial tool not affiliated with Anthropic.

## Features

- 🔍 **Full-text search** with SQLite FTS5 and dual tokenizers (porter + unicode61)
- 📝 **Rich markdown rendering** with syntax highlighting and formatting
- 🌳 **Conversation branching** support (when available in exports)
- 💻 **Cross-platform** - Works on macOS, Linux, and Windows (6 architectures)
- 🚀 **Fast** - Single Go binary with embedded database
- 🎨 **Multiple interfaces** - CLI for scripting, TUI for interactive use
- 🔄 **Auto-discovery** - Automatically finds Claude export files
- 📤 **Export formats** - JSON, CSV, and Markdown output
- 🔗 **Pipeline-friendly** - Designed for Unix pipeline integration
- 📊 **Statistics** - Detailed database and conversation analytics
- 🖥️ **Modern terminal support** - Enhanced features in Ghostty, Kitty, and WezTerm

## Installation

### Homebrew (macOS/Linux)

```bash
brew install neilberkman/shannon/shannon
```

### Install Script (macOS/Linux)

```bash
curl -sSL https://raw.githubusercontent.com/neilberkman/shannon/main/install.sh | bash
```

### From Source

```bash
go install github.com/neilberkman/shannon@latest
```

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/neilberkman/shannon/releases).

- **macOS**: `shannon_x.x.x_darwin_amd64.tar.gz` (Intel) or `shannon_x.x.x_darwin_arm64.tar.gz` (Apple Silicon)
- **Linux**: `shannon_x.x.x_linux_amd64.tar.gz` or `shannon_x.x.x_linux_arm64.tar.gz`
- **Windows**: `shannon_x.x.x_windows_amd64.zip` or `shannon_x.x.x_windows_arm64.zip`

### Scoop (Windows)

```bash
scoop bucket add shannon https://github.com/neilberkman/scoop-shannon
scoop install shannon
```

## Quick Start

```bash
# 1. Export your data from https://claude.ai/settings/data-privacy-controls
# 2. Find and import your conversations
shannon discover
shannon import ~/Downloads/claude-export*/conversations.json

# 3. Start searching!
shannon search "your search term"
shannon tui  # Interactive browser
```

## Getting Started

### 1. Export Your Claude Conversations

First, you need to export your conversation data from Claude:

1. Go to [Claude Data Privacy Controls](https://claude.ai/settings/data-privacy-controls){:target="_blank"}
2. In the **Data controls** section, click **Export data**
3. Claude will send you an email with a download link
4. Download and extract the ZIP file

### 2. Import Your Conversations

Once you have your export file:

```bash
# Auto-discover export files in common locations
shannon discover

# Import the conversations (usually named conversations.json)
shannon import path/to/conversations.json
```

## Usage

### Search

Basic search:

```bash
shannon search "machine learning"
```

Advanced search with filters:

```bash
# Boolean operators (case-insensitive)
shannon search "python AND django"
shannon search "react or vue or angular"
shannon search "error NOT timeout"

# Exact phrase search
shannon search '"machine learning model"'

# Wildcard search (prefix only)
shannon search "async*"

# Filter by sender
shannon search "python code" --sender human

# Filter by date range (using short aliases)
shannon search "bug" --after 2024-01-01 --before 2024-12-31

# Search within specific conversation
shannon search "function" --conversation 123

# Show context around search results
shannon search "error" --context --context-lines 3

# Export search results
shannon search "python" --format json --quiet
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
# Export single conversation to stdout
shannon export 123

# Export as JSON or CSV
shannon export 123 --format json
shannon export 123 --format csv --output export.csv

# Export multiple conversations
shannon export 123 456 789

# Pipe to other tools
shannon export 123 | less
shannon export 123 --format json | jq '.messages[] | select(.sender == "human")'
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
# Show database statistics
shannon stats
```

### Terminal Features

```bash
# Check what terminal features are supported
shannon terminal
```

Shannon provides enhanced features in modern terminals:

#### Ghostty, Kitty, WezTerm

- **Clickable conversation IDs** - Click to view conversations directly
- **Clickable URLs** - Auto-detected links become clickable
- **Rich hyperlinks** - Email addresses, GitHub repos, and file paths

#### All Terminals

- **Progressive enhancement** - Features gracefully degrade in basic terminals
- **Rich markdown rendering** - Syntax highlighting and formatting
- **Adaptive themes** - Automatically matches terminal light/dark mode

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
  - `q`: Quit application

- **Search Results**:
  - `↑/↓`: Navigate conversations
  - `Enter`: View full conversation
  - `Esc`: Back to browse mode
  - `q`: Quit application

- **Conversation View**:
  - `↑/↓`: Scroll messages
  - `g/G`: Go to top/bottom
  - `/`: Find text within conversation
  - `Esc`: Back to search results (or clear find if active)
  - `q`: Quit application

- **Find Mode** (within conversation):
  - `n/N`: Next/previous match
  - `Esc`: Clear find and return to conversation
  - `q`: Quit application

**TUI Features:**
- 🔍 **In-conversation search** - Find and highlight text within conversations
- 🔙 **Browser-like navigation** - ESC acts as back button, q quits completely  
- 💬 **Conversation-centric results** - Search shows conversations, not individual messages
- 📍 **Smart positioning** - Conversations start at the first message when opened
- ⌨️ **Consistent shortcuts** - Standard vim-like navigation (g/G for top/bottom)

## Search Syntax

- **Phrase search**: `"exact phrase"`
- **Wildcard**: `test*`
- **Boolean**: `machine AND learning`
- **Exclusion**: `python -javascript`

## Unix Pipeline Integration

Shannon is designed to work well with Unix pipelines:

```bash
# Export search results as JSON and process with jq
shannon search "error" --format json | jq '.results[] | .conversation_name'

# Export as CSV for analysis
shannon search "python" --format csv | cut -d, -f1,4 | sort | uniq

# List conversations as JSON and filter
shannon list --format json | jq '.conversations[] | select(.message_count > 100)'

# Quiet mode for cleaner output
shannon search "bug" --quiet

# Export conversation and process
shannon export 123 | grep "TODO"

# Pipeline from search to export
shannon search "python" --format json --quiet | \
  jq -r '.results[].conversation_id' | \
  sort -u | \
  head -5 | \
  xargs -I {} shannon export {}

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

## Limitations

- **Screenshots**: Screenshot attachments in conversations are not included in exports or searches. Only text content is indexed and exported.
- **File Attachments**: Other file attachments (PDFs, documents, etc.) are not currently supported.
- **Conversation Branches**: While branch information is preserved, navigation between branches is not yet implemented in the TUI.

## Terminal Compatibility

Shannon works in any terminal but provides enhanced features in modern emulators:

| Terminal               | Hyperlinks | Graphics | Advanced Input |
| ---------------------- | ---------- | -------- | -------------- |
| **Ghostty**            | ✅         | ✅\*     | ✅             |
| **Kitty**              | ✅         | ✅\*     | ✅             |
| **WezTerm**            | ✅         | ✅\*     | ✅             |
| **iTerm2**             | ✅         | ✅\*     | ❌             |
| **VS Code**            | ✅         | ❌       | ❌             |
| **Standard terminals** | ❌         | ❌       | ❌             |

\*Graphics support planned for future versions

Run `shannon terminal` to see what features are available in your current terminal.

## Development

### Requirements

- Go 1.21+
- SQLite3 (embedded, no external dependency needed)
- golangci-lint (for development)

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
