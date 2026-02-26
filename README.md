# Miniclaw
> Not open, nano or pico but miniclaw 🦞

MiniClaw is a small AI assistant runtime.
You can run it in local CLI mode (`agent`) or in gateway mode (`gateway`) for external channels.

## Fast Start

Use Docker Compose for the quickest setup.

1. Create local config:

```bash
cp config/config.example.json config/config.json
```

2. Create `.env` and add your API key:

```bash
cp .env.example .env
```

Then edit `.env` and set:

```bash
OPENAI_API_KEY=sk-your-real-key
```

3. Send one test prompt:

```bash
docker compose run --rm miniclaw agent --prompt "hello from miniclaw"
```

4. Start interactive chat:

```bash
docker compose run --rm miniclaw agent
```

Interactive chat tip: use `Ctrl+T` to toggle inline tool-call cards in conversation history.

That is enough to try MiniClaw end to end.

## Gateway Mode (Channels)

MiniClaw can also run as a channel gateway.

- Current channel support: `telegram` via `telego` long polling.
- Channel/runtime continuity: one provider session per channel session key (Telegram uses `telegram:<chat_id>`).
- Status endpoints for orchestration:
  - `GET /healthz` for liveness.
  - `GET /readyz` for readiness (channel running + provider health).

### Telegram Gateway Quickstart

1. In `config/config.json`, enable Telegram channel:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "",
      "allow_from": []
    }
  }
}
```

2. In `.env`, set Telegram secrets/allowlist:

```bash
TELEGRAM_BOT_TOKEN=123456:your-bot-token
TELEGRAM_ALLOW_FROM=123456789
```

`TELEGRAM_ALLOW_FROM` accepts comma-separated user IDs (for example `123,456,789`).

3. Start gateway mode:

```bash
go run . gateway
```

4. Check health:

```bash
curl -fsS http://127.0.0.1:18790/healthz
curl -fsS http://127.0.0.1:18790/readyz
```

## Documentation

For a high-level architecture and key concepts walkthrough, see `docs/OVERVIEW.md`.
For runtime type details, see `docs/AGENTS.md`.
For gateway and channel details, see `docs/GATEWAY.md`.

## Development Commands

This repository now uses Taskfile as the primary local workflow.

- Run CI gates: `task ci`
- Format check: `task fmt`
- Format fix: `task fmt:fix`
- Vet: `task vet`
- Test: `task test`
- Build: `task build`
- Run: `task run -- agent --prompt "hello"`
- Run gateway: `task run -- gateway`

## Agent types

MiniClaw supports configurable agent runtimes through `agents.defaults.type`:

- `generic-agent` (default): local MiniClaw runtime flow.
- `opencode-agent`: reserved runtime mode for OpenCode-backed orchestration.
- `fantasy-agent`: Fantasy-backed runtime flow (`charm.land/fantasy`), currently OpenAI provider only.

### Fantasy filesystem tools (phase 1)

When `agents.defaults.type` is `fantasy-agent`, MiniClaw enables a small filesystem toolset for the model:

- `read_file`
- `write_file`
- `append_file`
- `list_dir`
- `edit_file`

All tool operations are restricted to `agents.defaults.workspace`.
MiniClaw resolves that path, creates it if needed, and blocks path traversal/symlink escapes.

Tooling safety defaults:

- max tool iterations: `agents.defaults.max_tool_iterations` (default `20` when unset)
- max read payload: `256 KiB`
- max write/append/edit payload: `1 MiB`
- max directory entries per `list_dir`: `500`
- per-tool timeout: `10s`

Safety note: this is a workspace boundary, not an OS sandbox.
Choose a narrow workspace directory for production-like usage.

Example prompt:

```bash
go run . agent --prompt "Create notes/today.md with a checklist, append one more item, then read it back and summarize what you changed."
```

When tool-step limits are reached, MiniClaw runs one final no-tools summarization step so users still get a readable final answer.

## OpenAI provider

MiniClaw also supports OpenAI via `github.com/openai/openai-go/v3`.

### Quickstart

1. Set your API key:

```bash
export OPENAI_API_KEY=sk-...
```

2. In `config/config.json`, set:

- `agents.defaults.type` to `fantasy-agent` (for Fantasy runtime) or `generic-agent` (default local runtime)
- `agents.defaults.provider` to `openai`
- `agents.defaults.model` to an OpenAI model (for example `openai/gpt-5.2` or `gpt-5.2`)

3. Run:

```bash
go run . agent --prompt "hello from openai provider"
```

## OpenCode provider

MiniClaw can also connect to a running OpenCode server using `github.com/sst/opencode-sdk-go`.

1. Start OpenCode server in another terminal:

```bash
opencode serve --port 4096 --hostname 127.0.0.1
```

2. Set provider config in `config/config.json` to use `opencode`.

3. If basic auth is enabled, set:

```bash
export OPENCODE_SERVER_PASSWORD=your-password
```
