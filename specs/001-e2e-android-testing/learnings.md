# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## Pre-implementation (from failed attempt 1)

- **NEVER write orchestration code for MCP E2E tasks.** The first attempt produced ~6000 lines of shell scripts (scenario runners, prompt templates, report libraries) that duplicated what the runner already provides. Agents must use MCP tools directly against the live emulator.
- **Validate with real infrastructure, not synthetic data.** The first attempt's "validation" tasks tested the framework with fake data instead of booting an emulator. Every validation must touch the real app.
- **The parallel runner handles emulator boot, APK build+install, MCP server lifecycle.** Tasks just need `[needs: mcp-android, e2e-loop]` — the runner does the rest.
