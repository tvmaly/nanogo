package file_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	fileobs "github.com/tvmaly/nanogo/ext/obs/file"
)

func TestFileJSONL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	w, err := fileobs.New(fileobs.Config{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Record(context.Background(), event.Event{Kind: event.TurnStarted, At: time.Now()})
		}()
	}
	wg.Wait()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines int
	for sc.Scan() {
		lines++
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("bad jsonl line: %v", err)
		}
		if rec["kind"] != string(event.TurnStarted) {
			t.Fatalf("kind = %v", rec["kind"])
		}
	}
	if lines != 10 {
		t.Fatalf("lines = %d", lines)
	}
}
