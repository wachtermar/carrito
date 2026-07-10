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

func TestMultiplyCentsByQuantityRejectsOverflow(t *testing.T) {
	if _, err := MultiplyCentsByQuantity(999, "999999999999999999999999999999"); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestParseCentsInt64Boundaries(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	tests := []struct {
		input string
		want  int64
	}{
		{"92233720368547758.07", maxInt64},
		{"-92233720368547758.08", minInt64},
	}
	for _, test := range tests {
		got, err := ParseCents(test.input)
		if err != nil {
			t.Fatalf("ParseCents(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("ParseCents(%q) = %d, want %d", test.input, got, test.want)
		}
		if formatted := FormatAmount(got); formatted != test.input {
			t.Fatalf("FormatAmount(ParseCents(%q)) = %q", test.input, formatted)
		}
	}
}

func TestParseCentsRejectsOverflow(t *testing.T) {
	for _, input := range []string{
		"92233720368547758.08",
		"-92233720368547758.09",
		"999999999999999999999999999999999999.00",
	} {
		if _, err := ParseCents(input); err == nil {
			t.Fatalf("ParseCents(%q) should reject overflow", input)
		}
	}
}
