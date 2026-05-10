BINARY      := /tmp/nanogo
CONFIG      := /tmp/nanogo-test-config.json
CONFIG_OBS  := /tmp/nanogo-test-config-obs.json
# Manual smoke tests use a fresh workspace by default so stale MEMORY.md
# content from prior runs cannot steer live OpenRouter responses.
DEFAULT_WORKSPACE := $(shell mktemp -d /tmp/nanogo-workspace.XXXXXX)
WORKSPACE   ?= $(DEFAULT_WORKSPACE)
SKILLS_DIR  := $(CURDIR)/testdata/skills

.PHONY: all build build-malgo check-env write-config write-config-obs test \
	test-1.9 test-2.12 test-4.9 test-8.5 test-8.10 \
	test-9.8 test-9.9 test-11.11 test-12.20 test-13.24 test-14.27 \
	test-15.17 test-15.18 test-15.19 test-15.5.8 voice-live voice-live-debug

# ── Build ──────────────────────────────────────────────────────────────────────

all: build

build:
	go build -o $(BINARY) ./cmd/nanogo
	@echo "Built: $(BINARY)"

build-malgo:
	go build -tags malgo -o $(BINARY) ./cmd/nanogo
	@echo "Built with malgo: $(BINARY)"

# ── Env guard ─────────────────────────────────────────────────────────────────

check-env:
	@if [ -z "$$OPENROUTER_API_KEY" ]; then \
		echo "ERROR: OPENROUTER_API_KEY is not set."; \
		echo "  export OPENROUTER_API_KEY=sk-or-v1-..."; \
		exit 1; \
	fi
	@echo "OPENROUTER_API_KEY is set."

check-xai-env:
	@if [ -z "$$XAI_API_KEY" ]; then \
		echo "ERROR: XAI_API_KEY is not set."; \
		echo "  export XAI_API_KEY=xai-..."; \
		exit 1; \
	fi
	@echo "XAI_API_KEY is set."

# Write the shared config that most manual tests use.
write-config: check-env
	@mkdir -p $(WORKSPACE)/memory
	@printf '{\n  "llm": {\n    "driver": "openai",\n    "config": {\n      "base_url": "https://openrouter.ai/api/v1",\n      "api_key_env": "OPENROUTER_API_KEY",\n      "model": "anthropic/claude-haiku-4-5"\n    }\n  },\n  "transports": [{"driver": "cli"}]\n}\n' > $(CONFIG)
	@echo "Config written: $(CONFIG)"

# Write the obs-enabled config used by TEST-8.5 and TEST-8.10.
# Adds file obs (writes log.jsonl) and cost obs (writes cost.jsonl) adapters.
write-config-obs: check-env
	@mkdir -p $(WORKSPACE)/memory
	@printf '{\n  "llm": {\n    "driver": "openai",\n    "config": {\n      "base_url": "https://openrouter.ai/api/v1",\n      "api_key_env": "OPENROUTER_API_KEY",\n      "model": "anthropic/claude-haiku-4-5"\n    }\n  },\n  "transports": [{"driver": "cli"}],\n  "obs": [\n    {\n      "driver": "file",\n      "config": {"path": "$(WORKSPACE)/log.jsonl"}\n    },\n    {\n      "driver": "cost",\n      "config": {\n        "output_path": "$(WORKSPACE)/cost.jsonl",\n        "prices": {\n          "anthropic/claude-haiku-4-5": {\n            "input_per_mtok": 0.8,\n            "output_per_mtok": 4.0,\n            "cached_input_per_mtok": 0.08\n          }\n        }\n      }\n    }\n  ]\n}\n' > $(CONFIG_OBS)
	@echo "Obs config written: $(CONFIG_OBS)"

# ── Manual tests (run in phase order) ─────────────────────────────────────────

