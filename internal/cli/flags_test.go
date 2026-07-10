package cli

import (
	"io"
	"testing"
)

func TestParseInterspersedDetectsRegisteredBoolFlags(t *testing.T) {
	fs := newFlagSet("test", io.Discard)
	verbose := fs.Bool("verbose", false, "")
	format := fs.String("format", "text", "")
	jsonOutput := fs.Bool("json", false, "")

	err := parseInterspersed(fs, []string{"input.json", "--verbose", "--format", "compact", "--json"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !*verbose {
		t.Fatal("verbose was not parsed")
	}
	if *format != "compact" {
		t.Fatalf("format = %q, want compact", *format)
	}
	if !*jsonOutput {
		t.Fatal("json was not parsed")
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "input.json" {
		t.Fatalf("positionals = %#v", got)
	}
}
