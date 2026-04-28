// Package mutants implements a sensor that detects surviving mutants by
// applying simple arithmetic operator mutations and running go test.
package mutants

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tvmaly/nanogo/core/harness"
)

// Sensor runs mutation testing on Go source files in a directory.
type Sensor struct {
	dir string
}

// NewSensor creates a Sensor targeting the given directory.
func NewSensor(dir string) *Sensor { return &Sensor{dir: dir} }

// Analyze applies mutations and runs tests; returns signals for each survivor.
func (s *Sensor) Analyze(_ context.Context, _ harness.ToolResult) ([]harness.Signal, error) {
	mutations, err := generateMutations(s.dir)
	if err != nil {
		return nil, err
	}
	var signals []harness.Signal
	for _, m := range mutations {
		survived, err := testMutation(s.dir, m)
		if err != nil {
			continue
		}
		if survived {
			signals = append(signals, harness.Signal{
				Severity: "warn",
				Message:  fmt.Sprintf("surviving mutant in %s:%d — %s", m.file, m.line, m.desc),
			})
		}
	}
	return signals, nil
}

type mutation struct {
	file    string
	line    int
	desc    string
	src     []byte // mutated file content
	origSrc []byte // original content
}

// generateMutations finds all binary expressions in .go files and flips
// arithmetic operators (+ ↔ -, * ↔ /).
func generateMutations(dir string) ([]mutation, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []mutation
	for _, path := range entries {
		if isTestFile(path) {
			continue
		}
		orig, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, orig, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			flip, ok := flipOp(be.Op)
			if !ok {
				return true
			}
			pos := fset.Position(be.OpPos)
			origOp := be.Op
			be.Op = flip
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, f); err == nil {
				out = append(out, mutation{
					file:    filepath.Base(path),
					line:    pos.Line,
					desc:    fmt.Sprintf("%s → %s", origOp, flip),
					src:     buf.Bytes(),
					origSrc: orig,
				})
			}
			be.Op = origOp // restore
			return true
		})
	}
	return out, nil
}

func flipOp(op token.Token) (token.Token, bool) {
	switch op {
	case token.ADD:
		return token.SUB, true
	case token.SUB:
		return token.ADD, true
	case token.MUL:
		return token.QUO, true
	case token.QUO:
		return token.MUL, true
	}
	return op, false
}

func isTestFile(path string) bool {
	base := filepath.Base(path)
	return len(base) > 8 && base[len(base)-8:] == "_test.go"
}

// testMutation writes the mutated file, runs go test, restores the original.
// Returns true if the tests still pass (mutant survived).
func testMutation(dir string, m mutation) (bool, error) {
	target := filepath.Join(dir, m.file)
	if err := os.WriteFile(target, m.src, 0o644); err != nil {
		return false, err
	}
	defer os.WriteFile(target, m.origSrc, 0o644)

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil, nil // tests pass → mutant survived
}
