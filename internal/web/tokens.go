package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *UIHandlers) handleTokensGet(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.db.ListTokens(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	validOrgsList := []string{}
	for k := range models.ValidOrgs {
		validOrgsList = append(validOrgsList, k)
	}

	h.render(w, "tokens.html", map[string]interface{}{
		"Auth":   true,
		"Active": "tokens",
		"Tokens": tokens,
		"Orgs":   validOrgsList,
	})
}

func (h *UIHandlers) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	orgs := r.Form["orgs"]
	if len(orgs) == 0 {
		http.Error(w, "at least one org is required", http.StatusBadRequest)
		return
	}

	raw := make([]byte, 24)
	rand.Read(raw)
	plaintext := hex.EncodeToString(raw)

	t := &models.Token{
		Name:      name,
		TokenHash: auth.HashToken(plaintext),
		Orgs:      orgs,
	}

	if err := h.db.CreateToken(r.Context(), t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.renderPartial(w, "token_row_new.html", map[string]interface{}{
		"Token":     t,
		"Plaintext": plaintext,
	})
}

func (h *UIHandlers) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	
	if _, err := h.db.RevokeToken(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/ui/tokens")
	w.WriteHeader(http.StatusOK)
}
