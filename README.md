# runner-eol

[![CI](https://github.com/canblmz1/gh-runner-eol/actions/workflows/ci.yml/badge.svg)](https://github.com/canblmz1/gh-runner-eol/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/canblmz1/gh-runner-eol?sort=semver)](https://github.com/canblmz1/gh-runner-eol/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Find the pinned runner image that will stop taking jobs — before GitHub does.

> **Why now.** GitHub resumes full runner-version enforcement on **September 25, 2026**
> (github.com and GitHub Enterprise Cloud). Before that, brownouts — outdated runners are
> rejected for a day at a time, then silently stop taking jobs:
>
> | Date | What happens to outdated runners |
> |---|---|
> | Sep 7 | cannot register |
> | **Sep 9** | cannot register **and do not execute jobs** |
> | Sep 11 | cannot register |
> | Sep 14 · 16 · 18 | cannot register and do not execute jobs |
> | **Sep 25** | full enforcement, permanently |
>
> On Sep 3, 2026 GitHub shipped the API that publishes the exact end-of-life date per runner
> version. This tool is the thinnest possible layer on top of it. Source: [GitHub changelog](https://github.blog/changelog/2026-06-12-github-actions-minimum-version-enforcement-timeline-for-self-hosted-runners/).

**Seeing `Runner version v2.xxx.0 is deprecated and cannot receive messages`, or jobs stuck in `Queued`?** If you run Actions Runner Controller, auto-update is off. The tag in `values.yaml` *is* the outage date. GitHub's runner list cannot see an image you have not scaled up yet.

```
gh extension install canblmz1/gh-runner-eol
gh runner-eol scan ./charts --org <your-org>     # pinned source + official EOL dates
gh runner-eol audit <your-org> --scan .          # also the runners already registered
```

The live-runner half is a join over two GitHub APIs. The part GitHub will not build is the
source scan: Dockerfiles, ARC Helm `repository:`/`tag:` pairs, `RUNNER_VERSION=`, tarball URLs.

Try it without a token on the bundled fixtures (output from this repo, 2026-09-05):

```
$ gh runner-eol scan ./testdata --no-color

runner-eol scan  2026-09-05 11:07 UTC

5 pinned refs in testdata

Pinned in source
  PINNED    Dockerfile:3                              ghcr.io/actions/actions-runner:2.335.0
  PINNED    Dockerfile:4                              RUNNER_VERSION=2.334.0
  PINNED    arc-values.yaml:10                        ghcr.io/actions/actions-runner:2.336.0
  PINNED    arc-values.yaml:18                        tag: "2.334.0"  # ghcr.io/actions/actions-runner
  FLOAT     arc-values.yaml:21                        summerwind/actions-runner-dind:latest  (always newest; not reproducible)

Pass --org or --repo to turn PINNED into overdue / N days left.
```

A live lookup, no fleet required:

```
$ gh runner-eol check 2.334.0 2.336.0 -R <any-repo-you-can-read>
  OVERDUE   v2.334.0     runtime support ended 2026-08-10
  OK        v2.336.0     runtime support ends 2026-11-05
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
| Helm split repo / tag | `repository: ghcr.io/actions/actions-runner` then `tag: "2.336.0"` within 12 lines |
| ARC legacy image | `summerwind/actions-runner-dind:v2.331.0` |
| Release tarball / download URL | `actions-runner-linux-x64-2.333.0.tar.gz`, `releases/download/v2.333.0/` |
| Version variables | `ARG RUNNER_VERSION=`, `RUNNER_VERSION:`, `runnerVersion:` |
| Terraform / Packer | `variable "runner_version" { default = "2.336.0" }` (single-line) |
| Ansible | `github_runner_version: 2.336.0` |
| Chef | `default['github_runner']['version'] = '2.336.0'` |
| Floating tags | `:latest` on the image or on `tag:` — informational |

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
      - uses: github/codeql-action/upload-sarif@v4
        if: always()
        with:
          sarif_file: ${{ steps.eol.outputs.report }}
```

Pinned-source findings carry file and line, so they annotate the exact line in the PR.
Live-runner findings appear as logical locations in the code scanning tab.

`uses: canblmz1/gh-runner-eol@v0` builds the binary from **that tag's source** (not
`gh extension install …@latest`). The installed CLI matches the Action commit.

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

The docs say "update within 30 days of a release." The API returns a different, version-specific
window (recent versions have been on the order of two months, not thirty days). Do not compute
EOL from release dates. This tool asks GitHub.

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
