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

### Backward compatibility

- No behavior changes for `generic-agent` or `opencode-agent` in phase 1.
- Existing non-fantasy runtime flows and provider integrations remain unchanged.

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
   - explicit finalization behavior when stop limit is hit
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

### Default runtime limits (Phase 1)

| Setting | Default | Purpose |
| --- | --- | --- |
| `max_tool_iterations` | `20` | Bounds agent step loop when tools are enabled. |
| `max_read_bytes` | `262144` (256 KiB) | Prevents oversized `read_file` outputs. |
| `max_write_bytes` | `1048576` (1 MiB) | Caps payload size for write/append/edit operations. |
| `max_list_entries` | `500` | Prevents oversized `list_dir` results. |
| `max_tool_operation_duration` | `10s` | Bounds per-tool execution latency. |

Phase 1 should also introduce bounded tool defaults (internal constants first, config exposure optional):

- `MaxReadBytes = 262144` (256 KiB)
- `MaxWriteBytes = 1048576` (1 MiB per write/append/edit operation)
- `MaxListEntries = 500`
- `MaxToolOperationDuration = 10s` (per tool call)

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

TOCTOU hardening requirement (Phase 1): validate path containment on canonicalized path and re-check containment immediately before final open/create/rename operations.

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
  - Reject binary/unreadable content using deterministic text detection (invalid UTF-8 or NUL byte => `io_error`).
  - Enforce max bytes read (`MaxReadBytes`).
  - Return textual content.
- Errors:
  - not found, outside workspace, permission denied, binary/unreadable content, and related IO failures.

#### `write_file`

- Input:
  - `path` (required)
  - `content` (required)
- Behavior:
  - Overwrite full file content.
  - Create file if missing.
  - Enforce max content size (`MaxWriteBytes`) before touching disk.
  - New files should default to mode `0o644`.
  - Existing files keep current mode when overwritten.
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
  - Enforce max append payload (`MaxWriteBytes`) before touching disk.
- Result:
  - bytes appended, resulting size optional.

#### `list_dir`

- Input:
  - `path` (optional, defaults to `.` workspace root)
- Behavior:
  - Non-recursive listing.
  - Enforce max entries (`MaxListEntries`) and return a deterministic truncation notice when clipped.
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
  - Enforce max resulting content size (`MaxWriteBytes`) before write-back.
  - Write back updated content atomically if possible.
- Result:
  - match count and replaced count.

## 3) Fantasy Tool Adapter

Implement typed Fantasy tools using `core.NewAgentTool`.

Phase 1 should register all filesystem tools as sequential tools (`core.NewAgentTool`) rather than parallel tools (`core.NewParallelAgentTool`) for predictable ordering and simpler safety analysis. Parallel execution can be introduced in a future phase.

Tool names must be exactly:

- `read_file`
- `write_file`
- `append_file`
- `list_dir`
- `edit_file`

### Input schema strategy

Use typed structs for each tool input so Fantasy generates JSON schema automatically.

For better model reliability, include clear `json` and `description` tags on tool input struct fields. This improves generated schema quality and reduces malformed tool calls.

Example input shapes (conceptual):

- `read_file`: `{ "path": "..." }`
- `write_file`: `{ "path": "...", "content": "..." }`
- `append_file`: `{ "path": "...", "content": "..." }`
- `list_dir`: `{ "path": "..." }` (optional path)
- `edit_file`: `{ "path": "...", "old_text": "...", "new_text": "...", "replace_all": false }`

### Tool result strategy

Return plain text responses via `core.NewTextResponse(...)` and errors via `core.NewTextErrorResponse(...)` when appropriate.

Important Fantasy behavior: recoverable tool failures should be returned as `core.NewTextErrorResponse(...)` (with `IsError=true`). Returning a non-nil Go `error` from tool handlers should be reserved for truly fatal conditions because it may abort agent execution.

Response format should be concise and consistent, for example:

