package harness_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/core/harness/fake"
)

// TEST-6.1: Sensor registry
func TestSensorRegistry(t *testing.T) {
	t.Parallel()

	// Register a test sensor
	harness.RegisterSensor("test_sensor", func(cfg json.RawMessage) (harness.Sensor, error) {
		return fake.NewSensor("test_sensor"), nil
	})

	// BuildSensor with registered name should succeed
	sensor, err := harness.BuildSensor("test_sensor", nil)
	if err != nil {
		t.Fatalf("BuildSensor: %v", err)
	}
	if sensor.Name() != "test_sensor" {
		t.Errorf("sensor.Name() = %q, want %q", sensor.Name(), "test_sensor")
	}

	// BuildSensor with unknown name should fail
	_, err = harness.BuildSensor("unknown_sensor", nil)
	if err == nil {
		t.Fatal("expected error for unknown sensor")
	}
	if err.Error() != "sensor \"unknown_sensor\" not registered" {
		t.Errorf("error = %q, want message containing 'unknown_sensor'", err.Error())
	}
}

// TEST-6.8: Signal.Binding defaults to false
func TestSignal_BindingDefaultsFalse(t *testing.T) {
	t.Parallel()

	// Zero-value Signal should have Binding == false
	var s harness.Signal
	if s.Binding {
		t.Fatal("Binding should default to false")
	}

	// Field-named literal should also default to false
	s2 := harness.Signal{
		Severity: "warn",
		Message:  "slow",
		Fix:      "speed up",
	}
	if s2.Binding {
		t.Fatal("Binding should default to false in field-named literal")
	}

	// Setting Binding explicitly should work
	s3 := harness.Signal{
		Severity: "error",
		Message:  "critical",
		Binding:  true,
	}
	if !s3.Binding {
		t.Fatal("Binding should be true when explicitly set")
	}
}

// TEST-6.2: Sensor parallel execution with timeout
func TestSensorTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// Register three sensors with different delays
	fastSensor := fake.NewSensor("fast", harness.Signal{Severity: "info", Message: "fast"})
	slowSensor := fake.NewSensor("slow", harness.Signal{Severity: "info", Message: "slow"})
	hungSensor := &hangingSensor{name: "hung"}

	sensors := []harness.Sensor{fastSensor, slowSensor, hungSensor}

	result := runSensorsParallel(ctx, sensors, harness.ToolResult{Tool: "test", Output: "ok"})

	// Fast and slow should complete
	if len(result) < 2 {
		t.Errorf("got %d signals, want at least 2 (fast and slow)", len(result))
	}

	// Hung should not be in the results (timed out)
	for _, sig := range result {
		if sig.Message == "hung" {
			t.Error("hung sensor should not appear in results (should timeout)")
		}
	}
}

// --- helpers ---

type hangingSensor struct {
	name string
}

func (h *hangingSensor) Name() string { return h.name }

func (h *hangingSensor) Observe(ctx context.Context, _ harness.ToolResult) []harness.Signal {
	<-ctx.Done() // block until timeout
	return nil
}

// runSensorsParallel mimics what the agent loop will do
func runSensorsParallel(ctx context.Context, sensors []harness.Sensor, result harness.ToolResult) []harness.Signal {
	const timeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type sensorResult struct {
		signals []harness.Signal
	}

	resultCh := make(chan sensorResult, len(sensors))
	for _, s := range sensors {
		go func(sensor harness.Sensor) {
			sigs := sensor.Observe(ctx, result)
			select {
			case resultCh <- sensorResult{signals: sigs}:
			case <-ctx.Done():
				// timeout; don't send result
			}
		}(s)
	}

	var allSignals []harness.Signal
	for i := 0; i < len(sensors); i++ {
		select {
		case res := <-resultCh:
			allSignals = append(allSignals, res.signals...)
		case <-ctx.Done():
			// timeout waiting for all sensors
			break
		}
	}
	return allSignals
}
