package evolve

import (
	"fmt"
	"os/exec"
	"strings"
)

// SmokeTest invokes a binary with args and checks the output starts with "OK".
type SmokeTest struct {
	binary string
	args   []string
}

// NewSmokeTest creates a SmokeTest.
func NewSmokeTest(binary string, args []string) *SmokeTest {
	return &SmokeTest{binary: binary, args: args}
}

// Run executes the binary and verifies the response starts with "OK".
func (s *SmokeTest) Run() error {
	cmd := exec.Command(s.binary, s.args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("smoke binary error: %w\n%s", err, out)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "OK") {
		return fmt.Errorf("smoke response does not start with OK: %q", trimmed)
	}
	return nil
}
