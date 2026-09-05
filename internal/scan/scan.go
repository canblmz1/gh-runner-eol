// Package scan finds runner versions pinned in source: Dockerfiles, ARC Helm
// values, Kubernetes manifests, Terraform, Packer, shell scripts. These are the
// runners that do not exist yet but will be born on the next scale-up — the
// part of the fleet the live runner API cannot see.
package scan

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Finding is one pinned (or floating) runner reference in a file.
type Finding struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Match   string `json:"match"`
	Version string `json:"version,omitempty"` // empty when Floating
	Rule    string `json:"rule"`
	// Floating marks tags such as :latest that always resolve to the newest
	// image. They never go EOL but are not reproducible.
	Floating bool `json:"floating"`
}

type rule struct {
	name string
	re   *regexp.Regexp
	// group is the capture index of the version; 0 means floating tag.
	group int
}

const semver = `(2\.\d{3}\.\d+)`

var rules = []rule{
	{"ghcr-actions-runner-image", regexp.MustCompile(`ghcr\.io/actions/actions-runner(?:-[\w.-]+)?:v?` + semver), 1},
	{"ghcr-actions-runner-floating", regexp.MustCompile(`ghcr\.io/actions/actions-runner(?:-[\w.-]+)?:latest\b`), 0},
	{"summerwind-runner-image", regexp.MustCompile(`summerwind/actions-runner(?:-[\w.-]+)?:v?` + semver), 1},
	{"summerwind-runner-floating", regexp.MustCompile(`summerwind/actions-runner(?:-[\w.-]+)?:latest\b`), 0},
	{"runner-release-download", regexp.MustCompile(`actions/runner/releases/download/v` + semver), 1},
	{"runner-tarball", regexp.MustCompile(`actions-runner-(?:linux|osx|win)-(?:x64|arm64|arm)-` + semver), 1},
	// Covers RUNNER_VERSION=, RUNNER_VERSION:, runner_version=, runnerVersion: (ARC values).
	{"runner-version-variable", regexp.MustCompile(`(?i)\bRUNNER[_-]?VERSION\b["']?\s*[:=]\s*["']?v?` + semver), 1},
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".terraform": true,
	"dist": true, "build": true, ".idea": true, ".vscode": true,
}

const maxFileSize = 2 << 20 // 2 MiB

// Dir walks root and scans every text file.
func Dir(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		fs, err := Reader(filepath.ToSlash(rel), f)
		if err != nil {
			return nil
		}
		out = append(out, fs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Column < out[j].Column
	})
	return out, nil
}

// Reader scans a single file. Binary content is skipped.
func Reader(path string, r io.Reader) ([]Finding, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	head, _ := br.Peek(512)
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, nil
	}

	var out []Finding
	seen := map[string]bool{}
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 0, 64<<10), maxFileSize)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if !strings.Contains(line, "runner") && !strings.Contains(line, "RUNNER") && !strings.Contains(line, "Runner") {
			continue
		}
		for _, rl := range rules {
			for _, m := range rl.re.FindAllStringSubmatchIndex(line, -1) {
				f := Finding{
					Path:   path,
					Line:   lineNo,
					Column: m[0] + 1,
					Match:  line[m[0]:m[1]],
					Rule:   rl.name,
				}
				if rl.group == 0 {
					f.Floating = true
				} else {
					f.Version = line[m[2*rl.group]:m[2*rl.group+1]]
				}
				key := fmt.Sprintf("%d:%d:%s", lineNo, m[0], f.Version)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, f)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Versions returns the distinct pinned versions across findings, sorted.
func Versions(fs []Finding) []string {
	seen := map[string]bool{}
	for _, f := range fs {
		if f.Version != "" {
			seen[f.Version] = true
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