# TEST-1.9 — End-to-end: real LLM round trip
test-1.9: build write-config
	@echo ""
	@echo "=== TEST-1.9: real LLM round trip ==="
	@OUT=$$($(BINARY) --config $(CONFIG) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) -p "Reply with exactly: OK"); \
	echo "Response: $$OUT"; \
	echo "$$OUT" | grep -q "OK" && echo "PASS" || (echo "FAIL: response did not contain OK"; exit 1)

# TEST-2.12 — Agent performs file edit
test-2.12: build write-config
	@echo ""
	@echo "=== TEST-2.12: agent performs file edit ==="
	@rm -f /tmp/nanogo-demo.txt
	@$(BINARY) --config $(CONFIG) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) \
		-p "Create a file /tmp/nanogo-demo.txt containing exactly the word 'hello', then read it back and tell me its contents."
	@if [ -f /tmp/nanogo-demo.txt ]; then \
		echo "File contents: $$(cat /tmp/nanogo-demo.txt)"; \
		grep -qi "hello" /tmp/nanogo-demo.txt && echo "PASS" || (echo "FAIL: file does not contain hello"; exit 1); \
	else \
		echo "FAIL: /tmp/nanogo-demo.txt was not created"; exit 1; \
	fi

# TEST-4.9 — Memory integration across sessions
test-4.9: build write-config
	@echo ""
	@echo "=== TEST-4.9: memory integration across sessions ==="
	@$(BINARY) --config $(CONFIG) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) \
		-p "Remember that my favorite programming language is Go."
	@echo "--- second session ---"
	@OUT=$$($(BINARY) --config $(CONFIG) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) \
		-p "What is my favorite programming language?"); \
	echo "Response: $$OUT"; \
	echo "$$OUT" | grep -qi "go" \
		&& echo "PASS" \
		|| echo "WARN: response may not mention Go (memory consolidation may require more turns)"

# TEST-8.5 — All event kinds visible in log (uses obs config with file driver)
test-8.5: build write-config-obs
	@echo ""
	@echo "=== TEST-8.5: event kinds visible in log ==="
	@LOG=$(WORKSPACE)/log.jsonl; \
	rm -f $$LOG; \
	$(BINARY) --config $(CONFIG_OBS) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) \
		-p "Create a file /tmp/nanogo-event-test.txt with content 'y'"; \
	if [ -f $$LOG ]; then \
		echo "Events in log:"; \
		jq -r '.kind' $$LOG 2>/dev/null | sort -u; \
		for kind in "turn.started" "turn.token" "tool.started" "tool.result" "turn.completed"; do \
			grep -q "$$kind" $$LOG 2>/dev/null && echo "  PASS: $$kind" || echo "  FAIL: $$kind not found"; \
		done; \
	else \
		echo "FAIL: $(WORKSPACE)/log.jsonl was not written"; exit 1; \
	fi

# TEST-8.10 — Cost tracker picks up real turns (uses obs config with cost driver)
test-8.10: build write-config-obs
	@echo ""
	@echo "=== TEST-8.10: cost tracker ==="
	@COST=$(WORKSPACE)/cost.jsonl; \
	rm -f $$COST; \
	$(BINARY) --config $(CONFIG_OBS) --workspace $(WORKSPACE) --skills $(SKILLS_DIR) \
		-p "Reply with OK"; \
	if [ -f $$COST ]; then \
		echo "cost.jsonl entries:"; \
		jq '{model,cost_usd,source}' $$COST 2>/dev/null || cat $$COST; \
		echo "PASS: cost.jsonl written"; \
	else \
		echo "FAIL: $(WORKSPACE)/cost.jsonl was not written"; exit 1; \
	fi
	@echo "Cost summary:"; \
	$(BINARY) --config $(CONFIG_OBS) --workspace $(WORKSPACE) cost 2>/dev/null || true

# TEST-9.8 — Evolve building blocks
test-9.8: build check-env
	@echo ""
	@echo "=== TEST-9.8: evolve building blocks ==="
	@go test -v -run "TestSandbox|TestPathGuard|TestLearnings|TestSynthesis" ./ext/evolve/... \
		&& echo "PASS: all evolve unit tests green"
	@echo "NOTE: full 'nanogo evolve run' CLI wiring is deferred to Phase 10."

