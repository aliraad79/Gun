package models_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePx_RoundTrip(t *testing.T) {
	cases := []struct {
		in   string
		want models.Px
		out  string
	}{
		{"0", 0, "0.00000000"},
		{"1", 1_0000_0000, "1.00000000"},
		{"10000.50", 10000_5000_0000, "10000.50000000"},
		{"10000.5", 10000_5000_0000, "10000.50000000"},
		{"0.00000001", 1, "0.00000001"},
		{"0.00000000", 0, "0.00000000"},
		{"123.45678901", 123_4567_8901, "123.45678901"},
		// truncation past scale
		{"0.123456789", 1234_5678, "0.12345678"},
		// signs
		{"-1.5", -1_5000_0000, "-1.50000000"},
		{"+42", 42_0000_0000, "42.00000000"},
		// no leading int
		{".5", 5000_0000, "0.50000000"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := models.ParsePx(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.out, got.String())
		})
	}
}

func TestParseFixed_Errors(t *testing.T) {
	bad := []string{
		"",
		" ",
		"abc",
		"1.2.3",
		"1e10",
		"--5",
		"+",
		"-",
	}
	for _, s := range bad {
		t.Run(s, func(t *testing.T) {
			_, err := models.ParsePx(s)
			require.Error(t, err)
			assert.True(t, errors.Is(err, models.ErrInvalidFixed))
		})
	}
}

func TestPxComparisons(t *testing.T) {
	a, _ := models.ParsePx("10")
	b, _ := models.ParsePx("20")
	c, _ := models.ParsePx("10")

	assert.True(t, a.Lt(b))
	assert.True(t, b.Gt(a))
	assert.True(t, a.Eq(c))
	assert.True(t, a.Lte(c))
	assert.True(t, a.Gte(c))
	assert.Equal(t, -1, a.Cmp(b))
	assert.Equal(t, 1, b.Cmp(a))
	assert.Equal(t, 0, a.Cmp(c))
	assert.True(t, models.ZeroPx.IsZero())
}

func TestQtyArithmetic(t *testing.T) {
	a, _ := models.ParseQty("3.5")
	b, _ := models.ParseQty("2")

	assert.Equal(t, "5.50000000", a.Add(b).String())
	assert.Equal(t, "1.50000000", a.Sub(b).String())
	assert.True(t, a.Sub(b).IsPositive())
	assert.True(t, b.Sub(a).IsNegative())

	c, _ := models.ParseQty("5")
	d, _ := models.ParseQty("3")
	assert.Equal(t, d, models.MinQty(c, d))
	assert.Equal(t, d, models.MinQty(d, c))
}

func TestPxJSON_StringForm(t *testing.T) {
	type wrap struct {
		Price models.Px `json:"price"`
	}

	// marshal
	w := wrap{Price: 10000_5000_0000}
	b, err := json.Marshal(w)
	require.NoError(t, err)
	assert.JSONEq(t, `{"price":"10000.50000000"}`, string(b))

	// unmarshal from string
	var got wrap
	require.NoError(t, json.Unmarshal([]byte(`{"price":"10000.50"}`), &got))
	assert.Equal(t, models.Px(10000_5000_0000), got.Price)

	// unmarshal from JSON numeric
	require.NoError(t, json.Unmarshal([]byte(`{"price":10000.50}`), &got))
	assert.Equal(t, models.Px(10000_5000_0000), got.Price)
}

func TestQtyJSON_PrecisionPreserved(t *testing.T) {
	// values that would lose precision through float64
	type wrap struct {
		Vol models.Qty `json:"vol"`
	}

	in := `{"vol":"123.45678901"}`
	var got wrap
	require.NoError(t, json.Unmarshal([]byte(in), &got))
	assert.Equal(t, models.Qty(123_4567_8901), got.Vol)

	out, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"vol":"123.45678901"}`, string(out))
}

// type distinctness: Px and Qty should not be assignable to each other.
// (Compile-time check; this test just documents intent.)
func TestPxAndQtyAreDistinctTypes(t *testing.T) {
	var p models.Px = 1
	var q models.Qty = 1
	// Equal int64 values, distinct named types — compiler enforces.
	assert.Equal(t, int64(p), int64(q))
}
