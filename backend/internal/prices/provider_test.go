package prices

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider_ReturnsSeededPrice(t *testing.T) {
	p := NewMockPriceProvider()

	price, err := p.GetLatestPrice(context.Background(), "AAPL")

	require.NoError(t, err)
	assert.Equal(t, "AAPL", price.Symbol)
	assert.Equal(t, 195.00, price.Price)
	assert.Equal(t, "USD", price.Currency)
	assert.Equal(t, "mock", price.Source)
	assert.False(t, price.Timestamp.IsZero())
}

func TestMockProvider_TurkishAndCryptoSymbols(t *testing.T) {
	p := NewMockPriceProvider()

	thyao, err := p.GetLatestPrice(context.Background(), "THYAO.IS")
	require.NoError(t, err)
	assert.Equal(t, 295.00, thyao.Price)
	assert.Equal(t, "TRY", thyao.Currency)

	btc, err := p.GetLatestPrice(context.Background(), "BTC-USD")
	require.NoError(t, err)
	assert.Equal(t, 68000.00, btc.Price)
	assert.Equal(t, "USD", btc.Currency)
}

func TestMockProvider_NormalizesSymbolCasing(t *testing.T) {
	p := NewMockPriceProvider()

	price, err := p.GetLatestPrice(context.Background(), "aapl")

	require.NoError(t, err)
	assert.Equal(t, "AAPL", price.Symbol)
	assert.Equal(t, 195.00, price.Price)
}

func TestMockProvider_UnknownSymbolReturnsError(t *testing.T) {
	p := NewMockPriceProvider()

	_, err := p.GetLatestPrice(context.Background(), "UNKNOWN")

	assert.ErrorIs(t, err, ErrPriceUnavailable)
}

func TestMockProvider_SetOverridesPrice(t *testing.T) {
	p := NewMockPriceProvider()
	p.Set("AAPL", 200.00, "USD")
	p.Set("FOO", 12.34, "EUR")

	aapl, err := p.GetLatestPrice(context.Background(), "AAPL")
	require.NoError(t, err)
	assert.Equal(t, 200.00, aapl.Price)

	foo, err := p.GetLatestPrice(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, 12.34, foo.Price)
	assert.Equal(t, "EUR", foo.Currency)
}

func TestNewProvider_Selection(t *testing.T) {
	mock, err := NewProvider("mock")
	require.NoError(t, err)
	_, ok := mock.(*MockPriceProvider)
	assert.True(t, ok)

	yahoo, err := NewProvider("yahoo")
	require.NoError(t, err)
	_, ok = yahoo.(*YahooFinanceProvider)
	assert.True(t, ok)

	_, err = NewProvider("bogus")
	assert.Error(t, err)
}

// Ensure both concrete providers satisfy the interface at compile time.
var (
	_ PriceProvider = (*MockPriceProvider)(nil)
	_ PriceProvider = (*YahooFinanceProvider)(nil)
)

func TestErrPriceUnavailableIsSentinel(t *testing.T) {
	assert.True(t, errors.Is(ErrPriceUnavailable, ErrPriceUnavailable))
}

func TestValidateAndNormalizeSymbol(t *testing.T) {
	ok := []struct{ in, want string }{
		{"AAPL", "AAPL"},
		{"aapl", "AAPL"},
		{" aapl ", "AAPL"},
		{"THYAO.IS", "THYAO.IS"},
		{"BTC-USD", "BTC-USD"},
		{"brk.b", "BRK.B"},
	}
	for _, c := range ok {
		got, err := ValidateAndNormalizeSymbol(c.in)
		assert.NoErrorf(t, err, "%q should be valid", c.in)
		assert.Equal(t, c.want, got)
	}

	bad := []string{"", "   ", "XYZ FAKE", "A/B", "A;DROP", `A"B`, "🚀MOON", "THISISWAYTOOLONGSYMBOL123"}
	for _, in := range bad {
		_, err := ValidateAndNormalizeSymbol(in)
		assert.ErrorIsf(t, err, ErrInvalidSymbolFormat, "%q should be rejected", in)
	}
}
