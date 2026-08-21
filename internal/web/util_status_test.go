package web

import (
	"bytes"
	"regexp"
	"testing"
)

// utilStatus feeds the `status` field for Cursor and OpenCode Go quota cards.
//
// Fork change: it used to run its own scale - warning at 60, critical at 80 and
// "exhausted" at 95 - while every other provider uses 50/80/95 mapped to
// warning/danger/critical. Nothing downstream knew the extra state: `statusConfig`
// in app.js falls back to healthy for an unknown status, so a quota at 97%
// rendered a green tick labelled "Healthy"; `.status-badge[data-status]` has no
// exhausted rule, so the badge lost its colour; and STATUS_RANK scored it 0, so
// an exhausted quota never became the state a card leads with.
//
// The scale is now the product-wide one documented in DESIGN.md.
func TestUtilStatusUsesTheProductWideScale(t *testing.T) {
	cases := []struct {
		util float64
		want string
	}{
		{0, "healthy"},
		{49.9, "healthy"},
		{50, "warning"},
		{59.9, "warning"}, // was healthy under the old Cursor-only scale
		{60, "warning"},
		{79.9, "warning"},
		{80, "danger"}, // was critical under the old scale
		{94.9, "danger"},
		{95, "critical"}, // was exhausted under the old scale
		{100, "critical"},
		{140, "critical"},
	}

	for _, c := range cases {
		if got := utilStatus(c.util); got != c.want {
			t.Errorf("utilStatus(%.1f) = %q, want %q", c.util, got, c.want)
		}
	}
}

// Every status utilStatus can return must be one the dashboard can render: it
// has a `.status-badge[data-status=...]` rule, an entry in statusConfig and a
// STATUS_RANK score. Returning anything else degrades silently to "Healthy".
func TestUtilStatusNeverReturnsAnUnrenderableState(t *testing.T) {
	renderable := map[string]bool{
		"healthy": true, "warning": true, "danger": true, "critical": true,
	}
	for util := 0.0; util <= 200; util += 0.5 {
		if got := utilStatus(util); !renderable[got] {
			t.Fatalf("utilStatus(%.1f) = %q, which the dashboard cannot render", util, got)
		}
	}
}

// The dashboard renders a badge by writing a class or a data-status attribute
// and trusting style.css to have a matching rule. Nothing links the two, which
// is how `.exhausted-badge` shipped with no rule at all and how an unknown
// status silently degraded to a colourless badge. This checks the pairs that
// carry urgency.
//
// The selector must be followed by a declaration block: a bare substring match
// also accepts a renamed or commented-out rule, and a prefix match accepts
// `.exhausted-badge-something-else`.
func TestUrgencyBadgesHaveStyleRules(t *testing.T) {
	css, err := staticFS.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	js, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	hasRule := func(selector string) bool {
		return regexp.MustCompile(regexp.QuoteMeta(selector) + `\s*\{`).Match(css)
	}

	for _, status := range []string{"healthy", "warning", "danger", "critical"} {
		selector := `.status-badge[data-status="` + status + `"]`
		if !hasRule(selector) {
			t.Errorf("style.css has no rule for %s, so that badge renders without colour", selector)
		}
	}

	if bytes.Contains(js, []byte("exhausted-badge")) && !hasRule(".exhausted-badge") {
		t.Error("app.js renders .exhausted-badge but style.css has no rule for it")
	}
}
