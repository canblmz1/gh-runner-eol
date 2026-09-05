# Security

runner-eol only performs read-only GitHub API calls (`GET .../actions/runners`,
`GET .../actions/runners/deprecations/{version}`) and reads local files you point it at.
It never modifies runners, never writes to your repository, and never sends data anywhere
other than api.github.com.

The token it uses needs only **Self-hosted runners: read**. Do not grant it more.

The GitHub Action, by default, compiles the same commit you `uses:` (not a floating
`latest` release). Override with `version: vX.Y.Z` only if you intentionally want a
different binary than the Action tree.

To report a vulnerability, open a private security advisory on this repository
(Security → Advisories → Report a vulnerability). Please do not file public issues for
security reports.
