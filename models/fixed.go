package models

import (
	"errors"
	"fmt"
	"strings"
)

// Scale is the number of fractional decimal digits represented by Px and Qty.
// 8 decimals matches the crypto-industry standard (the "satoshi" precision)
// and gives a working range of roughly ±9.2 × 10^10 whole units, which covers
// every realistic spot price and quantity by many orders of magnitude.
const Scale = 8

// scaleFactor = 10^Scale. Multiplying or dividing by this converts between
// "whole units" and the internal scaled int64 representation.
const scaleFactor int64 = 1_0000_0000 // 10^8

// Px is a price expressed as a scaled int64. The zero value is the
// well-defined "zero price" and compares equal to itself with Eq / Cmp.
// Px deliberately does NOT implement multiplication: multiplying two
// fixed-point quantities overflows int64 at realistic exchange volumes,
// so any code path needing notional (price × quantity) must convert to
// big.Int or float64 outside the hot path.
type Px int64

// Qty is a quantity (volume) expressed as a scaled int64. Same scale as Px.
type Qty int64

// ZeroPx and ZeroQty are convenience constants for the zero value.
const (
	ZeroPx  Px  = 0
	ZeroQty Qty = 0
)

// ---------- comparison ----------

func (a Px) Cmp(b Px) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
func (a Px) Eq(b Px) bool  { return a == b }
func (a Px) Lt(b Px) bool  { return a < b }
func (a Px) Lte(b Px) bool { return a <= b }
func (a Px) Gt(b Px) bool  { return a > b }
func (a Px) Gte(b Px) bool { return a >= b }
func (a Px) IsZero() bool  { return a == 0 }

func (a Qty) Cmp(b Qty) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
func (a Qty) Eq(b Qty) bool      { return a == b }
func (a Qty) Lt(b Qty) bool      { return a < b }
func (a Qty) Lte(b Qty) bool     { return a <= b }
func (a Qty) Gt(b Qty) bool      { return a > b }
func (a Qty) Gte(b Qty) bool     { return a >= b }
func (a Qty) IsZero() bool       { return a == 0 }
func (a Qty) IsPositive() bool   { return a > 0 }
func (a Qty) IsNegative() bool   { return a < 0 }

// ---------- arithmetic ----------

// Add returns a + b. No overflow check: at Scale=8 and int64 range, callers
// would need to be adding values near ±9 × 10^10 whole units before this
// matters. Documented and intentional.
func (a Qty) Add(b Qty) Qty { return a + b }

// Sub returns a - b. Same overflow note as Add.
func (a Qty) Sub(b Qty) Qty { return a - b }

// MinQty returns the smaller of a and b.
func MinQty(a, b Qty) Qty {
	if a < b {
		return a
	}
	return b
}

// ---------- parsing & formatting ----------

// ErrInvalidFixed is returned when a string cannot be parsed as a fixed-point
// number at the package Scale.
var ErrInvalidFixed = errors.New("invalid fixed-point number")

// ParsePx parses a decimal string ("10000", "10000.50", "-3.14159") into Px.
// Anything beyond Scale fractional digits is truncated toward zero; this is
// the matching engine's job, not the parser's, to enforce tick size.
func ParsePx(s string) (Px, error) {
	n, err := parseFixed(s)
	if err != nil {
		return 0, err
	}
	return Px(n), nil
}

// ParseQty parses a decimal string into a Qty. See ParsePx for semantics.
func ParseQty(s string) (Qty, error) {
	n, err := parseFixed(s)
	if err != nil {
		return 0, err
	}
	return Qty(n), nil
}

// String formats a Px as a decimal string with exactly Scale fractional
// digits. Trailing zeros are kept so the round-trip is stable.
func (a Px) String() string { return formatFixed(int64(a)) }

// String formats a Qty as a decimal string with exactly Scale fractional
// digits.
func (a Qty) String() string { return formatFixed(int64(a)) }

func parseFixed(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidFixed)
	}

	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("%w: missing digits", ErrInvalidFixed)
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" && fracPart == "" {
		return 0, fmt.Errorf("%w: %q", ErrInvalidFixed, s)
	}

	var whole int64
	if intPart != "" {
		for i := 0; i < len(intPart); i++ {
			c := intPart[i]
			if c < '0' || c > '9' {
				return 0, fmt.Errorf("%w: bad digit %q in %q", ErrInvalidFixed, c, s)
			}
			whole = whole*10 + int64(c-'0')
		}
	}

	var frac int64
	if hasFrac {
		// Truncate to Scale digits.
		if len(fracPart) > Scale {
			fracPart = fracPart[:Scale]
		}
		for i := 0; i < len(fracPart); i++ {
			c := fracPart[i]
			if c < '0' || c > '9' {
				return 0, fmt.Errorf("%w: bad digit %q in %q", ErrInvalidFixed, c, s)
			}
			frac = frac*10 + int64(c-'0')
		}
		// Pad on the right to reach Scale digits.
		for i := len(fracPart); i < Scale; i++ {
			frac *= 10
		}
	}

	out := whole*scaleFactor + frac
	if neg {
		out = -out
	}
	return out, nil
}

func formatFixed(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := v / scaleFactor
	frac := v % scaleFactor

	// "%0*d" pads on the left; we want fixed Scale digits in the fractional part.
	out := fmt.Sprintf("%d.%0*d", whole, Scale, frac)
	if neg {
		return "-" + out
	}
	return out
}

// ---------- JSON ----------

// MarshalJSON emits Px as a JSON string ("10000.50000000") so it round-trips
// through any JSON consumer without precision loss.
func (a Px) MarshalJSON() ([]byte, error) { return marshalFixedJSON(int64(a)) }
func (a Qty) MarshalJSON() ([]byte, error) { return marshalFixedJSON(int64(a)) }

// UnmarshalJSON accepts either a JSON string ("10000.50") or a JSON number
// (10000.50). String form is preferred — numeric form loses precision on
// values that don't fit in a float64.
func (a *Px) UnmarshalJSON(b []byte) error {
	n, err := unmarshalFixedJSON(b)
	if err != nil {
		return err
	}
	*a = Px(n)
	return nil
}
func (a *Qty) UnmarshalJSON(b []byte) error {
	n, err := unmarshalFixedJSON(b)
	if err != nil {
		return err
	}
	*a = Qty(n)
	return nil
}

func marshalFixedJSON(v int64) ([]byte, error) {
	s := formatFixed(v)
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	out = append(out, s...)
	out = append(out, '"')
	return out, nil
}

func unmarshalFixedJSON(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("%w: empty json", ErrInvalidFixed)
	}
	// strip optional surrounding quotes
	if b[0] == '"' && b[len(b)-1] == '"' {
		b = b[1 : len(b)-1]
	}
	return parseFixed(string(b))
}
