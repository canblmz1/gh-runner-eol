# Changelog

Notable changes to `gh-runner-eol` are documented here. Release tags remain the source of truth for shipped code.

## [v0.1.5] - 2026-09-07

### Added

- Detect Terraform/Packer `variable "runner_version" { default = "…" }` pins.
- Detect Ansible-style GitHub runner version variables.
- Detect Chef runner-version attributes.

### Community

- Includes substantive external contributions merged through PRs #6 and #7.

## [v0.1.4] - 2026-09-05

### Fixed

- Install the checkout-built Action binary directly into the GitHub CLI extension directory instead of attempting `gh extension install` from a local path.

## [v0.1.3] - 2026-09-05

### Changed

- The GitHub Action now builds the exact invoked commit instead of installing a floating latest extension binary.
- Release version information is injected into built binaries.

## [v0.1.2] - 2026-09-05

### Added

- Detect ARC/Helm image pins where `repository:` and `tag:` are split across separate lines.
- Document GitHub's runner-enforcement brownout dates and the deprecation error users are likely to search for.

## [v0.1.1] - 2026-09-05

### Added

- GitHub Action scan mode with a default token path.
- SARIF findings for pins whose EOL date cannot be resolved.

### Changed

- Release workflow only publishes full semantic-version tags.
- Code scanning examples use CodeQL Action v4.
- Documentation paths are skipped during source scanning.

## [v0.1.0] - 2026-09-05

Initial public release.

- Audit live self-hosted runner fleets against GitHub's runner EOL schedule.
- Scan pinned runner versions in source.
- Support table, JSON, and SARIF output.
- Provide a GitHub CLI extension and GitHub Action.
- Add CI, release automation, tests, fixtures, contribution guidance, security policy, and issue templates.

[v0.1.5]: https://github.com/canblmz1/gh-runner-eol/compare/v0.1.4...v0.1.5
[v0.1.4]: https://github.com/canblmz1/gh-runner-eol/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/canblmz1/gh-runner-eol/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/canblmz1/gh-runner-eol/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/canblmz1/gh-runner-eol/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/canblmz1/gh-runner-eol/releases/tag/v0.1.0
