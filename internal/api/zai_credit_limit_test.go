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

// Fork change: два окна расхода приходят под одним типом, и раньше они
// перезаписывали друг друга — до панели доезжало то, что стояло в ответе
// последним. Полезная нагрузка ниже снята с живого ключа 17.08: пятичасовое
// окно на 7%, недельное на 91%.
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
		t.Fatalf("длинное окно %d%%, ожидалось 91%%", snap.TokensPercentage)
	}
	if !snap.TokensShortHasWindow {
		t.Fatal("короткое окно потеряно — именно оно упирается первым при интенсивной работе")
	}
	if snap.TokensShortPercentage != 7 {
		t.Fatalf("короткое окно %d%%, ожидалось 7%%", snap.TokensShortPercentage)
	}
	if got := ZaiWindowLabel(snap.TokensShortUnit, snap.TokensShortNumber); got != "5h" {
		t.Fatalf("подпись короткого окна %q, ожидалось \"5h\"", got)
	}
	if got := ZaiWindowLabel(snap.TokensUnit, snap.TokensNumber); got != "weekly" {
		t.Fatalf("подпись длинного окна %q, ожидалось \"weekly\"", got)
	}
}

// Порядок элементов в ответе провайдера не должен влиять на то, какое окно
// попадёт в основные поля: раньше выбор был позиционным.
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
		t.Fatalf("перестановка окон в ответе поменяла результат: длинное=%d%% короткое=%d%%",
			snap.TokensPercentage, snap.TokensShortPercentage)
	}
}
