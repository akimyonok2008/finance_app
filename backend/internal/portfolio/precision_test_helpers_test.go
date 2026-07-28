package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func testAmount(value string) money.Amount     { return money.MustAmount(value) }
func testQuantity(value string) money.Quantity { return money.MustQuantity(value) }
func testPrice(value string) money.Price       { return money.MustPrice(value) }
func testRatio(value string) money.Ratio       { return money.MustRatio(value) }
func testIndex(value string) money.IndexValue  { return money.MustIndexValue(value) }
func testAmountPtr(value string) *money.Amount { v := testAmount(value); return &v }
func testQuantityPtr(value string) *money.Quantity {
	v := testQuantity(value)
	return &v
}
func testPricePtr(value string) *money.Price { v := testPrice(value); return &v }

func requireAmountEqual(t *testing.T, expected string, actual money.Amount, msgAndArgs ...any) {
	t.Helper()
	require.True(t, testAmount(expected).EqualAmount(actual),
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func assertAmountEqual(t *testing.T, expected string, actual money.Amount, msgAndArgs ...any) {
	t.Helper()
	assert.True(t, testAmount(expected).EqualAmount(actual),
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func requireQuantityEqual(t *testing.T, expected string, actual money.Quantity, msgAndArgs ...any) {
	t.Helper()
	require.True(t, testQuantity(expected).EqualQuantity(actual),
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func assertQuantityEqual(t *testing.T, expected string, actual money.Quantity, msgAndArgs ...any) {
	t.Helper()
	assert.True(t, testQuantity(expected).EqualQuantity(actual),
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func assertPriceEqual(t *testing.T, expected string, actual money.Price, msgAndArgs ...any) {
	t.Helper()
	assert.True(t, testPrice(expected).Cmp(actual) == 0,
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func assertAmountValuesEqual(t *testing.T, expected, actual money.Amount, msgAndArgs ...any) {
	t.Helper()
	assert.True(t, expected.EqualAmount(actual),
		append([]any{"expected %s, got %s", expected.String(), actual.String()}, msgAndArgs...)...)
}

func assertIndexEqual(t *testing.T, expected string, actual money.IndexValue, msgAndArgs ...any) {
	t.Helper()
	assert.Equal(t, 0, testIndex(expected).Cmp(actual),
		append([]any{"expected %s, got %s", expected, actual.String()}, msgAndArgs...)...)
}

func assertIndexValuesEqual(t *testing.T, expected, actual money.IndexValue, msgAndArgs ...any) {
	t.Helper()
	assert.Equal(t, 0, money.QuantizeIndex(expected).Cmp(money.QuantizeIndex(actual)),
		append([]any{"expected %s, got %s", expected.String(), actual.String()}, msgAndArgs...)...)
}
