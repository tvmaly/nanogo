package mutants_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/ext/harness/mutants"
)

// wellCoveredCode has a test that kills all mutants.
const wellCoveredCode = `package pkg

func Add(a, b int) int { return a + b }
`

const wellCoveredTest = `package pkg

import "testing"

func TestAdd(t *testing.T) {
	if Add(1,2) != 3 { t.Fatal("wrong") }
	if Add(0,0) != 0 { t.Fatal("wrong") }
}
`

// weakCode has a function with no tests — mutants survive.
const weakCode = `package pkg

func Mul(a, b int) int { return a * b }
`

const weakTest = `package pkg
` // no tests at all

func TestMutantsSurvivingProducesSignal(t *testing.T) {
	t.Parallel()
	dir := setupPkg(t, weakCode, weakTest)
	sensor := mutants.NewSensor(dir)
	tr := harness.ToolResult{Tool: "write_file", Output: "ok"}
	signals, err := sensor.Analyze(context.Background(), tr)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(signals) == 0 {
		t.Error("expected signals for surviving mutants when there are no tests")
	}
}

func TestMutantsFullCoverageNoSignal(t *testing.T) {
	t.Parallel()
	dir := setupPkg(t, wellCoveredCode, wellCoveredTest)
	sensor := mutants.NewSensor(dir)
	tr := harness.ToolResult{Tool: "write_file", Output: "ok"}
	signals, err := sensor.Analyze(context.Background(), tr)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("expected no signals for well-covered code, got %d: %v", len(signals), signals)
	}
}

func setupPkg(t *testing.T, code, tests string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg_test.go"), []byte(tests), 0o644); err != nil {
		t.Fatal(err)
	}
	// Minimal go.mod so the package is buildable.
	gomod := "module pkg\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
