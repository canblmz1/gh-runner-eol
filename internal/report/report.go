// Package report assembles audit results and renders them as a human table,
// JSON, or SARIF.
package report

import (
	"sort"
	"time"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
	"github.com/canblmz1/gh-runner-eol/internal/gh"
	"github.com/canblmz1/gh-runner-eol/internal/scan"
)

// RunnerRef is the subset of runner identity kept in the report.
type RunnerRef struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}

// VersionGroup is every live runner sharing one version plus its assessment.
type VersionGroup struct {
	eol.Assessment
	Count   int         `json:"count"`
	Online  int         `json:"online"`
	Runners []RunnerRef `json:"runners"`
}

// PinnedFinding is a source-code pin with the EOL assessment of the version
// it references.
type PinnedFinding struct {
	scan.Finding
	Assessment *eol.Assessment `json:"assessment,omitempty"`
}

// Summary counts findings by status for quick exit-code and headline logic.
type Summary struct {
	Runners        int `json:"runners"`
	Versions       int `json:"versions"`
	Overdue        int `json:"overdue_runners"`
	Warning        int `json:"warning_runners"`
	Healthy        int `json:"healthy_runners"`
	Unknown        int `json:"unknown_runners"`
	PinnedTotal    int `json:"pinned_refs"`
	PinnedOverdue  int `json:"pinned_overdue"`
	PinnedWarning  int `json:"pinned_warning"`
	PinnedFloating int `json:"pinned_floating"`
}

// Report is the full audit result.
type Report struct {
	Tool        string          `json:"tool"`
	Command     string          `json:"command"`
	ToolVersion string          `json:"tool_version"`
	GeneratedAt time.Time       `json:"generated_at"`
	Scope       string          `json:"scope"`
	WarnDays    int             `json:"warn_days"`
	ScanPath    string          `json:"scan_path,omitempty"`
	Groups      []VersionGroup  `json:"versions"`
	Pinned      []PinnedFinding `json:"pinned,omitempty"`
	Summary     Summary         `json:"summary"`
	Errors      []string        `json:"errors,omitempty"`
}

// Build groups runners by version, attaches assessments, and computes the
// summary. assessments is keyed by normalized version ("" for unknown).
func Build(command, scope string, toolVersion string, now time.Time, warnDays int,
	runners []gh.Runner, assessments map[string]eol.Assessment,
	scanPath string, pins []scan.Finding) *Report {

	r := &Report{
		Tool:        "runner-eol",
		Command:     command,
		ToolVersion: toolVersion,
		GeneratedAt: now.UTC(),
		Scope:       scope,
		WarnDays:    warnDays,
		ScanPath:    scanPath,
	}

	byVersion := map[string]*VersionGroup{}
	for _, rn := range runners {
		v := rn.VersionString()
		g, ok := byVersion[v]
		if !ok {
			a, has := assessments[v]
			if !has {
				a = eol.Assess(eol.Schedule{Version: v}, now, warnDays)
			}
			g = &VersionGroup{Assessment: a}
			byVersion[v] = g
		}
		g.Count++
		if rn.Status == "online" {
			g.Online++
		}
		g.Runners = append(g.Runners, RunnerRef{ID: rn.ID, Name: rn.Name, OS: rn.OS, Status: rn.Status, Busy: rn.Busy})
	}
	for _, g := range byVersion {
		sort.Slice(g.Runners, func(i, j int) bool { return g.Runners[i].Name < g.Runners[j].Name })
		r.Groups = append(r.Groups, *g)
	}
	sort.Slice(r.Groups, func(i, j int) bool {
		a, b := r.Groups[i], r.Groups[j]
		if a.Status.Severity() != b.Status.Severity() {
			return a.Status.Severity() > b.Status.Severity()
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Version < b.Version
	})

	for _, p := range pins {
		pf := PinnedFinding{Finding: p}
		if p.Version != "" {
			a, ok := assessments[p.Version]
			if !ok {
				a = eol.Assess(eol.Schedule{Version: p.Version}, now, warnDays)
			}
			pf.Assessment = &a
		}
		r.Pinned = append(r.Pinned, pf)
	}
	sort.SliceStable(r.Pinned, func(i, j int) bool {
		return pinSeverity(r.Pinned[i]) > pinSeverity(r.Pinned[j])
	})

	r.Summary = summarize(r)
	return r
}

func pinSeverity(p PinnedFinding) int {
	if p.Assessment == nil {
		return -1
	}
	return p.Assessment.Status.Severity()
}

func summarize(r *Report) Summary {
	s := Summary{Versions: len(r.Groups), PinnedTotal: len(r.Pinned)}
	for _, g := range r.Groups {
		s.Runners += g.Count
		switch g.Status {
		case eol.StatusOverdue:
			s.Overdue += g.Count
		case eol.StatusWarning:
			s.Warning += g.Count
		case eol.StatusUnknown:
			s.Unknown += g.Count
		default:
			s.Healthy += g.Count
		}
	}
	for _, p := range r.Pinned {
		if p.Floating {
			s.PinnedFloating++
			continue
		}
		if p.Assessment == nil {
			continue
		}
		switch p.Assessment.Status {
		case eol.StatusOverdue:
			s.PinnedOverdue++
		case eol.StatusWarning:
			s.PinnedWarning++
		}
	}
	return s
}

// MaxSeverity is the worst status across live runners and pinned references.
func (r *Report) MaxSeverity() int {
	m := 0
	for _, g := range r.Groups {
		if s := g.Status.Severity(); s > m {
			m = s
		}
	}
	for _, p := range r.Pinned {
		if s := pinSeverity(p); s > m {
			m = s
		}
	}
	return m
}
