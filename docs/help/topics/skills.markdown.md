---
schema_version: help-topic/v1
id: skills.markdown
title: Markdown Skills
summary: Skills are editable markdown assets with structured frontmatter.
kind: guide
interfaces: [cli, gateway, tui]
audiences: [developer, operator, ai-agent]
tags: [skills, workspace]
related: [gateway.operations, verification.local]
source_paths: [modules/skills, workspace/skills]
invariants: [workspace_first_behavior]
last_verified: 2026-05-26
---
<rules>
- Prefer workspace assets when behavior can live as editable data.
</rules>
<summary>
Skills provide reusable prompts and agent patterns without expanding the kernel.
</summary>
<procedure>
Place skill markdown in the configured skills directory and use gateway skill operations or CLI skill commands to run it.
</procedure>
<examples>
`nanogo skill run demo --arg topic=fractions`
</examples>
<failure_modes>
Invalid frontmatter or missing arguments cause dispatch errors.
</failure_modes>
<verification>
Run `go test ./modules/skills/... ./cmd/nanogo`.
</verification>
