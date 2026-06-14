package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *UIHandlers) handleMemoriesGet(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if org == "" {
		org = "personal"
	}
	
	validOrgsList := []string{}
	for k := range models.ValidOrgs {
		validOrgsList = append(validOrgsList, k)
	}
	sort.Strings(validOrgsList)

	types := []string{
		models.TypeDecision, models.TypeSessionSummary, models.TypeArchitecture,
		models.TypeRunbook, models.TypeKnownIssue, models.TypeTask,
		models.TypePreference, models.TypeNote,
		models.TypeIdea, models.TypeSkill, models.TypeAgent, models.TypePromptPattern,
	}

	h.render(w, "memories.html", map[string]interface{}{
		"Auth":   true,
		"Active": "memories",
		"Org":    org,
		"Orgs":   validOrgsList,
		"Types":  types,
	})
}

func (h *UIHandlers) handleMemoriesList(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if org == "" {
		org = "personal"
	}

	params := db.SearchParams{
		Org:     org,
		Query:   r.URL.Query().Get("q"),
		Type:    r.URL.Query().Get("type"),
		Project: r.URL.Query().Get("project"),
		Repo:    r.URL.Query().Get("repo"),
		Limit:   50,
	}

	memories, err := h.db.SearchMemories(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderPartial(w, "memories_list.html", map[string]interface{}{
		"Memories": memories,
	})
}

func (h *UIHandlers) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.db.SoftDeleteMemory(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *UIHandlers) handleMemoryEditGet(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	m, err := h.db.GetMemoryByID(r.Context(), id)
	if err != nil || m == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderPartial(w, "memory_edit_row.html", map[string]interface{}{
		"Memory": m,
	})
}

func (h *UIHandlers) handleMemoryEditPost(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	
	m, err := h.db.GetMemoryByID(r.Context(), id)
	if err != nil || m == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	set := bson.D{}
	
	if title := r.FormValue("title"); title != "" {
		set = append(set, bson.E{Key: "title", Value: title})
	}
	if body := r.FormValue("body"); body != "" {
		set = append(set, bson.E{Key: "body", Value: body})
	}
	if impStr := r.FormValue("importance"); impStr != "" {
		if imp, err := strconv.Atoi(impStr); err == nil && imp >= 1 && imp <= 10 {
			set = append(set, bson.E{Key: "importance", Value: imp})
		}
	}
	
	tagsRaw := r.FormValue("tags")
	var tags []string
	for _, t := range strings.Split(tagsRaw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	if len(tags) > 0 || tagsRaw == "" {
		set = append(set, bson.E{Key: "tags", Value: tags})
	}

	if len(set) > 0 {
		if err := h.db.UpdateMemory(r.Context(), id, set); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	m, _ = h.db.GetMemoryByID(r.Context(), id)
	h.renderPartial(w, "memory_row.html", map[string]interface{}{
		"Memory": m,
	})
}
