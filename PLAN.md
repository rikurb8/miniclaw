# MiniClaw Plan: Top 3 Focus Areas

## 1) Core Agent Loop + Safe Tooling (MVP)

### What we will do
- Add a minimal tool interface and a small starter toolset (`read`, `glob`, `grep`, `bash`).
- Implement a simple model <-> tool loop with a hard iteration cap (`max_tool_iterations`).
- Add clear safety boundaries: workspace restriction, command guardrails, and output size limits.

### Why this matters
- This is the defining OpenClaw-like behavior: the assistant can do useful work, not only chat.
- A tiny, safe toolset keeps implementation simple while still being practical.
- Strong guardrails make the project safe to run locally and easier to reason about.

### Implementation hints
- Keep tools behind a small internal interface (`Name`, `Description`, `Run`).
- Start read-first and conservative; expand capability only after tests pass.
- Record tool calls/results in memory so behavior is transparent and debuggable.

## 2) Runtime Clarity + CLI UX

### What we will do
- Improve interactive UX with a few slash commands (`/help`, `/exit`, `/clear`, `/model`).
- Add lightweight runtime inspection (session id, in-memory conversation view).
- Standardize error output so failures are consistent and actionable.

### Why this matters
- A clear, predictable CLI is essential for day-to-day usage and learning.
- Runtime visibility helps us understand and debug agent decisions.
- Better error handling reduces friction and speeds iteration.

### Implementation hints
- Keep command parsing in a small dispatch table to avoid branching sprawl.
- Reuse existing `MemorySnapshot()` and session methods before adding new state.
- Prefer human-readable errors with concise context (config, provider, prompt, tool).

## 3) Documentation + Iterative Delivery Discipline

### What we will do
- Update docs to reflect actual behavior and boundaries of the current build.
- Document one clear request lifecycle including tool-call steps.
- Ship in small milestones with tests for each new concept.

### Why this matters
- The project goal is educational, so docs must teach both what and why.
- Small milestones reduce risk and keep architecture understandable.
- Test-backed increments make refactoring safe as capabilities grow.

### Implementation hints
- Keep docs close to code paths (`cmd`, `pkg/agent`, `pkg/provider`, future `pkg/tools`).
- Treat each milestone as a thin vertical slice that can be demonstrated quickly.
- Use tests as executable documentation for queue behavior, tool loop limits, and safety rules.
