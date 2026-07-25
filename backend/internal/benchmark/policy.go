package benchmark

import "fmt"

// EnvironmentMode is the deployment environment. Only production-grade
// environments may persist verified permanent awards; the value is derived from
// configuration, never inferred from the mere presence of an API key.
type EnvironmentMode string

const (
	EnvironmentDevelopment EnvironmentMode = "development"
	EnvironmentTest        EnvironmentMode = "test"
	EnvironmentDemo        EnvironmentMode = "demo"
	EnvironmentProduction  EnvironmentMode = "production"
)

// AwardMode is the benchmark award policy (BENCHMARK_AWARD_MODE).
type AwardMode string

const (
	// AwardModeDisabled shows catalogue and progress but writes no permanent
	// awards at all.
	AwardModeDisabled AwardMode = "disabled"
	// AwardModeDemo may create demo awards from synthetic data; such awards are
	// persistently marked demo and never counted as verified.
	AwardModeDemo AwardMode = "demo"
	// AwardModeVerifiedOnly persists only verified awards backed by real,
	// adjusted/total-return, fresh, complete data and a valid recipe version.
	AwardModeVerifiedOnly AwardMode = "verified_only"
)

// ParseAwardMode maps a config string to an AwardMode, defaulting safely to
// verified_only for unknown values.
func ParseAwardMode(s string) AwardMode {
	switch AwardMode(s) {
	case AwardModeDisabled:
		return AwardModeDisabled
	case AwardModeDemo:
		return AwardModeDemo
	case AwardModeVerifiedOnly:
		return AwardModeVerifiedOnly
	default:
		return AwardModeVerifiedOnly
	}
}

// AwardEligibilityDecision is the policy verdict for one evaluated benchmark.
type AwardEligibilityDecision struct {
	Eligible     bool
	Verification AwardVerification
	Reasons      []string
}

// AwardEligibilityPolicy centralizes the permanent-award rule so eligibility is
// decided in exactly one place, not scattered across handlers and repositories.
type AwardEligibilityPolicy struct {
	Mode AwardMode
}

// NewAwardEligibilityPolicy builds a policy for the given mode.
func NewAwardEligibilityPolicy(mode AwardMode) AwardEligibilityPolicy {
	return AwardEligibilityPolicy{Mode: mode}
}

// isVerifiedGrade reports whether a benchmark result is backed by data trusted
// enough for a verified award: verified quality, real (not synthetic), fresh
// (not stale), fully adjusted/total-return, corporate actions known, and a valid
// recipe version.
func isVerifiedGrade(result BenchmarkReturnResult) (bool, []string) {
	var reasons []string
	m := result.DataMetadata
	if result.RecipeVersion.VersionID == "" {
		reasons = append(reasons, "recipe version unavailable")
	}
	if m.Quality != DataQualityVerified {
		reasons = append(reasons, fmt.Sprintf("data quality is %q, not verified", m.Quality))
	}
	if m.IsSynthetic {
		reasons = append(reasons, "data is synthetic")
	}
	if m.UsedStaleData {
		reasons = append(reasons, "data is stale")
	}
	if !m.AllSeriesAdjusted && !m.AllSeriesTotalReturn {
		reasons = append(reasons, "not all legs are adjusted/total-return")
	}
	if !m.CorpActionsKnown {
		reasons = append(reasons, "corporate-action handling unknown for a leg")
	}
	return len(reasons) == 0, reasons
}

// CanPersistPermanentAward decides whether a benchmark result may become a
// permanent award and, if so, with what verification classification.
func (p AwardEligibilityPolicy) CanPersistPermanentAward(result BenchmarkReturnResult, env EnvironmentMode) AwardEligibilityDecision {
	verified, reasons := isVerifiedGrade(result)

	switch p.Mode {
	case AwardModeDisabled:
		return AwardEligibilityDecision{Eligible: false, Verification: AwardVerificationUnverified, Reasons: []string{"award mode is disabled"}}

	case AwardModeDemo:
		if verified {
			return AwardEligibilityDecision{Eligible: true, Verification: AwardVerificationVerified}
		}
		if result.DataMetadata.IsSynthetic {
			return AwardEligibilityDecision{Eligible: true, Verification: AwardVerificationDemo, Reasons: []string{"demo award from synthetic data"}}
		}
		return AwardEligibilityDecision{Eligible: false, Verification: AwardVerificationUnverified, Reasons: reasons}

	case AwardModeVerifiedOnly:
		if verified {
			return AwardEligibilityDecision{Eligible: true, Verification: AwardVerificationVerified}
		}
		return AwardEligibilityDecision{Eligible: false, Verification: AwardVerificationUnverified, Reasons: reasons}

	default:
		return AwardEligibilityDecision{Eligible: false, Verification: AwardVerificationUnverified, Reasons: []string{"unknown award mode"}}
	}
}
