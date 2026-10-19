package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"migrated-app/internal/config"
	"migrated-app/internal/httpapi"
	"migrated-app/internal/model"
	"migrated-app/internal/repository"
	"migrated-app/internal/service"
)

func buildRouter() http.Handler {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	if cfg.DatabaseURL == "" {
		log.Warn().Msg("DATABASE_URL is not set; database operations will fail")
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Error().Err(err).Msg("database ping failed")
	} else if err := model.EnsureSchema(ctx, db); err != nil {
		log.Error().Err(err).Msg("failed to ensure schema")
	}

	repo := repository.NewSQLUserRepository(db)
	svc, err := service.NewUserService(repo)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to build user service")
	}
	userHandler, err := httpapi.NewHandler(svc, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to build http handler")
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	r.Mount("/", userHandler)

	return r
}