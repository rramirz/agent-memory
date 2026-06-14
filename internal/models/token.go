package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Token is a DB-backed bearer token. TokenHash is never serialized to JSON.
type Token struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name      string             `bson:"name"          json:"name"`
	TokenHash string             `bson:"token_hash"    json:"-"`
	Orgs      []string           `bson:"orgs"          json:"orgs"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
	RevokedAt *time.Time         `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
}

type CreateTokenRequest struct {
	Name string   `json:"name"`
	Orgs []string `json:"orgs"`
}

// CreateTokenResponse includes the plaintext token, shown exactly once.
type CreateTokenResponse struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Orgs  []string `json:"orgs"`
	Token string   `json:"token"`
}

type ListTokensResponse struct {
	Tokens []Token `json:"tokens"`
	Total  int     `json:"total"`
}

// UpdateMemoryRequest carries a partial update; nil fields are left unchanged.
// Org is intentionally absent: a memory's org can never change.
type UpdateMemoryRequest struct {
	Title      *string   `json:"title,omitempty"`
	Body       *string   `json:"body,omitempty"`
	Tags       *[]string `json:"tags,omitempty"`
	Importance *int      `json:"importance,omitempty"`
	Type       *string   `json:"type,omitempty"`
	Project    *string   `json:"project,omitempty"`
	Repo       *string   `json:"repo,omitempty"`
	Scope      *string   `json:"scope,omitempty"`
	Status     *string   `json:"status,omitempty"`
}
