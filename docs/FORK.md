# Maintaining this fork

This fork is deliberately thin, and keeping it thin *is* the maintenance
strategy: every file that diverges from upstream is a file someone has to
reconcile by hand, forever. Before adding a change here, ask whether it can live
in the chart instead — charts are new files and never conflict.

## Syncing with upstream

```console
git remote add upstream https://github.com/onllm-dev/onWatch.git   # once
git fetch upstream --no-tags
git merge upstream/main
```

**`--no-tags` is not optional.** A fork inherits the upstream tag namespace
wholesale — this repository already carries 90 upstream tags, `v1.0.0` among
them. Release tags here use a `loom-v` prefix so the release workflow can never
be triggered by an upstream tag, but fetching upstream tags into this remote
would still clutter the namespace and confuse `git describe`.

After merging, bump `UPSTREAM_VERSION` to the upstream release you landed on,
and re-run the checks below.

## Files that diverge, and why

Everything under `charts/`, both `.github/workflows/fork-*.yml`, `UPSTREAM_VERSION`
and the two `*_test.go` files listed below are **new files**. They never
conflict. The list that matters is the one that touches upstream code:

| File | What changed | If the merge conflicts |
|---|---|---|
| `internal/metrics/metrics.go` | `scrapeMiniMax` also emits the weekly window as `weekly_<model>`; `scrapeZai` gates on the quota being declared rather than consumed | Both changes are marked in comments. Keep upstream's structure and re-apply the two blocks. |
| `internal/update/update.go` | `githubRepoSlug` and `releaseTagPrefix` constants, `normalizeTag` | Take upstream's version wholesale, then re-point the slug and re-add `normalizeTag`. Do not hand-merge: the retry/fallback logic around it changes often upstream. |
| `docs/PROMETHEUS_METRICS.md` | A note under "Label semantics" about weekly quotas and anchored matchers | Prefer upstream's text and re-append the note. |
| `VERSION` | This fork's own version line | Always keep ours. Upstream's number goes into `UPSTREAM_VERSION`. |
| `.gitignore` | One negation so the chart icon is not swallowed by `*.png` | Keep both. |
| `README.md` | Replaced; upstream's is preserved verbatim as `README.upstream.md` | Take upstream's into `README.upstream.md`, leave ours alone. |

Tests added by the fork (`internal/metrics/quota_coverage_test.go`,
`internal/update/fork_tag_test.go`) are new files and exist precisely so a
botched re-apply is loud rather than silent. If they fail after a sync, the
re-apply was wrong — do not adjust the tests to match.

## Checks before releasing

```console
go test ./internal/...
gofmt -l internal/
helm lint charts/loomwatch
helm unittest charts/loomwatch
python3 charts/loomwatch/hack/gen-params.py --check
```

Note that upstream's root package and parts of `internal/config` and
`internal/store` have pre-existing test failures unrelated to this fork — some
of them simply because a container runs as root and the tests assert that a path
is *not* writable. Compare against a clean checkout of the upstream base before
concluding a sync broke something.

## Releasing

```console
# bump VERSION, and charts/loomwatch/Chart.yaml version + appVersion to match
git tag -a loom-v1.2.3 -m "..."
git push origin loom-v1.2.3
```

The workflow refuses to publish if `VERSION`, `Chart.yaml: version` and
`Chart.yaml: appVersion` disagree, or if the README parameter tables are stale.
It publishes the image and the chart to `ghcr.io` and creates a GitHub release.

New GHCR packages are created **private**. Both the image and the chart package
must be public for anonymous pulls; the organisation may also need public
packages permitted at the org level before the per-package setting becomes
available.
