---
schema_version: help-topic/v1
id: sessions.persistence
title: Sessions Persistence
summary: Session state is accessed through stable session contracts.
kind: reference
interfaces: [gateway, tui]
audiences: [developer, operator]
tags: [sessions, persistence]
related: [gateway.operations, verification.local]
source_paths: [core/session, modules/gateway]
invariants: [session_contract_stable]
last_verified: 2026-05-26
---
<rules>
- Use session contracts rather than transport-specific storage.
</rules>
<summary>
Gateway interfaces can create, list, inspect, and delete sessions without knowing concrete storage details.
</summary>
<procedure>
Use `sessions.create`, `sessions.list`, `sessions.get`, `sessions.messages`, and `sessions.delete` gateway methods.
</procedure>
<examples>
`{"method":"sessions.list"}`
</examples>
<failure_modes>
If the store is not configured, session operations return unsupported or load errors.
</failure_modes>
<verification>
Run `go test ./core/session ./modules/gateway`.
</verification>
