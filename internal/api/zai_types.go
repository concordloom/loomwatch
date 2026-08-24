package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ZaiResponse is the generic wrapper for all Z.ai API responses
type ZaiResponse[T any] struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
	Data    T      `json:"data"`
}

// ZaiQuotaResponse is the response from GET /monitor/usage/quota/limit
type ZaiQuotaResponse struct {
	Limits []ZaiLimit `json:"limits"`
}

// ZaiLimit represents an individual limit (TIME_LIMIT or TOKENS_LIMIT)
type ZaiLimit struct {
	Type         string           `json:"type"`
	Unit         int              `json:"unit"`
	Number       int              `json:"number"`
	Usage        float64          `json:"usage"`
	CurrentValue float64          `json:"currentValue"`
	Remaining    float64          `json:"remaining"`
	Percentage   int              `json:"percentage"`
	NextResetMs  *int64           `json:"nextResetTime,omitempty"`
	UsageDetails []ZaiUsageDetail `json:"usageDetails,omitempty"`
}

// ZaiUsageDetail represents per-model usage breakdown
type ZaiUsageDetail struct {
	ModelCode string  `json:"modelCode"`
	Usage     float64 `json:"usage"`
}

// GetResetTime returns the reset time as a time.Time pointer.
// Returns nil if there is no reset time (TIME_LIMIT has no reset).
func (l *ZaiLimit) GetResetTime() *time.Time {
	if l.NextResetMs == nil {
		return nil
	}
	// Z.ai returns epoch milliseconds
	t := time.UnixMilli(*l.NextResetMs)
	return &t
}

// ZaiSnapshot is the storage representation (flat, for SQLite)
type ZaiSnapshot struct {
	ID         int64
	CapturedAt time.Time
	// TIME_LIMIT fields
	TimeLimit        int
	TimeUnit         int
	TimeNumber       int
	TimeUsage        float64
	TimeCurrentValue float64
	TimeRemaining    float64
	TimePercentage   int
	TimeUsageDetails string // JSON: [{"modelCode":"search-prime","usage":16}, ...]
	// TOKENS_LIMIT fields
	TokensLimit         int
	TokensUnit          int
	TokensNumber        int
	TokensUsage         float64
	TokensCurrentValue  float64
	TokensRemaining     float64
	TokensPercentage    int
	TokensNextResetTime *time.Time
	// Second spend window.
	//
	// Fork change: the Coding Plan caps spend across two windows at once - a
	// short one (five hours) and a long one (a week) - while the snapshot
	// stored only one, because both arrive under the same type and
	// overwrote each other. What you saw was whatever came last in someone
	// else's JSON: on 08-17 the weekly window was at 91% and the five-hour
	// one at 7%, and under heavy use the invisible one would have been hit
	// first. The long window stays in the fields above so that history and
	// the metric series keep their meaning; the short one lives here.
	TokensShortLimit         int
	TokensShortUnit          int
	TokensShortNumber        int
	TokensShortUsage         float64
	TokensShortCurrentValue  float64
	TokensShortRemaining     float64
	TokensShortPercentage    int
	TokensShortNextResetTime *time.Time
	TokensShortHasWindow     bool
}

// zaiUnitHours converts a window unit code into hours.
//
// The values are confirmed against live responses from 08-17: unit=3 with
// number=5 reset after five hours, unit=6 with number=1 after a week, unit=5
// with number=1 after a month. An unknown code yields zero: such a window is
// treated as the shortest and cannot displace a confirmed long one from the
// primary fields.
func zaiUnitHours(unit int) float64 {
	switch unit {
	case 3:
		return 1
	case 6:
		return 24 * 7
	case 5:
		return 24 * 30
	default:
		return 0
	}
}

// ZaiWindowSeconds is the length of a window in seconds, from the same
// descriptor ZaiWindowLabel names. Zero means the descriptor is not one the
// provider documents, and the caller must then publish nothing: a window of no
// length is a quantity, and a consumer that reads it as one draws exactly the
// wrong conclusion. Absence is the only value that says "not known".
func ZaiWindowSeconds(unit, number int) float64 {
	hours := zaiUnitHours(unit)
	if hours <= 0 || number <= 0 {
		return 0
	}
	return hours * float64(number) * 3600
}

