---
schema_version: help-topic/v1
id: troubleshooting.command_boundaries
title: Command Boundaries
summary: CLI commands should compose services and avoid owning business logic.
kind: troubleshooting
interfaces: [cli]
audiences: [developer, ai-agent]
tags: [cli, boundaries]
related: [architecture.boundaries, gateway.operations]
source_paths: [cmd/nanogo, AGENTS.md]
invariants: [cli_stays_thin]
last_verified: 2026-05-26
---
<rules>
- CLI handlers parse flags, wire dependencies, call services, and print results.
</rules>
<summary>
When a CLI command grows business logic, move the behavior into a module or extension and keep command code as composition.
</summary>
<procedure>
Define the module contract first, add tests, then wire the command to the service.
</procedure>
<examples>
`nanogo help` uses `modules/help` plus `ext/help/files`.
</examples>
<failure_modes>
Duplicated parsing or ranking logic in transports makes behavior inconsistent.
</failure_modes>
<verification>
Run command package tests and import guard scripts.
</verification>
