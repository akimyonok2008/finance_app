package rules

import "testing"

func TestEvaluate_ZeroFilterAlwaysEligible(t *testing.T) {
	ok, err := Evaluate(Filter{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("zero-value filter must admit everyone")
	}
}

func TestEvaluate_PortfolioWeightThreshold(t *testing.T) {
	filter := Filter{
		Metric:    MetricPortfolioWeight,
		Sectors:   []string{"technology"},
		Operator:  OpGTE,
		Threshold: 0.5,
	}

	tests := []struct {
		name  string
		facts []PositionFacts
		want  bool
	}{
		{
			name: "exactly at threshold is eligible",
			facts: []PositionFacts{
				{Sector: "technology", Weight: 0.5},
				{Sector: "financials", Weight: 0.5},
			},
			want: true,
		},
		{
			name: "below threshold is ineligible",
			facts: []PositionFacts{
				{Sector: "technology", Weight: 0.3},
				{Sector: "financials", Weight: 0.7},
			},
			want: false,
		},
		{
			name: "multiple matching sectors sum together",
			facts: []PositionFacts{
				{Sector: "technology", Weight: 0.3},
				{Sector: "healthcare", Weight: 0.3},
				{Sector: "financials", Weight: 0.4},
			},
			want: false, // only technology counts here, 0.3 < 0.5
		},
		{
			name: "case and whitespace insensitive sector matching",
			facts: []PositionFacts{
				{Sector: " Technology ", Weight: 0.6},
				{Sector: "financials", Weight: 0.4},
			},
			want: true,
		},
		{
			name:  "empty portfolio never meets a positive threshold",
			facts: nil,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(filter, tt.facts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluate_MultiSectorFilterSumsWeights(t *testing.T) {
	filter := Filter{
		Metric:    MetricPortfolioWeight,
		Sectors:   []string{"technology", "communication_services"},
		Operator:  OpGTE,
		Threshold: 0.5,
	}
	facts := []PositionFacts{
		{Sector: "technology", Weight: 0.3},
		{Sector: "communication_services", Weight: 0.3},
		{Sector: "financials", Weight: 0.4},
	}
	got, err := Evaluate(filter, facts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("combined technology + communication_services weight (0.6) should meet 0.5 threshold")
	}
}

func TestEvaluate_UnsupportedMetricErrors(t *testing.T) {
	_, err := Evaluate(Filter{Metric: "not_a_real_metric"}, nil)
	if err == nil {
		t.Fatal("expected an error for an unsupported metric, not a silent pass/fail")
	}
}

func TestEvaluate_UnsupportedOperatorErrors(t *testing.T) {
	filter := Filter{Metric: MetricPortfolioWeight, Operator: "~=", Threshold: 0.5}
	_, err := Evaluate(filter, []PositionFacts{{Sector: "technology", Weight: 1}})
	if err == nil {
		t.Fatal("expected an error for an unsupported operator")
	}
}

func TestEvaluate_Operators(t *testing.T) {
	facts := []PositionFacts{{Sector: "technology", Weight: 0.5}}
	tests := []struct {
		op   string
		want bool
	}{
		{OpGT, false},
		{OpGTE, true},
		{OpLT, false},
		{OpLTE, true},
		{OpEQ, true},
	}
	for _, tt := range tests {
		filter := Filter{Metric: MetricPortfolioWeight, Sectors: []string{"technology"}, Operator: tt.op, Threshold: 0.5}
		got, err := Evaluate(filter, facts)
		if err != nil {
			t.Fatalf("op %s: unexpected error: %v", tt.op, err)
		}
		if got != tt.want {
			t.Fatalf("op %s: Evaluate() = %v, want %v", tt.op, got, tt.want)
		}
	}
}
