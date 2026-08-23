# Prometheus Metrics

onWatch exposes a Prometheus-compatible `/metrics` endpoint so quota, credit, and agent-health data can be scraped into Prometheus, Grafana, and Alertmanager alongside your other observability data.

> Status: Beta. Metric names and labels may evolve based on feedback before 1.0. Please open issues or PRs against `onllm-dev/onWatch` with suggestions.

> **Rename (2026-08-17).** The product is called loomWatch and the series are
> `loomwatch_*`. For a transition period the exporter also publishes the former
> `onwatch_*` with the same values and a deprecated marker in HELP, so that
> existing rules and panels do not go silent at the moment of the rollout.
> Write new queries against `loomwatch_*`: the deprecated half will be removed.
> History accumulated before the rename stays under the old name - there is
> nothing to migrate it across with.


## Migration notes (pre-1.0)

If you deployed onWatch 2.11.40's beta metrics, the following changed in the 1.0 correctness pass. Update dashboards/alerts before upgrading.

| Removed/renamed | Replacement |
|---|---|
| `loomwatch_quota_time_until_reset_seconds` | `loomwatch_quota_reset_timestamp_seconds` (absolute Unix seconds; compute remaining as `metric - time()`) |
| `loomwatch_auth_token_status` | `loomwatch_agent_healthy` (honest name; same `1`/`0` semantics, measures poll freshness, **not** real OAuth validity) |
| `account_id=""` | `account_id="default"` (sentinel for single-account providers) |
| `account_id="default"` for `provider="zai"` | the real `provider_accounts.id`, one series per subscription (fork change: Z.ai is multi-account) |
| `loomwatch_credits_balance{provider,account_id}` | `loomwatch_credits_balance{provider,account_id,unit}` (unit disambiguates `usd`, `credits`, `prompt_credits`) |

## Enabling the Endpoint

| Variable | Purpose | Default |
|---|---|---|
| `ONWATCH_METRICS_TOKEN` | Bearer token required on `/metrics` requests | unset (endpoint is open, logs a warning at startup) |

- If `ONWATCH_METRICS_TOKEN` is unset, `/metrics` responds without auth and onWatch emits `WARN metrics endpoint is unauthenticated; set ONWATCH_METRICS_TOKEN to restrict /metrics access` at startup.
- If set, Prometheus must send `Authorization: Bearer <token>` on every scrape.

```bash
export ONWATCH_METRICS_TOKEN="$(openssl rand -hex 32)"
./onwatch --daemon
curl -H "Authorization: Bearer $ONWATCH_METRICS_TOKEN" http://localhost:8080/metrics
```

## Exposed Metrics

Standard Go runtime / process collectors (`go_*`, `process_*`) are also registered.

### Gauges

| Metric | Labels | Description |
|---|---|---|
| `loomwatch_quota_utilization_percent` | `provider`, `quota_type`, `account_id` | Current quota utilization as a percentage (0-100). |
| `loomwatch_quota_reset_timestamp_seconds` | `provider`, `quota_type`, `account_id` | Unix timestamp (seconds) at which the quota next resets. Compute remaining: `metric - time()`. Series is omitted when no reset is scheduled. |
| `loomwatch_credits_balance` | `provider`, `account_id`, `unit` | Remaining credit balance. `unit` is `usd` (OpenRouter), `credits` (Codex), or `prompt_credits` (Antigravity). |
| `loomwatch_agent_healthy` | `provider`, `account_id` | `1` if the polling agent has recent successful data (within `2 * pollInterval`), `0` if stale. Reflects **poll freshness**, not real OAuth validity. Series is omitted until the provider has produced at least one snapshot, which prevents startup false-positives. |
| `loomwatch_agent_last_cycle_age_seconds` | `provider`, `account_id` | Seconds since the last successful poll cycle. Companion to `loomwatch_agent_healthy`. |
| `loomwatch_build_info` | `version`, `go_version`, `commit` | Always `1`. Use for pinning alerts to a specific release. |
| `loomwatch_account_info` | `provider`, `account_id`, `account_name` | Join-metric (always `1`) mapping numeric `account_id` to human-readable `account_name`. See "Joining on account_name" below. |
| `loomwatch_api_integration_requests` | `integration` | Number of ingested API-integration usage events currently in the local DB, per integration. Not named `_total` because it's a DB snapshot, not an event-stream counter. |
| `loomwatch_api_integration_spend_usd` | `integration` | Cumulative USD spend tracked by API-integration ingestion (from the local DB). |

### Counters

Counters live outside the per-scrape reset path, so `rate()` / `increase()` queries work correctly.

| Metric | Labels | Description |
|---|---|---|
| `loomwatch_cycles_completed_total` | `provider`, `account_id` | Successful poll cycles. Use `rate(...)` for polling activity. |
| `loomwatch_cycles_failed_total` | `provider`, `account_id`, `reason` | Failed poll cycles, labelled by reason. |
| `loomwatch_scrape_errors_total` | `provider`, `error_type` | Errors while refreshing `/metrics` from the local store. Alert on `rate(...)` to detect broken metric collection itself. |

> Note: only the built-in `synthetic` agent emits `cycles_completed_total` / `cycles_failed_total`. Per-provider wiring for the remaining agents is a follow-up; alerts that depend on these counters should gate on `absent_over_time(loomwatch_cycles_completed_total{provider="..."}[1h])`.
>
> This is separate from the quota series, which every provider publishes: the counters are about polling cycles, not about quotas.

### Label semantics

