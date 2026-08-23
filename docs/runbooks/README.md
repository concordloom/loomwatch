# Alert runbooks

One section per alert shipped by the Helm chart. Each answers the same three
questions: what the alert observed, what it does **not** mean, and what decision
is in front of you.

These are deliberately written as decisions rather than instructions. Whether
you have a second account to move load to, whether you can raise a plan
mid-window, and who is allowed to decide either are properties of your
deployment, not of this software. What is written here is the part that is the
same everywhere: what the number means and which question to answer first.

A note that applies to every quota rule below. Utilisation is a percentage of
each plan's own limit, so 100 is the quota itself rather than an estimate. That
is also why these numbers cannot be summed: two accounts at 50% are not 100% of
anything. Use `max by (...)`, or filter.

---

## LoomwatchQuotaHigh

`utilisation > highThreshold` (default 80), for 10m, warning.

**What it observed.** One quota is 80% consumed inside its current window. The
window has not reset and will not until its own reset timestamp.

**What it does not mean.** Nothing is failing. No request has been refused. A
plan at 80% two hours before its window resets is a plan being used correctly.

**The first question: how far is the reset?**

```promql
(loomwatch_quota_reset_timestamp_seconds - time()) / 3600
```

- **Reset is near and the slope is flat** - no action. This is the normal shape
  of the end of a window. If this pattern repeats every window, the alert is
  telling you the plan is sized about right, not that something is wrong.
- **Reset is far** - the remaining budget has to last longer than the burn
  suggests it will. Go to the options below.

**Options, in the order they usually cost least.**

1. **Move load to another account of the same provider**, if one exists:
   ```promql
   count by (provider) (loomwatch_agent_healthy)
   ```
   This is only an option where your workloads can choose a credential.
2. **Reduce consumption** - the usual candidates are batch and scheduled work,
   which is also the work that can most often wait for the window to reset.
3. **Raise the plan.** Whether this takes effect inside the current window is a
   property of the provider, and worth knowing *before* you need it.

**What not to do.** Do not raise `highThreshold` to make the alert stop. The
threshold is a share of the plan's own limit, so raising it does not buy
headroom, it only shortens the warning. Raise it only if 80% is genuinely not
actionable for how this particular plan is used - and then say so in values.

---

## LoomwatchQuotaCritical

`utilisation > criticalThreshold` (default 95), for 5m, critical.

**What it observed.** Same measurement as above, with so little headroom left
that ordinary variance can exhaust it before anyone looks again.

**What happens at 100.** The provider starts refusing requests. The symptom
appears in your workloads, not here - this exporter will keep reporting happily
that the quota is full. If you want to know what that looks like before it
happens, that is a question about your own failure handling.

**The first question: is there anywhere to move?** If a second account for this
provider exists and your workloads can select it, that is almost always faster
than changing a plan. If there is not, the decision is between throttling
something and buying capacity, and it is a decision someone with budget
authority has to make. That person's name is the thing worth writing down here,
in your fork of this file.

---

## LoomwatchQuotaBurnsBeforeReset

`current + slope × seconds_until_reset > 100`, gated to windows resetting inside
`burn.maxHorizonSeconds`. For 30m, warning.

**What it observed.** A prediction, not a measurement: at the rate of the last
`burn.trendWindow`, this quota reaches 100% before its own window resets.

**What it does not mean.** That it will happen. A single batch job inside the
trend window can project a breach that never arrives. This is why the rule
refuses to predict across horizons much longer than its trend window at all -
extrapolating a day across a week is noise amplified, not evidence.

**The first question: is the slope representative?** Look at the series over the
trend window. A step that started when a job started, and is flat before and
after, is not a trend. A steady climb is.

- **Not representative** - no action. The alert clears when the slope flattens.
  If one recurring job keeps triggering it, either the trend window is shorter
  than that job's period, or the job genuinely is the shape of your consumption.
- **Representative** - act as for `LoomwatchQuotaCritical`, with the difference
  that this alert exists to give you time. Using it to act early is the whole
  point; waiting for critical wastes what it bought you.

---

## LoomwatchCollectorNotPolling

`last cycle age > pollInterval × 5`, for 10m, warning.

**What it observed.** Nothing new has arrived from this provider for five poll
intervals. The quota numbers on the dashboard are real, and old.

**Why this one outranks the quota alerts.** Every other rule for this provider
is now reading stale data, so their silence stops being evidence. An exhausted
quota behind a stopped collector looks exactly like a healthy one.

**Usual causes, cheapest to check first.**

1. **The credential expired or was revoked.** Most common by a wide margin.
2. **The provider's API is unreachable** from wherever the collector runs -
   which is a different question from whether it is reachable from your laptop.
3. **The provider changed its API.** Rarer, and it looks the same from here.

```promql
sum by (provider, error_type) (rate(loomwatch_scrape_errors_total[15m]))
```

`error_type` separates these: an auth failure is not a network failure. The
collector's own logs carry the rest.

**What not to do.** Do not silence this one. Silencing it does not hide one
alert, it makes every quota alert for that provider quietly meaningless while
leaving them looking healthy.

---

## LoomwatchCollectorStale

`agent_healthy < 1` for any account of the provider, for 15m, warning.

**What it observed.** The collector marked one of its own agents unhealthy,
because that account's last successful cycle is older than the staleness
threshold it was configured with.

**How this differs from the alert above.** `LoomwatchCollectorNotPolling`
measures the age of the data against the poll interval and fires per provider.
This one reports the collector's own verdict about a specific account. An
account whose credential broke while its siblings keep working shows up here
first, and possibly only here.

**The first question: which account?** The alert aggregates with `min by
(provider)`, so the label does not name it. Find it:

```promql
loomwatch_agent_healthy == 0
```

From there the causes are the same as above, scoped to that one account.

---

## LoomwatchAccountWithoutTeam

`loomwatch:account_team{team="unassigned"}`, for 1h, warning.

**What it observed.** An account is reporting quota, and no entry in
`metrics.prometheusRule.teams` says who owns it. Only rendered when a mapping
exists at all - with no mapping configured, nothing here fires.

**What it does not mean.** That anything is broken. This account's quota is
being collected and every other rule covers it. What it lacks is an owner, which
means it is absent from per-team dashboards and from anything routed by team.

**The decision is which of three things this account is.**

1. **A real account someone owns** - add it to `metrics.prometheusRule.teams`.
   Note that `accountId` must be quoted: a bare `1000000` is a YAML number and
   renders as `1e+06`, which maps an account that does not exist while leaving
   the real one exactly as unowned as before.
2. **Genuinely shared** - map it to a name that says so. `shared`, `platform`,
   whatever is true. A deliberate answer stops the alert and leaves a record;
   silencing it stops the alert and leaves nothing.
3. **Decommissioned** - remove it. Be aware that an account removed in the panel
   keeps exporting while it still has stored history, so this alert can outlive
   the decision by a while.

**What not to do.** Do not map it to `unassigned` to stop the noise. That value
is the sentinel this rule fires on, so it will not stop, and now the account
looks deliberately assigned to nobody.
