# Tooling Plan: Workspace Filesystem Tools for `fantasy-agent`

## Problem Statement

MiniClaw already supports multiple runtime types (`generic-agent`, `opencode-agent`, `fantasy-agent`) and has config fields related to workspace/tooling (`agents.defaults.workspace`, `agents.defaults.restrict_to_workspace`, `agents.defaults.max_tool_iterations`), but these are not currently wired into actual agent tool execution.

We need to enable `fantasy-agent` to safely operate in a dedicated workspace using a small set of filesystem/edit tools:

- `read_file`
- `write_file`
- `list_dir`
- `append_file`
- `edit_file`

The solution must be practical, secure by default, and easy to extend later to other agent types.

## Goals

- Enable `fantasy-agent` to call the five tools above through the Fantasy library (`charm.land/fantasy`).
- Constrain all file operations to a configured workspace root.
- Respect `agents.defaults.restrict_to_workspace` policy.
- Respect `agents.defaults.max_tool_iterations` as a loop bound to avoid runaway tool loops.
- Keep implementation scoped and minimally invasive to current architecture.
- Provide deterministic behavior and clear tool error messages.
- Add tests and docs sufficient for production confidence.

## Non-Goals (Phase 1)

- No changes to non-fantasy agents (`generic-agent`, `opencode-agent`).
- No recursive/glob/search tools in this phase.
- No binary/media file handling beyond plain text workflows.
- No remote/network tools in this phase.
- No persistent audit database (simple logging only).
- No approval workflow/interactive confirmations yet.

## Existing Context (Current State)

- `fantasy-agent` runtime path:
  - `cmd/agent.go` routes `fantasy-agent` to `pkg/provider/fantasy.New(...)`.
- Fantasy provider implementation:
  - `pkg/provider/fantasy/fantasy.go` currently uses `core.NewAgent(model)` with no tools.
- Config fields already present:
  - `agents.defaults.workspace`
  - `agents.defaults.restrict_to_workspace`
  - `agents.defaults.max_tool_iterations`
- These config fields are currently not used by runtime logic.
- Existing docs mention fantasy agent but not filesystem tooling behavior.

## Proposed Solution (High Level)

Introduce a small internal workspace plus tooling subsystem and wire it into `pkg/provider/fantasy` only.

### Core design

1. Resolve and initialize workspace root from config.
2. Guard and normalize all tool paths through a single boundary checker.
3. Implement filesystem operations behind one internal service API.
4. Adapt internal tool handlers into `fantasy.AgentTool` definitions.
5. Build Fantasy agent with:
   - `core.WithTools(...)`
   - stop condition based on `max_tool_iterations`
6. Preserve existing session and memory behavior in fantasy provider.

## Architecture

### New and updated components

- `pkg/workspace`
  - Workspace root resolution and policy guard.
- `pkg/tools/fs`
  - Internal filesystem service operations (read/write/append/list/edit).
- `pkg/tools/fantasy`
  - Adapter that exposes FS service operations as Fantasy tools.
- `pkg/provider/fantasy`
  - Composition root: creates workspace, tool service, fantasy tools, and executes prompts using them.

### Data flow

1. User prompt reaches `fantasy` provider.
2. Provider executes model call through Fantasy agent configured with toolset.
3. Model may call a tool.
4. Tool handler validates path against workspace and executes operation.
5. Tool returns structured text response (or explicit error) to model.
6. Model continues until finish or max tool iteration stop condition.

## Detailed Specification

## 1) Workspace Resolution and Guard

### Config Inputs

From `cfg.Agents.Defaults`:

- `Workspace string`
- `RestrictToWorkspace bool`
- `MaxToolIterations int`

### Resolution rules

- Trim whitespace.
- Expand leading `~` to user home directory.
- Convert to absolute path.
- Create directory if missing (`mkdir -p` semantics in Go).
- Normalize to a clean absolute path.

### Guard behavior

Define a guard that resolves user-supplied tool paths:

