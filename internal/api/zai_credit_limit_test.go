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
