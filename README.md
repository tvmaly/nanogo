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
| 7 | Scheduler + heartbeats (4 action kinds) + CLI management | Scheduled tutoring — daily vocabulary quiz at 8am, weekly progress review on Fridays | 🔲 In Progress |
| 8 | Obs interfaces + slog + file + cost adapter | Full observability and per-session cost tracking — know exactly what you spent and on what | 🔲 In Progress |
| 9 | Evolve extension (full, test-gated) | Self-improving tutor — agent proposes improvements to its own lesson files, tests them, deploys on green | 🔲 In Progress |
| 10 | Telegram + cron + otel + progressive tools + MCP + mutants + classifier-router | Full ecosystem — tutor on Telegram, mutation-tested lesson scripts, multi-model routing by difficulty | 🔲 In Progress |
| 11 | Web tutor UI extension: student lessons + parent admin + reporting | Family-friendly browser experience — student lessons, parent dashboards, lesson editing, and homeschool reporting | 🔲 Planned |

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
go test -coverprofile=cover.out ./core/agent/... ./core/memory/... ./core/tools/...
go tool cover -func=cover.out | tail -1
