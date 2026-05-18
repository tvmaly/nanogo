package main

import (
	"context"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/modules/heartbeat"
	"github.com/tvmaly/nanogo/modules/memory"
	"github.com/tvmaly/nanogo/modules/scheduler"
)

func startHeartbeats(ctx context.Context, cfg *config, provider llm.Provider, store session.Store, bus event.Bus, memStore *memory.Store) (func(), error) {
	if len(cfg.Heartbeats) == 0 {
		return func() {}, nil
	}
	driver := cfg.Scheduler.Driver
	if driver == "" {
		driver = "stdlib"
	}
	sched, err := scheduler.Build(driver, cfg.Scheduler.Config)
	if err != nil {
		return func() {}, err
	}
	sub := heartbeatSubmitter{cfg: cfg, provider: provider, store: store, bus: bus, memStore: memStore, model: cfg.modelForSource("heartbeat")}
	src, err := buildRuntimeToolSource(cfg, provider, store, bus, nil)
	if err != nil {
		return func() {}, err
	}
	rt := heartbeat.NewRuntime(sched, nil, src, sub, bus)
	for _, hb := range cfg.Heartbeats {
		_ = rt.Register(ctx, hb)
	}
	hbCtx, cancel := context.WithCancel(ctx)
	_ = sched.Start(hbCtx)
	return cancel, nil
}

type heartbeatSubmitter struct {
	cfg      *config
	provider llm.Provider
	store    session.Store
	bus      event.Bus
	memStore *memory.Store
	model    string
}

func (h heartbeatSubmitter) Submit(ctx context.Context, sessionID, message string) error {
	return runSingleShot(ctx, h.cfg, h.provider, h.store, h.bus, h.memStore, message, h.model, "heartbeat")
}
