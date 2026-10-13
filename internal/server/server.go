package server

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ejsadiarin/coredraw/internal/auth"
	"github.com/ejsadiarin/coredraw/internal/board"
	dbpkg "github.com/ejsadiarin/coredraw/internal/db"
	dbgen "github.com/ejsadiarin/coredraw/internal/db/sqlc"
)

// Server holds the dependencies for the coredraw API.
type Server struct {
	Port     string
	Pool     *pgxpool.Pool
	Queries  *dbgen.Queries
	Verifier *auth.Verifier
	Boards   *board.Handler
}

// New wires a Server. It panics on a nil verifier: serving without
// identity verification is never an option.
func New(port string, pool *pgxpool.Pool, queries *dbgen.Queries, verifier *auth.Verifier) *Server {
	if verifier == nil {
		panic("server: auth verifier is required")
	}
	return &Server{
		Port:     port,
		Pool:     pool,
		Queries:  queries,
		Verifier: verifier,
		Boards:   &board.Handler{Store: dbpkg.NewPgStore(queries)},
	}
}
