---
schema_version: help-topic/v1
id: troubleshooting.provider_credentials
title: Provider Credentials
summary: Provider-backed smokes require explicit credentials.
kind: troubleshooting
interfaces: [cli, tui]
audiences: [operator, developer]
tags: [openrouter, xai, credentials]
related: [voice.privacy, verification.local]
source_paths: [Makefile, cmd/nanogo/config.go]
invariants: [manual_provider_smokes_are_explicit]
last_verified: 2026-05-26
---
<rules>
- Local unit tests should not require live provider credentials.
</rules>
<summary>
OpenRouter smoke tests require `OPENROUTER_API_KEY`; xAI realtime voice requires `XAI_API_KEY`.
</summary>
<procedure>
Export the required environment variable before running the corresponding manual smoke target.
</procedure>
<examples>
`OPENROUTER_API_KEY=sk-or-v1-... make test-19.8`
</examples>
<failure_modes>
Missing keys fail fast through Makefile environment checks or provider configuration errors.
</failure_modes>
<verification>
Run `make check-env` for OpenRouter or `make check-xai-env` for xAI.
</verification>
