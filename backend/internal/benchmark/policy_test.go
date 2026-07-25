package benchmark

import "testing"

func verifiedResult() BenchmarkReturnResult {
	return BenchmarkReturnResult{
		RecipeVersion: RecipeVersionMetadata{VersionID: "SPY_v1"},
		DataMetadata: BenchmarkEvaluationMetadata{
			Quality: DataQualityVerified, AllSeriesAdjusted: true, AllSeriesTotalReturn: true,
			CorpActionsKnown: true,
		},
	}
}

func syntheticResult() BenchmarkReturnResult {
	return BenchmarkReturnResult{
		RecipeVersion: RecipeVersionMetadata{VersionID: "SPY_v1"},
		DataMetadata: BenchmarkEvaluationMetadata{
			Quality: DataQualitySynthetic, IsSynthetic: true, AllSeriesAdjusted: true,
			AllSeriesTotalReturn: true, CorpActionsKnown: true,
		},
	}
}

func rawResult() BenchmarkReturnResult {
	return BenchmarkReturnResult{
		RecipeVersion: RecipeVersionMetadata{VersionID: "SPY_v1"},
		DataMetadata:  BenchmarkEvaluationMetadata{Quality: DataQualityAcceptable, CorpActionsKnown: false},
	}
}

func TestPolicy_VerifiedOnly(t *testing.T) {
	p := NewAwardEligibilityPolicy(AwardModeVerifiedOnly)

	if d := p.CanPersistPermanentAward(verifiedResult(), EnvironmentProduction); !d.Eligible || d.Verification != AwardVerificationVerified {
		t.Fatalf("verified data must yield a verified award, got %+v", d)
	}
	if d := p.CanPersistPermanentAward(syntheticResult(), EnvironmentProduction); d.Eligible {
		t.Fatalf("synthetic data must be rejected under verified_only, got %+v", d)
	}
	if d := p.CanPersistPermanentAward(rawResult(), EnvironmentProduction); d.Eligible {
		t.Fatalf("raw data must be rejected under verified_only, got %+v", d)
	}
}

func TestPolicy_Demo(t *testing.T) {
	p := NewAwardEligibilityPolicy(AwardModeDemo)

	d := p.CanPersistPermanentAward(syntheticResult(), EnvironmentDemo)
	if !d.Eligible || d.Verification != AwardVerificationDemo {
		t.Fatalf("demo mode must produce a demo award from synthetic data, got %+v", d)
	}
	// Verified data in demo mode is still verified.
	if d := p.CanPersistPermanentAward(verifiedResult(), EnvironmentDemo); d.Verification != AwardVerificationVerified {
		t.Fatalf("verified data stays verified in demo mode, got %+v", d)
	}
}

func TestPolicy_Disabled(t *testing.T) {
	p := NewAwardEligibilityPolicy(AwardModeDisabled)
	if d := p.CanPersistPermanentAward(verifiedResult(), EnvironmentProduction); d.Eligible {
		t.Fatalf("disabled mode must never persist an award, got %+v", d)
	}
}

func TestPolicy_MissingRecipeVersionRejected(t *testing.T) {
	p := NewAwardEligibilityPolicy(AwardModeVerifiedOnly)
	res := verifiedResult()
	res.RecipeVersion.VersionID = ""
	if d := p.CanPersistPermanentAward(res, EnvironmentProduction); d.Eligible {
		t.Fatalf("a result without a recipe version must be rejected, got %+v", d)
	}
}

func TestParseAwardMode_DefaultsSafely(t *testing.T) {
	if ParseAwardMode("nonsense") != AwardModeVerifiedOnly {
		t.Fatal("unknown mode must default to verified_only")
	}
	if ParseAwardMode("demo") != AwardModeDemo {
		t.Fatal("demo must parse")
	}
	if ParseAwardMode("disabled") != AwardModeDisabled {
		t.Fatal("disabled must parse")
	}
}
