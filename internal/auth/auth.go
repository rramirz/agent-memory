package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/rramirz/agent-memory/internal/models"
)

// TokenStore maps bearer tokens to the orgs they are allowed to access.
// Token format in env: "token1:org1,org2\ntoken2:org3"
type TokenStore struct {
	tokens map[string][]string
}

func NewTokenStore(raw string) (*TokenStore, error) {
	ts := &TokenStore{tokens: make(map[string][]string)}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		token := strings.TrimSpace(parts[0])
		var orgs []string
		for _, o := range strings.Split(parts[1], ",") {
			if o := strings.TrimSpace(o); o != "" {
				orgs = append(orgs, o)
			}
		}
		if token != "" && len(orgs) > 0 {
			ts.tokens[token] = orgs
		}
	}
	return ts, nil
}

func (ts *TokenStore) CanAccessOrg(token, org string) bool {
	orgs, ok := ts.tokens[token]
	if !ok {
		return false
	}
	for _, o := range orgs {
		if o == org {
			return true
		}
	}
	return false
}

// BearerToken extracts the token from an Authorization: Bearer <token> header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	return strings.TrimPrefix(h, "Bearer ")
}

// TokenFinder looks up an active (non-revoked) DB token by its SHA-256 hex hash.
type TokenFinder interface {
	FindTokenByHash(ctx context.Context, hash string) (*models.Token, error)
}

// Authorizer resolves bearer tokens in order: admin token, env tokens, DB tokens.
type Authorizer struct {
	env        *TokenStore
	finder     TokenFinder
	adminToken string
}

func NewAuthorizer(env *TokenStore, finder TokenFinder, adminToken string) *Authorizer {
	return &Authorizer{env: env, finder: finder, adminToken: adminToken}
}

func (a *Authorizer) AdminEnabled() bool {
	return a.adminToken != ""
}

func (a *Authorizer) IsAdmin(token string) bool {
	if a.adminToken == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.adminToken)) == 1
}

func (a *Authorizer) CanAccessOrg(ctx context.Context, token, org string) bool {
	if token == "" {
		return false
	}
	if a.IsAdmin(token) {
		return true
	}
	if a.env != nil && a.env.CanAccessOrg(token, org) {
		return true
	}
	if a.finder == nil {
		return false
	}
	t, err := a.finder.FindTokenByHash(ctx, HashToken(token))
	if err != nil || t == nil {
		return false
	}
	for _, o := range t.Orgs {
		if o == org {
			return true
		}
	}
	return false
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
