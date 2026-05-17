# nanogo — Your Child's Personal AI Tutor, at Home, for Free

> A free, open-source alternative to AlphaSchool that runs on your own computer and remembers your child across every session.

---

## Who is this for?

**Parents who want more for their kids** — without a $200/month subscription.

If you've heard of AI tutoring platforms like AlphaSchool and thought "my child could really benefit from that," nanogo is built for you. It gives your child a patient, knowledgeable tutor available any time of day or night — one that never gets tired, never judges, and picks up exactly where it left off last time.

You don't need to be a programmer to use it. If you can open a terminal and paste a command, you're ready.

---

## Why should you care?

Most AI tutoring tools are either:
- **Expensive** — subscription fees that add up fast
- **Generic** — one-size-fits-all lessons with no memory of your child's progress
- **Cloud-dependent** — your child's learning history lives on someone else's server

nanogo is different:

- **Free to run** — you only pay for the AI calls you make (a few cents per session using cheap models)
- **Remembers your child** — it builds a persistent memory of what your child knows, struggles with, and enjoys
- **Runs on your computer** — your family's data stays with you
- **Fully customizable** — create lessons, quizzes, and study plans by writing simple text files
- **Voice mode** — Phase 15 adds a backend xAI Grok Voice session layer for talking to the tutor before the full web UI voice experience is built

---

## What it looks like in practice

Ask it to tutor your 10-year-old in fractions:

```
nanogo -p "Explain fractions to a 10-year-old. Start with what half a pizza means, then ask me a question."
```

Come back tomorrow and it already knows where you left off:

```
nanogo -p "I'm back. What should we work on today?"
```

It answers: *"Welcome back! Last time we covered equivalent fractions and you were doing great. Want to try a short quiz on adding fractions with different denominators?"*

Run a structured lesson defined in a file — for example, a daily math warm-up:

```
nanogo skill run daily-math --grade=4 --student=Emma
```

---

## Installation

### Step 1 — Install Go

nanogo is written in Go. Download and install it from **https://go.dev/dl** (free). Choose the installer for your operating system (Mac, Windows, or Linux) and follow the on-screen instructions. You only need to do this once.

Verify it worked by opening a terminal and typing:

```
go version
```

You should see something like `go version go1.22.0`.

### Step 2 — Download nanogo

```
git clone https://github.com/tvmaly/nanogo.git
cd nanogo
go build -o nanogo ./cmd/nanogo
```

This creates a `nanogo` program in the current folder.

### Step 3 — Get an API key

