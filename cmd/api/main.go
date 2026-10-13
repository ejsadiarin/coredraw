package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ejsadiarin/coredraw/internal/auth"
	dbgen "github.com/ejsadiarin/coredraw/internal/db/sqlc"
	"github.com/ejsadiarin/coredraw/internal/server"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func buildPool(ctx context.Context, databaseURL string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		slog.Error("invalid DATABASE_URL", "error", err)
		panic("coredraw: invalid DATABASE_URL")
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		slog.Error("failed to create pool", "error", err)
		panic("coredraw: failed to create pool")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("failed to ping database", "error", err)
		panic("coredraw: failed to ping database")
	}
	return pool
}

func main() {
	ctx := context.Background()
	port := getenv("PORT", "6971")

	pool := buildPool(ctx, os.Getenv("DATABASE_URL"))
	defer pool.Close()
	queries := dbgen.New(pool)

	verifier, err := auth.NewVerifier(auth.Config{
		Issuer:   getenv("JWT_ISSUER", "https://gateway.internal"),
		Audience: getenv("JWT_AUDIENCE", "coredraw"),
		JWKSURL:  getenv("JWT_JWKS_URL", "https://gateway.internal/.well-known/jwks.json"),
	})
	if err != nil {
		slog.Error("failed to establish JWT verification", "error", err)
		os.Exit(1)
	}

	srv := server.New(port, pool, queries, verifier)
	slog.Info("coredraw-api listening", "port", port)
	if err := http.ListenAndServe(":"+port, srv.Routes()); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
