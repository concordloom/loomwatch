---
product: loomWatch
kind: operator-console
updated: 2026-08-17
source_of_truth: internal/web/static/style.css
themes: [light, dark]
default_theme: system
fonts:
  primary: "Ubuntu, sans-serif"
  mono: "JetBrains Mono, SF Mono, Fira Code, monospace"
tokens:
  surface: [--surface-page, --surface-card, --surface-card-alt, --surface-raised, --surface-inset, --surface-overlay]
  text: [--text-primary, --text-secondary, --text-muted, --text-inverse]
  border: [--border-default, --border-light, --border-focus]
  status: [--status-healthy, --status-warning, --status-danger, --status-critical, --status-info]
  radius: [--radius-sm, --radius-md, --radius-lg, --radius-xl, --radius-full]
  motion: [--ease-out, --ease-in-out, --ease-drawer, --duration-press, --duration-hover, --duration-popover, --duration-dropdown, --duration-modal]
  layout: [--layout-max-width, --layout-card-min, --layout-gap, --layout-gap-sm]
---


# loomWatch - interface rules

Read this document **before** editing any file under `internal/web/`. It
describes what is already in the code, not a wish list: the source of truth is
`internal/web/static/style.css`, and where the two disagree the stylesheet wins
and the document is what needs fixing.

This used to be `design-system/onwatch/MASTER.md`, generated from abstract
recommendations. It described Material Design 3 with a purple accent, whereas
the code is teal and peach. An agent that read it would have faithfully built
somebody else's interface. The document is deleted: two truths are worse than
one.

## What this product is

An operator's console, not a showcase. People look at it to answer two questions
in a second: **what is close to its limit** and **when does it reset**. Anything
that gets in the way of those two answers is surplus, however pretty.

The consequences are not up for discussion:

- density beats whitespace: tables and cards sit next to each other, with no decorative padding;
- the number is the primary element; it is set in a monospaced font so digits do not jump between refreshes;
- colour carries state, not mood;
- animation explains a change, it does not entertain.

## Themes

Both themes are mandatory and equal. Dark is not "light inverted": it has its
own tokens in the `[data-theme="dark"]` block. Editing the palette in one theme
without the other is an unfinished edit.

Check before handing work over: text-to-background contrast of at least 4.5:1 in
**both** themes. The light theme breaks more often: grey text that reads on dark
becomes unreadable on light.

## Colour

| Role | Token | When |
|---|---|---|
| Teal | `--accent-teal` `#0D9488` | primary action, the "spend" series on a chart |
| Peach | `--accent-peach` `#F59E0B` | warning, secondary series |
| Blue | `--accent-blue` `#3B82F6` | neutral information, tool calls |
| Coral | `--accent-anthropic` `#D97757` | Anthropic affiliation |

Quota states are the only scale the operator makes decisions on:

| State | Threshold | Token |
|---|---|---|
| healthy | < 50% | `--status-healthy` |
| warning | 50-79% | `--status-warning` |
| danger | 80-94% | `--status-danger` |
| critical | >= 95% | `--status-critical` |

**The threshold is duplicated eighteen times across the code** - that is a fact,
not a design: `grep -rn ">= 95" --include="*.go" internal/` returns 18 places
(`internal/api/{gemini,grok,codex,kimi}_types.go`, `internal/web/handlers.go` x11,
`gemini_handlers.go`, `cursor_handlers.go`), plus `getThresholdClass` in `app.js`.
Changing the scale means editing all of them; "change it in two places" is advice
that will leave seventeen untouched.

Worse: **the scale is not the only one.** `internal/web/cursor_handlers.go` ->
`utilStatus` uses its own - the warning boundary is 60 rather than 50, there is
no `danger` state, and at >= 95 it returns `exhausted`, which appears neither in
the table above nor in the styles (`.exhausted-badge` is drawn in `app.js`, but
there is no CSS rule for it). Unifying this into one scale is a separate piece of
work; for now just know that Cursor's states are different.

