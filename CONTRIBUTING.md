# Contributing

Thanks for helping keep runner fleets alive.

## The fastest way to help

**Send us a pinned-version pattern we miss.** Open an issue with the "Missed pin" template
and paste the line from your Dockerfile / Helm values / Terraform / Packer / Ansible that
`runner-eol scan` did not catch. Adding a rule is a one-line regex in
`internal/scan/scan.go` plus a test case — a good first PR.

Other welcome contributions:

- Real-world output from `runner-eol audit` on fleets we can't see (GHEC data residency,
  enterprise scope, ARC at scale). Redact names, keep versions and dates.
- Report formats: Slack / Teams webhooks, Prometheus textfile, Markdown for `$GITHUB_STEP_SUMMARY`.
- Remediation: open a PR that bumps the pinned tag to the newest non-EOL version.

## Ground rules

- `go test ./...` and `gofmt -l .` must be clean.
- The risk model in `internal/eol` is pure and fully unit-tested — keep it that way.
- The tool never guesses an EOL date. If GitHub's API does not return one, the status is
  `unknown`, not an estimate. Please don't add heuristics based on release age.
- No new dependencies without a reason in the PR description.

## Local loop

```sh
go build -o gh-runner-eol .
./gh-runner-eol check 2.334.0 -R owner/repo        # any repo you admin
./gh-runner-eol scan ./testdata                     # no auth needed
```

## Releasing (maintainers)

```sh
git tag v0.2.0 && git push --tags
git tag -f v0 v0.2.0 && git push -f origin v0       # moves the Action major tag
```

`cli/gh-extension-precompile` builds every platform and attaches binaries to the release.
