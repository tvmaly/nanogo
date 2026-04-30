---
name: continue-tutor-session
description: Continue an adaptive tutoring session
kind: skill
model: anthropic/claude-sonnet-4
args:
  - session_id
---
Continue the tutoring session one step at a time. Use the active policy unless remediation or strategy switching is triggered.
