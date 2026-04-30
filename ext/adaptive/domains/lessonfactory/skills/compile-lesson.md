---
name: compile-lesson
description: Compile a rough parent lesson idea into an adaptive lesson bundle
kind: skill
model: anthropic/claude-haiku-4-5
triggers:
  cli: true
  rest: true
args:
  - source_path
---

Read the lesson idea at {{source_path}} and use the lessonfactory adaptive domain to compile a complete lesson bundle. Do not assign it until parent approval is recorded.