- Success: `ok: wrote 128 bytes to path/to/file`
- Error: `outside_workspace: resolved path is outside workspace root`

This consistency helps the model self-correct tool calls.

### Failure contract examples (stable categories)

- `invalid_path: path must not be empty`
- `outside_workspace: resolved path escapes workspace`
- `path_not_found: file does not exist`
- `permission_denied: cannot open file`
- `io_error: file appears to be binary or invalid utf-8`
- `edit_not_found: old_text not found`
- `ambiguous_edit: old_text matched multiple locations`

## 4) Provider Wiring (`pkg/provider/fantasy`)

### `New(cfg)` changes

- Resolve workspace and initialize guard/service.
- Build fantasy tool catalog.
- Store tools and iteration config in provider client.

### Prompt execution changes

When generating a response, create a Fantasy agent with options:

- `core.WithTools(tools...)`
- `core.WithStopConditions(...)` using step count bound derived from `max_tool_iterations` (when > 0)

`core.StepCountIs(n)` limits agent steps, not individual tool calls. This means a tool-heavy step can still include multiple tool calls, and execution can stop immediately after a tool step.

To preserve a usable UX when the limit is reached, add explicit finalization behavior: run one final step with tools disabled (for example via `PrepareStep` setting `DisableAllTools=true`) so the model can summarize outcomes for the user.

All filesystem tool handlers must honor `context.Context` cancellation and should use a per-tool timeout (`MaxToolOperationDuration`) derived from the prompt context.

Maintain current behavior for:

- health checks
- session creation
- in-memory history
- usage metadata extraction

When tools are enabled, persist full Fantasy step messages (`result.Steps[*].Messages`) into session history instead of only final assistant text. This preserves tool-call and tool-result context required for coherent multi-turn behavior.

### Iteration policy

- If `max_tool_iterations > 0`: enforce stop condition with `core.StepCountIs(max_tool_iterations)` plus explicit finalization step behavior.
- If unset or <= 0: use safe default (recommended: 20, aligned with config example).

### Tool-call validation and repair policy

- Validate tool call inputs against required parameters before execution (Fantasy performs required-field checks; keep local behavior deterministic).
- Phase 1 default: do not enable auto-repair (`core.WithRepairToolCall`) to keep behavior predictable.
- Optional follow-up: enable `WithRepairToolCall` for malformed JSON or minor schema mismatches if real-world prompts show high failure rates.

## 5) Security Considerations

- Strict path containment checks are mandatory.
- Resolve and verify symlink targets to prevent workspace escape.
- Re-validate canonical containment immediately before write/rename to reduce TOCTOU risk.
- Ensure no tool allows shell execution.
- Consider deny access to special files/devices (future hardening).
- Keep tool output bounded to avoid token explosion (using explicit phase-1 limits).
- Avoid leaking absolute host paths in normal responses; return workspace-relative paths when representable.

### Security model implications

#### Trust boundaries

- Model/tool-call output is untrusted input and must always be validated before filesystem access.
- Workspace root + containment checks are the primary security boundary in phase 1.
- Host OS permissions still apply; this feature does not introduce an OS-level sandbox.

#### Key risks and mitigations

- `workspace_escape_via_paths`
  - Risk: prompt/tool input attempts `..` traversal or absolute-path escape.
  - Mitigation (phase 1): canonicalize path, enforce containment, reject outside paths with `outside_workspace`.
  - Residual risk: low, assuming guard is consistently used by all tools.
- `workspace_escape_via_symlink`
  - Risk: symlink points outside workspace.
  - Mitigation (phase 1): resolve symlink target and enforce containment on effective path.
  - Residual risk: medium-low due to race windows; reduced by pre-operation re-check.
- `toctou_path_race`
  - Risk: validated path changes between check and write/rename.
  - Mitigation (phase 1): re-check canonical containment immediately before final open/create/rename.
  - Residual risk: medium; future hardening can add fd-based safe-open patterns where practical.
