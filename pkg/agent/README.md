# pkg/agent

`pkg/agent` provides the core agent execution primitives and local runtime wiring for MiniClaw prompts.

At a high level, this part of the system is responsible for:

- Holding per-agent runtime state (provider-created session identifier, model, system profile).
- Executing prompts directly or through a heartbeat-driven queue.
- Recording lightweight in-memory conversation history.
- Providing runtime orchestration helpers used by CLI and gateway entrypoints.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and `pkg/gateway/*` are entrypoints and orchestration surfaces.
- `pkg/agent/*` is the execution core and local runtime plumbing.
- `pkg/provider/*` talks to concrete AI providers.
- `pkg/bus/*` provides in-process message transport/events used by runtime flows.

Typical flow for local CLI mode:

1. `cmd/agent` loads config and provider client.
2. `pkg/agent/runtime` starts a `LocalSession`.
3. `LocalSession` uses `pkg/agent.Instance` to manage session + prompts.
4. Prompt requests move through `pkg/bus` and come back as provider results.
5. Usage metadata is attached so UI/logging layers can report token usage.

Gateway mode follows a similar prompt lifecycle, but execution is coordinated by `pkg/gateway/runtime_manager` with `pkg/agent.Instance` rather than the interactive chat runtime path.

## Package Map (Non-test Files And Subpackages)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/agent`

- `pkg/agent/instance.go`
  - Defines `Instance`, the main provider-backed agent object.
  - Handles session startup (`StartSession`), prompt execution (`Prompt`), prompt queueing (`EnqueueAndWait`), and shared state synchronization.

- `pkg/agent/loop.go`
  - Implements heartbeat loop behavior (`Run`) and queue draining.
  - Processes queued prompts either when signaled (`queueWake`) or on ticker intervals.

- `pkg/agent/memory.go`
  - Implements a thread-safe in-memory transcript store.
  - Tracks role/content/timestamp entries and exposes snapshot/clear helpers.

### Subpackage: `pkg/agent/profile`

- `pkg/agent/profile/defaults.go`
  - Selects which system profile template to use for a provider.
  - Encodes provider-specific defaults (for example, OpenCode currently maps to no template).

- `pkg/agent/profile/loader.go`
  - Loads embedded markdown prompt templates from `templates/`.
  - Exposes `ResolveSystemProfile(provider)` for callers that need the final system prompt text.

- `pkg/agent/profile/templates/default.md`
  - Embedded default system profile template content used when provider defaults request it.

### Subpackage: `pkg/agent/runtime`

- `pkg/agent/runtime/local_session.go`
  - Defines `LocalSession`, which wires together one agent instance, one message bus, a bus worker, and an optional heartbeat goroutine.
  - Exposes a prompt API used by CLI/chat flows and manages shutdown semantics.

- `pkg/agent/runtime/events.go`
  - Subscribes to bus events and maps event types to structured log levels.
  - Keeps runtime observability decoupled from command-layer code.

- `pkg/agent/runtime/usage.go`
  - Centralizes token-usage metadata encoding/decoding between provider results and bus metadata maps.
  - Shared by runtime and gateway paths to avoid format drift.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/agent/instance.go` (what an agent instance can do).
2. `pkg/agent/loop.go` (how heartbeat queue processing works).
3. `pkg/agent/runtime/local_session.go` (how components are composed for local execution).
4. `pkg/agent/runtime/usage.go` and `pkg/agent/runtime/events.go` (supporting concerns).
5. `pkg/agent/profile/loader.go` (how system prompt text is resolved).

That sequence gives you the core execution path before the supporting pieces.
