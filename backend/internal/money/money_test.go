package money

import (
	"encoding/json"
	"testing"
)

func TestParseExact(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"1", false},
		{"1.0", false},
		{"-1.5", false},
		{"0", false},
		{"", true},
		{"NaN", true},
		{"Inf", true},
		{"-Inf", true},
		{"1,000", true},
		{"1 000", true},
		{"1e10", true},
		{"abc", true},
		{"1.2.3", true},
	}
	for _, c := range cases {
		_, err := ParseAmount(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseAmount(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

func TestCanonicalFormEquivalence(t *testing.T) {
	forms := []string{"1", "1.0", "1.000000"}
	var canon string
	for i, s := range forms {
		a, err := ParseAmount(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if i == 0 {
			canon = a.String()
		} else if a.String() != canon {
			t.Errorf("canonical(%q)=%q want %q", s, a.String(), canon)
		}
	}
	if canon != "1" {
		t.Errorf("canonical = %q want 1", canon)
	}
}

func TestNegativeZero(t *testing.T) {
	a, err := ParseAmount("-0.00")
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != "0" {
		t.Errorf("got %q want 0", a.String())
	}
}

func TestAddSubExact(t *testing.T) {
	a, _ := ParseAmount("0.1")
	b, _ := ParseAmount("0.2")
	sum := a.Add(b)
	if sum.String() != "0.3" {
		t.Errorf("0.1+0.2 = %s want 0.3", sum.String())
	}
}

func TestQuantizeCashRoundHalfEven(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1.005", "1.00"}, // banker's rounding: 1.005 -> 1.00 (even)
		{"1.015", "1.02"}, // -> 1.02 (even)
		{"1.025", "1.02"},
		{"1.245", "1.24"},
	}
	for _, c := range cases {
		a, _ := ParseAmount(c.in)
		q, err := QuantizeCash(a, "USD")
		if err != nil {
			t.Fatal(err)
		}
		got, err := q.StringFixedCurrency("USD")
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("QuantizeCash(%s) = %s want %s", c.in, got, c.want)
		}
	}
}

func TestQuantizeCashUnknownCurrency(t *testing.T) {
	a, _ := ParseAmount("1.00")
	if _, err := QuantizeCash(a, "XYZ"); err == nil {
		t.Error("expected error for unknown currency")
	}
}

func TestCurrencyScaleSupportsPortfolioCurrencies(t *testing.T) {
	for _, currency := range []string{"USD", "EUR", "GBP", "TRY"} {
		scale, err := CurrencyScale(currency)
		if err != nil {
			t.Fatalf("CurrencyScale(%q): %v", currency, err)
		}
		if scale != 2 {
			t.Errorf("CurrencyScale(%q) = %d, want 2", currency, scale)
		}
	}
}

func TestQuantityMulPrice(t *testing.T) {
	q, _ := ParseQuantity("3")
	p, _ := ParsePrice("10.333333333333")
	amt := q.MulPrice(p)
	if amt.String() != "30.999999999999" {
		t.Errorf("got %s", amt.String())
	}
}

func TestDivExactZero(t *testing.T) {
	a, _ := ParseAmount("10")
	z, _ := ParseAmount("0")
	if _, err := a.DivExact(z, 8); err == nil {
		t.Error("expected division by zero error")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	a, _ := ParseAmount("123.450000")
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"123.45"` {
		t.Errorf("marshal = %s want \"123.45\"", b)
	}
	var a2 Amount
	if err := json.Unmarshal(b, &a2); err != nil {
		t.Fatal(err)
	}
	if !a.Equal(a2.Decimal) {
		t.Errorf("round trip mismatch")
	}
}

func TestJSONRejectsNaN(t *testing.T) {
	var a Amount
	if err := json.Unmarshal([]byte(`"NaN"`), &a); err == nil {
		t.Error("expected error")
	}
}

func TestJSONRejectsBareFloatLoss(t *testing.T) {
	// A bare JSON number is still accepted transitionally, but must be
	// routed through exact text, not float64 - verify high precision
	// survives.
	var a Amount
	if err := json.Unmarshal([]byte(`1.123456789012345678`), &a); err != nil {
		t.Fatal(err)
	}
	if a.String() != "1.123456789012345678" {
		t.Errorf("got %s, precision lost", a.String())
	}
}

func TestSQLScanValueRoundTrip(t *testing.T) {
	a, _ := ParseAmount("42.42")
	v, err := a.Value()
	if err != nil {
		t.Fatal(err)
	}
	var a2 Amount
	if err := a2.Scan(v); err != nil {
		t.Fatal(err)
	}
	if !a.Equal(a2.Decimal) {
		t.Errorf("scan/value mismatch: %s != %s", a.String(), a2.String())
	}
}

func TestIndexValueMulRatio(t *testing.T) {
	idx, _ := ParseIndexValue("100")
	ratio, _ := ParseRatio("1.05")
	got := idx.MulRatio(ratio)
	if got.String() != "105" {
		t.Errorf("got %s want 105", got.String())
	}
}

func TestQuantizeIndexHighPrecision(t *testing.T) {
	idx, _ := ParseIndexValue("100.123456789012345678901")
	q := QuantizeIndex(idx)
	if q.String() != "100.123456789012345679" {
		t.Errorf("got %s", q.String())
	}
}