// zaiWindowLabel is the human-readable name of a window, from its descriptor.
func ZaiWindowLabel(unit, number int) string {
	switch unit {
	case 3:
		return fmt.Sprintf("%dh", number)
	case 6:
		if number == 1 {
			return "weekly"
		}
		return fmt.Sprintf("%dw", number)
	case 5:
		if number == 1 {
			return "monthly"
		}
		return fmt.Sprintf("%dmo", number)
	default:
		return fmt.Sprintf("u%d×%d", unit, number)
	}
}

// ToSnapshot converts ZaiQuotaResponse to ZaiSnapshot
func (r *ZaiQuotaResponse) ToSnapshot(capturedAt time.Time) *ZaiSnapshot {
	snapshot := &ZaiSnapshot{
		CapturedAt: capturedAt,
	}

	// Spend windows are collected separately and ordered by duration rather
	// than by their position in the response. This used to be a switch, and
	// two windows of the same type overwrote each other - which one reached
	// the dashboard was decided by the element order in someone else's JSON.
	var spendWindows []ZaiLimit

	for _, limit := range r.Limits {
		switch limit.Type {
		case "TIME_LIMIT":
			snapshot.TimeLimit = limit.Unit * limit.Number
			snapshot.TimeUnit = limit.Unit
			snapshot.TimeNumber = limit.Number
			snapshot.TimeUsage = limit.Usage
			snapshot.TimeCurrentValue = limit.CurrentValue
			snapshot.TimeRemaining = limit.Remaining
			snapshot.TimePercentage = limit.Percentage
			if len(limit.UsageDetails) > 0 {
				b, _ := json.Marshal(limit.UsageDetails)
				snapshot.TimeUsageDetails = string(b)
			}
		// Fork change: CREDIT_LIMIT is the same spend window under another name.
		//
		// Z.ai labels the consumption quota by how the account is billed:
		// older subscriptions report TOKENS_LIMIT, newer ones CREDIT_LIMIT.
		// The payload is identical — same unit/number windows (5 hours and a
		// week), same percentage, same nextResetTime — so the two belong in the
		// same fields. Upstream's switch has no default branch, so an account
		// on the newer billing produced an empty snapshot and disappeared from
		// /metrics entirely: measured 2026-08-17 on a live max subscription
		// that exported no series at all while two others did.
		case "TOKENS_LIMIT", "CREDIT_LIMIT":
			spendWindows = append(spendWindows, limit)
		}
	}

	// The longest window takes the primary fields: that is where it lived
	// before the second one appeared, so the meaning of history and of the
	// metric series does not change. The next longest becomes the short one.
	sort.SliceStable(spendWindows, func(i, j int) bool {
		return zaiUnitHours(spendWindows[i].Unit)*float64(spendWindows[i].Number) <
			zaiUnitHours(spendWindows[j].Unit)*float64(spendWindows[j].Number)
	})

	if n := len(spendWindows); n > 0 {
		primary := spendWindows[n-1]
		snapshot.TokensLimit = primary.Unit * primary.Number
		snapshot.TokensUnit = primary.Unit
		snapshot.TokensNumber = primary.Number
		snapshot.TokensUsage = primary.Usage
		snapshot.TokensCurrentValue = primary.CurrentValue
		snapshot.TokensRemaining = primary.Remaining
		snapshot.TokensPercentage = primary.Percentage
		snapshot.TokensNextResetTime = primary.GetResetTime()

		if n > 1 {
			short := spendWindows[n-2]
			snapshot.TokensShortLimit = short.Unit * short.Number
			snapshot.TokensShortUnit = short.Unit
			snapshot.TokensShortNumber = short.Number
			snapshot.TokensShortUsage = short.Usage
			snapshot.TokensShortCurrentValue = short.CurrentValue
			snapshot.TokensShortRemaining = short.Remaining
			snapshot.TokensShortPercentage = short.Percentage
			snapshot.TokensShortNextResetTime = short.GetResetTime()
			snapshot.TokensShortHasWindow = true
		}
	}

	return snapshot
}

// ParseZaiResponse parses a Z.ai API response from JSON bytes
func ParseZaiResponse(data []byte) (*ZaiQuotaResponse, error) {
	var wrapper ZaiResponse[ZaiQuotaResponse]
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	if !wrapper.Success {
		return nil, fmt.Errorf("API error: code=%d, msg=%s", wrapper.Code, wrapper.Msg)
	}

	return &wrapper.Data, nil
}
