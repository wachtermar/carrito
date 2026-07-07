package money

import "testing"

func TestParseCents(t *testing.T) {
	tests := map[string]int64{
		"5.76":  576,
		"5,76":  576,
		"0.999": 100,
		"12":    1200,
	}
	for in, want := range tests {
		got, err := ParseCents(in)
		if err != nil {
			t.Fatalf("ParseCents(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseCents(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestMultiplyCentsByQuantity(t *testing.T) {
	got, err := MultiplyCentsByQuantity(199, "2.5")
	if err != nil {
		t.Fatal(err)
	}
	if got != 498 {
		t.Fatalf("got %d, want 498", got)
	}
}
