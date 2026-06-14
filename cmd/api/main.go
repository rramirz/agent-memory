package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rramirz/agent-memory/internal/auth"
	"github.com/rramirz/agent-memory/internal/config"
	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/handlers"
	"github.com/rramirz/agent-memory/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadAPIConfig()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	tokens, err := auth.NewTokenStore(cfg.Tokens)
	if err != nil {
		log.Error("load token store", "error", err)
		os.Exit(1)
	}

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()

	database, err := db.Connect(connectCtx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		log.Error("connect mongodb", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		if err := database.Close(shutCtx); err != nil {
			log.Error("disconnect mongodb", "error", err)
		}
	}()

	if err := database.EnsureIndexes(connectCtx); err != nil {
		log.Warn("ensure indexes", "error", err)
	}

	authz := auth.NewAuthorizer(tokens, database, cfg.AdminToken)

	memH := handlers.NewMemoryHandlers(database, authz)
	ctxH := handlers.NewContextHandlers(database, authz)
	admH := handlers.NewAdminHandlers(database, authz)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/healthz", handlers.Health)
	mux.HandleFunc("POST /v1/memories", memH.CreateMemory)
	mux.HandleFunc("GET /v1/memories/search", memH.SearchMemories)
	mux.HandleFunc("PATCH /v1/memories/{id}", memH.UpdateMemory)
	mux.HandleFunc("DELETE /v1/memories/{id}", memH.DeleteMemory)
	mux.HandleFunc("GET /v1/context", ctxH.GetContext)
	mux.HandleFunc("POST /v1/admin/tokens", admH.CreateToken)
	mux.HandleFunc("GET /v1/admin/tokens", admH.ListTokens)
	mux.HandleFunc("DELETE /v1/admin/tokens/{id}", admH.RevokeToken)

	web.Setup(mux, database, authz)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("starting memory-api", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
}
