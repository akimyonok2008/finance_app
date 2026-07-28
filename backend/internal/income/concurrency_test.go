package income

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentWorkers_CreditOnce asserts that two workers processing the same
// event concurrently credit a portfolio exactly once. The store's transactional
// claim (per (event, portfolio)) is the guard.
func TestConcurrentWorkers_CreditOnce(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-RACE", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	// Ingest once so both "workers" see the stored event.
	require.NoError(t, h.income.Ingest(context.Background()))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.income.Process(context.Background())
		}()
	}
	wg.Wait()

	// Exactly one $100 credit despite 8 concurrent processors.
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("100")), h.cashUSD(t, "u1"))
}

// TestClaimApplication_SecondClaimFails verifies the claim primitive directly.
func TestClaimApplication_SecondClaimFails(t *testing.T) {
	store := NewInMemoryStore()
	claimed, err := store.ClaimApplication(context.Background(), "e1", "p1", "u1")
	require.NoError(t, err)
	assert.True(t, claimed)
	again, err := store.ClaimApplication(context.Background(), "e1", "p1", "u1")
	require.NoError(t, err)
	assert.False(t, again) // in-flight: second claim is refused
}

// TestPrivacy_ViewsAreOwnerScoped asserts a user never sees another user's
// income application through the read path.
func TestPrivacy_ViewsAreOwnerScoped(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	h.fund(t, "u2", "50")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-PRIV", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	u1Views, err := h.income.ListIncomeEventViews(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, u1Views, 1)
	assertAmountEqual(t, "100", u1Views[0].NetAmount) // u1: 100 shares

	u2Views, err := h.income.ListIncomeEventViews(context.Background(), "u2")
	require.NoError(t, err)
	require.Len(t, u2Views, 1)
	assertAmountEqual(t, "50", u2Views[0].NetAmount) // u2: 50 shares, isolated

	// u2 cannot read u1's event via GetIncomeEventView.
	_, ok, err := h.income.GetIncomeEventView(context.Background(), "u2", portfolioID(t, h, "u1"), "manual_dev:AAPL-PRIV")
	require.NoError(t, err)
	assert.False(t, ok)
}
