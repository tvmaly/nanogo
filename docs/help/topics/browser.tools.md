---
schema_version: help-topic/v1
id: browser.tools
title: Browser Tools
summary: Optional local browser-control tools and gateway operations for lesson workflows.
kind: reference
interfaces: [cli, gateway, tools]
audiences: [developer, ai-agent, parent]
tags: [browser, tools, lessons]
related: [gateway.operations, tools.contracts, verification.local]
source_paths: [modules/browser, ext/browser/agentbrowser, cmd/nanogo/browser_cmd.go]
invariants: [browser_disabled_by_default, browser_eval_policy_gated, core_boundary]
last_verified: 2026-06-04
---
<rules>
- Browser support is disabled unless `browser.enabled` is true or a direct `nanogo browser` diagnostic command enables it.
- Agent tools use progressive disclosure: only `browser_session_start` is visible before a browser session exists.
- `browser_eval` is hidden unless `browser.allow_eval` is true.
- `file://` navigation is allowed only under configured lesson roots.
- Screenshots and PDFs return artifact paths, not inline bytes.
</rules>
<summary>
Nanogo can optionally control a local browser for student-visible lesson workflows. The provider-neutral service lives in `modules/browser`; the first concrete adapter shells out to `agent-browser`.
</summary>
<procedure>
Configure browser support with a top-level `browser` section, then use `nanogo browser doctor --driver agent-browser` or expose browser tools through the runtime source. Run `scripts/setup_agent_browser.sh` to install or update the local adapter and managed browser binary.
</procedure>
<examples>
`{"browser":{"enabled":true,"driver":"agent-browser","allow_file_roots":["workspace/lessons"],"max_sessions":2}}`

`nanogo browser open --driver agent-browser --headed https://example.com`
</examples>
<failure_modes>
Policy-denied navigation returns typed browser errors such as `domain_not_allowed` or `file_root_not_allowed`. `browser_eval` is a trusted-wrapper feature for local lessons, not a full sandbox, and Nanogo domain policy does not block JavaScript-initiated page network requests.
</failure_modes>
<verification>
Run `go test ./modules/browser/... ./modules/gateway ./ext/browser/agentbrowser ./cmd/nanogo` and `bash -n scripts/setup_agent_browser.sh`.
</verification>
