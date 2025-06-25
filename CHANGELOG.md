# Changelog

All notable changes to Shannon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.11] - 2025-06-25

### Added
- **Date display in TUI**: Search and browse modes now show conversation dates
  - Single date shown when conversation starts and ends on same day
  - Date range shown when conversation spans multiple days
  - Consistent formatting between browse and search modes

### Changed
- **README improvements**: 
  - Reorganized installation section to group package managers together
  - Updated introduction to better explain the problem Shannon solves
  - Applied prettier formatting

## [0.2.10] - 2025-06-24

### Fixed
- **CI/CD**: Fixed GoReleaser token usage in GitHub Actions workflow

## [0.2.9] - 2025-06-24

### Added
- **Scoop package manager support**: Added Scoop bucket for Windows users
  - Install with: `scoop bucket add shannon https://github.com/neilberkman/scoop-shannon && scoop install shannon`
  - Automated manifest generation through GoReleaser

## [0.2.8] - 2025-06-23

### Fixed
- **Critical**: Fixed panic when copying artifacts to clipboard on Linux systems
- Added proper error handling for clipboard operations with panic recovery
- Implemented fallback clipboard support using xclip/xsel/wl-copy on Linux
- Show user-friendly error message when clipboard is unavailable instead of crashing

## [0.2.7] - 2025-06-23

### Added
- **Artifact support in TUI**: New artifact focus mode for viewing and interacting with Claude artifacts
  - Press 'a' to enter artifact mode when artifacts are present
  - Navigate between artifacts with n/N
  - Expand/collapse artifacts with Tab (preview shows 10 lines, expanded shows all)
  - Save artifacts to files with 's'
  - Copy artifacts to clipboard with 'c'
  - Exit artifact mode with Escape
- **Advanced clipboard support**: Integrated golang.design/x/clipboard for better clipboard functionality
- **Unified conversation view**: Consolidated conversation display logic across browse and search modes
- **Artifact extraction**: Added comprehensive artifact extraction and rendering system
  - Supports code, markdown, HTML, SVG, React, and Mermaid artifacts
  - Smart file extension detection based on artifact type and language
  - Inline artifact rendering with syntax-aware formatting

### Fixed
- Fixed escape key behavior in artifact mode to properly return to conversation view
- Fixed escape key behavior in find mode to stay in conversation view
- Fixed notification timer to continue running after exiting artifact mode
- Fixed find highlighting to update when search query changes
- Improved artifact auto-scroll to properly detect artifact headers in decorative boxes

### Changed
- Artifact navigation keys changed from arrow keys to n/N for consistency with search
- Tab key now expands/collapses artifacts instead of focusing them
- Dynamic box width for artifacts based on content (capped at 100 chars)
- Removed debug output from artifact navigation

## [0.2.6] - 2025-06-23

### Fixed
- Empty search queries now show helpful error message instead of SQL error
- Special characters in search queries are properly handled
- Improved error messages for invalid search syntax

### Added
- Shorter date filter aliases: `--after` and `--before` (in addition to `--start-date` and `--end-date`)
- Better search examples in help text and README
- Comprehensive tests for query edge cases

### Changed
- Search help text now includes detailed examples of all query types

## [0.2.5] - 2025-06-23

### Fixed
- Fixed search behavior to treat multi-word queries as implicit AND instead of phrase search
  - `shannon search "machine learning"` now finds documents with both "machine" AND "learning"
  - Boolean operators (AND/OR/NOT) are now case-insensitive: `shannon search "python and django"`
  - Use quotes for exact phrase search: `shannon search '"exact phrase"'`

### Added
- Added comprehensive integration tests for search functionality including date filtering
- Added tests for FTS query processing and case-insensitive operators

## [0.2.4] - 2025-06-22

### Fixed
- **CRITICAL**: Fixed data loss bug where incremental imports would delete all existing messages
  - `INSERT OR REPLACE` was triggering CASCADE DELETE due to new conversation IDs
  - Now properly checks for existing conversations and uses UPDATE instead
- Fixed package name mismatch (import -> imports)

## [0.2.3] - 2025-06-22

### Added
- Added 'o' key shortcut to open conversations in claude.ai from TUI conversation view

### Changed
- Changed sender display from "Human"/"Assistant" to "You"/"Claude" for more natural conversation flow
- DRY: Created shared `FormatSender` function to centralize sender name formatting

## [0.2.2] - 2025-06-22

### Fixed
- Fixed date parsing in `recent` command to properly handle ISO 8601 format from database
- Fixed "1 days ago" pluralization issue using go-humanize library
- Improved relative time formatting for better readability

## [0.2.1] - 2025-06-22

### Changed
- Updated GoReleaser configuration to v2 format
- Improved Homebrew distribution support

### Fixed
- GoReleaser deprecation warnings

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