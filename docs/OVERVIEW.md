# MiniClaw Overview

MiniClaw is a small local runtime for AI agents.

It currently has two execution modes:

- `agent` mode: direct CLI prompting (single prompt or interactive chat).
- `gateway` mode: channel-driven prompting (Telegram first), with health/readiness endpoints.

Agent behavior is selected by `agents.defaults.type`:

- `generic-agent` (default) runs MiniClaw's local runtime flow.
- `opencode-agent` is a separate runtime mode for OpenCode-backed orchestration.
- `fantasy-agent` runs prompts through `charm.land/fantasy` (currently with OpenAI provider support).

For `fantasy-agent`, MiniClaw can execute workspace-bounded filesystem tools (`read_file`, `write_file`, `append_file`, `list_dir`, `edit_file`) during the model loop.

## Architecture (High Level)

### CLI agent mode

```text
User (terminal)
  |
  v
miniclaw agent (cmd/agent.go)
  - resolves prompt/input mode
  - loads config
  |
  v
agent.Instance (pkg/agent/*)
  - provider session lifecycle
  - direct/heartbeat prompt execution
  - in-memory memory log
  |
  v
provider.Client (pkg/provider/*)
  - Health
  - CreateSession
  - Prompt
```

### Gateway channel mode

```text
External channel (Telegram)
  |
  v
Channel adapter (pkg/channel/telegram)
  - receives inbound updates (long polling)
  - maps to MiniClaw inbound shape
  |
  v
Gateway service (pkg/gateway/service.go)
  - validates provider health
  - routes prompt to runtime manager
  - emits outbound reply per channel
  - serves /healthz and /readyz
  |
  v
Runtime manager (pkg/gateway/runtime_manager.go)
  - one agent runtime per session key
  - one provider session per channel conversation
  |
  v
Provider backend (OpenCode/OpenAI)
```

## Key Concepts

### Runtime Instance

`agent.Instance` is the core execution unit. It owns provider session ID, model/system settings, heartbeat behavior, and in-memory memory entries.

### Session

A provider-side session is created by `CreateSession` and reused across prompts.

- In `agent` mode: one runtime session per process run.
- In `gateway` mode: one runtime session per channel session key.

Telegram v1 uses chat-level continuity via `telegram:<chat_id>`.

### Channel Adapter

A channel adapter translates external transport events to MiniClaw inbound messages and sends outbound replies back.

- Current adapter: Telegram (`pkg/channel/telegram`) with long polling.
- Enabled via config under `channels.*`.
- Telegram allowlist can be applied through `channels.telegram.allow_from`.

### Health and Readiness

Gateway mode exposes two HTTP endpoints:

- `/healthz`: process liveness.
- `/readyz`: readiness based on channel runtime state and provider health checks.

Address is configured by `gateway.host` and `gateway.port`.

## Request Lifecycle

### `agent` mode

```text
CLI input -> resolve prompt -> runtime session exists
  -> provider prompt call -> append memory -> print response
```

### `fantasy-agent` tool loop

```text
Prompt -> Fantasy agent step
  -> optional tool call
  -> workspace guard validates path
  -> filesystem service executes bounded operation
  -> tool result fed back into next step
  -> repeat until completion or max tool iterations
  -> if limit reached: final no-tools summarization step
```

### `gateway` mode

```text
Channel update -> adapter builds inbound message
  -> runtime manager resolves session runtime
  -> provider prompt call -> adapter sends reply to channel
```

## Configuration That Matters Most

For day-to-day behavior, focus on:

- `agents.defaults.type`
- `agents.defaults.workspace`
- `agents.defaults.restrict_to_workspace`
- `agents.defaults.provider`
- `agents.defaults.model`
- `agents.defaults.max_tool_iterations`
- `channels.telegram.*`
- `gateway.host`
- `gateway.port`
- `heartbeat.enabled`
- `heartbeat.interval`

Provider-specific tuning remains under `providers.*`.

## Current Scope

MiniClaw today is intentionally lightweight:

- local CLI runtime (`agent`),
- channel gateway runtime (`gateway`) with Telegram first,
- in-memory runtime state only (no persistent local session store),
- provider-backed prompt execution with optional heartbeat queue support.
