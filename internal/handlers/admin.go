package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AdminHandlers struct {
	db   *db.DB
	auth *auth.Authorizer
}

func NewAdminHandlers(database *db.DB, authorizer *auth.Authorizer) *AdminHandlers {
	return &AdminHandlers{db: database, auth: authorizer}
}

func (h *AdminHandlers) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !h.auth.AdminEnabled() {
		writeError(w, http.StatusServiceUnavailable, "admin disabled")
		return false
	}
	token := auth.BearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization token")
		return false
	}
	if !h.auth.IsAdmin(token) {
		writeError(w, http.StatusForbidden, "admin token required")
		return false
	}
	return true
}

func (h *AdminHandlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	var req models.CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Orgs) == 0 {
		writeError(w, http.StatusBadRequest, "orgs is required")
		return
	}
	for _, org := range req.Orgs {
		if !models.ValidOrgs[org] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid org: %s", org))
			return
		}
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	plaintext := hex.EncodeToString(raw)

	t := &models.Token{
		Name:      req.Name,
		TokenHash: auth.HashToken(plaintext),
		Orgs:      req.Orgs,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.db.CreateToken(ctx, t); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save token")
		return
	}

	writeJSON(w, http.StatusCreated, models.CreateTokenResponse{
		ID:    t.ID.Hex(),
		Name:  t.Name,
		Orgs:  t.Orgs,
		Token: plaintext,
	})
}

func (h *AdminHandlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tokens, err := h.db.ListTokens(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}
	if tokens == nil {
		tokens = []models.Token{}
	}

	writeJSON(w, http.StatusOK, models.ListTokensResponse{
		Tokens: tokens,
		Total:  len(tokens),
	})
}

func (h *AdminHandlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	id, err := primitive.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	revoked, err := h.db.RevokeToken(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, "token not found or already revoked")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id.Hex(),
		"status": "revoked",
	})
}
