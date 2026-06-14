package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/contextgen"
	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/models"
)

type ContextHandlers struct {
	db        *db.DB
	auth      *auth.Authorizer
	generator *contextgen.Generator
}

func NewContextHandlers(database *db.DB, authorizer *auth.Authorizer) *ContextHandlers {
	return &ContextHandlers{
		db:        database,
		auth:      authorizer,
		generator: contextgen.New(database),
	}
}

func (h *ContextHandlers) GetContext(w http.ResponseWriter, r *http.Request) {
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

	project := r.URL.Query().Get("project")
	repo := r.URL.Query().Get("repo")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	files, err := h.generator.Generate(ctx, org, project, repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "context generation failed")
		return
	}

	writeJSON(w, http.StatusOK, models.ContextResponse{
		Org:     org,
		Project: project,
		Repo:    repo,
		Files:   files,
	})
}
