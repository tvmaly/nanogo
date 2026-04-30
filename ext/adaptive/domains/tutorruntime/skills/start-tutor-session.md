---
name: start-tutor-session
description: Start an adaptive tutoring session for a child and lesson
kind: skill
model: anthropic/claude-sonnet-4
triggers:
  cli: true
  rest: true
args:
  - child_id
  - lesson_id
---
Start a tutoring session for {{child_id}} using lesson {{lesson_id}}. Select a tutor policy through the tutorruntime adaptive domain and record outcomes after each meaningful child response.
