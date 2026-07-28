package performance

import (
	"testing"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func testAmount(value string) money.Amount     { return money.MustAmount(value) }
func testIndex(value string) money.IndexValue  { return money.MustIndexValue(value) }
func testAmountPtr(value string) *money.Amount { v := testAmount(value); return &v }

func assertIndexEqual(t *testing.T, expected string, actual money.IndexValue) {
	t.Helper()
	if actual.Cmp(testIndex(expected)) != 0 {
		t.Fatalf("index = %s, want %s", actual.String(), expected)
	}
}
