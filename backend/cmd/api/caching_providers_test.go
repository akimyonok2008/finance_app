package main

import (
	"context"
	"errors"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/ttlcache"
)

type countingWeightsSource struct {
	calls int
	value *portfolio.PortfolioSummary
	err   error
}

func (c *countingWeightsSource) GetPublicWeights(context.Context, string) (*portfolio.PortfolioSummary, error) {
	c.calls++
	return c.value, c.err
}

func TestCachingWeightsProvider_DedupesRepeatedCalls(t *testing.T) {
	src := &countingWeightsSource{value: &portfolio.PortfolioSummary{CurrentValue: 100}}
	cached := cachingWeightsProvider{inner: src, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}

	for i := 0; i < 5; i++ {
		summary, err := cached.GetPublicWeights(context.Background(), "u1")
		if err != nil {
			t.Fatal(err)
		}
		if summary.CurrentValue != 100 {
			t.Fatalf("got %v, want 100", summary.CurrentValue)
		}
	}
	if src.calls != 1 {
		t.Fatalf("inner provider called %d times, want 1", src.calls)
	}
}

func TestCachingWeightsProvider_DifferentUsersDoNotShareEntries(t *testing.T) {
	src := &countingWeightsSource{value: &portfolio.PortfolioSummary{CurrentValue: 100}}
	cached := cachingWeightsProvider{inner: src, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}

	if _, err := cached.GetPublicWeights(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.GetPublicWeights(context.Background(), "u2"); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 {
		t.Fatalf("inner provider called %d times for 2 distinct users, want 2", src.calls)
	}
}

func TestCachingWeightsProvider_ErrorIsNotCached(t *testing.T) {
	src := &countingWeightsSource{err: errors.New("boom")}
	cached := cachingWeightsProvider{inner: src, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}

	for i := 0; i < 3; i++ {
		if _, err := cached.GetPublicWeights(context.Background(), "u1"); err == nil {
			t.Fatal("expected error to propagate")
		}
	}
	if src.calls != 3 {
		t.Fatalf("a failed lookup must not be cached: inner called %d times, want 3", src.calls)
	}
}

type countingSummarySource struct {
	calls int
	value *portfolio.PortfolioSummary
}

func (c *countingSummarySource) GetSummary(context.Context, string) (*portfolio.PortfolioSummary, error) {
	c.calls++
	return c.value, nil
}

func TestCachingSummaryProvider_DedupesRepeatedCalls(t *testing.T) {
	src := &countingSummarySource{value: &portfolio.PortfolioSummary{CurrentValue: 250}}
	cached := cachingSummaryProvider{inner: src, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}

	for i := 0; i < 5; i++ {
		if _, err := cached.GetSummary(context.Background(), "u1"); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("inner provider called %d times, want 1", src.calls)
	}
}
