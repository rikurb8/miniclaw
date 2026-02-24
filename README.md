# Miniclaw
> Not open, nano or pico but miniclaw 🦞

Experimental AI assistant project, work in progress

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

4. Send a prompt:

```bash
go run . agent --prompt "hello from miniclaw"
```
