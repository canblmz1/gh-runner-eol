package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
)

// SARIF 2.1.0 — the minimum needed for GitHub code scanning upload and
// PR check annotations. Live-runner findings have no file location and are
// reported as logical locations; pinned references carry physical locations
// so they annotate the exact line in the PR.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	ShortDescription sarifMessage   `json:"shortDescription"`
	FullDescription  sarifMessage   `json:"fullDescription"`
	Help             sarifMessage   `json:"help"`
	DefaultConfig    sarifConfig    `json:"defaultConfiguration"`
	Properties       map[string]any `json:"properties,omitempty"`
}

type sarifConfig struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
	// PartialFingerprints let code scanning track a finding across runs.
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	Properties          map[string]any    `json:"properties,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation *sarifPhysical `json:"physicalLocation,omitempty"`
	LogicalLocations []sarifLogical `json:"logicalLocations,omitempty"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

type sarifLogical struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind"`
}

const (
	ruleRunnerOverdue = "runner-eol/runner-overdue"
	ruleRunnerWarning = "runner-eol/runner-expiring"
	ruleRunnerUnknown = "runner-eol/runner-unknown-version"
	rulePinnedOverdue = "runner-eol/pinned-overdue"
	rulePinnedWarning = "runner-eol/pinned-expiring"
	rulePinnedFloat   = "runner-eol/pinned-floating"
	rulePinnedUnknown = "runner-eol/pinned-unresolved"
)

func sarifRules() []sarifRule {
	mk := func(id, name, short, full, level string) sarifRule {
		return sarifRule{
			ID:               id,
			Name:             name,
			ShortDescription: sarifMessage{short},
			FullDescription:  sarifMessage{full},
			Help:             sarifMessage{"Upgrade to a runner version whose runtime support has not ended. GitHub publishes the schedule at GET /actions/runners/deprecations/{version}."},
			DefaultConfig:    sarifConfig{level},
		}
	}
	return []sarifRule{
		mk(ruleRunnerOverdue, "RunnerOverdue", "Live runner is past runtime end-of-life",
			"GitHub Actions will not queue jobs to self-hosted runners whose version is past its runtime_deprecates_at date. Jobs targeting these runners stay queued.", "error"),
		mk(ruleRunnerWarning, "RunnerExpiring", "Live runner reaches runtime end-of-life soon",
			"The runner version will stop receiving jobs within the warning window.", "warning"),
		mk(ruleRunnerUnknown, "RunnerUnknownVersion", "Runner version unknown to GitHub's schedule",
			"The runner did not report a version or the version is not present in GitHub's deprecation schedule (typically far too old).", "note"),
		mk(rulePinnedOverdue, "PinnedOverdue", "Source pins a runner version past runtime end-of-life",
			"A Dockerfile, Helm value, or script pins a runner version GitHub no longer serves. Any runner created from this reference will fail to receive jobs.", "error"),
		mk(rulePinnedWarning, "PinnedExpiring", "Source pins a runner version that expires soon",
			"The pinned runner version reaches runtime end-of-life within the warning window.", "warning"),
		mk(rulePinnedFloat, "PinnedFloating", "Source uses a floating runner image tag",
			"A :latest tag always resolves to the newest runner and never goes EOL, but builds are not reproducible. Informational.", "note"),
		mk(rulePinnedUnknown, "PinnedUnresolved", "Source pins a runner version whose EOL date could not be resolved",
			"The pinned version was found but GitHub's deprecation schedule could not be queried (no scope given, token lacks permission, or the version is unknown to GitHub). Grant a token that can read self-hosted runners to get a real verdict.", "note"),
	}
}

func levelFor(s eol.Status) string {
	switch s {
	case eol.StatusOverdue:
		return "error"
	case eol.StatusWarning:
		return "warning"
	case eol.StatusUnknown:
		return "note"
	}
	return "none"
}

// WriteSARIF emits a SARIF 2.1.0 log. Healthy/current findings are omitted so
// code scanning only shows actionable items.
func WriteSARIF(w io.Writer, r *Report) error {
	run := sarifRun{
		Tool: sarifTool{Driver: sarifDriver{
			Name:           "runner-eol",
			Version:        r.ToolVersion,
			InformationURI: "https://github.com/canblmz1/gh-runner-eol",
			Rules:          sarifRules(),
		}},
		Results: []sarifResult{},
	}

	for _, g := range r.Groups {
		var rule string
		switch g.Status {
		case eol.StatusOverdue:
			rule = ruleRunnerOverdue
		case eol.StatusWarning:
			rule = ruleRunnerWarning
		case eol.StatusUnknown:
			rule = ruleRunnerUnknown
		default:
			continue
		}
		v := g.Version
		if v == "" {
			v = "unknown"
		}
		run.Results = append(run.Results, sarifResult{
			RuleID:  rule,
			Level:   levelFor(g.Status),
			Message: sarifMessage{fmt.Sprintf("%d self-hosted runner(s) in %s on v%s: %s", g.Count, r.Scope, v, Describe(g.Assessment))},
			Locations: []sarifLocation{{LogicalLocations: []sarifLogical{{
				Name:               "v" + v,
				FullyQualifiedName: r.Scope + "/runners/v" + v,
				Kind:               "resource",
			}}}},
			PartialFingerprints: map[string]string{"runnerVersion/v1": r.Scope + ":" + v},
			Properties: map[string]any{
				"runnerCount":         g.Count,
				"onlineCount":         g.Online,
				"runtimeDeprecatesAt": g.RuntimeDeprecatesAt,
				"daysLeft":            g.DaysLeft,
			},
		})
	}

	for _, p := range r.Pinned {
		var rule, level, msg string
		switch {
		case p.Floating:
			rule, level = rulePinnedFloat, "note"
			msg = fmt.Sprintf("%s uses a floating tag; builds are not reproducible", p.Match)
		case p.Assessment != nil && p.Assessment.Status == eol.StatusOverdue:
			rule, level = rulePinnedOverdue, "error"
			msg = fmt.Sprintf("%s pins runner v%s: %s", p.Match, p.Version, Describe(*p.Assessment))
		case p.Assessment != nil && p.Assessment.Status == eol.StatusWarning:
			rule, level = rulePinnedWarning, "warning"
			msg = fmt.Sprintf("%s pins runner v%s: %s", p.Match, p.Version, Describe(*p.Assessment))
		case p.Assessment == nil || p.Assessment.Status == eol.StatusUnknown:
			rule, level = rulePinnedUnknown, "note"
			msg = fmt.Sprintf("%s pins runner v%s but its EOL date could not be resolved", p.Match, p.Version)
		default:
			continue
		}
		run.Results = append(run.Results, sarifResult{
			RuleID:  rule,
			Level:   level,
			Message: sarifMessage{msg},
			Locations: []sarifLocation{{PhysicalLocation: &sarifPhysical{
				ArtifactLocation: sarifArtifact{URI: p.Path, URIBaseID: "%SRCROOT%"},
				Region:           sarifRegion{StartLine: p.Line, StartColumn: p.Column},
			}}},
			PartialFingerprints: map[string]string{"pinnedRef/v1": fmt.Sprintf("%s:%s:%s", p.Path, p.Rule, p.Version)},
		})
	}

	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs:    []sarifRun{run},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}
