---
schema_version: help-topic/v1
id: architecture.boundaries
title: Architecture Boundaries
summary: Core stays small while modules and extensions own product behavior.
kind: architecture
interfaces: [cli]
audiences: [developer, ai-agent]
tags: [architecture, core, modules, ext]
related: [gateway.operations, tools.contracts, adaptive.evidence]
source_paths: [AGENTS.md, scripts/check_core_boundary.sh, scripts/check_imports.sh]
invariants: [core_must_not_import_ext, modules_help_must_not_import_ext]
last_verified: 2026-05-26
---
<rules>
- Put durable kernel contracts in `core/`.
- Put product subsystems in `modules/` and adapters in `ext/`.
</rules>
<summary>
Nanogo follows a microkernel-plus-adapters shape. Help is product behavior, so Phase 19.9 lives outside `core/`.
</summary>
<procedure>
Before adding code, identify the boundary and run import guard scripts after implementation.
</procedure>
<examples>
`modules/help` owns help contracts. `ext/help/files` owns filesystem loading.
</examples>
<failure_modes>
Boundary drift appears as failed import checks or product logic leaking into command handlers.
</failure_modes>
<verification>
Run `scripts/check_core_boundary.sh` and `scripts/check_imports.sh`.
</verification>
