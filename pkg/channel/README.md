# pkg/channel

`pkg/channel` defines transport adapter contracts and channel implementations that connect external messaging systems to MiniClaw runtime processing.

At a high level, this part of the system is responsible for:

- Defining the shared adapter interface used by channel integrations.
- Normalizing transport input into `pkg/bus.InboundMessage` values.
- Passing normalized messages to runtime handlers and returning replies.
- Providing concrete channel adapters (currently Telegram).

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and `pkg/gateway/*` are entrypoints and orchestration surfaces.
- `pkg/agent/*` is the execution core and local runtime plumbing.
- `pkg/provider/*` talks to concrete AI providers.
- `pkg/channel/*` bridges external transports to the runtime handler model.
- `pkg/bus/*` carries normalized messages/events through runtime flows.

Typical channel flow:

1. A channel adapter receives a transport-specific message/update.
2. The adapter converts it into a `bus.InboundMessage`.
3. The adapter calls the shared `channel.Handler`.
4. Handler output is converted back to transport-specific send operations.

## Package Map (Non-test Files And Subpackages)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/channel`

- `pkg/channel/channel.go`
  - Defines `Handler`, the transport-agnostic request/reply function type.
  - Defines `Adapter`, the interface implemented by each channel integration.

### Subpackage: `pkg/channel/telegram`

- `pkg/channel/telegram/telegram.go`
  - Implements the Telegram adapter using long polling.
  - Validates inbound updates, applies optional sender allow-list filtering, maps updates to bus messages, and sends replies.
  - Emits periodic typing indicators while handler execution is in progress.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/channel/channel.go` (shared contracts used by all adapters).
2. `pkg/channel/telegram/telegram.go` (one concrete adapter implementation).

That sequence gives you the abstraction first, then a full real-world implementation.
