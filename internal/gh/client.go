// Package gh is a thin, testable layer over the GitHub REST endpoints this
// tool needs: self-hosted runner listing and runner version deprecation
// lookups.
package gh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/canblmz1/gh-runner-eol/internal/eol"
)

// ErrNotFound is returned (possibly wrapped) by Doer implementations when the
// API answers 404. The deprecation endpoint uses 404 for versions GitHub no
// longer tracks, so callers must be able to distinguish it from real failures.
var ErrNotFound = errors.New("not found")

// Doer is the minimal HTTP surface the client needs; tests supply fakes.
type Doer interface {
	Get(ctx context.Context, path string, out any) error
}

// DefaultAPIVersion is the REST API date the deprecation endpoint is
// documented under. Override with RUNNER_EOL_API_VERSION.
const DefaultAPIVersion = "2026-03-10"

// Runner mirrors the fields of a self-hosted runner we care about.
type Runner struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	OS        string  `json:"os"`
	Status    string  `json:"status"` // online | offline
	Busy      bool    `json:"busy"`
	Ephemeral bool    `json:"ephemeral"`
	Version   *string `json:"version"`
	Labels    []Label `json:"labels"`
}

// Label is a runner label.
type Label struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// VersionString returns the reported version or "" when GitHub does not know it.
func (r Runner) VersionString() string {
	if r.Version == nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(*r.Version), "v")
}

type runnersPage struct {
	TotalCount int      `json:"total_count"`
	Runners    []Runner `json:"runners"`
}

type deprecationResponse struct {
	RunnerVersion            string     `json:"runner_version"`
	RuntimeDeprecatesAt      *time.Time `json:"runtime_deprecates_at"`
	RegistrationDeprecatesAt *time.Time `json:"registration_deprecates_at"`
}

// Client wraps a Doer with the operations used by the CLI.
type Client struct {
	api Doer
}

// NewClient builds a client over an arbitrary Doer.
func NewClient(d Doer) *Client { return &Client{api: d} }

// NewDefaultClient authenticates the way `gh` does: GH_TOKEN / GITHUB_TOKEN
// env vars first, then the gh CLI's stored credentials.
func NewDefaultClient() (*Client, error) {
	apiVersion := os.Getenv("RUNNER_EOL_API_VERSION")
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	rest, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{
			"X-GitHub-Api-Version": apiVersion,
			"User-Agent":           "gh-runner-eol",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("github auth: %w (set GH_TOKEN or run `gh auth login`)", err)
	}
	return NewClient(ghDoer{rest}), nil
}

type ghDoer struct{ c *api.RESTClient }

func (d ghDoer) Get(ctx context.Context, path string, out any) error {
	err := d.c.DoWithContext(ctx, "GET", path, nil, out)
	if err == nil {
		return nil
	}
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
		return fmt.Errorf("%s: %w", path, ErrNotFound)
	}
	return err
}

// ListRunners returns every self-hosted runner in scope, following pagination.
func (c *Client) ListRunners(ctx context.Context, scope Scope) ([]Runner, error) {
	const perPage = 100
	var all []Runner
	for page := 1; ; page++ {
		var pg runnersPage
		if err := c.api.Get(ctx, scope.RunnersPath(page, perPage), &pg); err != nil {
			return nil, fmt.Errorf("list runners for %s: %w", scope, err)
		}
		all = append(all, pg.Runners...)
		if len(pg.Runners) < perPage || len(all) >= pg.TotalCount {
			break
		}
		if page > 1000 { // hard stop against a misbehaving API
			return nil, fmt.Errorf("list runners for %s: pagination did not terminate", scope)
		}
	}
	return all, nil
}

// GetDeprecation looks up the end-of-life schedule for one runner version.
// A 404 is not an error: it yields Found == false.
func (c *Client) GetDeprecation(ctx context.Context, scope Scope, version string) (eol.Schedule, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	s := eol.Schedule{Version: version}
	if version == "" {
		return s, nil
	}
	var resp deprecationResponse
	err := c.api.Get(ctx, scope.DeprecationPath(version), &resp)
	if errors.Is(err, ErrNotFound) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("deprecation lookup %s @ %s: %w", version, scope, err)
	}
	s.Found = true
	s.RuntimeDeprecatesAt = resp.RuntimeDeprecatesAt
	s.RegistrationDeprecatesAt = resp.RegistrationDeprecatesAt
	if resp.RunnerVersion != "" {
		s.Version = strings.TrimPrefix(resp.RunnerVersion, "v")
	}
	return s, nil
}

// UniqueVersions returns the distinct versions reported by runners, sorted.
// The empty string is included when at least one runner reports no version.
func UniqueVersions(runners []Runner) []string {
	seen := map[string]bool{}
	for _, r := range runners {
		seen[r.VersionString()] = true
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