- `prompt_induced_destructive_write`
  - Risk: model follows malicious instructions and overwrites/deletes useful files inside workspace.
  - Mitigation (phase 1): scoped tool set (no shell), strict workspace boundary, deterministic edit semantics.
  - Residual risk: medium; approval workflows are future work.
- `data_exfiltration_from_workspace`
  - Risk: model reads sensitive files within workspace and includes contents in responses.
  - Mitigation (phase 1): workspace-only scope, response size limits, no external network tools added by this feature.
  - Residual risk: medium; users must choose safe workspace roots.
- `resource_exhaustion`
  - Risk: very large reads/lists/writes or runaway loops consume resources.
  - Mitigation (phase 1): explicit read/write/list limits, per-tool timeout, step-bound stop condition.
  - Residual risk: low-medium depending on workspace size and host limits.
- `sensitive_log_leakage`
  - Risk: logs expose file contents or absolute host paths.
  - Mitigation (phase 1): do not log file contents; prefer safe normalized/relative paths and error categories.
  - Residual risk: low if logging policy is enforced.
- `cross_session_state_leakage`
  - Risk: tool context/history from one session appears in another.
  - Mitigation (phase 1): keep session histories isolated and keyed by session ID.
  - Residual risk: low with current in-memory isolation model.

#### Security assumptions and out-of-scope items

- Users control the configured workspace root and should avoid pointing to overly broad directories (for example, home directory root) in production-like usage.
- No interactive approval or policy engine is included in phase 1.
- No MAC/container sandboxing is included in phase 1; future phases may add stronger process-level isolation.

## 6) Reliability Considerations

- Prefer atomic write pattern for `write_file` and `edit_file` (temporary file + rename) when feasible.
- Preserve existing file mode where practical on rewrite.
- Handle CRLF/LF transparently (no aggressive normalization in phase 1).
- Ensure deterministic sort order for directory listing.
- Define deterministic text/binary detection and keep behavior identical across platforms.
- Define deterministic truncation messaging for oversized reads/listings.
- Keep per-session tool execution sequential in phase 1 for deterministic ordering.
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
- recoverable errors return `core.NewTextErrorResponse(...)` and do not abort agent loop
- fatal tool errors (non-nil `error`) are treated as hard failures

#### `pkg/provider/fantasy`

- tools are attached during generation path
- max tool iteration stop condition configuration (`StepCountIs` semantics)
- limit-hit finalization step with tools disabled
- tool-enabled session history stores step messages (tool calls/results included)
- repair policy behavior (disabled by default in phase 1)
- existing prompt/session tests still pass

### Integration smoke tests (optional in phase 1)

- Run `fantasy-agent` prompt that causes file creation and readback in a temporary workspace.
- Add platform-oriented coverage for path/permission/symlink behavior (macOS + Linux in CI if available).
- Add cancellation smoke test (cancel context while tool is running).

### Test pass criteria matrix

| Scope | Required checks | Pass criteria |
| --- | --- | --- |
| Workspace guard | package unit tests | path traversal/symlink escape blocked with stable categories |
| FS service | package unit tests | deterministic read/write/append/list/edit behavior, limits enforced |
| Fantasy tools adapter | package unit tests | tool names/schema/error formatting stable |
| Fantasy provider wiring | package unit tests | tools attached, step limits/finalization/history behavior correct |
| Docs/config consistency | manual review in PR | README/docs/config example aligned with implemented behavior |
| Repo quality gates | `task fmt`, `task vet`, `task test`, `task build` | all commands succeed locally |

## 9) Documentation Updates Required

Update all of the following:

- `README.md`
  - Explain fantasy-agent filesystem tool capability and workspace restrictions.
  - Include a short end-to-end example prompt with tool usage and final response behavior.
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

## Known Limitations (Phase 1)

