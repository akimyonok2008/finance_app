package benchmark

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestSnapshotResolverNormalizesWeights(t *testing.T) {
	resolver := NewSnapshotRecipeResolver(DefaultRecipeSnapshots())
	resolved, err := resolver.Resolve(context.Background(), Recipes["BUFFETT_13F"], time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.Components) == 0 {
		t.Fatalf("expected resolved components")
	}
	if err := validateWeights(resolved.Components); err != nil {
		t.Errorf("resolved 13F weights must be valid: %v", err)
	}
	total := 0.0
	for _, c := range resolved.Components {
		if c.Symbol == "" {
			t.Errorf("resolved 13F leg must be a concrete symbol, got recipeRef %q", c.RecipeRef)
		}
		total += c.Weight
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("resolved weights must sum to 1, got %.6f", total)
	}
}

func TestSnapshotResolverPassesThroughUnknown(t *testing.T) {
	resolver := NewSnapshotRecipeResolver(nil)
	in := Recipes["SPY"]
	out, err := resolver.Resolve(context.Background(), in, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out.ID != in.ID {
		t.Errorf("unknown recipe should pass through unchanged, got %q", out.ID)
	}
}
