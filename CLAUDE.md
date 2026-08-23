# loomWatch

Go CLI for AI quota tracking. Polls 15 providers → SQLite → operator dashboard.

**Before changing anything under `internal/web/`, read [DESIGN.md](DESIGN.md)
first.** It covers the palette, density, states, motion rules and prohibitions.
The source of truth for styles is `internal/web/static/style.css`; DESIGN.md
describes that file, not an aspiration. The former
`design-system/onwatch/MASTER.md` is deleted: it described Material Design 3,
which is not in the code, and an agent following it built somebody else's
interface.

## Task

Background daemon (<50MB RAM) tracking: Anthropic, Synthetic, Z.ai, Copilot, Codex, MiniMax, Antigravity, Gemini, Cursor, Kimi Code, Grok, Moonshot, DeepSeek, OpenRouter, OpenCode Go.

## Code Map

```
main.go                     # CLI entry, daemon lifecycle
internal/
├── api/                    # HTTP clients + types per provider
│   └── {provider}_client.go, {provider}_types.go
├── store/                  # SQLite persistence per provider
│   └── store.go (schema), {provider}_store.go
├── tracker/                # Poll orchestration per provider
├── agent/                  # Background polling agents
├── web/                    # Dashboard server
│   ├── handlers.go         # API endpoints
│   ├── static/             # Embedded JS/CSS (embed.FS)
│   └── templates/          # HTML templates
├── config/                 # Config + container detection
└── notify/                 # Email + push notifications
```

## Objectives

1. **TDD-first**: Test → fail → implement → pass
2. **RAM-bounded**: 40MB limit, single SQLite conn, lean HTTP
3. **Single binary**: All assets via `embed.FS`

## Operations

**Always use `app.sh` for build and test - never run `go build` or `go test` directly.**

```bash
./app.sh --build            # Build production binary
./app.sh --test             # Run all tests with race detection and coverage
./app.sh --smoke            # Quick validation: vet + build check + short tests
go test -race ./... && go vet ./...   # Pre-commit (mandatory)
```

**Nix (alternative to app.sh, for build + dev shell):**

```bash
nix build .#onwatch         # Pure-Go static binary -> ./result/bin/onwatch
nix run .#onwatch           # Build + run
nix develop                 # Shell with go/gopls/gotools/gofumpt (or `direnv allow`)
```

On `go.sum` changes, update `vendorHash` in `flake.nix` (run `nix build .#onwatch`, paste the `got: sha256-...` from the error). Flake tracks `nixos-unstable` because `go.mod` requires Go >= 1.25.7.

## Verification cycle

Gopnik is installed in this repository, under `.claude/skills/`, and what it
checks is recorded in `gopnik.json`: Stage 1 is what runs here, Stage 2 is what
proves a change works on the deployed instance. Neither is worth rediscovering
per task - read the file.

Every task runs the same four steps:

1. **The statement.** Before work starts, `gopnik-critic` attacks the wording
   and the completion criteria. A task nobody can fail is a task nobody can
   finish.
2. **The approach.** Before code exists, `gopnik-critic` attacks the chosen
   solution. This is the cheapest slot in the cycle: refuting a plan costs
   minutes, refuting a finished implementation costs a rewrite.
3. **The implementation.**
4. **The gate.** `gopnik` attacks the completed change, both stages, before
   anything is called done. Its verdict - `READY` or `NOT READY` - goes into
   the pull request with the evidence behind it. A `BLOCKER` fix voids the
   verdict: what shipped is then not what was checked, so the next round is a
   new round.

The two skills are not interchangeable. The gate examines the **work**; the
critic examines **what is said about** the work. So the critic also runs
outside those slots, whenever a change carries a claim - "the cause is X",
"this is not our bug", "the class is closed", "this will not happen again".
`gopnik` will prove the change works and will not touch the diagnosis it was
built on.

Stage 2 talks to the deployed instance through a port-forward, not a public
address: `scripts/check-deployed.sh` opens one, proves the deployed revision
from the login page and from `onwatch_build_info`, runs the browser check with
its own negative case, and tears it down. Its address and credentials are
machine-local - they live in `gopnik.local.env`, which `.gitignore` keeps out of
commits. Create one from the variables that script documents. Nothing about a
particular deployment belongs in this repository.

## Guardrails

| Rule | Reason |
|------|--------|
| Never commit `.env`, `.db`, binaries | Security |
| Never log API keys | Security |
| Parameterized SQL only | Injection prevention |
| `context.Context` always | Leak prevention |
| `-race` before commit | Data race detection |
| `subtle.ConstantTimeCompare` for creds | Timing attacks |
| Bounded queries (cycles≤200, insights≤50) | Memory caps |

