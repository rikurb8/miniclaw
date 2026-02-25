# pkg/gateway

`pkg/gateway` runs the long-lived gateway process that wires channel adapters to agent runtime execution.

At a high level, this package is responsible for:

- Starting and supervising configured channel adapters.
- Routing inbound channel messages to per-session agent runtimes.
- Managing provider health and readiness state.
- Serving HTTP health/readiness endpoints for operations.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` entrypoints load config and launch runtime mode.
- `pkg/gateway/*` hosts service orchestration for channel-driven operation.
- `pkg/channel/*` provides transport adapters (for example Telegram).
- `pkg/agent/*` executes prompt flows.
- `pkg/provider/*` performs concrete provider API calls.

Typical gateway flow:

1. `NewService` resolves provider client and creates a runtime manager.
2. `Run` starts status server and all channel adapters.
3. Channel adapters invoke `handleInbound` for each normalized inbound message.
4. Runtime manager creates/reuses per-session agent instances and executes prompts.
5. Health/readiness endpoints expose operational state.

## Package Map (Non-test Files)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/gateway`

- `pkg/gateway/service.go`
  - Defines `Service`, the top-level gateway orchestrator.
  - Starts adapters, runs provider health checks, serves `/healthz` and `/readyz`, and tracks channel/provider state.

- `pkg/gateway/runtime_manager.go`
  - Defines `runtimeManager`, which owns session-keyed runtime instances.
  - Lazily initializes agent instances per session and serializes prompt execution per session.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/gateway/service.go` (`NewService` and `Run` orchestration flow).
2. `pkg/gateway/runtime_manager.go` (session runtime lifecycle and prompt routing).

That sequence gives you process-level orchestration first, then per-session execution behavior.
