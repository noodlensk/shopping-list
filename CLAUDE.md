# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Commands

Uses Task runner (Taskfile.yml) for all development commands. Run `task` to see all available tasks.

### Development
- `task build` - Build the Go binary
- `task run` - Build and run the application with config.yaml
- `task test` - Run tests with race detection
- `task dep` - Download and tidy Go modules

### Code Quality
- `task lint` - Run golangci-lint
- `task lint-fix` - Run golangci-lint with auto-fix
- `task fmt` - Format Go code with gofmt and goimports

### Setup
- `task setup-lint` - Install golangci-lint

### Environment
- `task start-env` - Start application with docker-compose
- `task stop-env` - Stop docker-compose environment

## Architecture Overview

This is a Go-based Telegram bot for managing shared shopping lists, following hexagonal/clean architecture principles:

### Core Structure
- **Domain Layer**: `internal/grocery/domain/` - Contains business entities (Item)
- **Application Layer**: `internal/grocery/app/` - Commands and queries using CQRS pattern
- **Adapters**: `internal/grocery/adapters/` - Data persistence (BoltDB repository)
- **Ports**: `internal/grocery/ports/` - External interfaces (Telegram bot)
- **Service**: `internal/grocery/service/` - Application composition and dependency injection

### Key Components
- **Application**: CQRS-style application with separate Commands and Queries structs
- **Commands**: AddItem, CompleteItem, RemoveCompletedItems
- **Queries**: GetItems (list all items)
- **Storage**: BoltDB embedded database for persistence
- **Interface**: Telegram bot with user allowlist

### Configuration
- Uses Viper for configuration management
- Config file: `config.yaml` (see `config.example.yaml` for template)
- Requires Telegram bot token and allowed user IDs
- Database path configurable via Storage.Path

### Dependencies
- BoltDB for embedded database
- Telegram Bot API v5
- Uber Zap for logging
- Viper for configuration

## Running the Application
1. Copy `config.example.yaml` to `config.yaml`
2. Update config.yaml with your Telegram bot token and allowed user IDs
3. Use `task start-env` for Docker Compose or `task run` for direct execution