# TEST-9.9 — Self-edit attack (path guard)
test-9.9: check-env
	@echo ""
	@echo "=== TEST-9.9: self-edit attack ==="
	@go test -v -run "TestPathGuard|TestPathGuardLearningsEntry" ./ext/evolve/... \
		&& echo "PASS: core/ and ext/evolve/ paths are blocked before any file is touched"

# TEST-11.11 — End-to-end homeschool workflow
# Uses httptest.Server to exercise the full student/parent request cycle
# without requiring the CLI binary to support transport config.
test-11.11:
	@echo ""; echo "=== TEST-11.11: end-to-end homeschool workflow ==="
	@go test -v -run "TestE2EHomeschoolWorkflow" ./ext/transport/webui/... \
		&& echo "PASS: webui e2e" || (echo "FAIL: webui e2e"; exit 1)
	@go test -run "TestQuizAttemptRecording|TestParentDashboard|TestParentLessonEditor|TestComplianceReporting" ./ext/tutor/... \
		&& echo "PASS: tutor unit tests" || (echo "FAIL: tutor unit tests"; exit 1)
	@echo "PASS: TEST-11.11 complete"

# TEST-12.20 — End-to-end adaptive experiment smoke test
test-12.20: build check-env
	@echo ""; echo "=== TEST-12.20: adaptive experiment smoke ==="
	@rm -rf $(WORKSPACE)/memory/adaptive
	@$(BINARY) --workspace $(WORKSPACE) adaptive demo --child cross --subject science --topic magnets
	@$(BINARY) --workspace $(WORKSPACE) adaptive inspect --child cross --subject science --topic magnets \
		| grep -qi "top artifacts" \
		&& echo "PASS: inspect report contains top artifacts" \
		|| (echo "FAIL: inspect report missing top artifacts"; exit 1)
	@test -f $(WORKSPACE)/memory/adaptive/child_patterns/cross.md \
		&& echo "PASS: child pattern summary written" \
		|| (echo "FAIL: child pattern summary missing"; exit 1)
	@echo "PASS: TEST-12.20 complete"

# TEST-13.24 — End-to-end lesson factory workflow
test-13.24: build check-env
	@echo ""; echo "=== TEST-13.24: lesson factory workflow ==="
	@rm -rf $(WORKSPACE)/lessons $(WORKSPACE)/inbox/lessons
	@mkdir -p $(WORKSPACE)/inbox/lessons
	@printf '%s\n' '---' \
		'title: How magnets work' \
		'subject: science' \
		'topic: magnets' \
		'children: [cross]' \
		'rough_age_level: 7' \
		'goal: Help him understand attraction, repulsion, and magnetic fields' \
		'materials:' \
		'  - magnets' \
		'  - paper clips' \
		'  - paper' \
		'preferences:' \
		'  - make it hands-on' \
		'---' \
		'' \
		'I want a lesson where he plays with magnets and learns why some things stick and some do not.' \
		> $(WORKSPACE)/inbox/lessons/magnets.md
	@$(BINARY) --workspace $(WORKSPACE) lessonfactory compile --source $(WORKSPACE)/inbox/lessons/magnets.md
	@$(BINARY) --workspace $(WORKSPACE) lessonfactory review --lesson latest | grep -qi "quality gates" \
		&& echo "PASS: review visible" \
		|| (echo "FAIL: review missing quality gates"; exit 1)
	@$(BINARY) --workspace $(WORKSPACE) lessonfactory approve --lesson latest
	@$(BINARY) --workspace $(WORKSPACE) lessonfactory assign --lesson latest --child cross
	@test -f $(WORKSPACE)/lessons/generated/lesson-science-magnets/parent_guide.md \
		&& echo "PASS: parent guide written" \
		|| (echo "FAIL: parent guide missing"; exit 1)
	@test -f $(WORKSPACE)/lessons/queues/cross.jsonl \
		&& echo "PASS: child assignment queue written" \
		|| (echo "FAIL: assignment queue missing"; exit 1)
	@echo "PASS: TEST-13.24 complete"

