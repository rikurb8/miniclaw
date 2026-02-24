# MiniClaw Overview

MiniClaw is a small agent runtime you run locally from the command line.

Think of it as a thin coordinator between three things:

- user input from the CLI,
- a runtime that manages session + state,
- an external model provider (currently OpenCode and OpenAI).

Agent behavior is selected by `agents.defaults.type`:

- `generic-agent` (default) runs MiniClaw's local runtime flow.
- `opencode-agent` is a separate runtime mode for OpenCode-backed orchestration.

## Architecture (High Level)

```text
User
  |
  v
miniclaw CLI (cmd/*)
  - parses command and input
  - loads config
  |
  v
Agent Runtime (pkg/agent/*)
  - starts/holds session
  - routes prompts
  - optional heartbeat queue
  - in-memory conversation log
  |
  v
Provider Interface (pkg/provider/*)
  - Health
  - CreateSession
  - Prompt
  |
  v
Provider backend (OpenCode/OpenAI)
```

## Key Concepts

### Runtime Instance

`agent.Instance` is the core unit of execution. One instance represents one running agent context in the process.

It holds:

- the selected provider client,
- the active remote session id,
- the default model,
- heartbeat behavior,
- local in-memory message history.

### Session

A session is created on the provider side and identified by `sessionID`.

- prompts in the same run are sent to the same session,
- session continuity is per running process,
- starting a new process starts a new session.

### Provider Boundary

MiniClaw runtime code depends on a small provider contract, not on OpenCode specifics directly.

This keeps provider-specific API details inside `pkg/provider/opencode` and makes future providers easier to add.

### Prompt Execution Modes

MiniClaw supports two ways to execute prompts:

- direct mode: send immediately,
- heartbeat mode: enqueue first, process on interval ticks.

Conceptually, heartbeat mode turns prompt handling into a simple pull loop.

### Memory

MiniClaw stores conversation entries (`user` and `assistant`) in process memory.

- useful for runtime state inspection,
- not persisted across restarts,
- separate from provider-side session storage.

## Request Lifecycle

```text
CLI input -> resolve prompt -> runtime checks session
  -> provider prompt call -> response text
  -> append to local memory -> print to user
```

With heartbeat enabled, `enqueue prompt` happens first and provider call is done by the heartbeat step.

## Configuration That Matters Most

For understanding runtime behavior, focus on:

- `agents.defaults.type`
- `agents.defaults.provider`
- `agents.defaults.model`
- `providers.opencode.*`
- `heartbeat.enabled`
- `heartbeat.interval`

Other config sections exist for broader project scope, but are not central to the current `agent` flow.

## Current Scope

MiniClaw today is best viewed as:

- a local CLI entrypoint,
- a single-session runtime per process,
- a provider-backed prompt loop with optional queued execution.

It is intentionally lightweight and does not yet act as a full multi-session, persisted orchestration platform.
