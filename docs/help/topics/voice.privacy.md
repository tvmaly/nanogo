---
schema_version: help-topic/v1
id: voice.privacy
title: Voice Privacy
summary: Voice support avoids raw PCM persistence by default.
kind: safety
interfaces: [tui, cli]
audiences: [operator, developer]
tags: [voice, privacy, safety]
related: [tui.slash_commands, troubleshooting.provider_credentials]
source_paths: [core/contracts/voice.go, modules/voice, ext/voice]
invariants: [no_raw_pcm_persistence_by_default]
last_verified: 2026-05-26
---
<rules>
- Do not persist raw microphone or speaker PCM unless an explicit debug flag asks for it.
</rules>
<summary>
Voice contracts separate STT, TTS, realtime adapters, and session behavior while preserving provider boundaries.
</summary>
<procedure>
Use `/stt on`, `/tts on`, and `/xai on` only when matching providers and credentials are configured.
</procedure>
<examples>
`/help voice privacy`
`/xai status`
</examples>
<failure_modes>
Missing provider credentials return configuration errors. Helper-backed Apple providers require local helper binaries.
</failure_modes>
<verification>
Run `go test ./modules/voice/... ./ext/voice/... ./cmd/nanogo`.
</verification>
