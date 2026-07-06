// Package pricing holds Claude model USD-per-token pricing and computes the
// cost of a token-usage record.
package pricing

import "regexp"

// Rate holds USD-per-token pricing for a single model (at a given speed).
type Rate struct {
	InputPerToken        float64
	OutputPerToken       float64
	CacheWrite5mPerToken float64
	CacheWrite1hPerToken float64
	CacheReadPerToken    float64
}

const perMillion = 1_000_000.0

// WebSearchCostPerRequest is the flat per-search charge for the web_search
// server tool: $10 per 1,000 searches.
const WebSearchCostPerRequest = 10.0 / 1000.0

// rate builds a Rate from published USD-per-million-token prices. Cache
// pricing multipliers (1.25x write/5m, 2x write/1h, 0.1x read on the base
// input price) are fixed across every Claude model and speed — see
// platform.claude.com/docs/en/about-claude/pricing.
func rate(inputPerMTok, outputPerMTok float64) Rate {
	return Rate{
		InputPerToken:        inputPerMTok / perMillion,
		OutputPerToken:       outputPerMTok / perMillion,
		CacheWrite5mPerToken: inputPerMTok * 1.25 / perMillion,
		CacheWrite1hPerToken: inputPerMTok * 2 / perMillion,
		CacheReadPerToken:    inputPerMTok * 0.1 / perMillion,
	}
}

// table maps a model ID, as it appears in the `model` field of a Claude Code
// transcript JSONL, to its standard-speed pricing. Cached from
// platform.claude.com/docs/en/about-claude/pricing on 2026-07-06.
//
// claude-sonnet-5 carries introductory pricing ($2/$10 per MTok) in effect
// through 2026-08-31; update to rate(3, 15) after that date.
var table = map[string]Rate{
	"claude-fable-5":  rate(10, 50),
	"claude-mythos-5": rate(10, 50),

	"claude-opus-4-8": rate(5, 25),
	"claude-opus-4-7": rate(5, 25),
	"claude-opus-4-6": rate(5, 25),
	"claude-opus-4-5": rate(5, 25),
	"claude-opus-4-1": rate(15, 75),
	"claude-opus-4-0": rate(15, 75),
	"claude-opus-4":   rate(15, 75),
	"claude-3-opus":   rate(15, 75),

	"claude-sonnet-5":   rate(2, 10), // introductory pricing through 2026-08-31
	"claude-sonnet-4-6": rate(3, 15),
	"claude-sonnet-4-5": rate(3, 15),
	"claude-sonnet-4-0": rate(3, 15),
	"claude-sonnet-4":   rate(3, 15),
	"claude-3-7-sonnet": rate(3, 15),
	"claude-3-5-sonnet": rate(3, 15),
	"claude-3-sonnet":   rate(3, 15),

	"claude-haiku-4-5": rate(1, 5),
	"claude-3-5-haiku": rate(0.8, 4),
	"claude-3-haiku":   rate(0.25, 1.25),
}

// fastTable holds Fast Mode pricing overrides. Fast mode is a research
// preview limited to Opus 4.8/4.7 at a premium rate — see
// platform.claude.com/docs/en/build-with-claude/fast-mode.
var fastTable = map[string]Rate{
	"claude-opus-4-8": rate(10, 50),
	"claude-opus-4-7": rate(30, 150),
}

// dateSuffix matches a trailing dated snapshot, e.g. "-20251101" or
// "-20240229".
var dateSuffix = regexp.MustCompile(`-\d{8}$`)

func lookupIn(m map[string]Rate, model string) (Rate, bool) {
	if r, ok := m[model]; ok {
		return r, true
	}
	if stripped := dateSuffix.ReplaceAllString(model, ""); stripped != model {
		if r, ok := m[stripped]; ok {
			return r, true
		}
	}
	return Rate{}, false
}

// Lookup returns the standard-speed pricing for model, matching by exact ID
// first, then by stripping a trailing dated-snapshot suffix (e.g.
// "claude-opus-4-1-20250805" falls back to "claude-opus-4-1"). ok is false
// for unrecognized models.
func Lookup(model string) (Rate, bool) {
	return lookupIn(table, model)
}

// LookupSpeed returns model's pricing for the given speed ("standard",
// "fast", or "" — treated as standard). Fast mode is only priced for models
// present in fastTable; requesting fast pricing for any other model returns
// ok=false rather than silently charging the standard rate, since a wrong
// silent fallback would understate cost.
func LookupSpeed(model, speed string) (Rate, bool) {
	if speed == "" || speed == "standard" {
		return Lookup(model)
	}
	if speed == "fast" {
		return lookupIn(fastTable, model)
	}
	return Rate{}, false
}

// Usage is a token/request accounting record to price. CacheCreation5mTokens
// and CacheCreation1hTokens are billed at different rates (1.25x vs 2x base
// input) — Claude transcripts report these separately, so don't collapse
// them into a single "cache creation" total before calling Cost.
type Usage struct {
	InputTokens           int64
	OutputTokens          int64
	CacheCreation5mTokens int64
	CacheCreation1hTokens int64
	CacheReadTokens       int64
	WebSearchRequests     int64
	Speed                 string // "standard" (default) or "fast"
}

// Cost computes the USD cost of u against model's pricing at u.Speed. ok is
// false, and cost 0, if model (at that speed) is not recognized.
func Cost(model string, u Usage) (usd float64, ok bool) {
	r, ok := LookupSpeed(model, u.Speed)
	if !ok {
		return 0, false
	}
	usd = float64(u.InputTokens)*r.InputPerToken +
		float64(u.OutputTokens)*r.OutputPerToken +
		float64(u.CacheCreation5mTokens)*r.CacheWrite5mPerToken +
		float64(u.CacheCreation1hTokens)*r.CacheWrite1hPerToken +
		float64(u.CacheReadTokens)*r.CacheReadPerToken +
		float64(u.WebSearchRequests)*WebSearchCostPerRequest
	return usd, true
}
