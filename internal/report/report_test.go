package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
	"github.com/canblmz1/gh-runner-eol/internal/gh"
	"github.com/canblmz1/gh-runner-eol/internal/scan"
)

func fixture(t *testing.T) *Report {
	t.Helper()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	d := func(s string) *time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return &v
	}
	sv := func(s string) *string { return &s }

	runners := []gh.Runner{
		{ID: 1, Name: "k8s-a", OS: "Linux", Status: "online", Version: sv("2.334.0")},
		{ID: 2, Name: "k8s-b", OS: "Linux", Status: "offline", Version: sv("2.334.0")},
		{ID: 3, Name: "mac-1", OS: "macOS", Status: "online", Version: sv("v2.337.0")},
		{ID: 4, Name: "legacy", OS: "Linux", Status: "online", Version: nil},
	}
	assess := map[string]eol.Assessment{
		"2.334.0": eol.Assess(eol.Schedule{Version: "2.334.0", Found: true, RuntimeDeprecatesAt: d("2026-08-10T17:08:55Z")}, now, 14),
		"2.337.0": eol.Assess(eol.Schedule{Version: "2.337.0", Found: true}, now, 14),
		"2.336.0": eol.Assess(eol.Schedule{Version: "2.336.0", Found: true, RuntimeDeprecatesAt: d("2026-09-15T00:00:00Z")}, now, 14),
	}
	pins := []scan.Finding{
		{Path: "Dockerfile", Line: 1, Column: 6, Match: "ghcr.io/actions/actions-runner:2.336.0", Version: "2.336.0", Rule: "ghcr-actions-runner-image"},
		{Path: "deploy/values.yaml", Line: 9, Column: 3, Match: "ghcr.io/actions/actions-runner:latest", Rule: "ghcr-actions-runner-floating", Floating: true},
	}
	return Build("audit", "acme", "test", now, 14, runners, assess, ".", pins)
}

func TestBuildGroupsAndSorts(t *testing.T) {
	r := fixture(t)
	if r.Summary.Runners != 4 || r.Summary.Versions != 3 {
		t.Fatalf("summary %+v", r.Summary)
	}
	if r.Groups[0].Version != "2.334.0" || r.Groups[0].Status != eol.StatusOverdue || r.Groups[0].Count != 2 || r.Groups[0].Online != 1 {
		t.Fatalf("first group should be the overdue pair: %+v", r.Groups[0])
	}
	if r.Groups[1].Status != eol.StatusUnknown {
		t.Fatalf("unknown should outrank healthy: %+v", r.Groups[1])
	}
	if r.Summary.Overdue != 2 || r.Summary.Unknown != 1 || r.Summary.Healthy != 1 {
		t.Fatalf("counts %+v", r.Summary)
	}
	if r.Summary.PinnedWarning != 1 || r.Summary.PinnedFloating != 1 {
		t.Fatalf("pinned counts %+v", r.Summary)
	}
	if r.MaxSeverity() != eol.StatusOverdue.Severity() {
		t.Fatal("max severity")
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	WriteTable(&buf, fixture(t), TableOptions{})
	out := buf.String()
	for _, want := range []string{
		"runner-eol audit acme",
		"4 self-hosted runners · 3 versions · 2 pinned refs in .",
		"OVERDUE   2 runners    v2.334.0     runtime support ended 2026-08-10   26 days overdue  (1 online)",
		"UNKNOWN   1 runner     (no version reported)",
		"OK        1 runner     v2.337.0     no end-of-life scheduled",
		"WARNING   Dockerfile:1",
		"FLOAT     deploy/values.yaml:9",
		"Runners: 2 overdue · 1 unknown · 1 healthy · 1 pinned refs at risk",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("color disabled but ANSI present")
	}
}

func TestWriteSARIFIsValidAndActionable(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSARIF(&buf, fixture(t)); err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct{ ID string } `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				Level     string `json:"level"`
				Locations []struct {
					Physical *struct {
						Artifact struct{ URI string }    `json:"artifactLocation"`
						Region   struct{ StartLine int } `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatal(err)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || len(log.Runs[0].Tool.Driver.Rules) != 6 {
		t.Fatalf("bad sarif envelope: %s", buf.String())
	}
	res := log.Runs[0].Results
	// overdue runners, unknown runners, expiring pin, floating pin — healthy omitted
	if len(res) != 4 {
		t.Fatalf("expected 4 results, got %d", len(res))
	}
	rules := map[string]bool{}
	for _, r := range res {
		rules[r.RuleID] = true
		if r.RuleID == "runner-eol/pinned-expiring" {
			if r.Locations[0].Physical == nil || r.Locations[0].Physical.Artifact.URI != "Dockerfile" || r.Locations[0].Physical.Region.StartLine != 1 {
				t.Fatalf("pinned result lacks physical location: %+v", r)
			}
		}
	}
	for _, want := range []string{"runner-eol/runner-overdue", "runner-eol/runner-unknown-version", "runner-eol/pinned-expiring", "runner-eol/pinned-floating"} {
		if !rules[want] {
			t.Fatalf("missing rule %s in %v", want, rules)
		}
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, fixture(t)); err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if back.Summary.Overdue != 2 || len(back.Groups) != 3 || back.Command != "audit" {
		t.Fatalf("round trip lost data: %+v", back.Summary)
	}
}
