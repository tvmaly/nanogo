package files

import (
	"context"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/modules/help"
)

func TestValidHelpPackLoads(t *testing.T) {
	pack, err := New("../../../testdata/help/valid_pack").Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pack.SchemaVersion != help.PackSchemaVersion || len(pack.Topics) != 2 {
		t.Fatalf("pack = %#v", pack)
	}
}

func TestInvalidHelpPacksAreRejected(t *testing.T) {
	cases := map[string]string{
		"../../../testdata/help/duplicate_id_pack":    "duplicate topic id",
		"../../../testdata/help/broken_link_pack":     "missing.topic",
		"../../../testdata/help/missing_section_pack": "failure_modes",
		"../../../testdata/help/unsafe_path_pack":     "unsafe source path",
	}
	for dir, want := range cases {
		t.Run(dir, func(t *testing.T) {
			_, err := New(dir).Load(context.Background())
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}
