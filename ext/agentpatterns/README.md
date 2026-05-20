# ext/agentpatterns

`ext/agentpatterns` is the Phase 18 v3 contract-backed pattern runtime.

The package implements concrete workflow recipes outside the kernel while using
`core/contracts` at its public boundaries. Runtime callers provide tools,
subagents, traces, approvals, and agent execution through injected contracts.

Default behavior is conservative: `single` is the default pattern, peer worker
handoffs are rejected, traces are intended to be redacted by concrete sinks, and
fake-backed tests do not require provider credentials.
