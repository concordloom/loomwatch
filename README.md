# loomwatch

**Prometheus exporter for prepaid LLM subscription quotas - per account, per
team, with alerts that name an owner.**

Exports how much of each provider plan is left, per account and per quota
window, so exhaustion is something you get told about days early rather than
something you discover when a job starts failing at three in the morning.

```console
helm install loomwatch oci://ghcr.io/concordloom/charts/loomwatch \
  --set auth.providers.MINIMAX_API_KEY=... \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true
```

→ **[Helm chart documentation](charts/loomwatch/README.md)** (193 parameters,
alerting rules, runbooks, dashboard)

---

## What this is

A fork of [onllm-dev/onWatch](https://github.com/onllm-dev/onWatch), which is a
genuinely good tool aimed at a developer's laptop: a menubar app, a local
dashboard, SQLite, self-update. Almost all of the code here is theirs, and the
provider integrations, ten-plus of them, are entirely their work. The full
feature list lives in [README.upstream.md](README.upstream.md), preserved
verbatim - it describes upstream's laptop product, and not all of it is present
here.

This fork points the same collector at a different target: a cluster, where the
interface that matters is `/metrics` and the consumer is Prometheus rather than
a human glancing at a menubar.

## What is different

**Ownership.** An alert saying a quota is at 92% does not say whose it is.
`metrics.prometheusRule.teams` maps accounts to the teams that pay for them and
publishes `loomwatch:account_team`, so alerts and dashboards can be routed and
filtered by owner. An account nobody mapped is published as `unassigned` and
alerted on, rather than quietly missing from every per-team view.

Ownership lives in chart values rather than in the collector because account
rows exist for three providers only; the rest report under `account_id`
`"default"` and have no row a column could hang on. Values reach every provider
that exports at all, survive a volume that does not persist, and are reviewed
in git rather than clicked into a panel.

**Alerting rules that are actually checked.** Five rules plus the ownership one,
each carrying a `runbook_url` that resolves to a real section. Every rule is
evaluated by `promtool` on every pull request, and so is every dashboard query -
before this fork, nothing had ever parsed either. A rule that cannot be
evaluated does not fire and does not complain, which is the failure mode the
rules themselves are written to avoid.

**A Helm chart** with a values schema that refuses configurations which come up
and then quietly fail to do their job - a team name that would break the rule
that carries it, an account id YAML would turn into `1e+06`, two entries
claiming the same account.

**The laptop product's surfaces are gone.** Self-update, the installers and the
marketing page came with the fork and could not work here: releases carry no
assets, the container is read-only and distroless, and the installers downloaded
upstream's binary. The unit of delivery is the image.

Everything else is upstream's, including every provider integration.

## Which providers reach `/metrics`

Upstream's dashboard tracks fifteen providers. Ten of them have a metrics export
path, and those are the ones this chart can alert on:

> Anthropic, Codex, Copilot, Z.ai, MiniMax, Antigravity, Gemini, OpenRouter,
> Moonshot, DeepSeek.

**Synthetic, Cursor, Kimi, Grok and OpenCode Go are collected and shown in the
dashboard, but export no metrics at all.** They have agents and trackers and no
`scrape` path, so no rule here can see them. This is upstream's shape rather
than something the fork removed, and it is written down here because a
monitoring tool that is vague about what it does not monitor is worse than one
that monitors less. Tracked as
[#19](https://github.com/concordloom/loomwatch/issues/19).

## Upstream surfaces that are still here

Two of upstream's laptop interfaces remain in the tree, deliberately: deleting
upstream files costs a conflict on every sync, and these cost nothing where
they sit.

The **menubar HTTP surface is compiled into the image, and answers only on
loopback**. `/menubar` and the `/api/menubar/*` routes are registered by the
ordinary build - no tags involved - but their handlers call `isLoopbackRequest`
and return 404 to anything else, so a deployment behind an ingress exposes none
of it. That is what makes it harmless here and useful on a laptop: the GNOME
extension under `gnome-extension/` is a real consumer, talking to a daemon on
localhost.

What is *not* built is the macOS tray application: those files are behind
`menubar && darwin` and never enter the image.

## Alerting

The chart ships five rules. Thresholds are shares of the plan's own limit rather
than guesses about what counts as a lot of tokens - the collector already
reports utilisation as a percentage, so 100 is the quota itself.

The burn rule answers a different question: *will this run out before the window
resets*. It is written as `current + slope × seconds_remaining` rather than with
`predict_linear`, because that function takes a scalar horizon and therefore
cannot use each series' own reset time - feeding it the reset vector is a parse
error, and the resulting rule never evaluates while still reporting healthy in
some UIs. It also refuses to predict across horizons much longer than its trend
window, where extrapolation is noise amplification rather than evidence.

Details and every parameter: [chart README](charts/loomwatch/README.md).

## Versioning

This fork has its own version line, recorded in `VERSION`, with the upstream
release it was cut from recorded separately in `UPSTREAM_VERSION`. Release tags
carry a `loom-v` prefix because a fork inherits the upstream tag namespace
wholesale - `v*` is both occupied and unsafe here.

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
