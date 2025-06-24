# Shannon Roadmap

This document outlines planned features and improvements for Shannon. Items are roughly organized by priority, but this is subject to change based on user feedback and contributions.

## 🚀 Near Term (v0.3.x)

### CLI Artifact Support
- [ ] `shannon artifacts list <conversation-id>` - List all artifacts in a conversation
- [ ] `shannon artifacts extract <conversation-id>` - Extract all artifacts to files
- [ ] `shannon artifacts extract <conversation-id> --index N` - Extract specific artifact
- [ ] `shannon artifacts search "query"` - Search for artifacts across all conversations
- [ ] `shannon view <conversation-id> --show-artifacts` - Display artifacts inline in CLI

### Enhanced Clipboard Support
- [ ] Investigate cross-platform clipboard libraries that don't require X11 headers
- [ ] Support for rich content types (HTML, images) where possible
- [ ] Fallback to `xclip`/`xsel`/`pbcopy` commands when available

### TUI Improvements
- [ ] **Rich markdown rendering** in conversation view
  - [ ] Syntax highlighting for code blocks
  - [ ] Proper formatting for lists, headers, emphasis
  - [ ] Table rendering
  - [ ] Inline code styling
- [ ] Multi-select for bulk operations (export, delete)
- [ ] Conversation search/filter in browse mode
- [ ] Configurable key bindings
- [ ] Theme customization (colors, styles)

## 🎯 Medium Term (v0.4.x)

### Database Enhancements
- [ ] Full-text search for artifacts content
- [ ] Conversation tagging/categorization system
- [ ] Custom metadata fields
- [ ] Export/import database backups

### Advanced Search
- [ ] Search within date ranges in TUI
- [ ] Search by artifact type/language
- [ ] Saved search queries
- [ ] Search history

### Integration Features
- [ ] Export to Obsidian/Notion/Roam format
- [ ] API mode for programmatic access
- [ ] Watch mode for auto-import of new exports
- [ ] Git integration for version controlling conversations

## 🌟 Long Term (v0.5.x+)

### Conversation Analysis
- [ ] Token usage statistics
- [ ] Conversation topic modeling
- [ ] Sentiment analysis
- [ ] Code language detection and statistics

### Branch Support
- [ ] Visual branch navigation in TUI
- [ ] Diff view between branches
- [ ] Branch merging tools
- [ ] Export specific branches

### Advanced Artifact Features
- [ ] Artifact versioning/history
- [ ] Side-by-side artifact comparison
- [ ] Artifact syntax validation
- [ ] Direct artifact execution (with sandboxing)

### Collaboration
- [ ] Shared conversation libraries
- [ ] Commenting on conversations
- [ ] Conversation templates
- [ ] Team workspaces

## 💡 Ideas & Experiments

- **Plugin System**: Allow custom extractors, renderers, and exporters
- **AI Integration**: Use LLMs to summarize conversations, extract insights
- **Mobile Companion**: iOS/Android app for viewing conversations
- **Web UI**: Browser-based interface as alternative to TUI
- **Real-time Sync**: Direct integration with Claude.ai API (when available)

## Contributing

We welcome contributions! If you're interested in working on any of these features:

1. Check if there's an existing issue for the feature
2. Open an issue to discuss the implementation approach
3. Submit a PR with tests and documentation

For major features, please discuss first to ensure alignment with the project direction.

## Completed Features ✅

- ✅ Artifact extraction and rendering in TUI (v0.2.7)
- ✅ Unified conversation view (v0.2.7)
- ✅ Find/search within conversations (v0.2.x)
- ✅ Clickable links in supported terminals (v0.2.x)
- ✅ Rich markdown rendering (v0.1.x)

---

*This roadmap is a living document and will be updated as priorities shift and new ideas emerge.*