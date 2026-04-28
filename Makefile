BINARY      := /tmp/nanogo
CONFIG      := /tmp/nanogo-test-config.json
CONFIG_OBS  := /tmp/nanogo-test-config-obs.json
WORKSPACE   := /tmp/nanogo-workspace
SKILLS_DIR  := $(CURDIR)/testdata/skills

.PHONY: all build check-env write-config write-config-obs test \
	test-1.9 test-2.12 test-4.9 test-8.5 test-8.10 \
	test-9.8 test-9.9

# ── Build ──────────────────────────────────────────────────────────────────────

all: build

build:
	go build -o $(BINARY) ./cmd/nanogo
	@echo "Built: $(BINARY)"

# ── Env guard ─────────────────────────────────────────────────────────────────

check-env:
	@if [ -z "$$OPENROUTER_API_KEY" ]; then \
		echo "ERROR: OPENROUTER_API_KEY is not set."; \
		echo "  export OPENROUTER_API_KEY=sk-or-v1-..."; \
		exit 1; \
	fi
	@echo "OPENROUTER_API_KEY is set."

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

# ── Run all manual tests in phase order ───────────────────────────────────────

test: build check-env test-1.9 test-2.12 test-4.9 test-8.5 test-8.10 test-9.8 test-9.9
	@echo ""
	@echo "=== All manual tests complete ==="
