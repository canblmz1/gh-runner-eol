// Package cli wires the commands together. All I/O is injected so the whole
// binary can be exercised from tests.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
	"github.com/canblmz1/gh-runner-eol/internal/gh"
	"github.com/canblmz1/gh-runner-eol/internal/report"
)

// Exit codes. 1 is reserved for policy violations so CI can distinguish
// "your fleet is at risk" from "the tool broke".
const (
	ExitOK       = 0
	ExitFindings = 1
	ExitError    = 2
)

// errPolicy signals that findings crossed the --fail-on threshold.
var errPolicy = errors.New("policy threshold reached")

type globals struct {
	org        string
	repo       string
	enterprise string
	format     string
	output     string
	warnDays   int
	noColor    bool

	version   string
	stdout    io.Writer
	stderr    io.Writer
	now       func() time.Time
	newClient func() (*gh.Client, error)
}

// Execute runs the CLI and returns the process exit code.
func Execute(version string, args []string, stdout, stderr io.Writer) int {
	g := &globals{
		version:   version,
		stdout:    stdout,
		stderr:    stderr,
		now:       time.Now,
		newClient: gh.NewDefaultClient,
	}
	root := newRoot(g)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.ExecuteContext(context.Background())
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, errPolicy):
		return ExitFindings
	default:
		fmt.Fprintf(stderr, "runner-eol: %v\n", err)
		return ExitError
	}
}

func newRoot(g *globals) *cobra.Command {
	root := &cobra.Command{
		Use:   "runner-eol",
		Short: "Early warning for GitHub self-hosted runner end-of-life",
		Long: `runner-eol reconciles three things GitHub keeps apart:

  live self-hosted runners        (GET .../actions/runners)
  the versions pinned in source   (Dockerfile, ARC Helm values, scripts)
  GitHub's official EOL schedule  (GET .../actions/runners/deprecations/{version})

and tells you which runners will stop receiving jobs, and when.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       g.version,
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.org, "org", "", "organization login")
	pf.StringVarP(&g.repo, "repo", "R", "", "repository in OWNER/NAME form")
	pf.StringVar(&g.enterprise, "enterprise", "", "enterprise slug (GHEC)")
	pf.StringVarP(&g.format, "format", "f", "table", "output format: table, json, sarif")
	pf.StringVarP(&g.output, "output", "o", "", "write output to file instead of stdout")
	pf.IntVar(&g.warnDays, "warn-days", 14, "flag versions whose runtime support ends within N days")
	pf.BoolVar(&g.noColor, "no-color", false, "disable ANSI colors")

	root.AddCommand(newAuditCmd(g), newCheckCmd(g), newScanCmd(g))
	return root
}

// scopeFrom resolves the target from a positional argument or flags.
func (g *globals) scopeFrom(positional string) (gh.Scope, error) {
	set := 0
	for _, v := range []string{positional, g.org, g.repo, g.enterprise} {
		if v != "" {
			set++
		}
	}
	if set == 0 {
		return gh.Scope{}, errors.New("no target: pass ORG, OWNER/REPO or enterprise:SLUG, or use --org/--repo/--enterprise")
	}
	if set > 1 {
		return gh.Scope{}, errors.New("specify exactly one of a positional target, --org, --repo, --enterprise")
	}
	switch {
	case positional != "":
		return gh.ParseTarget(positional)
	case g.org != "":
		return gh.Scope{Kind: gh.ScopeOrg, Name: g.org}, nil
	case g.enterprise != "":
		return gh.Scope{Kind: gh.ScopeEnterprise, Name: g.enterprise}, nil
	default:
		return gh.ParseTarget(g.repo)
	}
}

func (g *globals) hasScope() bool {
	return g.org != "" || g.repo != "" || g.enterprise != ""
}

// assessVersions resolves the EOL schedule for each distinct version. API
// failures for a single version degrade that version to "unknown" and are
// surfaced in the report rather than aborting the run.
func (g *globals) assessVersions(ctx context.Context, c *gh.Client, scope gh.Scope, versions []string) (map[string]eol.Assessment, []string) {
	now := g.now()
	out := make(map[string]eol.Assessment, len(versions))
	var errs []string
	for _, v := range versions {
		if v == "" {
			out[v] = eol.Assess(eol.Schedule{}, now, g.warnDays)
			continue
		}
		s, err := c.GetDeprecation(ctx, scope, v)
		if err != nil {
			errs = append(errs, err.Error())
			s = eol.Schedule{Version: v}
		}
		out[v] = eol.Assess(s, now, g.warnDays)
	}
	return out, errs
}

// emit writes the report in the requested format.
func (g *globals) emit(r *report.Report) error {
	w := g.stdout
	toFile := g.output != ""
	if toFile {
		f, err := os.Create(g.output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	switch g.format {
	case "json":
		return report.WriteJSON(w, r)
	case "sarif":
		return report.WriteSARIF(w, r)
	case "table", "":
		color := !g.noColor && !toFile && colorEnabled(g.stdout)
		report.WriteTable(w, r, report.TableOptions{Color: color})
		return nil
	default:
		return fmt.Errorf("unknown format %q (want table, json or sarif)", g.format)
	}
}

func colorEnabled(w io.Writer) bool {
	if w != os.Stdout {
		return false
	}
	return term.FromEnv().IsColorEnabled()
}

// failOnSeverity maps the --fail-on flag to a severity threshold.
func failOnSeverity(flag string) (int, error) {
	switch flag {
	case "none":
		return 100, nil
	case "overdue", "critical":
		return eol.StatusOverdue.Severity(), nil
	case "warning", "warn":
		return eol.StatusWarning.Severity(), nil
	case "unknown":
		return eol.StatusUnknown.Severity(), nil
	}
	return 0, fmt.Errorf("unknown --fail-on %q (want overdue, warning, unknown or none)", flag)
}
