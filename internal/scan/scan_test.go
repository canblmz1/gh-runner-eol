package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderDetectsCommonPins(t *testing.T) {
	src := `
FROM ghcr.io/actions/actions-runner:2.335.0
ARG RUNNER_VERSION=2.334.0
RUN curl -o runner.tgz -L https://github.com/actions/runner/releases/download/v2.333.0/actions-runner-linux-x64-2.333.0.tar.gz
image: summerwind/actions-runner-dind:v2.331.0
runnerVersion: "2.336.0"
      RUNNER_VERSION: '2.337.0'
image: ghcr.io/actions/actions-runner:latest
# unrelated: version 2.335.0 without runner keyword
`
	fs, err := Reader("Dockerfile", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	byRule := map[string]Finding{}
	for _, f := range fs {
		if _, dup := byRule[f.Rule]; !dup {
			byRule[f.Rule] = f
		}
	}
	want := map[string]string{
		"ghcr-actions-runner-image": "2.335.0",
		"runner-version-variable":   "2.334.0",
		"runner-release-download":   "2.333.0",
		"runner-tarball":            "2.333.0",
		"summerwind-runner-image":   "2.331.0",
	}
	var camel bool
	for _, f := range fs {
		if f.Rule == "runner-version-variable" && f.Version == "2.336.0" {
			camel = true
		}
	}
	if !camel {
		t.Fatalf("runnerVersion: (ARC camelCase) not matched; got %+v", fs)
	}
	for rule, v := range want {
		f, ok := byRule[rule]
		if !ok {
			t.Fatalf("rule %s not matched; got %+v", rule, fs)
		}
		if f.Version != v {
			t.Fatalf("rule %s version = %s want %s", rule, f.Version, v)
		}
	}
	fl, ok := byRule["ghcr-actions-runner-floating"]
	if !ok || !fl.Floating || fl.Version != "" {
		t.Fatalf("floating tag not detected: %+v", fl)
	}
	// RUNNER_VERSION appears twice (2.334.0 and 2.337.0)
	vs := Versions(fs)
	if strings.Join(vs, ",") != "2.331.0,2.333.0,2.334.0,2.335.0,2.336.0,2.337.0" {
		t.Fatalf("versions = %v", vs)
	}
	if fs[0].Line != 2 || fs[0].Column != 6 {
		t.Fatalf("first finding position = %d:%d", fs[0].Line, fs[0].Column)
	}
}

func TestReaderDetectsHCLVariableBlock(t *testing.T) {
	src := `
variable runner_version { default = 2.336.0 }
variable "runner_version" { default = "2.336.0" }
locals { runner_version = 2.339.0 }
`
	fs, err := Reader("main.tf", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var hclVariableHits int
	for _, f := range fs {
		switch f.Rule {
		case "runner-version-hcl-variable":
			if f.Version != "2.336.0" {
				t.Fatalf("rule runner-version-hcl-variable version = %s want 2.336.0", f.Version)
			}
			hclVariableHits++
		case "runner-version-variable":
			// The locals{} assignment on line 4 is already covered by the
			// existing rule; it must not also fire the new HCL-block rule
			// (no "variable" keyword on that line).
			if f.Version != "2.339.0" {
				t.Fatalf("rule runner-version-variable version = %s want 2.339.0", f.Version)
			}
		default:
			t.Fatalf("unexpected rule %s matched: %+v", f.Rule, f)
		}
	}
	if hclVariableHits != 2 {
		t.Fatalf("runner-version-hcl-variable matched %d times, want 2 (bare and quoted variable name); got %+v", hclVariableHits, fs)
	}
}

func TestReaderHelmSplitRepositoryTag(t *testing.T) {
	src := `
# official-chart shape: repo and tag on separate lines
image:
  repository: ghcr.io/actions/actions-runner
  pullPolicy: IfNotPresent
  tag: "2.336.0"
# summerwind split + floating
    repository: summerwind/actions-runner-dind
    tag: latest
# stray tag must not match — no runner repo in the window
tag: "2.300.0"
`
	fs, err := Reader("values.yaml", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var pin, float *Finding
	for i := range fs {
		switch fs[i].Rule {
		case "helm-image-tag":
			pin = &fs[i]
		case "helm-image-tag-floating":
			float = &fs[i]
		}
	}
	if pin == nil || pin.Version != "2.336.0" || !strings.Contains(pin.Match, "ghcr.io/actions/actions-runner") {
		t.Fatalf("split pin not found: %+v", fs)
	}
	if float == nil || !float.Floating {
		t.Fatalf("split :latest not found: %+v", fs)
	}
	for _, f := range fs {
		if f.Version == "2.300.0" {
			t.Fatalf("stray tag matched: %+v", f)
		}
	}
}

func TestReaderHelmTagOutsideWindowIgnored(t *testing.T) {
	var b strings.Builder
	b.WriteString("repository: ghcr.io/actions/actions-runner\n")
	for i := 0; i < helmTagWindow+1; i++ {
		b.WriteString("unrelated: true\n")
	}
	b.WriteString("tag: \"2.336.0\"\n")
	fs, err := Reader("values.yaml", strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.Rule == "helm-image-tag" {
			t.Fatalf("tag beyond window should be ignored: %+v", f)
		}
	}
}

func TestReaderSkipsBinary(t *testing.T) {
	fs, err := Reader("bin", strings.NewReader("runner\x00ghcr.io/actions/actions-runner:2.335.0"))
	if err != nil || len(fs) != 0 {
		t.Fatalf("binary should be skipped: %v %v", fs, err)
	}
}

func TestDirWalksAndSkips(t *testing.T) {
	root := t.TempDir()
	must := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "deploy", "values.yaml"), "template:\n  spec:\n    containers:\n      - image: ghcr.io/actions/actions-runner:2.334.0\n")
	must(filepath.Join(root, "node_modules", "x", "Dockerfile"), "FROM ghcr.io/actions/actions-runner:2.300.0\n")
	must(filepath.Join(root, ".git", "config"), "RUNNER_VERSION=2.300.0\n")
	must(filepath.Join(root, "README.md"), "nothing here\n")

	fs, err := Dir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %+v", fs)
	}
	if fs[0].Path != "deploy/values.yaml" || fs[0].Line != 4 || fs[0].Version != "2.334.0" {
		t.Fatalf("unexpected finding %+v", fs[0])
	}
}
