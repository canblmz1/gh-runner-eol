package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
	"github.com/canblmz1/gh-runner-eol/internal/report"
)

func newCheckCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check VERSION [VERSION...]",
		Short: "Look up the end-of-life schedule for specific runner versions",
		Example: `  gh runner-eol check 2.334.0 2.337.0 --org acme
  gh runner-eol check v2.336.0 -R acme/api --format json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := g.requireScope()
			if err != nil {
				return err
			}
			client, err := g.newClient()
			if err != nil {
				return err
			}
			versions := make([]string, 0, len(args))
			for _, a := range args {
				versions = append(versions, strings.TrimPrefix(strings.TrimSpace(a), "v"))
			}
			assessments, errs := g.assessVersions(cmd.Context(), client, scope, versions)

			ordered := make([]eol.Assessment, 0, len(versions))
			for _, v := range versions {
				ordered = append(ordered, assessments[v])
			}

			switch g.format {
			case "json":
				enc := json.NewEncoder(g.stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"scope":       scope.String(),
					"checked_at":  g.now().UTC(),
					"warn_days":   g.warnDays,
					"assessments": ordered,
					"errors":      errs,
				})
			case "sarif":
				return fmt.Errorf("check does not support sarif; use audit")
			}

			worst := 0
			for _, a := range ordered {
				fmt.Fprintf(g.stdout, "  %-8s  v%-10s  %s\n", a.Status.Label(), a.Version, report.Describe(a))
				if a.Status.Severity() > worst {
					worst = a.Status.Severity()
				}
			}
			for _, e := range errs {
				fmt.Fprintf(g.stderr, "warning: %s\n", e)
			}
			if worst >= eol.StatusOverdue.Severity() {
				return errPolicy
			}
			return nil
		},
	}
	return cmd
}
