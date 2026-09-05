# runner-eol

[![CI](https://github.com/canblmz1/gh-runner-eol/actions/workflows/ci.yml/badge.svg)](https://github.com/canblmz1/gh-runner-eol/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/canblmz1/gh-runner-eol?sort=semver)](https://github.com/canblmz1/gh-runner-eol/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Early warning for GitHub self-hosted runners that are about to stop receiving jobs.

> **Why now.** GitHub resumes full runner-version enforcement on **September 25, 2026**
> (github.com and GitHub Enterprise Cloud), with runtime brownouts on Sep 7–18 where outdated
> runners silently stop taking jobs. On Sep 3, 2026 GitHub shipped the API that publishes the
> exact end-of-life date per runner version. This tool is the thinnest possible layer on top of it.

GitHub retires runner versions on its own schedule. Runners that fall behind are rejected with
`Runner version vX.Y.Z is deprecated and cannot receive messages` and every job targeting them
sits in the queue forever. Actions Runner Controller disables self-update, so a pinned image
tag is a scheduled outage — you just don't know the date.

Since September 2026 GitHub publishes that date. `runner-eol` reconciles three things GitHub
keeps apart and tells you what breaks, and when:

```
live self-hosted runners       GET .../actions/runners                (reports each runner's version)
versions pinned in source      Dockerfile · ARC Helm values · scripts (the runners you haven't scaled up yet)
GitHub's official EOL dates    GET .../actions/runners/deprecations/{version}
```

```
$ gh runner-eol audit acme --scan .

runner-eol audit acme  2026-09-05 07:50 UTC

43 self-hosted runners · 3 versions · 4 pinned refs in .

  OVERDUE   18 runners   v2.334.0     runtime support ended 2026-08-10   26 days overdue
  WARNING   12 runners   v2.336.0     runtime support ends 2026-09-15   10 days left
  OK        13 runners   v2.337.0     no end-of-life scheduled

Pinned in source
  OVERDUE   Dockerfile:1                  ghcr.io/actions/actions-runner:2.335.0   runtime support ended 2026-08-14   22 days overdue
  WARNING   deploy/values.yaml:5          ghcr.io/actions/actions-runner:2.336.0   runtime support ends 2026-09-15   10 days left
  OK        deploy/values.yaml:12         RUNNER_VERSION=2.337.0   no end-of-life scheduled
  FLOAT     deploy/legacy.yaml:7          summerwind/actions-runner:latest  (always newest; not reproducible)

Runners: 18 overdue · 12 warning · 13 healthy · 2 pinned refs at risk
```

Exit code `1` when anything crosses `--fail-on` (default: `overdue`), `2` on tool errors.

## Install

As a `gh` extension (recommended):

```sh
gh extension install canblmz1/gh-runner-eol
gh runner-eol audit my-org
```

From source:

```sh
go install github.com/canblmz1/gh-runner-eol@latest
```

Authentication is whatever `gh` already has. `GH_TOKEN` / `GITHUB_TOKEN` override it.

Try it without any auth on the bundled fixture:

```sh
gh runner-eol scan ./testdata
```

## Commands

```
runner-eol audit [ORG | OWNER/REPO | enterprise:SLUG]   live fleet (+ --scan DIR for pinned source)
runner-eol scan  [PATH]                                 pinned source only; add --org/--repo to resolve EOL dates
runner-eol check VERSION...                              EOL schedule for specific versions
```

Common flags: `--format table|json|sarif`, `--output FILE`, `--warn-days N` (default 14),
`--fail-on overdue|warning|unknown|none`, `--no-color`.

### What `scan` detects

| Pattern | Example |
| --- | --- |
| Official image | `ghcr.io/actions/actions-runner:2.336.0` |
| ARC legacy image | `summerwind/actions-runner-dind:v2.331.0` |
| Release tarball / download URL | `actions-runner-linux-x64-2.333.0.tar.gz`, `releases/download/v2.333.0/` |
| Version variables | `ARG RUNNER_VERSION=`, `RUNNER_VERSION:`, `runnerVersion:` (ARC Helm) |
| Floating tags | `:latest` — reported as informational |

`.git`, `node_modules`, `vendor`, `.terraform`, `dist`, `build` are skipped; binaries and files over 2 MiB are ignored.

## GitHub Action

Run weekly and surface findings in code scanning / PR checks:

```yaml
on:
  schedule: [{ cron: "0 7 * * 1" }]
  pull_request:
    paths: ["**/Dockerfile*", "**/values*.yaml", "**/*.tf", "**/*.pkr.hcl"]

permissions:
  contents: read
  security-events: write

jobs:
  runner-eol:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: canblmz1/gh-runner-eol@v0
        id: eol
        with:
          target: my-org
          token: ${{ secrets.RUNNER_EOL_TOKEN }}   # fine-grained PAT or App token: Self-hosted runners: read
          scan: .
          fail-on: warning
      - uses: github/codeql-action/upload-sarif@v3
        if: always()
        with:
          sarif_file: ${{ steps.eol.outputs.report }}
```

Pinned-source findings carry file and line, so they annotate the exact line in the PR.
Live-runner findings appear as logical locations in the code scanning tab.

No token that can read runners yet? Use `mode: scan` — it only needs the checkout and still
resolves EOL dates when the token allows (otherwise pins are reported as `unknown`).

## Permissions

| Scope | Endpoint | Fine-grained token permission | Classic scope |
| --- | --- | --- | --- |
| Organization | `/orgs/{org}/actions/runners[/deprecations/{v}]` | Organization → Self-hosted runners: **read** | `admin:org` |
| Repository | `/repos/{o}/{r}/actions/runners[/deprecations/{v}]` | Repository → Administration: **read** | `repo` |
| Enterprise | `/enterprises/{e}/actions/runners[/deprecations/{v}]` | Enterprise → Self-hosted runners: read | `manage_runners:enterprise` |

The default `GITHUB_TOKEN` in a workflow cannot read organization-level runners; use a
fine-grained PAT or a GitHub App installation token.

## How the risk model works

For every distinct version (live or pinned) one call to the deprecation endpoint yields
`runtime_deprecates_at`. Then:

| Status | Condition | Exit impact |
| --- | --- | --- |
| `OVERDUE` | `runtime_deprecates_at` is in the past — GitHub no longer queues jobs | fails at `--fail-on overdue` |
| `WARNING` | ends within `--warn-days` | fails at `--fail-on warning` |
| `OK` | ends later, or no date scheduled yet (current release) | never |
| `UNKNOWN` | runner reported no version, or API returned 404 (version too old to be tracked) | fails at `--fail-on unknown` |

Observed behaviour worth knowing: the dates GitHub publishes are **not** "30 days after the
next release" as the docs imply — in practice they land 63–71 days after the following
release, and the exact day varies. That is why this tool asks GitHub instead of guessing.

## Scope

Applies to github.com and GitHub Enterprise Cloud (including data residency). GitHub Enterprise
Server does not enforce runner deprecation and does not expose the deprecation endpoint.

## Development

```sh
go test ./...
go build -o gh-runner-eol .
./gh-runner-eol check 2.334.0 -R owner/repo
```

Releases are built by `cli/gh-extension-precompile` on `v*` tags.

## License

MIT