## Notes

**Adding a provider:**
1. `internal/api/{provider}_client.go` + `_types.go`
2. `internal/store/{provider}_store.go`
3. `internal/tracker/{provider}_tracker.go`
4. `internal/agent/{provider}_agent.go`
5. Add to `internal/web/handlers.go` endpoints
6. Update dashboard JS in `internal/web/static/app.js`

**API Docs:** See `docs/` for provider-specific setup (COPILOT_SETUP.md, CODEX_SETUP.md, ANTIGRAVITY_SETUP.md, GEMINI_SETUP.md, CURSOR_SETUP.md, KIMI_SETUP.md, GROK_SETUP.md, MOONSHOT_SETUP.md, DEEPSEEK_SETUP.md, OPENCODE_SETUP.md)

**Containers:** `IsDockerEnvironment()` in `config.go` detects Docker/K8s. Containers run foreground only.

**Release process:**

This fork releases itself, and the steps below are the only ones that apply.
Upstream's process does not: it bumps version badges this README does not have,
tags in the `v*` namespace, and dispatches
`.github/workflows/release.yml`, which is upstream's workflow, has no fork
guard, and would publish upstream's code under this fork's name.

1. Bump the version in all three places, which must agree:
   - `VERSION` (the single source of truth, e.g. `1.7.1`)
   - `charts/loomwatch/Chart.yaml`: `version` and `appVersion`
   - `charts/loomwatch/Chart.yaml`: the tag inside the
     `artifacthub.io/images` annotation. It is easy to miss because it sits
     above `appVersion` in the file and repeats it in a different shape;
     `.github/workflows/fork-chart.yml` fails the build when the two disagree.
2. Land it on `main` through a pull request - `main` is protected.
3. Tag the merge commit and push the tag:
   `git tag loom-v1.7.1 && git push origin loom-v1.7.1`

Pushing a `loom-v*` tag is the whole trigger. `.github/workflows/fork-release.yml`
checks that the tag, `VERSION` and the chart all agree, lints and unit-tests the
chart, then publishes the container image and the chart to `ghcr.io` and creates
the GitHub release. A fork release carries no binaries: the unit of delivery is
the image.

The `loom-v` prefix is not decoration. `v*` belongs to the 91 upstream tags this
repository inherited, so an upstream tag arriving through a sync must never be
able to trigger a release here.

Do not run `./app.sh --release` locally: it cross-compiles five platforms' worth
of binaries this fork does not ship, and a release has to come out of the
pipeline rather than off a workstation.

**Anthropic Rate Limit Bypass:** Anthropic's usage API has aggressive rate limits (~5 requests per token, then 429 for ~5 min). loomWatch bypasses this by refreshing the OAuth token when rate limited - each new access token gets a fresh rate limit window. Implementation details:
- `internal/agent/anthropic_agent.go`: Detects 429, calls `RefreshAnthropicToken`, saves new tokens, retries
- `internal/api/anthropic_oauth.go`: OAuth token refresh endpoint (`console.anthropic.com/v1/oauth/token`)
- `internal/api/anthropic_token_unix.go`: Writes to macOS Keychain + file for persistence
- `internal/api/anthropic_token_windows.go`: Writes to credentials file
- Refresh tokens are one-time use (OAuth rotation) - MUST save new refresh token after each refresh
- See: [issue #16](https://github.com/onllm-dev/onWatch/issues/16), [anthropics/claude-code#31021](https://github.com/anthropics/claude-code/issues/31021)

## Style

- Use `-` (hyphen) instead of `—` (em dash) in all text
- **English only in the repository.** Commit messages, code comments, doc files,
  identifiers, log lines and UI strings are all English. This is an open-source
  product: contributors who cannot read Russian must be able to follow the
  history and the code. Chatting with the maintainer in Russian is fine -
  anything that lands in git is not. This applies to agents too: do not mirror
  the language of a request into the repository, and do not match surrounding
  text if you ever find non-English text there - report it instead.

  Enforced mechanically, because the written rule alone did not hold:
  `scripts/check-english.sh` scans tracked files and commit messages, the
  `.githooks/` commit-msg and pre-commit hooks run it locally (enable once per
  clone with `git config core.hooksPath .githooks`), and
  `.github/workflows/fork-english-only.yml` runs it on every push and pull
  request.
