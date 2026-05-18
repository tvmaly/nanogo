# cmd/nanogo Composition

`cmd/nanogo` stays thin: it parses flags, loads config, wires providers, stores,
events, tools, memory, transports, and command handlers, then delegates behavior
to `core/`, `modules/`, and `ext/` packages.

Phase 17.5 keeps the main binary ready for Phase 18 flow orchestration by using
one runtime tool-source path for prompt, REPL, skill, heartbeat, and transport
turns. That path wires `spawn` with a configured `core/agent.SubagentRunner`, so
delegation keeps using normal sessions, event publishing, tool filtering,
subagent concurrency limits, and subagent timeouts.

Configured transports are started through the `modules/transport` registry and a
small local `transport.App` adapter. Future flow commands should reuse the same
composition pattern instead of adding another agent loop in `core/`.

Relevant checks:

```sh
go test ./cmd/nanogo
scripts/check_imports.sh
```
