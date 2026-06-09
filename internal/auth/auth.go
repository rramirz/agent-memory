package auth

import (
	"net/http"
	"strings"
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
