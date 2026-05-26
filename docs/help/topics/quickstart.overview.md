---
schema_version: help-topic/v1
id: quickstart.overview
title: Quickstart Overview
summary: Start nanogo, choose an interface, and verify local behavior.
kind: guide
interfaces: [cli, tui, gateway]
audiences: [operator, developer, ai-agent]
tags: [quickstart, local, help]
related: [tui.slash_commands, gateway.operations, verification.local]
source_paths: [README.md, cmd/nanogo]
invariants: [core_stays_small, help_is_local]
last_verified: 2026-05-26
---
<rules>
- Use the local help system for product and operator guidance.
- Normal help lookup does not call an LLM, retrieval system, or provider.
</rules>
<summary>
Nanogo can run as a CLI, TUI, OpenAI-compatible API, or Gateway WebSocket service. Help topics are checked-in local documents with stable IDs.
</summary>
<procedure>
Run `nanogo help` to list root topics. Run `nanogo help search gateway` to search. Run `nanogo help tui.slash_commands` to view one topic.
</procedure>
<examples>
`/tmp/nanogo help`
`/tmp/nanogo help search voice privacy`
</examples>
<failure_modes>
If a topic is missing, use search to find the stable ID. If validation fails, fix the topic metadata or required sections.
</failure_modes>
<verification>
Run `go test ./modules/help/... ./ext/help/files/... ./cmd/nanogo`.
</verification>
