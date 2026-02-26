# pkg/provider

`pkg/provider` defines provider-facing contracts and concrete provider implementations used to execute prompts.

At a high level, this part of the system is responsible for:

- Exposing a provider-agnostic `Client` interface for agent/runtime layers.
- Resolving which provider client to construct from configuration.
- Normalizing provider responses into shared result/usage types.
- Implementing concrete provider clients (OpenCode, OpenAI, Fantasy/OpenAI).

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and runtime entrypoints load config and call `provider.New`.
- `pkg/agent/*` drives provider sessions and prompts through the `Client` interface.
- `pkg/provider/*` handles provider-specific API details and response normalization.
- `pkg/bus/*` and `pkg/channel/*` move messages before/after provider calls.

Typical prompt lifecycle:

1. Runtime resolves a provider via `provider.New`.
2. Provider client creates or reuses a session.
3. Runtime calls `Prompt(...)` with session/model/input context.
4. Provider returns `types.PromptResult` with normalized text + usage metadata.

## Package Map (Non-test Files And Subpackages)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/provider`

- `pkg/provider/provider.go`
  - Defines the shared `Client` interface.
  - Implements provider factory selection based on `config.Agents.Defaults.Provider`.

### Subpackage: `pkg/provider/types`

- `pkg/provider/types/types.go`
  - Defines normalized provider result metadata and token usage types.
  - Shared by provider implementations and runtime/UI consumers.

### Subpackage: `pkg/provider/opencode`

- `pkg/provider/opencode/opencode.go`
  - Implements OpenCode SDK-backed provider behavior.
  - Supports session creation, prompt execution, health checks, optional basic auth, and token usage extraction.

### Subpackage: `pkg/provider/openai`

- `pkg/provider/openai/openai.go`
  - Implements OpenAI SDK-backed provider behavior using Conversations/Responses APIs.
  - Handles model normalization, session creation, prompt execution, health checks, and usage extraction.

### Subpackage: `pkg/provider/fantasy`

- `pkg/provider/fantasy/fantasy.go`
  - Implements an in-memory-session provider using `charm.land/fantasy` with OpenAI backend.
  - Maintains local message history per session and returns normalized prompt results.
  - Wires workspace-bounded filesystem tools (`read_file`, `write_file`, `append_file`, `list_dir`, `edit_file`) for `fantasy-agent`.
  - Applies tool-step loop bounds and a final no-tools summarization step when iteration limit is hit.

### Related tool/workspace packages

- `pkg/workspace`
  - Resolves workspace root and enforces path containment with stable error categories.
- `pkg/tools/fs`
  - Provides bounded filesystem operations behind an internal service API.
- `pkg/tools/fantasy`
  - Adapts filesystem service methods to Fantasy `AgentTool` definitions.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/provider/provider.go` (shared contract + factory behavior).
2. `pkg/provider/types/types.go` (normalized result model).
3. `pkg/provider/opencode/opencode.go` or `pkg/provider/openai/openai.go` (primary concrete implementations).
4. `pkg/provider/fantasy/fantasy.go` (alternative in-memory/fantasy path).

That sequence gives you abstraction first, then concrete provider details.
