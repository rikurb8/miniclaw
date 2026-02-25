# Supported Agents

MiniClaw selects agent runtime behavior through `agents.defaults.type` in config.

## Built-in agent profile (v1)

- MiniClaw includes a built-in default profile template at `pkg/agent/profile/templates/default.md`.
- For non-OpenCode providers, MiniClaw injects this profile as system instructions.
- For OpenCode provider, MiniClaw does not inject a local system profile by default because OpenCode server-side agent prompts are the source of truth.

## `generic-agent`

- Default runtime mode.
- Uses MiniClaw local runtime flow (`pkg/agent/*`) with provider selected by `agents.defaults.provider`.
- Best choice for standard local CLI runs.

## `opencode-agent`

- OpenCode-oriented runtime mode.
- Uses the same local runtime flow but keeps a distinct runtime type for OpenCode orchestration compatibility.
- Use when you want the OpenCode runtime identity in config.

## `fantasy-agent`

- Fantasy-powered runtime mode using `charm.land/fantasy`.
- Current provider support: `openai` only.
- Maintains in-process conversation history per session and executes prompts through Fantasy's agent API.

## Config Example

```json
{
  "agents": {
    "defaults": {
      "type": "fantasy-agent",
      "provider": "openai",
      "model": "openai/gpt-5.2"
    }
  }
}
```

See `docs/OVERVIEW.md` for architecture details.
