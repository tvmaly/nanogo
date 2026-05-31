package obs

import (
	"context"
	"fmt"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

type ObserverConfig struct {
	Source                string
	FailurePolicy         FailurePolicy
	Disabled              bool
	FlushOnError          bool
	FlushOnRunFinish      bool
	Now                   func() time.Time
	NewID                 func(string) string
	IncludeTokenText      bool
	IncludeToolResultText bool
}

type EventObserver struct {
	w   Writer
	cfg ObserverConfig
}

func NewEventObserver(w Writer, cfg ObserverConfig) *EventObserver {
	if cfg.Source == "" {
		cfg.Source = "core/event"
	}
	if cfg.FailurePolicy == "" {
		cfg.FailurePolicy = FailureBestEffort
	}
	return &EventObserver{w: w, cfg: cfg}
}

func (o *EventObserver) Observe(ctx context.Context, ev event.Event) error {
	if o.cfg.Disabled {
		return nil
	}
	rec := o.record(ev)
	err := o.w.Append(ctx, rec)
	if err == nil && o.shouldFlush(rec.Type) {
		if f, ok := o.w.(Flusher); ok {
			err = f.Flush()
		}
	}
	if err != nil && o.cfg.FailurePolicy == FailureFailFast {
		return err
	}
	return nil
}

func (o *EventObserver) record(ev event.Event) ObservationRecord {
	at := ev.At
	if at.IsZero() {
		at = o.now()
	}
	rec := ObservationRecord{
		SchemaVersion: SchemaVersion,
		ID:            o.id(string(ev.Kind)),
		Type:          observationType(ev.Kind),
		Time:          at.UTC(),
		Source:        o.cfg.Source,
		Session:       ev.Session,
		Turn:          ev.Turn,
		Attributes:    map[string]any{"event_kind": string(ev.Kind)},
	}
	o.addPayload(&rec, ev)
	return rec
}

func (o *EventObserver) addPayload(rec *ObservationRecord, ev event.Event) {
	switch p := ev.Payload.(type) {
	case nil:
	case string:
		switch ev.Kind {
		case event.Error:
			rec.Severity = "error"
			rec.Message = p
			rec.Error = &ErrorInfo{Message: p}
		case event.TokenDelta:
			if o.cfg.IncludeTokenText {
				rec.Attributes["text"] = p
			}
			rec.Attributes["bytes"] = len(p)
		default:
			rec.Message = p
			rec.Attributes["payload"] = p
		}
	case event.TurnCompletedPayload:
		rec.Message = p.Text
		rec.Attributes["model"] = p.Model
		rec.Attributes["source"] = p.Source
		rec.Attributes["skill"] = p.Skill
		rec.Attributes["subagent_of"] = p.SubagentOf
		rec.Attributes["input_tokens"] = p.InputTokens
		rec.Attributes["output_tokens"] = p.OutputTokens
		rec.Attributes["cached_input_tokens"] = p.CachedInputTokens
		if len(p.ServerToolUse) > 0 {
			rec.Attributes["server_tool_use"] = p.ServerToolUse
		}
	case event.SignalPayload:
		rec.Severity = p.Severity
		rec.Message = p.Message
		rec.Attributes["sensor"] = p.SensorName
		rec.Attributes["fix"] = p.Fix
		rec.Attributes["binding"] = p.Binding
		rec.Attributes["tool"] = p.ToolName
		if p.Fix != "" {
			rec.RepairHints = []RepairHint{{Kind: "sensor_fix", Message: p.Fix}}
		}
	case map[string]string:
		for k, v := range p {
			if ev.Kind == event.ToolCallResult && k == "result" && !o.cfg.IncludeToolResultText {
				rec.Attributes["result_bytes"] = len(v)
				continue
			}
			rec.Attributes[k] = v
		}
	default:
		rec.Attributes["payload"] = fmt.Sprintf("%v", p)
	}
}

func (o *EventObserver) shouldFlush(typ string) bool {
	return (typ == "run.failure" && o.cfg.FlushOnError) || (typ == "run.finish" && o.cfg.FlushOnRunFinish)
}

func (o *EventObserver) now() time.Time {
	if o.cfg.Now != nil {
		return o.cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func (o *EventObserver) id(seed string) string {
	if o.cfg.NewID != nil {
		return o.cfg.NewID(seed)
	}
	return fmt.Sprintf("obs-%d", o.now().UnixNano())
}

func observationType(kind event.Kind) string {
	switch kind {
	case event.TurnStarted:
		return "run.start"
	case event.TurnCompleted:
		return "run.finish"
	case event.Error:
		return "run.failure"
	case event.TokenDelta:
		return "llm.token"
	case event.ToolCallStarted:
		return "tool.start"
	case event.ToolCallResult:
		return "tool.result"
	case event.SkillTriggered:
		return "skill.triggered"
	case event.SensorSignal:
		return "validation.signal"
	case event.AskUser:
		return "ask_user.request"
	case event.MemoryUpdated:
		return "memory.updated"
	case event.HeartbeatFired:
		return "heartbeat.fired"
	case event.EvolveProposed:
		return "evolve.proposed"
	case event.EvolveApplied:
		return "evolve.applied"
	case event.EvolveReverted:
		return "evolve.reverted"
	default:
		return "event." + string(kind)
	}
}
