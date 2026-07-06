package pricing

import "testing"

func TestLookup(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"current alias", "claude-opus-4-8", true},
		{"introductory sonnet 5", "claude-sonnet-5", true},
		{"dated snapshot falls back to family", "claude-opus-4-1-20250805", true},
		{"dated snapshot without dot version", "claude-opus-4-20250514", true},
		{"dated legacy sonnet", "claude-3-5-sonnet-20241022", true},
		{"unknown model", "gpt-4o", false},
		{"empty model", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := Lookup(tt.model)
			if ok != tt.want {
				t.Errorf("Lookup(%q) ok = %v; want %v", tt.model, ok, tt.want)
			}
		})
	}
}

func TestLookupSpeed(t *testing.T) {
	tests := []struct {
		name  string
		model string
		speed string
		want  bool
	}{
		{"standard explicit", "claude-opus-4-8", "standard", true},
		{"empty speed defaults to standard", "claude-sonnet-4-6", "", true},
		{"fast mode supported model", "claude-opus-4-8", "fast", true},
		{"fast mode unsupported model", "claude-haiku-4-5", "fast", false},
		{"unrecognized speed value", "claude-opus-4-8", "priority", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := LookupSpeed(tt.model, tt.speed)
			if ok != tt.want {
				t.Errorf("LookupSpeed(%q, %q) ok = %v; want %v", tt.model, tt.speed, ok, tt.want)
			}
		})
	}
}

func TestCost(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		usage   Usage
		wantUSD float64
		wantOK  bool
	}{
		{
			name:    "opus 4.8 input and output only",
			model:   "claude-opus-4-8",
			usage:   Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			wantUSD: 5 + 25,
			wantOK:  true,
		},
		{
			name:  "opus 4.8 with split cache writes and a search",
			model: "claude-opus-4-8",
			usage: Usage{
				CacheCreation5mTokens: 1_000_000,
				CacheCreation1hTokens: 1_000_000,
				CacheReadTokens:       1_000_000,
				WebSearchRequests:     100,
			},
			wantUSD: 5*1.25 + 5*2 + 5*0.1 + 100*0.01,
			wantOK:  true,
		},
		{
			name:    "fast mode uses the premium rate",
			model:   "claude-opus-4-8",
			usage:   Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "fast"},
			wantUSD: 10 + 50,
			wantOK:  true,
		},
		{
			name:    "unrecognized model costs zero",
			model:   "some-future-model",
			usage:   Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			wantUSD: 0,
			wantOK:  false,
		},
		{
			name:    "zero usage",
			model:   "claude-haiku-4-5",
			usage:   Usage{},
			wantUSD: 0,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Cost(tt.model, tt.usage)
			if ok != tt.wantOK {
				t.Fatalf("Cost() ok = %v; want %v", ok, tt.wantOK)
			}
			if diff := got - tt.wantUSD; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("Cost() = %v; want %v", got, tt.wantUSD)
			}
		})
	}
}
