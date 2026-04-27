package agent

import (
	"strings"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/core/llm"
)

type SignalContext struct {
	Binding  []harness.Signal
	Advisory []harness.Signal
}

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

func RenderAdvisoryBlock(signals []harness.Signal, sensorNames map[string]struct{}) string {
	if len(signals) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, sig := range signals {
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

func InjectSignalsIntoMessages(msgs []llm.Message, ctx SignalContext) []llm.Message {
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
