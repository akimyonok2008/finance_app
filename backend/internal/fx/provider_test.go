package fx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRate_ToUSD(t *testing.T) {
	p := NewMockFXProvider()
	ctx := context.Background()

	r, err := p.GetRate(ctx, "USD", "USD")
	require.NoError(t, err)
	assert.Equal(t, 1.0, r)

	r, err = p.GetRate(ctx, "TRY", "USD")
	require.NoError(t, err)
	assert.InDelta(t, 0.031, r, 1e-9)

	r, err = p.GetRate(ctx, "EUR", "USD")
	require.NoError(t, err)
	assert.InDelta(t, 1.08, r, 1e-9)

	r, err = p.GetRate(ctx, "GBP", "USD")
	require.NoError(t, err)
	assert.InDelta(t, 1.27, r, 1e-9)
}

func TestConvert_ToUSD(t *testing.T) {
	p := NewMockFXProvider()
	ctx := context.Background()

	// 25000 TRY -> USD = 775
	v, err := p.Convert(ctx, 25000, "TRY", "USD")
	require.NoError(t, err)
	assert.InDelta(t, 775.0, v, 1e-6)

	// USD -> USD is identity.
	v, err = p.Convert(ctx, 1950, "USD", "USD")
	require.NoError(t, err)
	assert.InDelta(t, 1950.0, v, 1e-6)
}

func TestConvert_NormalizesCurrencyCasing(t *testing.T) {
	p := NewMockFXProvider()
	v, err := p.Convert(context.Background(), 100, "try", "usd")
	require.NoError(t, err)
	assert.InDelta(t, 3.1, v, 1e-6)
}

func TestConvert_ReverseFromUSD(t *testing.T) {
	p := NewMockFXProvider()
	// USD -> TRY = 1 / 0.031
	v, err := p.Convert(context.Background(), 31, "USD", "TRY")
	require.NoError(t, err)
	assert.InDelta(t, 1000.0, v, 1e-6)
}

func TestGetRate_UnsupportedCurrency(t *testing.T) {
	p := NewMockFXProvider()
	_, err := p.GetRate(context.Background(), "JPY", "USD")
	assert.ErrorIs(t, err, ErrUnsupportedCurrency)
	_, err = p.Convert(context.Background(), 10, "USD", "JPY")
	assert.ErrorIs(t, err, ErrUnsupportedCurrency)
}

var _ FXProvider = (*MockFXProvider)(nil)
