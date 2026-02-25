# pkg/ui

`pkg/ui` contains terminal user-interface components used by MiniClaw CLI flows.

At a high level, this package is responsible for:

- Providing interactive and one-shot terminal chat experiences.
- Translating prompt function callbacks into async UI events.
- Rendering transcript/status/runtime metadata in a consistent style.
- Encapsulating Bubble Tea state management away from command-layer code.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` entrypoints decide whether to run interactive or one-shot mode.
- `pkg/agent/*` and `pkg/provider/*` execute prompts.
- `pkg/ui/*` renders operator-facing terminal interaction around prompt execution.

Typical chat flow:

1. Entry point provides a `PromptFunc` callback into UI.
2. UI model captures keyboard input and issues async prompt commands.
3. Prompt results/errors are converted into transcript entries.
4. Styled views render history, status, and token/runtime metadata.

## Package Map (Non-test Files And Subpackages)

This list intentionally covers non-test code for quick exploration.

### Subpackage: `pkg/ui/chat`

- `pkg/ui/chat/run.go`
  - Defines UI entrypoints (`RunInteractive`, `RunOneShot`) and callback contracts.
  - Owns top-level Bubble Tea program startup/shutdown behavior.

- `pkg/ui/chat/model.go`
  - Implements Bubble Tea state model, update loop, transcript handling, and viewport behavior.
  - Handles boot animation, keybindings, prompt dispatch, and usage counters.

- `pkg/ui/chat/styles.go`
  - Defines the shared style palette used by chat rendering.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/ui/chat/run.go` (public UI entrypoints).
2. `pkg/ui/chat/model.go` (interaction and rendering state machine).
3. `pkg/ui/chat/styles.go` (visual theme definitions).

That sequence gives you API surface first, then runtime behavior and styling.
