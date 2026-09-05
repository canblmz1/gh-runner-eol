package cli

import (
	"github.com/spf13/cobra"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
	"github.com/canblmz1/gh-runner-eol/internal/report"
	"github.com/canblmz1/gh-runner-eol/internal/scan"
)

func newScanCmd(g *globals) *cobra.Command {
	var failOn string
	cmd := &cobra.Command{
		Use:   "scan [PATH]",
		Short: "Find runner versions pinned in source and resolve their EOL dates",
		Long: `Scans Dockerfiles, ARC Helm values, Kubernetes manifests, Terraform, Packer
and shell scripts for pinned actions/runner versions.

With --org/--repo/--enterprise the pinned versions are resolved against
GitHub's EOL schedule. Without a scope, pins are listed but not assessed.`,
		Example: `  gh runner-eol scan .
  gh runner-eol scan ./infra --org acme --format sarif -o pins.sarif`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			threshold, err := failOnSeverity(failOn)
			if err != nil {
				return err
			}
			pins, err := scan.Dir(path)
			if err != nil {
				return err
			}

			assessments := map[string]eol.Assessment{}
			var errs []string
			scopeName := ""
			if g.hasScope() {
				scope, err := g.scopeFrom("")
				if err != nil {
					return err
				}
				scopeName = scope.String()
				client, err := g.newClient()
				if err != nil {
					return err
				}
				assessments, errs = g.assessVersions(cmd.Context(), client, scope, scan.Versions(pins))
			}

			r := report.Build("scan", scopeName, g.version, g.now(), g.warnDays, nil, assessments, path, pins)
			r.Errors = errs
			if !g.hasScope() {
				// Without a scope nothing was assessed; strip the synthetic
				// "unknown" assessments so output does not imply a lookup happened.
				for i := range r.Pinned {
					r.Pinned[i].Assessment = nil
				}
			}
			if err := g.emit(r); err != nil {
				return err
			}
			if g.hasScope() && r.MaxSeverity() >= threshold {
				return errPolicy
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&failOn, "fail-on", "overdue", "exit 1 when any pinned version is at least this severe: overdue, warning, unknown, none")
	return cmd
}
