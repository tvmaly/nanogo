---
schema_version: help-topic/v1
id: gateway.operations
title: Gateway Operations
summary: Shared interface-facing operations for local and remote transports.
kind: reference
interfaces: [gateway, gatewayws, openaiapi, tui]
audiences: [developer, ai-agent]
tags: [gateway, operations, help]
related: [architecture.boundaries, tui.slash_commands, sessions.persistence]
source_paths: [modules/gateway, ext/transport/gatewayws, ext/transport/openaiapi]
invariants: [core_boundary, transports_do_not_duplicate_product_logic]
last_verified: 2026-05-26
---
<rules>
- Transports dispatch gateway operations instead of parsing domain files directly.
- Unconfigured reserved namespaces return `unsupported`, not `unknown_method`.
</rules>
<summary>
The gateway provides normalized operations for chat, sessions, skills, tools, costs, models, voice state, xAI realtime control, events, and help.
</summary>
<procedure>
Use `Service.Dispatch` with methods such as `help.search`, `help.topic`, `help.suggest`, `help.render`, and `help.validate`.
</procedure>
<examples>
`{"method":"help.search","params":{"query":"tools","limit":5}}`
</examples>
<failure_modes>
Invalid JSON params return `invalid_request`. Missing help composition returns `unsupported`.
</failure_modes>
<verification>
Run `go test ./modules/gateway ./ext/transport/gatewayws ./ext/transport/openaiapi`.
</verification>
