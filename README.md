# Miniclaw
> Not open, nano or pico but miniclaw 🦞

MiniClaw is a small terminal AI assistant runner.
You configure a provider/model, then run one-off prompts or an interactive chat loop.

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

That is enough to try MiniClaw end to end.

## Documentation

For a high-level architecture and key concepts walkthrough, see `docs/OVERVIEW.md`.
For runtime type details, see `docs/AGENTS.md`.

## Development Commands

This repository now uses Taskfile as the primary local workflow.

- Run CI gates: `task ci`
- Format check: `task fmt`
- Format fix: `task fmt:fix`
- Vet: `task vet`
- Test: `task test`
- Build: `task build`
- Run: `task run -- agent --prompt "hello"`

## Agent types

MiniClaw supports configurable agent runtimes through `agents.defaults.type`:

- `generic-agent` (default): local MiniClaw runtime flow.
- `opencode-agent`: reserved runtime mode for OpenCode-backed orchestration.
- `fantasy-agent`: Fantasy-backed runtime flow (`charm.land/fantasy`), currently OpenAI provider only.

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
