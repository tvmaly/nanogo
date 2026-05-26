---
schema_version: help-topic/v1
id: tui.slash_commands
title: TUI Slash Commands
summary: Commands available inside the local Bubble Tea operator console.
kind: command
interfaces: [tui, tui.chat]
audiences: [operator, developer, ai-agent]
tags: [tui, commands, gateway, help]
related: [gateway.operations, voice.privacy, troubleshooting.provider_credentials]
source_paths: [ext/transport/tui, modules/gateway]
invariants: [transports_use_gateway, no_help_chat_mutation]
last_verified: 2026-05-26
---
<rules>
- Slash help opens a help pane and does not append chat messages.
- The `?` key remains normal chat punctuation in Phase 19.9.
</rules>
<summary>
The TUI supports chat, session, skill, tool, cost, event, model, voice, xAI, and help controls from one terminal surface.
</summary>
<procedure>
Type `/help` for contextual suggestions, `/help voice privacy` to search, `/help topic voice.privacy` to open a topic, and `/help validate` to check the local help pack.
In the Chat pane, use Up and Down to scroll one line, PageUp and PageDown to scroll by page, and Home or End to jump to the oldest or newest visible transcript.
</procedure>
<examples>
`/help`
`/help topic gateway.operations`
`/model current`
`/model search gpt`
`/cost`
`/exit`
</examples>
<failure_modes>
If help reports unsupported, the gateway was composed without a help service. If a topic does not render, run `nanogo help validate`.
</failure_modes>
<verification>
Run `go test ./ext/transport/tui ./modules/gateway`.
</verification>
