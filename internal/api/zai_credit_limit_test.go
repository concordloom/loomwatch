package api

import (
	"testing"
	"time"
)

// Fork change: an account billed in credits reports CREDIT_LIMIT instead of
// TOKENS_LIMIT. Upstream's switch ignored it, so such a subscription produced
// an empty snapshot and vanished from /metrics — exactly the failure the quota
// contour exists to prevent. Payload below is the shape a live max
// subscription returned on 2026-08-17.
func TestZaiCreditLimitIsReadAsSpendQuota(t *testing.T) {
	payload := []byte(`{
	  "code": 200,
	  "data": {
	    "level": "max",
	    "limits": [
	      {"type":"CREDIT_LIMIT","unit":3,"number":5,"percentage":12},
	      {"type":"CREDIT_LIMIT","unit":6,"number":1,"percentage":74,"nextResetTime":1787577488998}
	    ]
	  },
	  "success": true
	}`)

	resp, err := ParseZaiResponse(payload)
	if err != nil {
		t.Fatalf("ParseZaiResponse: %v", err)
	}

	snap := resp.ToSnapshot(time.Now().UTC())
	if snap.TokensPercentage != 74 {
		t.Fatalf("percentage %d, want 74 — a credit-billed account is invisible",
			snap.TokensPercentage)
	}
	if snap.TokensLimit != 6 {
		t.Fatalf("limit %d, want 6 (unit*number of the weekly window)", snap.TokensLimit)
	}
	if snap.TokensNextResetTime == nil {
		t.Fatal("no reset time: the burn-before-reset prediction would never fire")
	}
}

// The older billing must keep working exactly as before.
func TestZaiTokensLimitStillRead(t *testing.T) {
	payload := []byte(`{
	  "code": 200,
	  "data": {
	    "level": "max",
	    "limits": [
	      {"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":89,"nextResetTime":1787364164997},
	      {"type":"TIME_LIMIT","unit":5,"number":1,"usage":4000,"currentValue":0}
	    ]
	  },
	  "success": true
	}`)

	resp, err := ParseZaiResponse(payload)
	if err != nil {
		t.Fatalf("ParseZaiResponse: %v", err)
	}

	snap := resp.ToSnapshot(time.Now().UTC())
	if snap.TokensPercentage != 89 {
		t.Fatalf("percentage %d, want 89", snap.TokensPercentage)
	}
	if snap.TimeUsage != 4000 {
		t.Fatalf("time usage %v, want 4000", snap.TimeUsage)
	}
}

// Fork change: the two spend windows arrive under the same type, and they
// used to overwrite each other - what reached the dashboard was whatever came
// last in the response. The payload below was captured from a live key on
// 08-17: the five-hour window at 7%, the weekly one at 91%.
func TestZaiKeepsBothSpendWindows(t *testing.T) {
	payload := []byte(`{
	  "code": 200,
	  "data": {
	    "level": "max",
	    "limits": [
	      {"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":7,"nextResetTime":1787004000000},
	      {"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":91,"nextResetTime":1787364164997},
	      {"type":"TIME_LIMIT","unit":5,"number":1,"usage":4000,"currentValue":0}
	    ]
	  },
	  "success": true
	}`)

	resp, err := ParseZaiResponse(payload)
	if err != nil {
		t.Fatalf("ParseZaiResponse: %v", err)
	}
	snap := resp.ToSnapshot(time.Now().UTC())

	if snap.TokensPercentage != 91 {
		t.Fatalf("long window %d%%, expected 91%%", snap.TokensPercentage)
	}
	if !snap.TokensShortHasWindow {
		t.Fatal("short window lost - it is the one hit first under heavy use")
	}
	if snap.TokensShortPercentage != 7 {
		t.Fatalf("short window %d%%, expected 7%%", snap.TokensShortPercentage)
	}
	if got := ZaiWindowLabel(snap.TokensShortUnit, snap.TokensShortNumber); got != "5h" {
		t.Fatalf("short window label %q, expected \"5h\"", got)
	}
	if got := ZaiWindowLabel(snap.TokensUnit, snap.TokensNumber); got != "weekly" {
		t.Fatalf("long window label %q, expected \"weekly\"", got)
	}
}

// The element order in the provider response must not decide which window
// lands in the primary fields: the choice used to be positional.
func TestZaiWindowChoiceIgnoresPayloadOrder(t *testing.T) {
	reversed := []byte(`{
	  "code": 200,
	  "data": {
	    "level": "max",
	    "limits": [
	      {"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":91},
	      {"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":7}
	    ]
	  },
	  "success": true
	}`)

	resp, err := ParseZaiResponse(reversed)
	if err != nil {
		t.Fatalf("ParseZaiResponse: %v", err)
	}
	snap := resp.ToSnapshot(time.Now().UTC())

	if snap.TokensPercentage != 91 || snap.TokensShortPercentage != 7 {
		t.Fatalf("reordering the windows in the response changed the result: long=%d%% short=%d%%",
			snap.TokensPercentage, snap.TokensShortPercentage)
	}
}
