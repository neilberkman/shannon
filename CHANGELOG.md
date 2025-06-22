# Changelog

All notable changes to Shannon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2025-06-22

### Added
- **In-conversation search** - Find and highlight text within conversations using `/` key
- **Browser-like navigation** - ESC acts as back button across all TUI modes
- **Conversation-centric search results** - Search shows conversations instead of individual messages
- **Smart conversation positioning** - Conversations start at the first message when opened from search
- **Consistent keyboard shortcuts** - Standard vim-like navigation (g/G for top/bottom, q to quit)
- **Enhanced TUI functionality** - Major improvements to search and navigation workflow

### Changed
- **Complete TUI workflow overhaul** - Fixed ESC behavior, find functionality, and navigation consistency
- **Search results architecture** - Converted from message-centric to conversation-centric display
- **README documentation** - Updated with comprehensive TUI features and keyboard shortcuts
- **Code organization** - Removed debug logging and cleaned up unused functions

### Fixed
- **Find functionality** - Fixed broken in-conversation search that wasn't working properly
- **ESC key behavior** - Implemented proper browser-like back button functionality
- **Word wrapping** - Fixed broken text display in conversation view
- **Test failures** - Fixed NewMarkdownRenderer width handling and hyperlink test assertions
- **Linting issues** - Removed orphaned test files and fixed all golangci-lint warnings

## [0.1.0] - Initial Release

### Added
- Basic conversation import functionality
- Core search capabilities
- CLI interface for all operations
- SQLite database backend
- Configuration management