**Colour is never the only carrier of meaning.** There is always a state label or
a percentage next to it: a colour-blind reader and a monochrome screenshot must
read the same.

## Typography

- The interface uses `--font-primary` (Ubuntu).
- **Every number uses `--font-mono`.** Percentages, volumes, identifiers, reset
  times. A proportional font makes digits jitter on every refresh, and the
  dashboard refreshes constantly.
- Card heading: 13-14px, semibold, letter-spaced (`letter-spacing`); the primary
  number: 28-32px. There should be no intermediate sizes between them.

## Density and grid

- Content column width is `--layout-max-width` (1400px).
- Cards: `repeat(auto-fit, minmax(var(--layout-card-min), 1fr))`, gap `--layout-gap`.
- Radius: cards `--radius-lg`, controls `--radius-md`, badges `--radius-full`.
- Check these widths: 375, 768, 1024, 1440. The page must never scroll
  horizontally; a wide table scrolls inside its own container rather than
  dragging the page with it.

## Motion

Curves and durations are defined as tokens. Do not write raw numbers in new code -
**there are still plenty in the old code**: `style.css` holds around thirty
declarations with raw literals, and the tokens `--duration-hover`,
`--duration-popover`, `--duration-modal` and `--ease-drawer` are never used. That
is debt, not an example: when you edit such a spot, move it onto tokens.

```css
--ease-out:    cubic-bezier(0.23, 1, 0.32, 1);   /* enter and exit */
--ease-in-out: cubic-bezier(0.77, 0, 0.175, 1);  /* movement across the screen */
--ease-drawer: cubic-bezier(0.32, 0.72, 0, 1);   /* drawers */
```

| What | Budget |
|---|---|
| Hover, colour change | 100-160 ms |
| Press | ~120 ms, `transform: scale(0.97)` |
| Tooltips, small popovers | 125-200 ms |
| Dropdowns | 150-250 ms |
| Modals, drawers | 200-500 ms |

Rules whose violation is a defect:

1. **`ease-in` is banned in the interface.** It starts slowly and delays exactly
   the moment the animation exists for. Entrances are always `--ease-out`.
2. **Nothing longer than 300 ms**, except modals and drawers. The violators in the
   current code are known and waiting their turn: `cardEntrance` and
   `sectionEntrance` at 400 ms, `spin 800ms` on the loading indicator, and
   `lw-value-changed 600ms` on the changed-number highlight. The last one is a
   deliberate exception: it is not a response to a human action but a "value
   updated" marker, and it has to have time to reach the eye. The rest are debt.
3. **`transition: all` is banned** - it animates properties that trigger layout
   recalculation. List the properties explicitly and prefer `transform` and
   `opacity`.
4. **Do not put an entrance animation on something seen hundreds of times a day.**
   Table row hover, tab switching - no entrance animation.
5. **Group entrances use a 30-80 ms delay between elements.** The stagger is
   decorative and must never delay the ability to click.
6. **`prefers-reduced-motion` is always respected**: motion is disabled and state
   changes instantly.

## Components

**Quota card.** Heading, countdown to reset, large percentage, a line of absolute
values, a bar, the state, and the reset time. If the provider does not report
absolute values, say so in words rather than drawing `0 / 0`: a zero under a red
90% reads as a broken reading.

**Subscription card in the overview** (`.account-overview-card`). It leads with
the window closest to its limit: the name, a badge for the worst state, a large
percentage (`--font-mono`, 34px), below it which window this is and how long
until reset, then a bar. The remaining windows go below in a compact four-column
grid: label, mini bar, percentage, countdown. The order is exactly this because
"which subscription is about to run out" has to be answered at a glance, not by
reading every window in turn.

The countdowns are live: they are updated by the same per-second ticker as the
card counters, from a timestamp on the element itself (`data-reset-at`). That
means they stay correct even when a poll is late or fails.

