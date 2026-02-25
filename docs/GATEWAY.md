# Gateway and Channels

MiniClaw gateway mode lets external channels send prompts into the same runtime/provider flow used by CLI mode.

Run it with:

```bash
miniclaw gateway
```

## Current Channel Support

- `telegram` (first implementation), built with `github.com/mymmrac/telego`.
- Update delivery mode: long polling.

## Message Routing Model

1. Channel adapter receives inbound message.
2. Adapter maps it to MiniClaw inbound structure (`channel`, `chat_id`, `session_key`, `content`).
3. Gateway runtime manager selects (or creates) one `agent.Instance` per `session_key`.
4. Prompt is sent to the configured provider.
5. Outbound text is sent back through the same channel adapter.

## Session Continuity

- Gateway keeps one runtime per session key in memory.
- Telegram v1 session key format: `telegram:<chat_id>`.
- Result: each Telegram chat gets its own provider session continuity while process is running.

## Health Endpoints

Gateway starts a small HTTP status server using `gateway.host` and `gateway.port`.

- `GET /healthz`: liveness endpoint (process is up).
- `GET /readyz`: readiness endpoint (at least one channel running and provider healthy).

## Telegram Configuration

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "",
      "allow_from": []
    }
  },
  "gateway": {
    "host": "0.0.0.0",
    "port": 18790
  }
}
```

Notes:

- `channels.telegram.token` is required when Telegram is enabled.
- `channels.telegram.allow_from` is optional. If set, only listed sender IDs are accepted.
- Environment overrides are supported:
  - `TELEGRAM_BOT_TOKEN` overrides `channels.telegram.token`.
  - `TELEGRAM_ALLOW_FROM` overrides `channels.telegram.allow_from` with a comma-separated list.
- Non-text updates are ignored in v1.

## Docker Healthcheck Example

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://127.0.0.1:18790/healthz || exit 1
```
