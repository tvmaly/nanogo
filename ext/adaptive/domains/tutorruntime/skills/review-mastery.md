---
name: review-mastery
description: Review mastery evidence for a child and topic
kind: skill
model: anthropic/claude-sonnet-4
args:
  - child_id
  - topic
---
Review correctness, hints, transfer, and retention evidence for {{child_id}} on {{topic}}.
