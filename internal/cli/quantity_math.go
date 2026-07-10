package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalQuantityRE = regexp.MustCompile(`^[+-]?\d+(?:[.,]\d{1,3})?$`)

func parseNonNegativeQuantity(raw string) (*big.Rat, error) {
	normalized := strings.TrimSpace(raw)
	if !decimalQuantityRE.MatchString(normalized) {
		return nil, fmt.Errorf("quantity must be an ordinary decimal with at most three decimal places: %q", raw)
	}
	q, err := parseSignedQuantity(raw)
	if err != nil {
		return nil, err
	}
	if q.Sign() < 0 {
		return nil, fmt.Errorf("quantity must be non-negative: %q", raw)
	}
	if q.Cmp(big.NewRat(10_000, 1)) > 0 {
		return nil, fmt.Errorf("quantity must not exceed 10000: %q", raw)
	}
	return q, nil
}

func parseSignedQuantity(raw string) (*big.Rat, error) {
	original := raw
	raw = strings.TrimSpace(raw)
	if !decimalQuantityRE.MatchString(raw) {
		if raw == "" {
			return nil, errors.New("empty quantity")
		}
		return nil, fmt.Errorf("quantity must be an ordinary decimal with at most three decimal places: %q", original)
	}
	raw = strings.ReplaceAll(raw, ",", ".")
	q, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid quantity %q", original)
	}
	return q, nil
}

func ratJSONNumber(q *big.Rat) json.Number {
	return json.Number(formatRat(q))
}

func formatRat(q *big.Rat) string {
	if q == nil {
		return "0"
	}
	s := q.FloatString(3)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func multiplyCentsRat(cents int64, q *big.Rat) (int64, error) {
	total := new(big.Rat).Mul(new(big.Rat).SetInt64(cents), q)
	rounded := roundRatToInt(total)
	if !rounded.IsInt64() {
		return 0, errors.New("cart value is outside the supported range")
	}
	return rounded.Int64(), nil
}

func roundRatToInt(r *big.Rat) *big.Int {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	sign := num.Sign()
	if sign < 0 {
		num.Abs(num)
	}
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(num, den, rem)
	rem.Mul(rem, big.NewInt(2))
	if rem.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if sign < 0 {
		q.Neg(q)
	}
	return q
}

func addCents(left, right int64) (int64, error) {
	total := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !total.IsInt64() {
		return 0, errors.New("cart total is outside the supported range")
	}
	return total.Int64(), nil
}