**Top line of the page** (`.page-summary`). The number of subscriptions, then the
worst percentage across all windows of all subscriptions with the subscription
name, the window and a countdown, and per-state counters on the right. It is
computed from the same response the cards are drawn from, so it cannot diverge
from them. Empty states are not shown: a row of zeroes on a console is noise.
While the overview holds a single subscription, or the provider serves its data
by another path, the line collapses to the heading.

**Table.** The "Account" column appears only in all-subscriptions mode. Numeric
columns are monospaced and right-aligned. The spend figure is shown as a
percentage when the provider gives no absolutes: a bare value with no scale is
meaningless.

**State badge.** An icon (`svg.status-icon`) plus a word. Never colour alone.
There is no indicator dot inside the badge - `#status-dot` in the header is the
connection indicator, a different element.

**Subscription dropdown.** Shown only when there is more than one subscription.
The choice is remembered in `localStorage` under the key
`onwatch-<provider>-account` (the keys are deliberately not renamed - changing
them would reset everyone's choice).

## The mark

Warp threads of differing length are the subscriptions, the cross thread is the
limit, and one thread has gone past the limit and is coloured with the accent.
The mark mirrors what the operator is looking at, and so needs no explanation.

It lives in three places: `internal/web/static/favicon.svg` (a `--radius-md`
tile, teal background, white threads, accent `#FDE68A` - peach on teal almost
blends in), `.brand-icon` in the header and on the login screen (`currentColor`
plus `.brand-thread-hot` with `--accent-peach`), and the menu bar icon set (key
`star`).

The wordmark is `loom` in the regular weight and `Watch` in semibold: the pair
reads as one word stressed on the second half. In the markup that is
`loom<b>Watch</b>`; there is no need to rewrite it as two `<span>`s.

## Screens

**Login.** Two halves: the console's purpose on a teal fill on the left, the form
on the right. There are deliberately no numbers on the left - showing percentages
before login would mean either inventing them or showing data to someone who has
not identified themselves yet. The threads there are decorative and pushed into
the bottom right corner under a veil: in the first version they ran through the
text and destroyed its contrast. At widths up to 768 the left half is hidden
entirely.

**Settings.** Sections in a left-hand column (`.settings-tabs`, sticky), content
on the right, the save button in a sticky bar at the bottom. A horizontal strip
of tabs already broke at five sections and stretched the form across the full
screen width. On a narrow screen the column becomes a strip above the form again.

**Menu bar** (`internal/web/static/menubar.html`) is a separate document with its
own palette and its own variables; it does not load the shared `style.css`. Its
accent and status colours were aligned with the dashboard by hand, so a palette
change in `style.css` will not reach it - edit both files.

## Icons

SVG from the shared set only, `viewBox="0 0 24 24"`, stroke width 2, size set by
class. Emoji as icons are banned.

## Accessibility

- Every control has a visible focus state; tab order matches the visual order.
- Icon buttons carry an `aria-label`.
- The tap target is at least 44x44 px.
- Progress bars carry `role="progressbar"` and `aria-valuenow`.
- Hover must never be the only way to reach information.

## What not to do

- Do not introduce a new colour outside the tokens: five almost identical teals is debt, not a palette.
- Do not change sizes on hover (`scale` on a card) - the neighbours jump.
- Do not rename `localStorage` keys or `~/.onwatch/**` paths: the first resets
  people's settings, the second diverges from the directories on disk.
- Do not touch metric series names from UI work: alerting rules depend on them.

## Where things live

| File | What |
|---|---|
| `internal/web/static/style.css` | tokens and all styles |
| `internal/web/static/app.js` | behaviour, card and table rendering |
| `internal/web/templates/dashboard.html` | dashboard markup |
| `internal/web/templates/settings.html` | settings |
| `internal/web/templates/login.html` | login |
| `internal/web/templates/layout.html` | shared shell |

## Provenance

loomWatch is a fork of [onWatch](https://github.com/onllm-dev/onwatch) under
GPL-3.0. The product name and the visual design are ours; the licence, the
copyright headers in the sources and the upstream attribution in the footer are
kept - the licence requires that, and it is not a redesign matter.
