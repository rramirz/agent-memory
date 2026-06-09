package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/models"
)

type MemoryHandlers struct {
	db     *db.DB
	tokens *auth.TokenStore
}

func NewMemoryHandlers(database *db.DB, tokens *auth.TokenStore) *MemoryHandlers {
	return &MemoryHandlers{db: database, tokens: tokens}
}

func (h *MemoryHandlers) CreateMemory(w http.ResponseWriter, r *http.Request) {
	token := auth.BearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization token")
		return
	}

	var req models.CreateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Org == "" {
		writeError(w, http.StatusBadRequest, "org is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	if !models.ValidOrgs[req.Org] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid org: %s", req.Org))
		return
	}
	if !h.tokens.CanAccessOrg(token, req.Org) {
		writeError(w, http.StatusForbidden, "token not authorized for this org")
		return
	}

	m := &models.Memory{
		Org:         req.Org,
		Project:     req.Project,
		Repo:        req.Repo,
		Workstation: req.Workstation,
		Scope:       req.Scope,
		Type:        req.Type,
		Title:       req.Title,
		Body:        req.Body,
		Tags:        req.Tags,
		Importance:  req.Importance,
		Status:      models.StatusActive,
		Source:      req.Source,
	}
	if m.Scope == "" {
		m.Scope = models.ScopeRepo
	}
	if m.Importance == 0 {
		m.Importance = 5
	}
	if m.Source == "" {
		m.Source = models.SourceManual
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.db.CreateMemory(ctx, m); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save memory")
		return
	}

	writeJSON(w, http.StatusCreated, models.CreateMemoryResponse{
		ID:     m.ID.Hex(),
		Status: "created",
	})
}

func (h *MemoryHandlers) SearchMemories(w http.ResponseWriter, r *http.Request) {
	token := auth.BearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization token")
		return
	}

	org := r.URL.Query().Get("org")
	if org == "" {
		writeError(w, http.StatusBadRequest, "org is required")
		return
	}
	if !h.tokens.CanAccessOrg(token, org) {
		writeError(w, http.StatusForbidden, "token not authorized for this org")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	params := db.SearchParams{
		Org:     org,
		Query:   r.URL.Query().Get("q"),
		Project: r.URL.Query().Get("project"),
		Repo:    r.URL.Query().Get("repo"),
		Type:    r.URL.Query().Get("type"),
		Tag:     r.URL.Query().Get("tag"),
		Limit:   limit,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	memories, err := h.db.SearchMemories(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	if memories == nil {
		memories = []models.Memory{}
	}

	writeJSON(w, http.StatusOK, models.SearchMemoriesResponse{
		Memories: memories,
		Total:    len(memories),
	})
}
