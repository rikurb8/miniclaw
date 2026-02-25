# MiniClaw Development Guide

Minimal operating guide for coding agents in this repository. The goal of this project is to be easy to read and reason with. Comments should be added when they provide value for the reader.

## IMPORTANT PRINCIPLES
- This is also a educational project so remember to explain clearly why changes are made

## Build/Test/Lint Commands

- **CI gates**: `task ci`
- **Format check**: `task fmt` (checks `gofmt -l` output)
- **Format fix**: `task fmt:fix`
- **Module tidy check**: `task mod`
- **Static checks**: `task vet` or `task lint`
- **Auto-fix + checks**: `task lint:fix`
- **All tests**: `task test` or `go test ./...`
- **Build**: `task build` or `go build -o miniclaw .`
- **Modernize**: `task modernize`
- **Clean**: `task clean`

Taskfile is the primary workflow for local development and CI.

## Code Style Guidelines

- **Imports**: Use `goimports` formatting, group stdlib, external, internal packages
- **Formatting**: Use gofumpt (stricter than gofmt), enabled in golangci-lint
- **Naming**: Standard Go conventions - PascalCase for exported, camelCase for unexported
- **Types**: Prefer explicit types, use type aliases for clarity (e.g., `type AgentName string`)
- **Error handling**: Return errors explicitly, use `fmt.Errorf` for wrapping
- **Context**: Always pass `context.Context` as first parameter for operations
- **Interfaces**: Define interfaces in consuming packages, keep them small and focused
- **Structs**: Use struct embedding for composition, group related fields
- **Constants**: Use typed constants with iota for enums, group in const blocks
- **Testing**: Use testify's `require` package, parallel tests with `t.Parallel()`,
  `t.Setenv()` to set environment variables. Always use `t.TempDir()` when in
  need of a temporary directory. This directory does not need to be removed.
- **JSON tags**: Use snake_case for JSON field names
- **File permissions**: Use octal notation (0o755, 0o644) for file permissions
- **Log messages**: Log messages must start with a capital letter (e.g., "Failed to save session" not "failed to save session")
  - This is enforced by `task lint:log` which runs as part of `task lint`
- **Comments**: End comments in periods unless comments are at the end of the line.

## Testing Guidelines

- Use Go `testing` package (no project-wide testify requirement).
- Prefer table-driven tests and `t.Run` for variants/subtests.
- Use `t.Setenv` for env isolation and `t.TempDir` for temp files.
- Keep tests deterministic and offline; do not call real external APIs.
- Use explicit failure messages with got/want values.

## Formatting

- Always format modified Go files before finishing.
- Useful commands:
  - `task fmt`
  - `task fmt:fix`
  - `gofmt -w <files>`

## Finish Checklist

- `task fmt`
- `task vet`
- `task test` (or targeted package test for small edits)
- `task build`
- Run `task mod` only when dependencies changed
- Update docs if behavior/config/CLI changed

## Change Philosophy

- Keep diffs minimal and focused.
- Preserve existing architecture and naming patterns.
- Avoid unrelated refactors in targeted fixes.
- Add or update tests when behavior changes.
