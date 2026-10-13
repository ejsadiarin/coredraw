package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ejsadiarin/coredraw/internal/board"
	dbgen "github.com/ejsadiarin/coredraw/internal/db/sqlc"
)

var (
	integrationPool  *pgxpool.Pool
	integrationSetup sync.Once
	integrationErr   error
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}
	integrationSetup.Do(func() {
		integrationErr = setupIntegrationPool()
	})
	if integrationErr != nil {
		panic(integrationErr)
	}
	os.Exit(m.Run())
}

func setupIntegrationPool() error {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("coredb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		return err
	}
	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return err
	}
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		return err
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return err
	}
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	integrationPool = pool
	return nil
}

func testStore(t *testing.T) *PgStore {
	t.Helper()
	if testing.Short() {
		t.Skip("docker-backed integration test")
	}
	return NewPgStore(dbgen.New(integrationPool))
}

func TestPgStoreBoardRoundTrip_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("docker-backed integration test")
	}
	ctx := context.Background()
	store := testStore(t)
	userID := uuid.New()

	created, err := store.CreateBoard(ctx, userID, "payments")
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}
	if created.Revision != 1 || created.Title != "payments" {
		t.Errorf("created = %+v, want title/revision payments/1", created)
	}

	got, err := store.GetBoard(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if string(got.Snapshot) != "{}" || got.Doc != "" || got.SnapshotVersion != 1 {
		t.Errorf("defaults = %q/%q/%d, want {}/\"\"/1", got.Snapshot, got.Doc, got.SnapshotVersion)
	}

	if _, err := store.GetBoard(ctx, uuid.New(), created.ID); err != board.ErrNotFound {
		t.Errorf("other user GetBoard: err = %v, want ErrNotFound", err)
	}
}

func TestPgStoreSaveRevisionGuard_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("docker-backed integration test")
	}
	ctx := context.Background()
	store := testStore(t)
	userID := uuid.New()

	created, err := store.CreateBoard(ctx, userID, "ledger")
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}

	snap := json.RawMessage(`{"store":{"v":2}}`)
	updated, err := store.SaveBoard(ctx, userID, created.ID, snap, 2, "# v2", created.Revision)
	if err != nil {
		t.Fatalf("SaveBoard: %v", err)
	}
	if updated.Revision != created.Revision+1 {
		t.Errorf("revision = %d, want %d", updated.Revision, created.Revision+1)
	}
	var gotSnap, wantSnap map[string]any
	if err := json.Unmarshal(updated.Snapshot, &gotSnap); err != nil {
		t.Fatalf("saved snapshot is not JSON: %v", err)
	}
	if err := json.Unmarshal(snap, &wantSnap); err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	if fmt.Sprint(gotSnap) != fmt.Sprint(wantSnap) || updated.Doc != "# v2" || updated.SnapshotVersion != 2 {
		t.Errorf("payload not persisted: %s/%s/%d", updated.Snapshot, updated.Doc, updated.SnapshotVersion)
	}

	if _, err := store.SaveBoard(ctx, userID, created.ID, snap, 2, "# stale", created.Revision); err != board.ErrRevisionConflict {
		t.Errorf("stale save: err = %v, want ErrRevisionConflict", err)
	}

	if _, err := store.SaveBoard(ctx, userID, uuid.New(), snap, 2, "", 1); err != board.ErrNotFound {
		t.Errorf("missing save: err = %v, want ErrNotFound", err)
	}

	if _, err := store.SaveBoard(ctx, uuid.New(), created.ID, snap, 2, "", updated.Revision); err != board.ErrNotFound {
		t.Errorf("other-user save: err = %v, want ErrNotFound (indistinguishable from missing)", err)
	}
}

func TestPgStoreListRenameDelete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("docker-backed integration test")
	}
	ctx := context.Background()
	store := testStore(t)
	userID := uuid.New()
	otherID := uuid.New()

	first, err := store.CreateBoard(ctx, userID, "first")
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}
	if _, err := store.CreateBoard(ctx, otherID, "other"); err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}

	boards, err := store.ListBoards(ctx, userID, 50, 0)
	if err != nil {
		t.Fatalf("ListBoards: %v", err)
	}
	if len(boards) != 1 || boards[0].ID != first.ID {
		t.Errorf("list scoped = %d boards, want only the caller's one", len(boards))
	}

	renamed, err := store.RenameBoard(ctx, userID, first.ID, "renamed")
	if err != nil {
		t.Fatalf("RenameBoard: %v", err)
	}
	if renamed.Title != "renamed" {
		t.Errorf("title = %q, want renamed", renamed.Title)
	}

	if err := store.DeleteBoard(ctx, userID, first.ID); err != nil {
		t.Fatalf("DeleteBoard: %v", err)
	}
	if _, err := store.GetBoard(ctx, userID, first.ID); err != board.ErrNotFound {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
	if err := store.DeleteBoard(ctx, userID, first.ID); err != board.ErrNotFound {
		t.Errorf("second delete: err = %v, want ErrNotFound", err)
	}
}
