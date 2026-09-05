package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
)

// TableOptions controls the human-readable renderer.
type TableOptions struct {
	Color bool
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiGray   = "\x1b[90m"
)

func paint(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + ansiReset
}

func statusColor(s eol.Status) string {
	switch s {
	case eol.StatusOverdue:
		return ansiRed
	case eol.StatusWarning:
		return ansiYellow
	case eol.StatusUnknown:
		return ansiGray
	default:
		return ansiGreen
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Describe is the one-line human explanation of an assessment.
func Describe(a eol.Assessment) string {
	switch a.Status {
	case eol.StatusOverdue:
		return fmt.Sprintf("runtime support ended %s   %s overdue",
			a.RuntimeDeprecatesAt.Format("2006-01-02"), plural(-*a.DaysLeft, "day", "days"))
	case eol.StatusWarning:
		return fmt.Sprintf("runtime support ends %s   %s left",
			a.RuntimeDeprecatesAt.Format("2006-01-02"), plural(*a.DaysLeft, "day", "days"))
	case eol.StatusHealthy:
		return fmt.Sprintf("runtime support ends %s   %s left",
			a.RuntimeDeprecatesAt.Format("2006-01-02"), plural(*a.DaysLeft, "day", "days"))
	case eol.StatusCurrent:
		return "no end-of-life scheduled"
	default:
		return a.Reason
	}
}

// WriteTable renders the report for terminals.
func WriteTable(w io.Writer, r *Report, opt TableOptions) {
	c := opt.Color
	title := "runner-eol " + r.Command
	if r.Scope != "" {
		title += " " + r.Scope
	}
	fmt.Fprintf(w, "%s  %s\n\n",
		paint(c, ansiBold, title),
		paint(c, ansiDim, r.GeneratedAt.Format("2006-01-02 15:04 UTC")))

	head := plural(r.Summary.Runners, "self-hosted runner", "self-hosted runners")
	if r.Command == "scan" {
		head = ""
	}
	if r.Summary.Versions > 0 {
		head += fmt.Sprintf(" · %s", plural(r.Summary.Versions, "version", "versions"))
	}
	if r.ScanPath != "" {
		sep := " · "
		if head == "" {
			sep = ""
		}
		head += fmt.Sprintf("%s%s in %s", sep, plural(r.Summary.PinnedTotal, "pinned ref", "pinned refs"), r.ScanPath)
	}
	fmt.Fprintln(w, head)

	if len(r.Groups) > 0 {
		fmt.Fprintln(w)
		for _, g := range r.Groups {
			v := g.Version
			if v == "" {
				v = "(no version reported)"
			} else {
				v = "v" + v
			}
			extra := ""
			if g.Online < g.Count {
				extra = paint(c, ansiDim, fmt.Sprintf("  (%d online)", g.Online))
			}
			if g.RegistrationBlocked {
				extra += paint(c, ansiDim, "  registration blocked")
			}
			fmt.Fprintf(w, "  %s  %-11s  %-12s %s%s\n",
				paint(c, statusColor(g.Status), fmt.Sprintf("%-8s", g.Status.Label())),
				plural(g.Count, "runner", "runners"),
				v,
				Describe(g.Assessment),
				extra)
		}
	}

	if len(r.Pinned) > 0 {
		fmt.Fprintf(w, "\n%s\n", paint(c, ansiBold, "Pinned in source"))
		for _, p := range r.Pinned {
			loc := fmt.Sprintf("%s:%d", p.Path, p.Line)
			switch {
			case p.Floating:
				fmt.Fprintf(w, "  %s  %-40s  %s\n",
					paint(c, ansiGray, fmt.Sprintf("%-8s", "FLOAT")), loc,
					p.Match+"  (always newest; not reproducible)")
			case p.Assessment != nil:
				fmt.Fprintf(w, "  %s  %-40s  %s   %s\n",
					paint(c, statusColor(p.Assessment.Status), fmt.Sprintf("%-8s", p.Assessment.Status.Label())),
					loc, p.Match, Describe(*p.Assessment))
			default:
				fmt.Fprintf(w, "  %s  %-40s  %s\n", fmt.Sprintf("%-8s", "PINNED"), loc, p.Match)
			}
		}
	}

	if len(r.Errors) > 0 {
		fmt.Fprintf(w, "\n%s\n", paint(c, ansiYellow, "Warnings"))
		for _, e := range r.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	if r.Command == "scan" {
		s := r.Summary
		fmt.Fprintf(w, "\nPinned refs: %d overdue · %d expiring · %d floating\n", s.PinnedOverdue, s.PinnedWarning, s.PinnedFloating)
		return
	}

	var parts []string
	s := r.Summary
	if s.Overdue > 0 {
		parts = append(parts, paint(c, ansiRed, fmt.Sprintf("%d overdue", s.Overdue)))
	}
	if s.Warning > 0 {
		parts = append(parts, paint(c, ansiYellow, fmt.Sprintf("%d warning", s.Warning)))
	}
	if s.Unknown > 0 {
		parts = append(parts, paint(c, ansiGray, fmt.Sprintf("%d unknown", s.Unknown)))
	}
	parts = append(parts, paint(c, ansiGreen, fmt.Sprintf("%d healthy", s.Healthy)))
	if s.PinnedOverdue+s.PinnedWarning > 0 {
		parts = append(parts, fmt.Sprintf("%d pinned refs at risk", s.PinnedOverdue+s.PinnedWarning))
	}
	fmt.Fprintf(w, "\nRunners: %s\n", strings.Join(parts, " · "))
}
