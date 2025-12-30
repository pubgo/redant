# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Refactored project structure to separate command implementations from core framework code
- Moved CompletionCommand to cmds/completioncmd directory
- Removed configuration file support and related features for better simplicity
- Removed fsnotify dependency used for config hot-reload
- Improved `--list-commands` output format:
  - Removed "COMMANDS" header for cleaner display
  - Command descriptions now appear below command names
  - Commands with defined `Args` now display argument information (name, type, defaults, requirements)
  - Reduced indentation for more compact display
- Improved `--list-flags` output format:
  - Subcommand paths no longer include root command name
  - Removed trailing colons from subcommand paths
- Enhanced global flags display:
  - All root command options (non-hidden) are now displayed as global flags in help and `--list-flags`
  - Previously only predefined global flags (help, version, etc.) were shown
  - Custom flags defined in root command's `Options` are now properly displayed

## [1.0.0] - 2025-12-24

### Added
- Initial release of Redant, a powerful Go CLI framework
- Command tree structure with support for complex nested commands
- Multi-source configuration from command line flags, environment variables, and configuration files
- Middleware system based on Chi router pattern
- Excellent help system inspired by Go toolchain
- Easy testing with clear separation of standard input/output
- Colon-separated command paths (`command:sub-cmd` format)
- Flexible parameter formats (positional, query string, form data, JSON)
- Global flag system with unified management
- Support for various argument formats including:
  - Positional arguments
  - Query string format (e.g., `name=value&age=30`)
  - Form data format (e.g., `name=value age=30`)
  - JSON format (object and array)
- Comprehensive example applications demonstrating all features
- Environment variable support with multiple fallback options
- Enum and enum array value types
- Command-specific and global option handling
- Automatic help generation for commands and flags
- Subcommand conflict resolution
- Argument parsing with type validation
- Support for required and optional flags with default values
- Flag shorthand support
- Nested command support with inheritance
- Rich set of value types (string, int, bool, etc.)
- Customizable help templates
- Comprehensive test examples