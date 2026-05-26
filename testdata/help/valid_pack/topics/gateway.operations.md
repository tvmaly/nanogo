---
schema_version: help-topic/v1
id: gateway.operations
title: Gateway Operations
summary: Gateway operation help.
kind: reference
interfaces: [gateway]
audiences: [developer]
tags: [gateway]
related: []
source_paths:
  - modules/gateway
invariants:
  - core_boundary
last_verified: 2026-05-26
---
<rules>
Use gateway operations.
</rules>
<summary>
Gateway help operations are local.
</summary>
<procedure>
Dispatch help.search.
</procedure>
<examples>
{"method":"help.search"}
</examples>
<failure_modes>
Unconfigured help returns unsupported.
</failure_modes>
<verification>
Run gateway tests.
</verification>
