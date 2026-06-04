# Nanogo Phase Development Workflow

Use these templates when requesting implementation from a phase plan and an acceptance-test file.

## Standard Phase Request

```text
Please implement phase development for nanogo.

phase_plan: <PLAN_FILE>
acceptance_tests: <ACCEPTANCE_FILE>

Follow the red-green-refactor TDD process in AGENTS.md. First review the existing code and tests, then add the smallest failing test before implementing.

Before finishing, run:

- GOCACHE=/tmp/go-cache go test <TARGETS>
- make verify-local

Update README.md and TODO.md if behavior, usage, or phase status changes.
```

## Investigation Scope

```text
Limit pre-implementation investigation to:

- Directories: <DIRS>
- Files: <FILES>

Out of scope:

- <OUT_OF_SCOPE_AREAS>

Use core/ only for contract checks and do not modify it unless the plan explicitly requires core changes.

First use rg to inspect related symbols. Before editing tracked files, briefly report which files you expect to change and why.
```

## External Tool Permission Context

```text
You may use the external tool <TOOL> for this work.

- Executable path: <ABSOLUTE_PATH>
- Read targets: <READ_PATHS>
- Write destinations: <WRITE_PATHS>
- Allowed permission scope: <SCOPE>

If it is unavailable or blocked, keep PATH exploration minimal. Use <FALLBACK_APPROACH> instead and include the reason the tool was not run in the final report.
```

## Acceptance Checklist

```text
Treat these acceptance criteria as the implementation checklist:

- <ACCEPTANCE_ITEM_1>
- <ACCEPTANCE_ITEM_2>
- <ACCEPTANCE_ITEM_3>

For each item, state the matching test name, acceptance scenario, or verification command as work progresses. If anything remains incomplete, explain why and list the next concrete task.
```

## Completion Report

The final report should include:

- What changed.
- Tests added or updated.
- Verification commands run.
- Tests not run and why.
- Follow-up risks or TODOs.
