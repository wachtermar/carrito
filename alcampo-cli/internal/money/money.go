package money

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type Money struct {
	Amount   string `json:"amount,omitempty"`
	Currency string `json:"currency,omitempty"`
	Cents    int64  `json:"cents,omitempty"`
}

func New(amount, currency string) (Money, error) {
	if currency == "" {
		currency = "EUR"
	}
	cents, err := ParseCents(amount)
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: FormatAmount(cents), Currency: currency, Cents: cents}, nil
}

var decimalRE = regexp.MustCompile(`^\s*([+-]?)(\d+)(?:[,.](\d+))?\s*$`)

func ParseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty money amount")
	}
	m := decimalRE.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid money amount %q", s)
	}
	euros, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return 0, err
	}
	frac := m[3]
	for len(frac) < 3 {
		frac += "0"
	}
	if len(frac) > 3 {
		frac = frac[:3]
	}
	millis, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	cents := euros*100 + millis/10
	if millis%10 >= 5 {
		cents++
	}
	if m[1] == "-" {
		cents = -cents
	}
	return cents, nil
}

func FormatAmount(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func Format(cents int64, currency string) string {
	if currency == "" {
		currency = "EUR"
	}
	return FormatAmount(cents) + " " + currency
}

func ParseQuantity(s string) (*big.Rat, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return nil, errors.New("empty quantity")
	}
	q, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid quantity %q", s)
	}
	if q.Sign() < 0 {
		return nil, fmt.Errorf("quantity must be non-negative: %q", s)
	}
	return q, nil
}

func MultiplyCentsByQuantity(cents int64, qty string) (int64, error) {
	q, err := ParseQuantity(qty)
	if err != nil {
		return 0, err
	}
	total := new(big.Rat).Mul(new(big.Rat).SetInt64(cents), q)
	return roundRatToInt(total), nil
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
