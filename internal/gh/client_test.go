package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fakeDoer struct {
	responses map[string]string // path → JSON body
	calls     []string
}

func (f *fakeDoer) Get(_ context.Context, path string, out any) error {
	f.calls = append(f.calls, path)
	body, ok := f.responses[path]
	if !ok {
		return fmt.Errorf("%s: %w", path, ErrNotFound)
	}
	return json.Unmarshal([]byte(body), out)
}

func TestParseTarget(t *testing.T) {
	cases := map[string]Scope{
		"acme":                {Kind: ScopeOrg, Name: "acme"},
		"acme/api":            {Kind: ScopeRepo, Owner: "acme", Repo: "api"},
		"enterprise:acme-inc": {Kind: ScopeEnterprise, Name: "acme-inc"},
	}
	for in, want := range cases {
		got, err := ParseTarget(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %+v want %+v", in, got, want)
		}
	}
	for _, bad := range []string{"", "a/b/c", "enterprise:", "/x", "x/"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Fatalf("%q should fail", bad)
		}
	}
}

func TestScopePaths(t *testing.T) {
	s := Scope{Kind: ScopeRepo, Owner: "acme", Repo: "api"}
	if got := s.DeprecationPath("2.334.0"); got != "repos/acme/api/actions/runners/deprecations/2.334.0" {
		t.Fatal(got)
	}
	e := Scope{Kind: ScopeEnterprise, Name: "acme-inc"}
	if got := e.RunnersPath(2, 100); got != "enterprises/acme-inc/actions/runners?per_page=100&page=2" {
		t.Fatal(got)
	}
}

func TestListRunnersPaginates(t *testing.T) {
	scope := Scope{Kind: ScopeOrg, Name: "acme"}
	var page1 []string
	for i := 0; i < 100; i++ {
		page1 = append(page1, fmt.Sprintf(`{"id":%d,"name":"r%d","os":"Linux","status":"online","version":"2.334.0","labels":[]}`, i, i))
	}
	f := &fakeDoer{responses: map[string]string{
		scope.RunnersPath(1, 100): fmt.Sprintf(`{"total_count":102,"runners":[%s]}`, strings.Join(page1, ",")),
		scope.RunnersPath(2, 100): `{"total_count":102,"runners":[
			{"id":100,"name":"r100","os":"Linux","status":"offline","version":null,"labels":[]},
			{"id":101,"name":"r101","os":"Linux","status":"online","version":"v2.337.0","labels":[]}]}`,
	}}
	c := NewClient(f)
	runners, err := c.ListRunners(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 102 {
		t.Fatalf("got %d runners", len(runners))
	}
	if len(f.calls) != 2 {
		t.Fatalf("expected 2 page fetches, got %d", len(f.calls))
	}
	vs := UniqueVersions(runners)
	want := []string{"", "2.334.0", "2.337.0"}
	if strings.Join(vs, ",") != strings.Join(want, ",") {
		t.Fatalf("versions = %v", vs)
	}
}

func TestGetDeprecation(t *testing.T) {
	scope := Scope{Kind: ScopeRepo, Owner: "o", Repo: "r"}
	f := &fakeDoer{responses: map[string]string{
		scope.DeprecationPath("2.334.0"): `{"runner_version":"2.334.0","runtime_deprecates_at":"2026-08-10T17:08:55Z"}`,
		scope.DeprecationPath("2.337.0"): `{"runner_version":"2.337.0","runtime_deprecates_at":null}`,
	}}
	c := NewClient(f)
	ctx := context.Background()

	s, err := c.GetDeprecation(ctx, scope, "v2.334.0")
	if err != nil || !s.Found || s.RuntimeDeprecatesAt == nil || s.RegistrationDeprecatesAt != nil {
		t.Fatalf("2.334.0: %+v %v", s, err)
	}
	s, err = c.GetDeprecation(ctx, scope, "2.337.0")
	if err != nil || !s.Found || s.RuntimeDeprecatesAt != nil {
		t.Fatalf("2.337.0: %+v %v", s, err)
	}
	s, err = c.GetDeprecation(ctx, scope, "2.320.0")
	if err != nil || s.Found {
		t.Fatalf("404 should yield Found=false without error: %+v %v", s, err)
	}
	s, err = c.GetDeprecation(ctx, scope, "")
	if err != nil || s.Found || len(f.calls) != 3 {
		t.Fatalf("empty version must not hit the API: %+v %v calls=%d", s, err, len(f.calls))
	}
}
