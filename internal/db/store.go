package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ejsadiarin/coredraw/internal/board"
	dbgen "github.com/ejsadiarin/coredraw/internal/db/sqlc"
)

// PgStore is the Postgres-backed board.Store. All scoping is enforced by
// the queries themselves (every statement filters on user_id).
type PgStore struct {
	q dbgen.Querier
}

var _ board.Store = (*PgStore)(nil)

// NewPgStore wraps generated queries. Pass sqlc.New(pool) in production,
// or any Querier implementation in tests.
func NewPgStore(q dbgen.Querier) *PgStore {
	return &PgStore{q: q}
}

func toBoard(m dbgen.Board) board.Board {
	return board.Board{
		ID:              m.ID,
		Title:           m.Title,
		Snapshot:        json.RawMessage(append([]byte(nil), m.Snapshot...)),
		SnapshotVersion: m.SnapshotVersion,
		Doc:             m.DocMd,
		Revision:        m.Revision,
		CreatedAt:       m.CreatedAt.Time,
		UpdatedAt:       m.UpdatedAt.Time,
	}
}

func (s *PgStore) CreateBoard(ctx context.Context, userID uuid.UUID, title string) (board.Board, error) {
	row, err := s.q.CreateBoard(ctx, dbgen.CreateBoardParams{
		ID:     uuid.New(),
		UserID: userID,
		Title:  title,
	})
	if err != nil {
		return board.Board{}, err
	}
	return toBoard(row), nil
}

func (s *PgStore) ListBoards(ctx context.Context, userID uuid.UUID, limit, offset int) ([]board.Board, error) {
	rows, err := s.q.ListBoards(ctx, dbgen.ListBoardsParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	boards := make([]board.Board, 0, len(rows))
	for _, r := range rows {
		boards = append(boards, board.Board{
			ID:        r.ID,
			Title:     r.Title,
			Revision:  r.Revision,
			CreatedAt: r.CreatedAt.Time,
			UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return boards, nil
}

func (s *PgStore) GetBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) (board.Board, error) {
	row, err := s.q.GetBoard(ctx, dbgen.GetBoardParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return board.Board{}, board.ErrNotFound
		}
		return board.Board{}, err
	}
	return toBoard(row), nil
}

func (s *PgStore) SaveBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (board.Board, error) {
	row, err := s.q.SaveBoard(ctx, dbgen.SaveBoardParams{
		ID:              id,
		UserID:          userID,
		Column3:         snapshot, // $3::jsonb: the snapshot payload (sqlc cannot name cast params)
		SnapshotVersion: version,
		DocMd:           doc,
		Revision:        revision,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return board.Board{}, err
		}
		// No row matched: either the board is missing for this user, or
		// the revision is stale. Disambiguate with a read.
		if _, getErr := s.q.GetBoard(ctx, dbgen.GetBoardParams{ID: id, UserID: userID}); getErr != nil {
			return board.Board{}, board.ErrNotFound
		}
		return board.Board{}, board.ErrRevisionConflict
	}
	return toBoard(row), nil
}

func (s *PgStore) RenameBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, title string) (board.Board, error) {
	row, err := s.q.RenameBoard(ctx, dbgen.RenameBoardParams{ID: id, UserID: userID, Title: title})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return board.Board{}, board.ErrNotFound
		}
		return board.Board{}, err
	}
	return toBoard(row), nil
}

func (s *PgStore) DeleteBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	n, err := s.q.DeleteBoard(ctx, dbgen.DeleteBoardParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return board.ErrNotFound
	}
	return nil
}
