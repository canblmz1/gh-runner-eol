package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/canblmz1/gh-runner-eol/internal/gh"
	"github.com/canblmz1/gh-runner-eol/internal/report"
	"github.com/canblmz1/gh-runner-eol/internal/scan"
)

func newAuditCmd(g *globals) *cobra.Command {
	var (
		scanPath string
		failOn   string
	)
	cmd := &cobra.Command{
		Use:   "audit [ORG | OWNER/REPO | enterprise:SLUG]",
		Short: "Audit live self-hosted runners (and optionally pinned source) against GitHub's EOL schedule",
		Example: `  gh runner-eol audit acme
  gh runner-eol audit acme/api --scan . --format sarif -o runner-eol.sarif
  gh runner-eol audit --enterprise acme-inc --warn-days 30 --fail-on warning`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			positional := ""
			if len(args) == 1 {
				positional = args[0]
			}
			scope, err := g.scopeFrom(positional)
			if err != nil {
				return err
			}
			threshold, err := failOnSeverity(failOn)
			if err != nil {
				return err
			}

			client, err := g.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			runners, err := client.ListRunners(ctx, scope)
			if err != nil {
				return err
			}

			var pins []scan.Finding
			if scanPath != "" {
				pins, err = scan.Dir(scanPath)
				if err != nil {
					return err
				}
			}

			versions := union(gh.UniqueVersions(runners), scan.Versions(pins))
			assessments, errs := g.assessVersions(ctx, client, scope, versions)

			r := report.Build("audit", scope.String(), g.version, g.now(), g.warnDays, runners, assessments, scanPath, pins)
			r.Errors = errs
			if err := g.emit(r); err != nil {
				return err
			}
			if r.MaxSeverity() >= threshold {
				return errPolicy
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scanPath, "scan", "", "also scan this directory for pinned runner versions (Dockerfile, Helm values, scripts)")
	cmd.Flags().StringVar(&failOn, "fail-on", "overdue", "exit 1 when any finding is at least this severe: overdue, warning, unknown, none")
	return cmd
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, xs := range [][]string{a, b} {
		for _, x := range xs {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	return out
}

// requireScope is shared by commands where the scope only comes from flags.
func (g *globals) requireScope() (gh.Scope, error) {
	if !g.hasScope() {
		return gh.Scope{}, fmt.Errorf("the deprecation endpoint is scoped: pass --org, --repo or --enterprise")
	}
	return g.scopeFrom("")
}