- Empty path -> error (`invalid_path`).
- Relative paths are interpreted relative to workspace root.
- Absolute paths:
  - If `restrict_to_workspace=true`: only allowed if final resolved path is inside workspace.
  - If `restrict_to_workspace=false`: may be allowed, but Phase 1 recommendation is still to require workspace containment for simplicity and safety.
- Prevent `..` traversal escapes.
- Resolve symlinks and reject effective targets outside workspace.
- Return canonical absolute path for execution.

### Error semantics (stable messages)

Return clear, concise error strings with stable categories:

- `invalid_path`
- `outside_workspace`
- `path_not_found`
- `permission_denied`
- `io_error`
- `ambiguous_edit`
- `edit_not_found`

Exact text can include details, but the category keyword should be present.

## 2) Filesystem Service API (Internal)

Create an internal service abstraction (not user-facing) to isolate tool logic from Fantasy adapter code.

Suggested methods:

- `ReadFile(ctx, path) -> ReadResult`
- `WriteFile(ctx, path, content) -> WriteResult`
- `AppendFile(ctx, path, content) -> AppendResult`
- `ListDir(ctx, path) -> ListResult`
- `EditFile(ctx, path, oldText, newText, replaceAll) -> EditResult`

### Operation contracts

#### `read_file`

- Input:
  - `path` (required)
- Behavior:
  - Read UTF-8 text file.
  - Return textual content.
  - Optionally enforce a size limit for response safety (recommended).
- Errors:
  - not found, outside workspace, permission denied, binary/unreadable content, and related IO failures.

#### `write_file`

- Input:
  - `path` (required)
  - `content` (required)
- Behavior:
  - Overwrite full file content.
  - Create file if missing.
  - Parent directory handling:
    - recommended: create parent directories automatically inside workspace.
- Result:
  - bytes written and path.

#### `append_file`

- Input:
  - `path` (required)
  - `content` (required)
- Behavior:
  - Append to file.
  - Create file if missing.
- Result:
  - bytes appended, resulting size optional.

#### `list_dir`

- Input:
  - `path` (optional, defaults to `.` workspace root)
- Behavior:
  - Non-recursive listing.
  - Return sorted entries.
  - Include minimal metadata per entry (name, type, size optional).
- Result:
  - list payload suitable for LLM consumption.

#### `edit_file`

- Input:
  - `path` (required)
  - `old_text` (required)
  - `new_text` (required)
  - `replace_all` (optional, default false)
- Behavior:
  - Exact string replacement.
  - If `replace_all=false`, require exactly one match.
  - If no match -> `edit_not_found`.
  - If multiple matches and `replace_all=false` -> `ambiguous_edit`.
  - Write back updated content atomically if possible.
- Result:
  - match count and replaced count.

## 3) Fantasy Tool Adapter

Implement typed Fantasy tools using `core.NewAgentTool`.

Tool names must be exactly:

- `read_file`
- `write_file`
- `append_file`
- `list_dir`
- `edit_file`

### Input schema strategy

Use typed structs for each tool input so Fantasy generates JSON schema automatically.

Example input shapes (conceptual):

- `read_file`: `{ "path": "..." }`
- `write_file`: `{ "path": "...", "content": "..." }`
- `append_file`: `{ "path": "...", "content": "..." }`
- `list_dir`: `{ "path": "..." }` (optional path)
- `edit_file`: `{ "path": "...", "old_text": "...", "new_text": "...", "replace_all": false }`

### Tool result strategy

Return plain text responses via `core.NewTextResponse(...)` and errors via `core.NewTextErrorResponse(...)` when appropriate.

Response format should be concise and consistent, for example:

- Success: `ok: wrote 128 bytes to path/to/file`
- Error: `outside_workspace: resolved path is outside workspace root`

This consistency helps the model self-correct tool calls.

## 4) Provider Wiring (`pkg/provider/fantasy`)

### `New(cfg)` changes

- Resolve workspace and initialize guard/service.
- Build fantasy tool catalog.
- Store tools and iteration config in provider client.

### Prompt execution changes

When generating a response, create a Fantasy agent with options:

- `core.WithTools(tools...)`
- `core.WithStopConditions(...)` using step count bound derived from `max_tool_iterations` (when > 0)

