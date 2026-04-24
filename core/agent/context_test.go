package agent_test

import (
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/core/llm"
)

// TEST-6.9: Non-binding signals use advisory path
func TestContextAdvisoryOnly(t *testing.T) {
	t.Parallel()

	// Create advisory signals (Binding: false)
	signals := []harness.Signal{
		{
			Severity: "warn",
			Message:  "consider rechecking",
			Fix:      "recheck the work",
			Binding:  false,
		},
	}

	sigCtx := agent.SignalContext{
		Advisory: signals,
	}

	// Start with a simple message
	msgs := []llm.Message{
		{Role: "user", Content: "test"},
	}

	result := agent.InjectSignalsIntoMessages(msgs, sigCtx)

	// Should have original message plus signal
	if len(result) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(result))
	}

	// Binding block should NOT be present
	for _, msg := range result {
		if strings.Contains(msg.Content, "<binding_state>") {
			t.Error("found <binding_state> in advisory-only context")
		}
	}

	// Advisory signals should be present
	found := false
	for _, msg := range result {
		if strings.Contains(msg.Content, "consider rechecking") {
			found = true
		}
	}
	if !found {
		t.Error("advisory signal not found in messages")
	}
}

// TEST-6.10: Binding signals render in dedicated block
func TestContextBindingError(t *testing.T) {
	t.Parallel()

	// Create binding error signal
	signals := []harness.Signal{
		{
			Severity: "error",
			Message:  "mastery_probability=0.41 below threshold 0.90",
			Fix:      "Do not mark concept as complete. Route to review queue.",
			Binding:  true,
		},
	}

	sigCtx := agent.SignalContext{
		Binding: signals,
	}

	msgs := []llm.Message{
		{Role: "user", Content: "test"},
	}

	result := agent.InjectSignalsIntoMessages(msgs, sigCtx)

	if len(result) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(result))
	}

	// Binding block should be present
	found := false
	var bindingContent string
	for _, msg := range result {
		if strings.Contains(msg.Content, "<binding_state>") {
			found = true
			bindingContent = msg.Content
			break
		}
	}
	if !found {
		t.Error("<binding_state> block not found")
	}

	// Check exact format
	expected := "<binding_state>\n[error] mastery_probability=0.41 below threshold 0.90\n  fix: Do not mark concept as complete. Route to review queue.\n</binding_state>"
	if bindingContent != expected {
		t.Errorf("binding block mismatch:\nGot:\n%q\n\nWant:\n%q", bindingContent, expected)
	}
}

// TEST-6.11: Binding block precedes advisory block
func TestContextBindingPrecedesAdvisory(t *testing.T) {
	t.Parallel()

	sigCtx := agent.SignalContext{
		Binding: []harness.Signal{
			{Severity: "warn", Message: "A", Binding: true},
		},
		Advisory: []harness.Signal{
			{Severity: "warn", Message: "B", Binding: false},
		},
	}

	msgs := []llm.Message{
		{Role: "user", Content: "test"},
	}

	result := agent.InjectSignalsIntoMessages(msgs, sigCtx)

	// Find indices of binding and advisory blocks
	var bindingIdx, advisoryIdx int
	fullContent := ""
	for _, msg := range result {
		fullContent += msg.Content
	}

	bindingIdx = strings.Index(fullContent, "<binding_state>")
	advisoryIdx = strings.Index(fullContent, "[sensor]") // advisory signals start with [sensor]

	if bindingIdx == -1 {
		t.Error("<binding_state> not found")
	}
	if advisoryIdx == -1 {
		// Advisory might not have [sensor] tag, just check for message
		advisoryIdx = strings.Index(fullContent, "B")
	}

	if bindingIdx != -1 && advisoryIdx != -1 && bindingIdx > advisoryIdx {
		t.Errorf("binding block at %d comes after advisory at %d", bindingIdx, advisoryIdx)
	}
}

// TEST-6.12: Multiple binding signals in sensor order
func TestContextBindingSensorOrder(t *testing.T) {
	t.Parallel()

	sigCtx := agent.SignalContext{
		Binding: []harness.Signal{
			{Severity: "warn", Message: "grader-msg", Binding: true},
			{Severity: "warn", Message: "prereq-msg", Binding: true},
		},
	}

	msgs := []llm.Message{
		{Role: "user", Content: "test"},
	}

	result := agent.InjectSignalsIntoMessages(msgs, sigCtx)

	// Find the binding block
	var bindingContent string
	for _, msg := range result {
		if strings.Contains(msg.Content, "<binding_state>") {
			bindingContent = msg.Content
			break
		}
	}

	if bindingContent == "" {
		t.Fatal("no binding block found")
	}

	// Check there's exactly one binding block
	blockCount := strings.Count(bindingContent, "<binding_state>")
	if blockCount != 1 {
		t.Errorf("expected 1 <binding_state> block, got %d", blockCount)
	}

	// Check order of messages inside block
	graderIdx := strings.Index(bindingContent, "grader-msg")
	prereqIdx := strings.Index(bindingContent, "prereq-msg")

	if graderIdx == -1 || prereqIdx == -1 {
		t.Fatal("messages not found in binding block")
	}

	if graderIdx > prereqIdx {
		t.Errorf("grader-msg at %d comes after prereq-msg at %d", graderIdx, prereqIdx)
	}
}

// TEST-6.17: Message integrity
func TestBindingMessageIntegrity(t *testing.T) {
	t.Parallel()

	// Message with special characters
	longMsg := "a" // placeholder
	for i := 0; i < 500; i++ {
		longMsg += "x"
	}

	sigCtx := agent.SignalContext{
		Binding: []harness.Signal{
			{
				Severity: "error",
				Message:  "Line 1\nLine 2 with <angle> & \"quote\"",
				Fix:      longMsg,
				Binding:  true,
			},
		},
	}

	msgs := []llm.Message{
		{Role: "user", Content: "test"},
	}

	result := agent.InjectSignalsIntoMessages(msgs, sigCtx)

	// Find binding block
	var bindingContent string
	for _, msg := range result {
		if strings.Contains(msg.Content, "<binding_state>") {
			bindingContent = msg.Content
			break
		}
	}

	// Check exact content preservation
	if !strings.Contains(bindingContent, "Line 1\nLine 2 with <angle> & \"quote\"") {
		t.Error("message not preserved verbatim")
	}

	if !strings.Contains(bindingContent, longMsg) {
		t.Error("fix message not preserved verbatim")
	}
}
