---
name: schedule-retention-review
description: Schedule retention review items after tutoring
kind: skill
model: anthropic/claude-sonnet-4
args:
  - child_id
  - lesson_id
---
Schedule spaced review items for {{child_id}} after lesson {{lesson_id}}.