nanogo uses AI models through [OpenRouter](https://openrouter.ai) — a free-to-join service that gives you access to powerful AI at very low cost. Sign up at **https://openrouter.ai**, then create an API key in your account dashboard.

Set it in your terminal (replace `your-key-here` with your actual key):

**Mac / Linux:**
```
export OPENROUTER_API_KEY=your-key-here
```

**Windows (Command Prompt):**
```
set OPENROUTER_API_KEY=your-key-here
```

Phase 15 includes an optional realtime voice extension. The first provider target is xAI Grok Voice. You need an xAI account with API access and purchased credits before live voice calls will work.

**Mac / Linux:**

```
export XAI_API_KEY=your-xai-key-here
export XAI_REALTIME_MODEL=grok-voice-think-fast-1.0
```

**Windows (Command Prompt):**
```
set XAI_API_KEY=your-xai-key-here
set XAI_REALTIME_MODEL=grok-voice-think-fast-1.0
```

Deepgram Voice Agent is planned behind the same adapter pattern for later testing:

**Mac / Linux:**
```
export DEEPGRAM_API_KEY=your-deepgram-key-here
```

**Windows (Command Prompt):**
```
set DEEPGRAM_API_KEY=your-deepgram-key-here
```

To avoid setting this every time, add it to your shell profile (`.zshrc` or `.bashrc` on Mac/Linux, or System Environment Variables on Windows).

### Step 4 — Try it

```
./nanogo -p "Hello! I am a parent setting this up for my 8-year-old. Can you introduce yourself and ask what subject they want to study today?"
```

---

## Creating your first lesson

Lessons in nanogo are plain text files called "skills." Create a folder called `skills/` and add a file called `math-basics.md`:

```markdown
---
name: math-basics
description: Daily math warm-up for kids
args:
  - grade
  - student
---
You are a friendly math tutor. Your student is {{student}}, who is in grade {{grade}}.
Start with a warm greeting, then give them three math problems appropriate for their grade level.
After they answer each one, give encouraging feedback and move to the next.
At the end, summarize how they did and suggest what to practice next.
```

Run it with:

```
./nanogo --skills=./skills skill run math-basics --grade=3 --student=Emma
```

nanogo will ask for any missing information interactively if you forget to supply it.

You can make a skill for any subject: reading comprehension, spelling practice, science questions, history flashcards, foreign language vocabulary — anything a human tutor could teach.

---

## How the memory works

After each session, nanogo automatically:

1. Saves the conversation to a local history file on your computer
2. Runs a background pass that reads the history and updates a `MEMORY.md` file
3. Loads that memory at the start of every future session

This means the tutor genuinely remembers across days and weeks:

- Which topics your child has covered
- Where they struggled and where they excelled
- Their name, grade level, and learning style
- What you worked on in the last session

Everything is stored as plain text files in `~/.nanogo/workspace/` on your own machine — you can read and edit them any time.

---

## Workspace contracts and local verification

Phase 17 moved nanogo toward a smaller execution kernel and more behavior in modules and editable workspace assets. The preferred place to change tutor behavior is now plain files before Go code:

```text
workspace/tools/<name>/tool.yaml
workspace/tools/<name>/prompt.md
workspace/tools/<name>/tests.yaml
workspace/skills/*.md
workspace/tutorials/*.md
workspace/policies/*.md
```

Workspace tool definitions are validated by `ext/workspace`: missing commands, unknown manifest fields, path traversal, duplicate names, and missing tests fail with field-specific errors.

For local verification, run:

```bash
make verify-local
```

That target builds the binary, runs normal and race-detector tests, runs `go vet`, checks the Phase 17 core boundary, enforces the `core/` import invariant, checks required fakes, and verifies the core LOC budget.

Current architecture split:

| Directory | Role | Examples |
|---|---|---|
| `core/` | Tiny execution kernel and stable contracts used directly by the agent loop. | `agent`, `event`, `harness`, `llm`, `session`, `tools` |
| `modules/` | Stable first-party runtime subsystems: shared contracts, registries, and default product behavior. | `modules/obs`, `modules/transport`, `modules/memory`, `modules/tools/builtin` |
| `ext/` | Optional concrete adapters, providers, domains, and experiments that plug into core or module contracts. | `ext/obs/slog`, `ext/transport/rest`, `ext/scheduler/cron`, `ext/adaptive`, `ext/voice` |
| `workspace/` | Editable data-first behavior and user/product assets. | `workspace/tools/...`, `workspace/skills/*.md`, `workspace/policies/*.md` |

A useful rule of thumb: `modules/` is the first-party subsystem layer, while `ext/` is the plug-in and adaptation layer. For example, `modules/obs` defines the shared observability API and fan-out behavior, while `ext/obs/slog`, `ext/obs/file`, and `ext/obs/cost` are concrete observers. Similarly, `modules/transport` defines the transport contract and registry, while `ext/transport/cli`, `ext/transport/rest`, and `ext/transport/webui` are concrete transports.

Not every `modules/` package is interfaces-only. `modules/memory`, `modules/skills`, and `modules/tools/builtin` contain default first-party behavior. They stay outside `core/` because they are product/runtime behavior rather than kernel primitives.

---

## Features — what this means for your family

| Feature | What it means for you |
|---|---|
| **Persistent memory across sessions** | The tutor remembers your child's progress every time — no need to re-explain the basics each session |
| **Custom lesson files** | Write a simple text file to define any lesson, quiz, or study plan — no coding required |
| **Interactive Q&A** | If the tutor needs more information (grade, topic, name), it asks your child directly |
| **Your data stays home** | All history and memory files live on your own computer — nothing is sent to a third-party server beyond the AI call itself |
| **Any subject** | Math, reading, science, history, coding, languages — if a human tutor could teach it, nanogo can too |
| **Always available** | 3am panic before a test? nanogo is there, patient as ever |
| **Very low cost** | A typical 20-minute tutoring session costs less than one cent using the default model |
| **Multiple tutor personalities** | Define a strict grammar checker, an encouraging math coach, and a Socratic science guide — each as a separate skill file |
| **Open source and free forever** | No subscription, no lock-in. Audit the code, modify it, share it with other parents |

---

## How nanogo evolves to fit your child

Phases 12–14 introduce a self-improving education loop that no static tutoring app can match. Here is what that means in plain language:

### For parents

**Turn a rough idea into a ready-to-use lesson.** Write a few sentences about what you want your child to learn — "I want Emma to understand fractions using cooking recipes" — and the system builds a complete, leveled lesson bundle: introduction, worked examples, practice questions, a rubric, and a parent summary. You review and approve before anything reaches your child.

**Lessons improve over time based on your child's actual results.** Every session generates a signal — did your child master the concept, get frustrated, breeze through, or stall on a specific step? The system archives those outcomes and, over multiple sessions, shifts toward lesson variants and teaching approaches that work best for that specific child. You can see exactly which variants are running and why.

**No two children get the same tutor.** A child who learns visually by analogy gets different explanations than one who prefers step-by-step worked examples. The system tracks what works per child and applies it automatically.

**You stay in control.** Every generated lesson requires parent approval before it is used. You can inspect, edit, or reject any variant. The archive is plain text files on your machine — readable and auditable at any time.

### For teachers and homeschool educators

**Rapidly prototype differentiated instruction.** Write a single rough prompt per topic and let the factory generate multiple pathways: a visual/analogical path, a step-by-step procedural path, a challenge path for advanced students, and a remediation path for students who struggle. Each pathway is a plain markdown file you can edit directly.

**Data-driven lesson refinement without a data science background.** Mastery scores, retention signals, and engagement data accumulate automatically. The system surfaces which pathways produce the best outcomes per student profile so you can spend your time teaching, not analyzing spreadsheets.

**Reusable skill libraries.** Every lesson produced by the factory becomes a reusable skill file. Build a library of proven, child-tested lessons over a semester and share them with other families or colleagues — they are plain text and require no special tools to read or run.

---

## Cost estimate

nanogo uses `anthropic/claude-haiku-4-5` by default via OpenRouter:

| Session | Approximate cost |
|---|---|
| 10-minute tutoring session | ~$0.005 (half a cent) |
| 30-minute deep dive | ~$0.015 |
| A full week of daily sessions | ~$0.10 |

For comparison: AlphaSchool costs roughly $2500-$5000/month. nanogo's AI costs for equivalent usage run under $2/month.

---

## Questions and feedback

Open an issue at **https://github.com/tvmaly/nanogo/issues**. Parent feedback directly shapes the roadmap.

---

## Implementation Status

This table shows each build phase, what AI tutor capability it unlocks, and whether it is complete.

| Phase | Description | AI Tutor Capability | Status |
|-------|-------------|---------------------|--------|
| 1 | Event bus + LLM interface + Router + OpenAI ext + CLI transport | Basic single-question tutoring — child asks, tutor answers | ✅ Complete |
| 2 | Tool interface + 5 builtins + agent loop + session + subagent concurrency | Tutor can read/write files, run code, and delegate to specialist sub-tutors (math agent, grammar agent) | ✅ Complete |
| 3 | Skills frontmatter + dispatcher + `ask_user` integration | Named lesson plans ("do my math homework", "quiz me on fractions") — tutor asks for missing details interactively | ✅ Complete |
| 4 | Memory (consolidator + dream + curator) | Tutor remembers your child across sessions — past mistakes, strengths, learning style, goals | ✅ Complete |
| 5 | REST + REPL transports | Multi-interface access — tutor available via browser/API (REST) and interactive terminal (REPL) simultaneously | ✅ Complete |
| 6 | Harness interfaces + sensors + binding-signal support | Tutor self-corrects when it makes a mistake — test failures inject feedback that forces revision | ✅ Complete |
| 7 | Scheduler + heartbeats (4 action kinds) + CLI management | Scheduled tutoring — daily vocabulary quiz at 8am, weekly progress review on Fridays | ✅ Complete |
| — | **Post-phase-7 integration fixes:** CLI transport `init()` registration, router factory in `ext/llm/router/`, signal injection wired, `SubagentRunner` isolated sessions, session-backed `ask_user`, tools allowlist, config loading from `~/.nanogo/config.json` | All runtime-wiring gaps from REVIEW.md closed; Phase 8 prerequisites met | ✅ Complete |
| 8 | Obs interfaces + slog + file + cost adapter | Full observability and per-session cost tracking — know exactly what you spent and on what | ✅ Complete |
| 9 | Evolve extension (full, test-gated) | Self-improving tutor — agent proposes improvements to its own lesson files, tests them, deploys on green | ✅ Complete |
| 10 | Telegram + cron + otel + progressive tools + MCP + mutants + classifier-router | Full ecosystem — tutor on Telegram, mutation-tested lesson scripts, multi-model routing by difficulty | ✅ Complete |
| 10.5 | Contract-driven progressive tool design | Nanogo can expose small, safe tool surfaces by default and author new tools from clear operation contracts, manifests, compact outputs, and tests | ✅ Complete |
| 11 | Web tutor UI extension: student lessons + parent admin + reporting | Family-friendly browser experience — student lessons, parent dashboards, lesson editing, and homeschool reporting | ✅ Complete |
| 12 | Adaptive experiment engine: artifacts, outcomes, archive, islands, scoring | System learns which lesson variants actually work — tracks mastery gain, engagement, and retention per child | ✅ Complete |
| 13 | Adaptive lesson factory: rough parent markdown → polished child-specific lesson bundles | Parents write a rough idea; the system generates complete, leveled lessons tailored to their child's style | ✅ Complete |
| 14 | Adaptive tutor runtime: live policy selection, mastery scoring, remediation, evolve loop | Tutor adapts its teaching style in real time — hints, pacing, difficulty, and encouragement evolve per child | ✅ Complete |
| 15 | Realtime voice extension: xAI Grok Voice backend sessions, voice smoke CLI, optional local audio evaluation | Talk to the tutor through a backend voice session now, with browser microphone/speaker handling still cleanly separable for later phases | ✅ Complete |
| 15.5 | Live hands-free voice loop: MacBook microphone capture, xAI realtime streaming, speaker playback | Talk to the tutor hands-free from the command line with raw audio kept private unless debug files are requested | ✅ Complete |

---

## Quick Start Guide

Use this section if you want the shortest path from clone to a working local build and the current acceptance test suite.

### 1. Install prerequisites

You need:
- Go 1.22 or newer
- Git
- An OpenRouter API key for live LLM tests or manual runs

Verify the basics:

```bash
go version
git --version
```

### 2. Clone the repository

```bash
git clone https://github.com/tvmaly/nanogo.git
cd nanogo
```

### 3. Build the binary

```bash
go build -o nanogo ./cmd/nanogo
```

### 4. Set your API key

Mac / Linux:

```bash
export OPENROUTER_API_KEY=your-key-here
```

Windows Command Prompt:

```bat
set OPENROUTER_API_KEY=your-key-here
```

### 5. Run a first prompt

```bash
./nanogo -p "Reply with exactly: OK"
```

If your setup is working, the response should begin with `OK`.

### 6. Run the automated acceptance gates

```bash
go test -race ./...
scripts/check_imports.sh
scripts/loc_budget.sh
scripts/check_fakes.sh
```

If you also want the coverage gate used from Phase 4 onward:

```bash
go test -coverprofile=cover.out ./core/agent/... ./modules/memory/... ./modules/tools/builtin/...
go tool cover -func=cover.out | tail -1
```

## Tool Contracts And Progressive Disclosure

Phase 10.5 adds a contract-first tool layer for extension authors and future self-evolution work. New tools can be written as operations with one clear contract: name, description, input schema, output schema, bounded output rules, safety metadata, data-access mode, examples, and tests. Nanogo adapts that contract into `core/tools.Tool` values, generated help, manifests, and optional future CLI/MCP surfaces without duplicating invocation logic.

Progressive disclosure keeps the default tool surface small. A manifest can make only `tool_list`, `tool_help`, `tool_reveal`, and a few safe tools visible at first, while hidden tools remain real callable tools once revealed for the current session. The agent loop refreshes tool schemas after reveal, so a tool can be revealed and used in the same turn.

Configured tool sources use the `tools.sources` config block. With no config, nanogo keeps the existing builtin-only behavior.

```json
{
  "tools": {
    "sources": [
      {"driver": "builtin"},
      {
        "driver": "progressive",
        "config": {
          "manifest": "./tools/progressive.json",
          "sources": [{"driver": "adaptive", "config": {"root": "~/.nanogo/workspace"}}]
        }
      }
    ]
  }
}
```

---

## Running Manual Tests

The manual tests exercise the full stack end-to-end against a real LLM via OpenRouter. They cover one test per completed phase in the order phases were delivered.

### With `make` (recommended)

```bash
# Build the binary and run all manual tests in phase order:
make test

# Or run a single test by phase:
make test-1.9    # TEST-1.9  — real LLM round trip
make test-2.12   # TEST-2.12 — agent creates and reads a file
make test-4.9    # TEST-4.9  — memory persists across two sessions
make test-8.5    # TEST-8.5  — event kinds visible in log
make test-8.10   # TEST-8.10 — cost tracker records real turns
make test-9.8    # TEST-9.8  — evolve building blocks (sandbox, path guard, learnings)
make test-9.9    # TEST-9.9  — self-edit attack rejected by path guard
make test-12.20  # TEST-12.20 — adaptive experiment demo and inspect report
make test-13.24  # TEST-13.24 — lesson factory compile/review/approve/assign
```

`make test` will fail immediately if `OPENROUTER_API_KEY` is not set.

### Without `make`

1. **Build the binary:**
   ```bash
   go build -o /tmp/nanogo ./cmd/nanogo
   ```

2. **Set your API key:**
   ```bash
   export OPENROUTER_API_KEY=sk-or-v1-...
   ```

3. **Write a config file** (`/tmp/nanogo-test-config.json`):
   ```json
   {
     "llm": {
       "driver": "openai",
       "config": {
         "base_url": "https://openrouter.ai/api/v1",
         "api_key_env": "OPENROUTER_API_KEY",
         "model": "anthropic/claude-haiku-4-5"
       }
     },
     "transports": [{"driver": "cli"}]
   }
   ```

4. **Run each test in order:**

   **TEST-1.9 — Real LLM round trip**
   ```bash
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "Reply with exactly: OK"
   # Pass: output contains OK
   ```

   **TEST-2.12 — Agent performs file edit**
   ```bash
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "Create a file /tmp/nanogo-demo.txt containing exactly the word 'hello', then read it back and tell me its contents."
   cat /tmp/nanogo-demo.txt
   # Pass: file exists and contains "hello"
   ```

   **TEST-4.9 — Memory persists across sessions**
   ```bash
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "Remember that my favorite programming language is Go."

   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "What is my favorite programming language?"
   # Pass: second response mentions Go
   ```

   **TEST-8.5 — All event kinds visible**
   ```bash
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "Create a file /tmp/nanogo-event-test.txt with content 'y'"
   jq -r '.kind' /tmp/nanogo-workspace/log.jsonl | sort -u
   # Pass: includes turn.started, turn.token, tool.started, tool.result, turn.completed
   ```

   **TEST-8.10 — Cost tracker records real turns**
   ```bash
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     --skills testdata/skills \
     -p "Reply with OK"
   /tmp/nanogo --config /tmp/nanogo-test-config.json \
     --workspace /tmp/nanogo-workspace \
     cost
   # Pass: cost.jsonl exists or cost summary prints
   ```

   **TEST-9.8 — Evolve building blocks**
   ```bash
   go test -v -run "TestSandbox|TestPathGuard|TestLearnings|TestSynthesis" ./ext/evolve/...
   # Pass: all four tests green
   ```

   **TEST-9.9 — Self-edit attack rejected**
   ```bash
   go test -v -run "TestPathGuard|TestPathGuardLearningsEntry" ./ext/evolve/...
   # Pass: IsBlocked returns true for core/ and ext/evolve/ paths; rejection logged
   ```

   **TEST-12.20 — Adaptive experiment smoke**
   ```bash
   /tmp/nanogo --workspace /tmp/nanogo-workspace adaptive demo --child cross --subject science --topic magnets
   /tmp/nanogo --workspace /tmp/nanogo-workspace adaptive inspect --child cross --subject science --topic magnets
   # Pass: demo selects a winner, writes child patterns, and inspect prints top artifacts
   ```

   **TEST-13.24 — Lesson factory workflow**
   ```bash
   mkdir -p /tmp/nanogo-workspace/inbox/lessons
   cp ext/adaptive/domains/lessonfactory/testdata/magnets.md /tmp/nanogo-workspace/inbox/lessons/magnets.md
   /tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory compile --source /tmp/nanogo-workspace/inbox/lessons/magnets.md
   /tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory review --lesson latest
   /tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory approve --lesson latest
   /tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory assign --lesson latest --child cross
   # Pass: bundle, review, parent guide, child pathways, and assignment queue are written
   ```

## Adaptive Experiment Engine

Phase 12 adds an extension-only adaptive experiment engine under `ext/adaptive/`. It stores artifacts and outcomes under `memory/adaptive/`, scores outcomes across mastery, retention, transfer, engagement, quality, parent rating, frustration, time, and cost, and writes parent-readable reports.

Manual smoke:

```bash
./nanogo --workspace /tmp/nanogo-workspace adaptive demo --child cross --subject science --topic magnets
./nanogo --workspace /tmp/nanogo-workspace adaptive inspect --child cross --subject science --topic magnets
```

The demo creates fake adaptive artifacts, records outcomes, selects a winner, writes `memory/adaptive/child_patterns/<child-id>.md`, and creates an inspect report under `memory/adaptive/reports/experiments/`.

## Adaptive Lesson Factory

Phase 13 adds `ext/adaptive/domains/lessonfactory/`, an extension-only adaptive domain that compiles rough parent-authored markdown into deterministic lesson bundles under `lessons/generated/<lesson-id>/`.

Generated bundles include `lesson.yaml`, `parent_guide.md`, `child_summary.md`, per-child default/hands-on/remediation pathways, age/depth levels, activities, quick checks, rubrics, transfer questions, retention review, `sources.md`, and `review.md`. Assignment is blocked until parent approval is recorded.

CLI workflow:

```bash
/tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory compile --source /tmp/nanogo-workspace/inbox/lessons/magnets.md
/tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory review --lesson latest
/tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory approve --lesson latest
/tmp/nanogo --workspace /tmp/nanogo-workspace lessonfactory assign --lesson latest --child cross
```

The domain registers adaptive artifact kinds for lesson bundles, pathways, rubrics, and templates. It also exposes a `lessonfactory` tool source with parse, compile, review, package, assign, parent-review, child-outcome, and template-mutation operations.

## Adaptive Tutor Runtime

Phase 14 adds `ext/adaptive/domains/tutorruntime/`, an extension-only adaptive domain for live tutoring sessions. It selects tutor policies from parent pins and archive outcomes, records turn evidence, grades answers deterministically, updates mastery, schedules retention reviews, recommends remediation, tracks misconceptions through profile approval, mutates policy versions, and writes parent-readable session summaries.

Initial tutor policies live under `ext/adaptive/domains/tutorruntime/policies/`: `socratic-guide`, `worked-example-first`, `hands-on-remediation`, `visual-analogy`, `story-explanation`, `retrieval-practice`, `challenge-mode`, and `gentle-coach`.

Runtime files live under:

```text
memory/adaptive/tutorruntime/sessions.jsonl
memory/adaptive/tutorruntime/turns.jsonl
memory/adaptive/tutorruntime/pending_reviews.jsonl
memory/adaptive/tutorruntime/strategy_switches.jsonl
memory/adaptive/tutorruntime/policy_activations.jsonl
memory/adaptive/artifacts/tutor_policies/
memory/adaptive/reports/tutorruntime/
```

Manual smoke:

```bash
make test-14.27
```

The `tutorruntime` tool source exposes policy selection, turn recording, answer grading, mastery update, misconception detection, remediation recommendation, review scheduling, and session summary tools.

## Realtime Voice Extension

Phase 15 adds `ext/voice/`, an extension-only backend voice session layer that can later be consumed by the web UI. The internal contract is modeled around OpenAI Realtime-style events so provider adapters can be swapped without changing the session manager.

The first concrete provider target is xAI Grok Voice using:

```text
wss://api.x.ai/v1/realtime?model=grok-voice-think-fast-1.0
```

Configuration uses `XAI_API_KEY` and `XAI_REALTIME_MODEL`. The xAI account must have voice/API credits enabled; buy credits from xAI before running the smoke or live voice commands. Deepgram Voice Agent is named as a planned second adapter using `DEEPGRAM_API_KEY`, with automated Deepgram tests deferred until requested.

The backend API supports voice session start, audio append/commit/clear, text-only turns, response creation, event streaming, transcript/raw event persistence under `memory/voice/`, and clean session close. The provider WebSocket layer uses `github.com/coder/websocket`.

Build and run a text-only xAI smoke test:

```bash
go build -o /tmp/nanogo ./cmd/nanogo
export XAI_API_KEY=your-xai-key-here
export XAI_REALTIME_MODEL=grok-voice-think-fast-1.0
/tmp/nanogo --workspace /tmp/nanogo-workspace voice smoke \
  --provider xai \
  --child cross \
  --text "Say hello in one short sentence."
```

Run the Makefile xAI smoke tests:

```bash
make test-15.17   # text-only realtime voice
make test-15.18   # raw PCM file input/output
```

Evaluate local microphone/speaker plumbing with `github.com/gen2brain/malgo`:

```bash
go build -tags malgo -o /tmp/nanogo ./cmd/nanogo
/tmp/nanogo --workspace /tmp/nanogo-workspace voice smoke \
  --provider xai \
  --child cross \
  --mic \
  --speaker

make test-15.19
```

`malgo` is optional and isolated behind the `malgo` build tag because it uses cgo/miniaudio and may require local audio permissions. The backend voice session API does not depend on `malgo`, so browser-based audio can replace local device handling in a later phase.

### Phase 15.5 — live hands-free voice

Phase 15.5 adds a live local mic/speaker loop for command-line use:

```bash
go build -tags malgo -o /tmp/nanogo ./cmd/nanogo
export XAI_API_KEY=your-xai-key-here
export XAI_REALTIME_MODEL=grok-voice-think-fast-1.0
/tmp/nanogo --workspace /tmp/nanogo-workspace voice live \
  --provider xai \
  --child cross
```

You can also use the Makefile target:

```bash
make voice-live
```

The live loop streams MacBook microphone audio to xAI, relies on xAI `server_vad` to detect spoken turns, and plays response audio through local speakers. macOS may prompt for microphone permission. Headphones are recommended to reduce speaker-to-microphone feedback.

By default, live sessions persist normalized events and transcripts under `memory/voice/`, but not raw microphone or speaker PCM. Raw PCM is saved only when explicitly requested:

```bash
/tmp/nanogo --workspace /tmp/nanogo-workspace voice live \
  --provider xai \
  --child cross \
  --save-capture-pcm /tmp/nanogo-workspace/voice/capture.pcm \
  --save-playback-pcm /tmp/nanogo-workspace/voice/playback.pcm
```

```bash
make voice-live-debug
```
