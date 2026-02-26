# Supported Agents

MiniClaw selects agent runtime behavior through `agents.defaults.type` in config.

These runtime types apply to both `agent` (CLI) and `gateway` (channel) commands.

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
- Enables workspace-bounded filesystem tools: `read_file`, `write_file`, `append_file`, `list_dir`, `edit_file`.
- Resolves workspace from `agents.defaults.workspace` (creates it when missing) and blocks traversal/symlink escape attempts.
- Enforces tool-step limits using `agents.defaults.max_tool_iterations` (default `20` when unset).
- On tool-step limit hit, runs one final no-tools step so the user still gets a final summary response.
- When tools are enabled, persists full fantasy step messages (tool calls/results included) into session history for multi-turn coherence.

### Fantasy tool limits (phase 1)

- `read_file`: max `256 KiB`
- `write_file` / `append_file` / `edit_file`: max `1 MiB` payload
- `list_dir`: max `500` entries (deterministic truncation)
- per-tool timeout: `10s`

Safety note: this is not a host-level sandbox; host OS permissions still apply.

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