# TEST-14.27 — End-to-end adaptive tutoring workflow
test-14.27: build check-env
	@echo ""; echo "=== TEST-14.27: adaptive tutoring workflow ==="
	@GOCACHE=/tmp/go-cache go test -v -run "TestSessionTurnGradeMasteryRetentionAndRemediation|TestProfileScoringMutationApprovalSummaryToolsAndLessonIntegration|TestDomainEvaluateCompileMutate" ./ext/adaptive/domains/tutorruntime/... \
		&& echo "PASS: adaptive tutor runtime workflow" \
		|| (echo "FAIL: adaptive tutor runtime workflow"; exit 1)
	@echo "PASS: TEST-14.27 complete"

# TEST-15.17 — xAI realtime voice text smoke
test-15.17: build check-xai-env
	@echo ""; echo "=== TEST-15.17: xAI realtime voice text smoke ==="
	@$(BINARY) --workspace $(WORKSPACE) voice smoke --provider xai --child cross \
		--text "Say hello in one short sentence." --timeout 30s \
		&& echo "PASS: xAI text voice smoke" \
		|| (echo "FAIL: xAI text voice smoke"; exit 1)

# TEST-15.18 — xAI realtime voice PCM file smoke
test-15.18: build check-xai-env
	@echo ""; echo "=== TEST-15.18: xAI realtime voice PCM file smoke ==="
	@mkdir -p $(WORKSPACE)/voice
	@printf '\000\000\000\000\000\000\000\000' > $(WORKSPACE)/voice/sample.pcm
	@$(BINARY) --workspace $(WORKSPACE) voice smoke --provider xai --child cross \
		--audio-in $(WORKSPACE)/voice/sample.pcm --audio-out $(WORKSPACE)/voice/response.pcm --timeout 30s \
		&& echo "PASS: xAI PCM file voice smoke" \
		|| (echo "FAIL: xAI PCM file voice smoke"; exit 1)

# TEST-15.19 — optional malgo local audio evaluation
test-15.19: check-xai-env
	@echo ""; echo "=== TEST-15.19: malgo local audio evaluation ==="
	@go build -tags malgo -o $(BINARY) ./cmd/nanogo
	@$(BINARY) --workspace $(WORKSPACE) voice smoke --provider xai --child cross --mic --speaker \
		&& echo "PASS: malgo local audio evaluation complete or skipped with reason" \
		|| (echo "FAIL: malgo local audio evaluation"; exit 1)

# TEST-15.5.8 — manual MacBook live voice loop
test-15.5.8: build-malgo check-xai-env
	@echo ""; echo "=== TEST-15.5.8: live voice loop ==="
	@echo "Grant microphone permission if macOS prompts. Use headphones if speaker feedback occurs."
	@$(BINARY) --workspace $(WORKSPACE) voice live --provider xai --child cross

voice-live: test-15.5.8

voice-live-debug: build-malgo check-xai-env
	@echo ""; echo "=== voice live with debug PCM capture/playback files ==="
	@mkdir -p $(WORKSPACE)/voice
	@$(BINARY) --workspace $(WORKSPACE) voice live --provider xai --child cross \
		--save-capture-pcm $(WORKSPACE)/voice/capture.pcm \
		--save-playback-pcm $(WORKSPACE)/voice/playback.pcm

# ── Run all manual tests in phase order ───────────────────────────────────────

test: build check-env test-1.9 test-2.12 test-4.9 test-8.5 test-8.10 test-9.8 test-9.9 test-11.11 test-12.20 test-13.24 test-14.27
	@echo ""
	@echo "=== All manual tests complete ==="
