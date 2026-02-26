# pkg/config

`pkg/config` loads MiniClaw runtime configuration from disk and applies selected environment overrides.

At a high level, this package is responsible for:

- Defining the full JSON config schema used by runtime components.
- Resolving the active config file path using environment + cwd fallbacks.
- Parsing config JSON into typed Go structs.
- Applying env-based overrides for sensitive or deployment-specific values.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and `pkg/gateway/*` are entrypoints and orchestration surfaces.
- `pkg/config/*` provides typed runtime configuration for all layers.
- `pkg/agent/*`, `pkg/provider/*`, `pkg/channel/*`, and `pkg/logger/*` consume config values.

Typical startup flow:

1. Entry point calls `config.LoadConfig()`.
2. Config file path is resolved (`MINICLAW_CONFIG`, then cwd fallbacks).
3. JSON is unmarshaled into `Config`.
4. Selected env values override file values (for example Telegram token settings).

## Agent defaults fields worth knowing

`agents.defaults` contains runtime controls used across agent types. For fantasy tooling behavior, these fields are important:

- `workspace`: workspace root used for filesystem tools.
- `restrict_to_workspace`: workspace safety policy flag.
- `max_tool_iterations`: step-bound limit for tool loops.

See `config/config.example.json` and `README.md` for practical guidance.

## Package Map (Non-test Files)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/config`

- `pkg/config/config.go`
  - Defines root config model (`Config`) and nested subsystem settings.
  - Implements file resolution, JSON loading, and env override helpers.

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `pkg/config/config.go` top-level structs (what can be configured).
2. `LoadConfig` and `findConfigPath` (where config comes from).
3. `applyEnvOverrides` (which values can be injected via env).

That sequence gives you the schema first, then runtime resolution behavior.
