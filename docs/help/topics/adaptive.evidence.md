---
schema_version: help-topic/v1
id: adaptive.evidence
title: Adaptive Evidence
summary: Adaptive behavior is promoted through evidence, gates, and archives.
kind: guide
interfaces: [cli]
audiences: [developer, ai-agent]
tags: [adaptive, evidence, evolution]
related: [architecture.boundaries, verification.local]
source_paths: [ext/adaptive, ext/evolve, AGENTS.md]
invariants: [promotion_requires_evidence]
last_verified: 2026-05-26
---
<rules>
- Mutation should be cheap and promotion should be hard.
</rules>
<summary>
Adaptive modules record candidates, evaluations, scores, gates, promotion decisions, and rollback context.
</summary>
<procedure>
Represent candidate behavior as data, evaluate deterministically, record evidence, and compare against baseline before promotion.
</procedure>
<examples>
`nanogo adaptive demo --child cross --subject science --topic magnets`
</examples>
<failure_modes>
Promotion without a score, gate result, or archive entry is unsafe.
</failure_modes>
<verification>
Run `go test ./ext/adaptive/... ./ext/evolve/...`.
</verification>
