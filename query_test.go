package setalabs

import (
	"math"
	"testing"
)

func TestQueryMatchesPHPHTTPBuildQuery(t *testing.T) {
	query, err := buildQuery(Params{
		{Name: "b", Value: "2"},
		{Name: "a", Value: "1"},
		{Name: "q", Value: "x y"},
		{Name: "flag", Value: true},
		{Name: "off", Value: false},
		{Name: "n", Value: nil},
		{Name: "ids", Value: []any{1, nil, "x"}},
		{Name: "f", Value: map[string]any{"x": "a/b"}},
		{Name: "whole", Value: 1.0},
		{Name: "frac", Value: 1.5},
		{Name: "big", Value: 1e16},
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = "b=2&a=1&q=x%20y&flag=1&off=0&ids%5B0%5D=1&ids%5B2%5D=x" +
		"&f%5Bx%5D=a%2Fb&whole=1&frac=1.5&big=1.0E%2B16"
	if query != want {
		t.Fatalf("buildQuery() = %s, want %s", query, want)
	}
}

func TestMapQueryIsSorted(t *testing.T) {
	query, err := buildQuery(map[string]any{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "a=1&b=2" {
		t.Fatalf("buildQuery() = %s, want a=1&b=2", query)
	}
}

func TestStringQueryKeepsRawText(t *testing.T) {
	query, err := buildQuery("?language=en&q=a b")
	if err != nil {
		t.Fatal(err)
	}
	if query != "language=en&q=a b" {
		t.Fatalf("buildQuery() = %q", query)
	}
}

func TestFloatStringMatchesPHP(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{0.0, "0"},
		{math.Copysign(0, -1), "-0"},
		{-1.5, "-1.5"},
		{0.1, "0.1"},
		{0.0001, "0.0001"},
		{1e-5, "1.0E-5"},
		{1e-7, "1.0E-7"},
		{1.23e-8, "1.23E-8"},
		{1.1, "1.1"},
		{10.0, "10"},
		{1.0 / 3.0, "0.33333333333333"},
		{1e14, "1.0E+14"},
		{1e16, "1.0E+16"},
		{math.Inf(1), "INF"},
		{math.Inf(-1), "-INF"},
		{math.NaN(), "NAN"},
	}
	for _, test := range tests {
		if got := formatPHPFloat(test.value); got != test.want {
			t.Errorf("formatPHPFloat(%v) = %s, want %s", test.value, got, test.want)
		}
	}
}
