# Changelog

All notable changes to Shannon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- TUI root model pattern for better state management
- Consolidated shared TUI code (rendering and styles)
- Simplified branch detection with tree diff approach  
- Dynamic xargs command using Cobra's Find()
- Comprehensive error handling and TUI logging
- Unit tests for config, search, and platform packages
- Cross-platform directory support
- Full-text search with SQLite FTS5
- Multiple export formats (JSON, CSV, Markdown)
- Interactive TUI for browsing conversations
- Conversation statistics and analytics
- Branch detection for conversation trees
- Advanced search query syntax with boolean operators
- Pipeline integration with xargs command

### Changed
- Replaced deprecated strings.Title with golang.org/x/text/cases
- Improved error handling throughout the codebase
- Enhanced code modularity and maintainability

### Fixed
- Critical linting issues and error handling
- Memory leaks in TUI state management
- Cross-platform compatibility issues

## [0.1.0] - Initial Release

### Added
- Basic conversation import functionality
- Core search capabilities
- CLI interface for all operations
- SQLite database backend
- Configuration management