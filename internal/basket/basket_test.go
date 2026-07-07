package basket

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	lines, err := Parse(strings.NewReader(`
# comment
54178 2 # milk
12345 0.5
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Ref != "54178" || lines[0].Qty != "2" || lines[0].Comment != "milk" {
		t.Fatalf("unexpected first line: %+v", lines[0])
	}
	if lines[1].LineNo != 4 {
		t.Fatalf("line number = %d, want 4", lines[1].LineNo)
	}
}
