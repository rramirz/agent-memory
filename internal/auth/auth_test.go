package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/rramirz/agent-memory/internal/models"
)

type fakeFinder struct {
	tokens map[string]*models.Token
	err    error
}

func (f *fakeFinder) FindTokenByHash(_ context.Context, hash string) (*models.Token, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tokens[hash], nil
}

func TestAuthorizerCanAccessOrg(t *testing.T) {
	env, err := NewTokenStore("envtok:personal,arrive\n# comment\nother:logicbroker")
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}

	dbPlain := "db-token-plaintext"
	finder := &fakeFinder{tokens: map[string]*models.Token{
		HashToken(dbPlain): {Name: "home-mac", Orgs: []string{"personal"}},
	}}

	tests := []struct {
		name   string
		authz  *Authorizer
		token  string
		org    string
		expect bool
	}{
		{"env token allowed org", NewAuthorizer(env, finder, "admintok"), "envtok", "personal", true},
		{"env token second org", NewAuthorizer(env, finder, "admintok"), "envtok", "arrive", true},
		{"env token wrong org", NewAuthorizer(env, finder, "admintok"), "envtok", "logicbroker", false},
		{"env comment line not a token", NewAuthorizer(env, finder, "admintok"), "# comment", "personal", false},
		{"db token allowed org", NewAuthorizer(env, finder, "admintok"), dbPlain, "personal", true},
		{"db token wrong org", NewAuthorizer(env, finder, "admintok"), dbPlain, "arrive", false},
		{"revoked/unknown db token", NewAuthorizer(env, finder, "admintok"), "revoked-token", "personal", false},
		{"db lookup error denies", NewAuthorizer(env, &fakeFinder{err: errors.New("boom")}, "admintok"), dbPlain, "personal", false},
		{"admin token any org", NewAuthorizer(env, finder, "admintok"), "admintok", "logicbroker", true},
		{"admin disabled not all-access", NewAuthorizer(env, finder, ""), "admintok", "logicbroker", false},
		{"empty token denied", NewAuthorizer(env, finder, "admintok"), "", "personal", false},
		{"nil finder env still works", NewAuthorizer(env, nil, "admintok"), "envtok", "personal", true},
		{"nil finder db token denied", NewAuthorizer(env, nil, "admintok"), dbPlain, "personal", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.authz.CanAccessOrg(context.Background(), tc.token, tc.org)
			if got != tc.expect {
				t.Errorf("CanAccessOrg(%q, %q) = %v, want %v", tc.token, tc.org, got, tc.expect)
			}
		})
	}
}

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name       string
		adminToken string
		token      string
		expect     bool
	}{
		{"match", "secret", "secret", true},
		{"mismatch", "secret", "wrong", false},
		{"disabled", "", "secret", false},
		{"disabled empty token", "", "", false},
		{"empty bearer", "secret", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAuthorizer(nil, nil, tc.adminToken)
			if got := a.IsAdmin(tc.token); got != tc.expect {
				t.Errorf("IsAdmin(%q) with admin %q = %v, want %v", tc.token, tc.adminToken, got, tc.expect)
			}
			if got := a.AdminEnabled(); got != (tc.adminToken != "") {
				t.Errorf("AdminEnabled() = %v", got)
			}
		})
	}
}
