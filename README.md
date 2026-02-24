# Miniclaw
> Not open, nano or pico but miniclaw 🦞

Experimental AI assistant project, work in progress

## Documentation

For a high-level architecture and key concepts walkthrough, see `docs/OVERVIEW.md`.

## Agent types

MiniClaw supports configurable agent runtimes through `agents.defaults.type`:

- `generic-agent` (default): local MiniClaw runtime flow.
- `opencode-agent`: reserved runtime mode for OpenCode-backed orchestration.

## OpenCode provider (first supported provider)

MiniClaw currently supports connecting to a running OpenCode server using
`github.com/sst/opencode-sdk-go`.

### Quickstart

1. Start OpenCode server in another terminal:

```bash
opencode serve --port 4096 --hostname 127.0.0.1
```

2. Copy `config/config.example.json` to `config/config.json` and update any values you need.

3. If your OpenCode server uses basic auth, set the password env var:

```bash
export OPENCODE_SERVER_PASSWORD=your-password
```

4. Send a single prompt:

```bash
go run . agent --prompt "hello from miniclaw"
```

5. Start interactive mode (same session for all turns):

```bash
go run . agent
```

## OpenAI provider

MiniClaw also supports OpenAI via `github.com/openai/openai-go/v3`.

### Quickstart

1. Set your API key:

```bash
export OPENAI_API_KEY=sk-...
```

2. In `config/config.json`, set:

- `agents.defaults.provider` to `openai`
- `providers.openai.api_key_env` to `OPENAI_API_KEY`
- `agents.defaults.model` to an OpenAI model (for example `openai/gpt-5.2` or `gpt-5.2`)

3. Run:

```bash
go run . agent --prompt "hello from openai provider"
```
