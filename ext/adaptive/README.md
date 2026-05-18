# Adaptive Extension

`ext/adaptive` owns adaptive education artifacts, outcomes, archives, reports,
and tutor/lesson domains. It remains an extension: it may depend on core
contracts, but core and modules must not depend on it.

Phase 17.5 adds schema-version metadata to adaptive artifact and outcome records
when they are written to the JSONL archive. This gives Phase 18 AgentFlow rollout
and reward plumbing a safer evidence surface: records can be inspected, migrated,
and compared without guessing which durable shape produced them.

Keep adaptive responsibilities separate:

- archives record evidence;
- domains compile, evaluate, and mutate educational artifacts;
- reports summarize evidence for parents;
- future AgentFlow integration should append outcomes, not auto-promote tutor
  policy changes.

Relevant checks:

```sh
go test ./ext/adaptive/...
scripts/check_imports.sh
```