- `provider` - `anthropic`, `codex`, `copilot`, `zai`, `minimax`, `antigravity`, `gemini`, `openrouter`, `api_integrations`.
- `quota_type` - provider-specific quota identifier. For Gemini, Antigravity, and MiniMax this is the model ID (`gemini-2.5-pro`, etc.) so **cardinality grows as new models appear**; configure Prometheus retention accordingly.
  - **MiniMax weekly window:** plans that have one also export `weekly_<model>` (for example `weekly_general` or `weekly_MiniMax-M2`) alongside the rolling five-hour `<model>` series, each with its own `loomwatch_quota_reset_timestamp_seconds`. Accounts without a weekly window emit no `weekly_*` series rather than a flat zero. Select the weekly window with `quota_type=~"weekly_.*"` or exclude it with `quota_type!~"weekly_.*"`.
  - Note when writing rules: `quota_type!~"video"` does **not** exclude `weekly_video`, because PromQL label matchers are fully anchored. Use `quota_type!~".*video"` if the intent is "no video quota of any window".
  - **Z.ai quotas:** `tokens` and `time` exist whenever the provider declares the quota, not only after consumption begins. A plan that reports a percentage while leaving the numeric budget at zero still exports (upstream issue #112); a quota the plan does not have emits no series.
- `account_id` - numeric account ID for multi-account providers (Codex, MiniMax); `"default"` for single-account providers.
- `account_name` - human-readable account name from `loomwatch_account_info` (join-metric).
- `unit` - on `loomwatch_credits_balance` only: `usd` | `credits` | `prompt_credits`.

## Example PromQL

**Minutes until quota reset:**
```promql
(loomwatch_quota_reset_timestamp_seconds - time()) / 60
```

**Join numeric account_id with account_name for Grafana:**
```promql
loomwatch_quota_utilization_percent * on(provider, account_id) group_left(account_name) loomwatch_account_info
```

> Join on **both** labels. `account_id` is unique per provider, not globally:
> every single-account provider reports `account_id="default"`, so joining on
> `account_id` alone aborts the whole query with a many-to-one error rather
> than returning partial data.
>
> `loomwatch_account_info` covers every provider that reports a quota. It used
> to exist only for the three that keep account rows, which made this join
> answer for a subset and say nothing about the rest - an unmatched series is
> removed, not reported. Providers with no account rows appear with
> `account_name="default"`. `api_integrations` has no entry: it is the
> collector's own ingestion path, not a subscription anybody owns.

**Attribute a quota to the team that owns it:**
```promql
max by (provider, quota_type, account_id, team) (
  loomwatch_quota_utilization_percent
  * on (provider, account_id) group_left(team) loomwatch:account_team
)
```

`loomwatch:account_team` is a recording rule published by the Helm chart from
`metrics.prometheusRule.teams`, not a series the collector exports. Unlike
`loomwatch_account_info` it covers **every** account that reports at all: an
account absent from the mapping is published with `team="unassigned"` rather
than dropped, so the join above never loses a series and a forgotten account
stays visible. `LoomwatchAccountWithoutTeam` alerts on exactly that value.

Ownership lives in chart values rather than in the collector's database because
account rows exist for three providers only; the rest report under
`account_id="default"` and have no row for a column to live on. A mapping in
values also survives a volume that does not persist, and is versioned in git.

There is no meaningful `sum by (team)` here. Utilisation is a percentage of each
plan's own limit, so two accounts at 50% do not make 100% of anything - use
`max by (team)` or filter by team, not addition.

**Scrape-error rate:**
```promql
rate(loomwatch_scrape_errors_total[5m])
```

## Example Scrape Config

```yaml
# prometheus.yml
scrape_configs:
  - job_name: onwatch
    metrics_path: /metrics
    scheme: http
    static_configs:
      - targets: ["onwatch.internal:8080"]
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/loomwatch_token
```

## Example Alert Rules

```yaml
groups:
  - name: onwatch
    rules:
      # Suppress for 10m after process start to avoid startup false-positives.
      - alert: OnwatchAgentStale
        expr: |
          (loomwatch_agent_healthy == 0)
          and on() (time() - process_start_time_seconds{job="onwatch"} > 600)
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "onWatch {{ $labels.provider }}/{{ $labels.account_id }} stale"
          description: "No successful poll in over 2x the poll interval. Common cause: expired OAuth refresh token."

      - alert: OnwatchQuotaNearLimit
        expr: loomwatch_quota_utilization_percent >= 90
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.provider }} quota {{ $labels.quota_type }} at {{ $value | printf \"%.0f\" }}%"

      - alert: OnwatchQuotaExhausted
        expr: loomwatch_quota_utilization_percent >= 99
        for: 1m
        labels:
          severity: critical

      - alert: OnwatchMetricsCollectionBroken
        expr: rate(loomwatch_scrape_errors_total[10m]) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "onWatch cannot refresh metrics from its own store"

      - alert: OnwatchQuotaResetMissed
        # Fires if a reset timestamp in the past persists, suggesting the agent
        # didn't pick up the new cycle.
        expr: (loomwatch_quota_reset_timestamp_seconds - time()) < -900
        for: 10m
```

## Notes & limitations

- Metrics are refreshed on every scrape from the SQLite store. At typical 30-60s scrape intervals the cost is negligible.
- Most metrics are gauges that `Reset()` each scrape, so series for a provider disappear if it becomes unconfigured. Counters (`loomwatch_cycles_*_total`, `loomwatch_scrape_errors_total`) are preserved across scrapes.
- `loomwatch_agent_healthy` reflects poll freshness, not real OAuth validity. A transient network blip or onWatch restart will flip it to 0. For true OAuth-expiry alerting, watch logs or the `/api/*/health` dashboard endpoints.
- `account_id` is a numeric ID. Use `loomwatch_account_info` for Grafana panels that need human-readable labels.

## Related

- README: [Prometheus metrics endpoint (Beta)](../README.md)
- Issue thread: [onllm-dev/onWatch#61](https://github.com/onllm-dev/onWatch/issues/61)
