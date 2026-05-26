---
schema_version: help-topic/v1
id: tools.contracts
title: Tools Contracts
summary: Tools expose bounded schemas, metadata, and deterministic test surfaces.
kind: reference
interfaces: [cli, gateway, tui]
audiences: [developer, ai-agent]
tags: [tools, contracts, safety]
related: [architecture.boundaries, gateway.operations]
source_paths: [core/tools, ext/tools/contract, modules/tools/builtin]
invariants: [bounded_tool_output, path_guards_remain_strong]
last_verified: 2026-05-26
---
<rules>
- Tool contracts define inputs, outputs, metadata, and safety boundaries.
</rules>
<summary>
Nanogo tools are discoverable through source contracts and can be listed through gateway tool catalog operations.
</summary>
<procedure>
Add tests for tool schema, output bounds, path guards, and error handling before implementation.
</procedure>
<examples>
`tool_help` explains a progressive tool without revealing hidden implementation details.
</examples>
<failure_modes>
Unbounded output and hidden filesystem access are contract violations.
</failure_modes>
<verification>
Run `go test ./core/tools ./ext/tools/... ./modules/tools/builtin/...`.
</verification>
