package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

func parseNonNegativeQuantity(raw string) (*big.Rat, error) {
	q, err := parseSignedQuantity(raw)
	if err != nil {
		return nil, err
	}
	if q.Sign() < 0 {
		return nil, fmt.Errorf("quantity must be non-negative: %q", raw)
	}
	return q, nil
}

func parseSignedQuantity(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if raw == "" {
		return nil, errors.New("empty quantity")
	}
	q, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid quantity %q", raw)
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

func multiplyCentsRat(cents int64, q *big.Rat) int64 {
	total := new(big.Rat).Mul(new(big.Rat).SetInt64(cents), q)
	return roundRatToInt(total)
}

func roundRatToInt(r *big.Rat) int64 {
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
	return q.Int64()
}
