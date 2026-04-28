package evolve

import (
	"fmt"
	"os/exec"
	"strings"
)

// Gate runs vet, test, and build commands in order.
type Gate struct {
	vet   []string
	test  []string
	build []string
}

// NewGate creates a Gate with the given command slices (each is argv).
func NewGate(vet, test, build []string) *Gate {
	return &Gate{vet: vet, test: test, build: build}
}

// Run executes each gate command in the given working directory.
// Returns an error describing which command failed.
func (g *Gate) Run(dir string) error {
	for label, argv := range map[string][]string{
		"vet":   g.vet,
		"test":  g.test,
		"build": g.build,
	} {
		_ = label
		cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %w\n%s", strings.Join(argv, " "), err, out)
		}
	}
	return nil
}
