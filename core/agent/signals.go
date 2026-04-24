package agent

import (
	"strings"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/core/llm"
)

// SignalContext holds signals emitted by sensors for a given turn.
type SignalContext struct {
	Binding   []harness.Signal // binding signals, in sensor-registration order
	Advisory  []harness.Signal // non-binding signals
}

// RenderBindingBlock returns the formatted binding state block, or empty if no binding signals.
func RenderBindingBlock(signals []harness.Signal) string {
	if len(signals) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("<binding_state>\n")
	for _, sig := range signals {
		buf.WriteString("[")
		buf.WriteString(sig.Severity)
		buf.WriteString("] ")
		buf.WriteString(sig.Message)
		buf.WriteString("\n")
		if sig.Fix != "" {
			buf.WriteString("  fix: ")
			buf.WriteString(sig.Fix)
			buf.WriteString("\n")
		}
	}
	buf.WriteString("</binding_state>")
	return buf.String()
}

// RenderAdvisoryBlock returns the formatted advisory signals block, or empty if no advisory signals.
func RenderAdvisoryBlock(signals []harness.Signal, sensorNames map[string]struct{}) string {
	if len(signals) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, sig := range signals {
		// Find sensor name (if available from context)
		sensorName := "sensor"
		buf.WriteString("[")
		buf.WriteString(sensorName)
		buf.WriteString("] ")
		buf.WriteString(sig.Severity)
		buf.WriteString(": ")
		buf.WriteString(sig.Message)
		buf.WriteString("\n")
		if sig.Fix != "" {
			buf.WriteString("  fix: ")
			buf.WriteString(sig.Fix)
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

// InjectSignalsIntoMessages inserts signal context into the message list.
// Binding signals go into a dedicated system message, advisory signals into another.
// Both are inserted before the next LLM turn's user message (at the end of current msgs).
func InjectSignalsIntoMessages(msgs []llm.Message, ctx SignalContext) []llm.Message {
	// Insert binding block (if any) before advisory (if any)
	if len(ctx.Binding) > 0 {
		bindingBlock := RenderBindingBlock(ctx.Binding)
		msgs = append(msgs, llm.Message{
			Role:    "system",
			Content: bindingBlock,
		})
	}

	if len(ctx.Advisory) > 0 {
		advisoryBlock := RenderAdvisoryBlock(ctx.Advisory, nil)
		msgs = append(msgs, llm.Message{
			Role:    "system",
			Content: advisoryBlock,
		})
	}

	return msgs
}
