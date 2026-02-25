# pkg/logger

`pkg/logger` builds MiniClaw's application logger and centralizes log-format behavior.

At a high level, this package is responsible for:

- Constructing a `slog.Logger` from config and env overrides.
- Supporting human-readable text logs and structured JSON logs.
- Normalizing field/caller handling in JSON mode.
- Keeping log output behavior consistent across packages.

## How It Fits In The System

MiniClaw has a few major layers:

- `cmd/*` and `pkg/gateway/*` initialize process-level logging.
- `pkg/logger/*` creates the logger implementation and formatting behavior.
- Runtime packages log through `slog` with component-scoped fields.

Typical startup interaction:

1. Entry point reads `config.Logging`.
2. `logger.New` resolves effective format/level/source flags.
3. A logger is returned and used as process default.
4. Packages emit logs with `component` fields for filtering.

## Package Map (Non-test Files)

This list intentionally covers non-test code for quick exploration.

### Root package: `pkg/logger`

- `pkg/logger/logger.go`
  - Defines logger config resolution and construction.
  - Implements a custom JSON `slog.Handler` used for stable machine-readable output.
  - Preserves a consistent top-level envelope (`level`, `timestamp`, `component`, `message`, `fields`, `caller`).

## Mental Model For Explorers

If you are new to this code, a practical read order is:

1. `New` and `newWithWriter` (how logger instances are created).
2. `parseLevel`/format resolution (config + env precedence).
3. `entryHandler.Handle` and `applyAttr` (JSON output shape and field flattening).

That sequence gives you setup flow first, then serialization details.
