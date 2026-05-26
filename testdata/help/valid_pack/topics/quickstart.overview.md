---
schema_version: help-topic/v1
id: quickstart.overview
title: Quickstart Overview
summary: Start with local help.
kind: guide
interfaces: [cli, tui]
audiences: [operator]
tags: [quickstart]
related:
  - gateway.operations
source_paths:
  - README.md
invariants:
  - local_only
last_verified: 2026-05-26
---
<rules>
Use local deterministic help.
</rules>
<summary>
Start with local help.
</summary>
<procedure>
Run nanogo help.
</procedure>
<examples>
nanogo help
</examples>
<failure_modes>
Missing topics return not_found.
</failure_modes>
<verification>
Run go test.
</verification>
