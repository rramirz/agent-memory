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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var validStatuses = map[string]bool{
	models.StatusActive:     true,
	models.StatusSuperseded: true,
	models.StatusArchived:   true,
	models.StatusDeleted:    true,
}

type MemoryHandlers struct {
	db   *db.DB
	auth *auth.Authorizer
}

func NewMemoryHandlers(database *db.DB, authorizer *auth.Authorizer) *MemoryHandlers {
	return &MemoryHandlers{db: database, auth: authorizer}
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
	if !h.auth.CanAccessOrg(r.Context(), token, req.Org) {
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
	if !h.auth.CanAccessOrg(r.Context(), token, org) {
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

func (h *MemoryHandlers) loadAuthorizedMemory(w http.ResponseWriter, r *http.Request) (*models.Memory, bool) {
	token := auth.BearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization token")
		return nil, false
	}

	id, err := primitive.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return nil, false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	m, err := h.db.GetMemoryByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load memory")
		return nil, false
	}
	if m == nil || m.Status == models.StatusDeleted {
		writeError(w, http.StatusNotFound, "memory not found")
		return nil, false
	}
	if !h.auth.CanAccessOrg(r.Context(), token, m.Org) {
		writeError(w, http.StatusForbidden, "token not authorized for this org")
		return nil, false
	}
	return m, true
}

func (h *MemoryHandlers) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	m, ok := h.loadAuthorizedMemory(w, r)
	if !ok {
		return
	}

	var req models.UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	set := bson.D{}
	if req.Title != nil {
		if *req.Title == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		set = append(set, bson.E{Key: "title", Value: *req.Title})
	}
	if req.Body != nil {
		if *req.Body == "" {
			writeError(w, http.StatusBadRequest, "body cannot be empty")
			return
		}
		set = append(set, bson.E{Key: "body", Value: *req.Body})
	}
	if req.Tags != nil {
		set = append(set, bson.E{Key: "tags", Value: *req.Tags})
	}
	if req.Importance != nil {
		if *req.Importance < 1 || *req.Importance > 10 {
			writeError(w, http.StatusBadRequest, "importance must be between 1 and 10")
			return
		}
		set = append(set, bson.E{Key: "importance", Value: *req.Importance})
	}
	if req.Type != nil {
		set = append(set, bson.E{Key: "type", Value: *req.Type})
	}
	if req.Project != nil {
		set = append(set, bson.E{Key: "project", Value: *req.Project})
	}
	if req.Repo != nil {
		set = append(set, bson.E{Key: "repo", Value: *req.Repo})
	}
	if req.Scope != nil {
		set = append(set, bson.E{Key: "scope", Value: *req.Scope})
	}
	if req.Status != nil {
		if !validStatuses[*req.Status] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid status: %s", *req.Status))
			return
		}
		set = append(set, bson.E{Key: "status", Value: *req.Status})
	}
	if len(set) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.db.UpdateMemory(ctx, m.ID, set); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update memory")
		return
	}

	updated, err := h.db.GetMemoryByID(ctx, m.ID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated memory")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *MemoryHandlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	m, ok := h.loadAuthorizedMemory(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.db.SoftDeleteMemory(ctx, m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete memory")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":     m.ID.Hex(),
		"status": "deleted",
	})
}
