# loomwatch

**LLM subscription quota monitoring for servers.**

Exports how much of each provider plan is left — per account, per quota window —
so exhaustion is something you get told about, not something you discover when a
job starts failing.

```console
helm install loomwatch oci://ghcr.io/concordloom/charts/loomwatch \
  --set auth.providers.MINIMAX_API_KEY=... \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true
```

→ **[Helm chart documentation](charts/loomwatch/README.md)** (178 parameters,
alerting rules, dashboard)

---

## What this is

A fork of [onllm-dev/onWatch](https://github.com/onllm-dev/onWatch), which is a
genuinely good tool aimed at a developer's laptop: a menubar app, a local
dashboard, SQLite, self-update. Almost all of the code here is theirs, and the
provider integrations — ten-plus of them — are entirely their work. The full
feature list lives in [README.upstream.md](README.upstream.md).

This fork points the same collector at a different target: a cluster, where the
interface that matters is `/metrics` and the consumer is Prometheus rather than
a human glancing at a menubar.

## What is different

**The weekly quota window is exported.** Upstream reads it from its own store
but never publishes it, so an account could sit at 80% of its weekly budget with
every quota alert silent. It is now exported as `weekly_<model>` with its own
reset timestamp.

**A quota series exists when the provider declares the quota, not when it has
been consumed.** Upstream gated the Z.ai token series on usage being above zero,
which hid the series exactly while the account was still healthy — and hid it
permanently for plans that report a percentage while leaving usage at zero
([upstream #112](https://github.com/onllm-dev/onWatch/issues/112)). Zero
utilisation is now a real reading; an absent series means the plan has no such
quota.

**Self-update points at this repository**, so the service cannot offer to
replace itself with a build that lacks the above. Self-update is unsupported in
a container regardless: there the unit of delivery is the image.

**A Helm chart** with alerting rules, a Grafana dashboard and a values schema
that refuses configurations which come up and then quietly fail to do their job.

That is the whole delta. Everything else is upstream's.

## Alerting

The chart ships five rules. Thresholds are shares of the plan's own limit rather
than guesses about what counts as a lot of tokens — the collector already
reports utilisation as a percentage, so 100 is the quota itself.

The burn rule answers a different question: *will this run out before the window
resets*. It is written as `current + slope × seconds_remaining` rather than with
`predict_linear`, because that function takes a scalar horizon and therefore
cannot use each series' own reset time — feeding it the reset vector is a parse
error, and the resulting rule never evaluates while still reporting healthy in
some UIs. It also refuses to predict across horizons much longer than its trend
window, where extrapolation is noise amplification rather than evidence.

Details and every parameter: [chart README](charts/loomwatch/README.md).

## Versioning

This fork has its own version line, recorded in `VERSION`, with the upstream
release it was cut from recorded separately in `UPSTREAM_VERSION`. Release tags
carry a `loom-v` prefix because a fork inherits the upstream tag namespace
wholesale — `v*` is both occupied and unsafe here.

A derived scheme such as `2.13.3-loom.1` was considered and rejected: under
SemVer a hyphen suffix denotes a pre-release, so it sorts *below* the release it
is built on, and anything comparing versions reads the fork as older.

## Contributing, and syncing with upstream

The fork is deliberately thin, and keeping it thin is the maintenance strategy.
See [docs/FORK.md](docs/FORK.md) for exactly which files diverge and how to
re-apply them when pulling upstream changes.

## License

GPL-3.0-only, inherited from upstream. This fork carries the same license and
the same copyright; modifications are marked in the files that carry them.
