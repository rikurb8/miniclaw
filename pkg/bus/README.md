# pkg/bus

`pkg/bus` provides the in-process message transport primitives used by MiniClaw runtime flows.

At a high level, this package is responsible for:

- Carrying inbound messages from entrypoints into runtime processing.
- Carrying outbound messages back to callers/UI layers.
- Registering channel-scoped handlers used by runtime orchestration.
- Broadcasting lightweight lifecycle events for observability.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and `pkg/gateway/*` are entrypoints and orchestration surfaces.
- `pkg/agent/*` is the execution core and local runtime plumbing.
- `pkg/provider/*` talks to concrete AI providers.
- `pkg/bus/*` provides in-process transport/events used by runtime flows.

Typical local runtime interaction:

1. Callers publish prompts as `InboundMessage` values.
2. Runtime workers consume inbound messages and execute prompt logic.
3. Results are published as `OutboundMessage` values.
4. Lifecycle updates are emitted as `Event` values for logging/telemetry.

## Package Map (Non-test Files)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/bus`

- `pkg/bus/types.go`
  - Defines shared transport types: `InboundMessage`, `OutboundMessage`, and `MessageHandler`.
  - Keeps runtime-facing message shape stable across callers.

- `pkg/bus/bus.go`
  - Defines `MessageBus`, the in-memory queue + handler registry.
  - Implements inbound/outbound publish/consume behavior and close semantics.

- `pkg/bus/events.go`
  - Defines event enums and payload shape used for runtime lifecycle signaling.
  - Implements event fan-out subscriptions with non-blocking publish behavior.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/bus/types.go` (what data moves through the bus).
2. `pkg/bus/bus.go` (core queue operations and shutdown behavior).
3. `pkg/bus/events.go` (event broadcasting and subscription lifecycle).

That sequence gives you the core transport model before event details.
