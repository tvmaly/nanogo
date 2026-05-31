package main

import (
	// LLM providers and routers.
	_ "github.com/tvmaly/nanogo/ext/llm/classifier-router"
	_ "github.com/tvmaly/nanogo/ext/llm/openai"
	_ "github.com/tvmaly/nanogo/ext/llm/router"

	// Transport drivers.
	_ "github.com/tvmaly/nanogo/ext/transport/cli"
	_ "github.com/tvmaly/nanogo/ext/transport/repl"
	_ "github.com/tvmaly/nanogo/ext/transport/rest"
	_ "github.com/tvmaly/nanogo/ext/transport/webui"

	// Scheduler drivers.
	_ "github.com/tvmaly/nanogo/ext/scheduler/stdlib"

	// Harness sensors.
	_ "github.com/tvmaly/nanogo/ext/harness/context_guard"
	_ "github.com/tvmaly/nanogo/ext/harness/file_changed"
	_ "github.com/tvmaly/nanogo/ext/harness/gotest"
	_ "github.com/tvmaly/nanogo/ext/harness/vet"

	// Adaptive domains and tool sources.
	_ "github.com/tvmaly/nanogo/ext/adaptive/domains/tutorruntime"
	_ "github.com/tvmaly/nanogo/ext/adaptive/tools"
)
