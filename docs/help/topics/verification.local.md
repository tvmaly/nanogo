---
schema_version: help-topic/v1
id: verification.local
title: Local Verification
summary: Use focused tests first, then run the full local verification gate.
kind: procedure
interfaces: [cli]
audiences: [developer, ai-agent]
tags: [tests, verification]
related: [quickstart.overview, architecture.boundaries]
source_paths: [Makefile, scripts/check_imports.sh, scripts/check_core_boundary.sh]
invariants: [tests_before_completion]
last_verified: 2026-05-26
---
<rules>
- Do not claim tests passed unless they actually ran.
</rules>
<summary>
Local verification builds the binary, runs unit and race tests, runs vet, and checks architecture guard scripts.
</summary>
<procedure>
Run focused package tests for the touched surface, then run `make verify-local` before completion.
</procedure>
<examples>
`go test ./modules/help/... ./ext/help/files/...`
`make verify-local`
</examples>
<failure_modes>
Race tests are slower but catch shared-state bugs. Guard scripts catch dependency direction regressions.
</failure_modes>
<verification>
The verification command is itself `make verify-local`.
</verification>
