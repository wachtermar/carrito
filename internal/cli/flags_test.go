package cli

import (
	"io"
	"testing"
)

func TestParseInterspersedDetectsRegisteredBoolFlags(t *testing.T) {
	fs := newFlagSet("test", io.Discard)
	strict := fs.Bool("strict-servings", false, "")
	defaultPantry := fs.String("default-pantry", "none", "")
	ready := fs.Bool("require-cook-ready", false, "")

	err := parseInterspersed(fs, []string{"mealplan.json", "--strict-servings", "--default-pantry", "minimal-spanish", "--require-cook-ready"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !*strict {
		t.Fatal("strict-servings was not parsed")
	}
	if *defaultPantry != "minimal-spanish" {
		t.Fatalf("default-pantry = %q, want minimal-spanish", *defaultPantry)
	}
	if !*ready {
		t.Fatal("require-cook-ready was not parsed")
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "mealplan.json" {
		t.Fatalf("positionals = %#v", got)
	}
}