Maintain current behavior for:

- health checks
- session creation
- in-memory history
- usage metadata extraction

### Iteration policy

- If `max_tool_iterations > 0`: enforce stop condition.
- If unset or <= 0: use safe default (recommended: 20, aligned with config example).

## 5) Security Considerations

- Strict path containment checks are mandatory.
- Resolve and verify symlink targets to prevent workspace escape.
- Ensure no tool allows shell execution.
- Consider deny access to special files/devices (future hardening).
- Keep tool output bounded to avoid token explosion (recommended max bytes and max entries).
- Avoid leaking absolute host paths unnecessarily in responses (prefer workspace-relative where possible).

## 6) Reliability Considerations

- Prefer atomic write pattern for `write_file` and `edit_file` (temporary file + rename) when feasible.
- Preserve existing file mode where practical on rewrite.
- Handle CRLF/LF transparently (no aggressive normalization in phase 1).
- Ensure deterministic sort order for directory listing.
- Make error messages deterministic for tests.

## 7) Observability

Add structured logs around tool execution in fantasy provider context:

- tool name
- normalized target path (safe form)
- success or failure
- duration
- error category (if any)

Do not log full file contents.

## 8) Testing Plan

### Unit tests

#### `pkg/workspace`

- path normalization
- `~` expansion
- relative and absolute path resolution
- traversal rejection
- symlink escape rejection
- restrict flag behavior

#### `pkg/tools/fs`

- read/write/append/list/edit happy paths
- missing path
- permission denied
- edit zero-match
- edit multi-match ambiguity
- `replace_all` behavior

#### `pkg/tools/fantasy`

- tool registration names
- schema shape sanity checks
- response formatting consistency

#### `pkg/provider/fantasy`

- tools are attached during generation path
- max tool iteration stop condition configuration
- existing prompt/session tests still pass

### Integration smoke tests (optional in phase 1)

- Run `fantasy-agent` prompt that causes file creation and readback in a temporary workspace.

## 9) Documentation Updates Required

Update all of the following:

- `README.md`
  - Explain fantasy-agent filesystem tool capability and workspace restrictions.
- `docs/AGENTS.md`
  - Add a section for tool-enabled fantasy behavior.
- `docs/OVERVIEW.md`
  - Update architecture/request flow with tool execution loop.
- `config/config.example.json`
  - Ensure workspace/restrict/max_tool_iterations guidance is clear.

Also include a user-facing note on safety assumptions and limitations.

## 10) Rollout Strategy

- Phase 1: Enable tools for `fantasy-agent` only.
- Keep defaults conservative and safe.
- Validate via unit tests and `task fmt`, `task vet`, `task test`, `task build`.
- Phase 2 (future): abstract tool subsystem for reuse by `generic-agent` and gateway runtime.

## 11) Open Questions and Decisions to Confirm

1. Absolute path policy when `restrict_to_workspace=false`:
   - Option A (recommended): still enforce workspace containment in phase 1.
   - Option B: permit absolute external paths when restriction is disabled.
2. Parent directory creation behavior for write/append:
   - Option A (recommended): auto-create.
   - Option B: fail if parent is missing.
3. Read/list size limits:
   - Explicit max bytes and max entries defaults are needed.
4. `edit_file` semantics:
   - Exact-match only in phase 1 (recommended) versus regex or pattern edits.

## 12) Implementation Checklist

- [ ] Add workspace resolver and guard package with tests.
- [ ] Add filesystem service package with tests.
- [ ] Add Fantasy tool catalog adapter with tests.
- [ ] Wire tool catalog + iteration stop into fantasy provider.
- [ ] Ensure existing fantasy tests pass and extend where needed.
- [ ] Update README/docs/config example.
- [ ] Run quality gates: `task fmt`, `task vet`, `task test`, `task build`.

## Acceptance Criteria

- `fantasy-agent` can successfully use all five tools within workspace.
- Path escape attempts are blocked with clear errors.
- Tool iteration bound is enforced.
- Existing runtime behavior remains stable.
- Documentation reflects the new behavior and config semantics.
- CI-equivalent local checks pass.
