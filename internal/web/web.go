package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/db"
)

//go:embed templates/*.html static/*
var content embed.FS

type UIHandlers struct {
	db    *db.DB
	authz *auth.Authorizer
}

func Setup(mux *http.ServeMux, database *db.DB, authorizer *auth.Authorizer) {

	h := &UIHandlers{
		db:    database,
		authz: authorizer,
	}

	staticFS, _ := fs.Sub(content, "static")
	mux.Handle("GET /ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.FS(staticFS))))

	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/memories", http.StatusFound)
	})

	mux.HandleFunc("GET /ui/login", h.handleLoginGet)
	mux.HandleFunc("POST /ui/login", h.handleLoginPost)
	mux.HandleFunc("POST /ui/logout", h.handleLogout)

	mux.HandleFunc("GET /ui/memories", h.requireAdmin(h.handleMemoriesGet))
	mux.HandleFunc("GET /ui/memories/list", h.requireAdmin(h.handleMemoriesList))
	mux.HandleFunc("POST /ui/memories/{id}/delete", h.requireAdmin(h.handleMemoryDelete))
	mux.HandleFunc("GET /ui/memories/{id}/edit", h.requireAdmin(h.handleMemoryEditGet))
	mux.HandleFunc("POST /ui/memories/{id}/edit", h.requireAdmin(h.handleMemoryEditPost))

	mux.HandleFunc("GET /ui/tokens", h.requireAdmin(h.handleTokensGet))
	mux.HandleFunc("POST /ui/tokens", h.requireAdmin(h.handleTokenCreate))
	mux.HandleFunc("POST /ui/tokens/{id}/revoke", h.requireAdmin(h.handleTokenRevoke))
}

func (h *UIHandlers) render(w http.ResponseWriter, name string, data interface{}) {
	t := template.Must(template.New("layout.html").Funcs(template.FuncMap{
		"hasOrg": func(orgs []string, target string) bool {
			for _, o := range orgs {
				if o == target {
					return true
				}
			}
			return false
		},
		"join": strings.Join,
		"map": func(key string, value interface{}) map[string]interface{} {
			return map[string]interface{}{key: value}
		},
	}).ParseFS(content, "templates/layout.html", "templates/"+name, "templates/memory_row.html", "templates/token_row.html"))
	err := t.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *UIHandlers) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	t := template.Must(template.New(name).Funcs(template.FuncMap{
		"hasOrg": func(orgs []string, target string) bool {
			for _, o := range orgs {
				if o == target {
					return true
				}
			}
			return false
		},
		"join": strings.Join,
		"map": func(key string, value interface{}) map[string]interface{} {
			return map[string]interface{}{key: value}
		},
	}).ParseFS(content, "templates/memory_row.html", "templates/token_row.html", "templates/token_row_new.html", "templates/"+name))
	err := t.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *UIHandlers) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.authz.AdminEnabled() {
			w.WriteHeader(http.StatusServiceUnavailable)
			h.render(w, "admin_disabled.html", nil)
			return
		}
		cookie, err := r.Cookie("am_admin_token")
		if err != nil || cookie.Value == "" || !h.authz.IsAdmin(cookie.Value) {
			http.Redirect(w, r, "/ui/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (h *UIHandlers) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	if !h.authz.AdminEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		h.render(w, "admin_disabled.html", nil)
		return
	}
	cookie, err := r.Cookie("am_admin_token")
	if err == nil && cookie.Value != "" && h.authz.IsAdmin(cookie.Value) {
		http.Redirect(w, r, "/ui/memories", http.StatusFound)
		return
	}
	h.render(w, "login.html", map[string]interface{}{"Auth": false})
}

func (h *UIHandlers) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if !h.authz.AdminEnabled() {
		http.Error(w, "Admin disabled", http.StatusServiceUnavailable)
		return
	}
	token := r.FormValue("token")
	if !h.authz.IsAdmin(token) {
		h.render(w, "login.html", map[string]interface{}{
			"Auth":  false,
			"Error": "Invalid admin token",
		})
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "am_admin_token",
		Value:    token,
		Path:     "/ui",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/ui/memories", http.StatusFound)
}

func (h *UIHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "am_admin_token",
		Value:    "",
		Path:     "/ui",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/ui/login", http.StatusFound)
}
