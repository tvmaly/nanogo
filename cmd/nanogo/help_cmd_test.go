package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestHelpCmdListsSearchesRendersAndValidates(t *testing.T) {
	root := "../../testdata/help/valid_pack"
	out := captureStdout(t, func() {
		if err := runHelpCmd([]string{"--root", root}); err != nil {
			t.Fatalf("help: %v", err)
		}
	})
	if !strings.Contains(out, "quickstart.overview") {
		t.Fatalf("out = %s", out)
	}
	out = captureStdout(t, func() {
		if err := runHelpCmd([]string{"--root", root, "search", "gateway"}); err != nil {
			t.Fatalf("search: %v", err)
		}
	})
	if !strings.Contains(out, "gateway.operations") {
		t.Fatalf("out = %s", out)
	}
	out = captureStdout(t, func() {
		if err := runHelpCmd([]string{"--root", root, "gateway.operations"}); err != nil {
			t.Fatalf("topic: %v", err)
		}
	})
	if !strings.Contains(out, "Gateway Operations") || !strings.Contains(out, "verification") {
		t.Fatalf("out = %s", out)
	}
	out = captureStdout(t, func() {
		if err := runHelpCmd([]string{"--root", root, "validate"}); err != nil {
			t.Fatalf("validate: %v", err)
		}
	})
	if !strings.Contains(out, "ok") {
		t.Fatalf("out = %s", out)
	}
}

func TestHelpCmdValidateReportsInvalidPack(t *testing.T) {
	err := runHelpCmd([]string{"--root", "../../testdata/help/duplicate_id_pack", "validate"})
	if err == nil || !strings.Contains(err.Error(), "duplicate topic id") {
		t.Fatalf("err = %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}