- No approval workflow before file modifications.
- No recursive search/glob tools.
- `edit_file` supports exact string replacement only (no regex/pattern edits).
- No OS-level sandboxing beyond workspace containment and host permissions.
- Filesystem tools are sequential only; no parallel tool execution in this phase.

## 11) Open Questions and Decisions to Confirm

1. Absolute path policy when `restrict_to_workspace=false`:
   - Option A (recommended): still enforce workspace containment in phase 1.
   - Option B: permit absolute external paths when restriction is disabled.
2. Parent directory creation behavior for write/append:
   - Option A (recommended): auto-create.
   - Option B: fail if parent is missing.
3. Read/list size limits:
   - Option A (recommended): keep fixed phase-1 defaults (`256 KiB`, `500`) and tune in phase 2.
   - Option B: expose limits as user-configurable fields immediately.
4. `edit_file` semantics:
   - Exact-match only in phase 1 (recommended) versus regex or pattern edits.
5. Tool-call repair behavior:
   - Option A (recommended): keep `WithRepairToolCall` disabled in phase 1 for deterministic behavior.
   - Option B: enable repair to improve resilience to malformed tool JSON.
6. Limit-hit response behavior:
   - Option A (recommended): force one final no-tools summarization step.
   - Option B: stop immediately when step limit is reached.
7. Binary detection strictness:
   - Option A (recommended): reject on NUL byte or invalid UTF-8.
   - Option B: allow lossy replacement decoding for some files.

## Decision Log (Phase 1)

- Use sequential Fantasy tools (`core.NewAgentTool`) and defer `core.NewParallelAgentTool`.
- Enforce step-bound loop control with `core.StepCountIs(max_tool_iterations)`.
- On iteration-limit hit, run one final step with tools disabled to produce a user-facing summary.
- Keep `WithRepairToolCall` disabled by default for deterministic behavior.
- Return recoverable tool issues via `core.NewTextErrorResponse(...)`; reserve non-nil `error` for fatal failures.
- Preserve full Fantasy step messages (`result.Steps[*].Messages`) in session history when tools are enabled.
- Apply fixed phase-1 limits (read/write/list/timeout) with deterministic truncation and stable error categories.

## 12) Implementation Checklist

- [ ] Add workspace resolver and guard package with tests.
- [ ] Add filesystem service package with tests.
- [ ] Add Fantasy tool catalog adapter with tests.
- [ ] Wire tool catalog + iteration stop into fantasy provider.
- [ ] Add explicit limit-hit finalization behavior (final no-tools step).
- [ ] Preserve full Fantasy step messages in session history when tools are enabled.
- [ ] Document and enforce tool error policy (`TextErrorResponse` vs fatal `error`).
- [ ] Decide and document `WithRepairToolCall` phase-1 policy.
- [ ] Add explicit phase-1 limits (`MaxReadBytes`, `MaxWriteBytes`, `MaxListEntries`, per-tool timeout).
- [ ] Add deterministic binary/text detection and truncation behavior.
- [ ] Add TOCTOU re-check before final write/rename operations.
- [ ] Standardize tool responses to workspace-relative paths where possible.
- [ ] Ensure existing fantasy tests pass and extend where needed.
- [ ] Update README/docs/config example.
- [ ] Run quality gates: `task fmt`, `task vet`, `task test`, `task build`.

## Acceptance Criteria

- `fantasy-agent` can successfully use all five tools within workspace.
- Path escape attempts are blocked with clear errors.
- Tool iteration bound is enforced.
- Hitting the iteration bound still yields a final user-facing summary response.
- Multi-turn tool sessions retain tool-call/result context in history.
- Tool limits are enforced deterministically (read/write/list/timeout) with stable truncation/error messaging.
- Cancellation of prompt context terminates in-flight tool work promptly.
- Existing runtime behavior remains stable.
- Documentation reflects the new behavior and config semantics.
- CI-equivalent local checks pass.
