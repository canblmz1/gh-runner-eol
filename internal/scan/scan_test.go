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
