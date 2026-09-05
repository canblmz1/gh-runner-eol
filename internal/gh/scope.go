package gh

import (
	"fmt"
	"net/url"
	"strings"
)

// ScopeKind selects which level of the GitHub API a query is issued against.
type ScopeKind string

const (
	ScopeRepo       ScopeKind = "repo"
	ScopeOrg        ScopeKind = "org"
	ScopeEnterprise ScopeKind = "enterprise"
)

// Scope identifies where runners live. The deprecation endpoint exists at all
// three levels, so the same scope is used for both runner listing and EOL
// lookups.
type Scope struct {
	Kind  ScopeKind
	Owner string // repo owner (ScopeRepo)
	Repo  string // repo name (ScopeRepo)
	Name  string // org login or enterprise slug
}

// ParseTarget accepts the shorthand forms:
//
//	acme                 → organization
//	acme/api             → repository
//	enterprise:acme-inc  → enterprise
func ParseTarget(s string) (Scope, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Scope{}, fmt.Errorf("target is empty")
	}
	if rest, ok := strings.CutPrefix(s, "enterprise:"); ok {
		if rest == "" {
			return Scope{}, fmt.Errorf("enterprise slug is empty")
		}
		return Scope{Kind: ScopeEnterprise, Name: rest}, nil
	}
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return Scope{}, fmt.Errorf("repository must be owner/name, got %q", s)
		}
		return Scope{Kind: ScopeRepo, Owner: parts[0], Repo: parts[1]}, nil
	}
	return Scope{Kind: ScopeOrg, Name: s}, nil
}

func (s Scope) String() string {
	switch s.Kind {
	case ScopeRepo:
		return s.Owner + "/" + s.Repo
	case ScopeEnterprise:
		return "enterprise:" + s.Name
	default:
		return s.Name
	}
}

func (s Scope) base() string {
	switch s.Kind {
	case ScopeRepo:
		return "repos/" + url.PathEscape(s.Owner) + "/" + url.PathEscape(s.Repo)
	case ScopeEnterprise:
		return "enterprises/" + url.PathEscape(s.Name)
	default:
		return "orgs/" + url.PathEscape(s.Name)
	}
}

// RunnersPath is the paginated self-hosted runner listing.
func (s Scope) RunnersPath(page, perPage int) string {
	return fmt.Sprintf("%s/actions/runners?per_page=%d&page=%d", s.base(), perPage, page)
}

// DeprecationPath is the runner version end-of-life lookup.
func (s Scope) DeprecationPath(version string) string {
	return s.base() + "/actions/runners/deprecations/" + url.PathEscape(version)
}
