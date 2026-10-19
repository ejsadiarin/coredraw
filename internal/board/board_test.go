package board

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ejsadiarin/coredraw/internal/auth"
)

type stubStore struct {
	create func(ctx context.Context, userID uuid.UUID, title string) (Board, error)
	list   func(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Board, error)
	get    func(ctx context.Context, userID uuid.UUID, id uuid.UUID) (Board, error)
	save   func(ctx context.Context, userID uuid.UUID, id uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (Board, error)
	rename func(ctx context.Context, userID uuid.UUID, id uuid.UUID, title string) (Board, error)
	del    func(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
}

func (s *stubStore) CreateBoard(ctx context.Context, userID uuid.UUID, title string) (Board, error) {
	return s.create(ctx, userID, title)
}

func (s *stubStore) ListBoards(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Board, error) {
	return s.list(ctx, userID, limit, offset)
}

func (s *stubStore) GetBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) (Board, error) {
	return s.get(ctx, userID, id)
}

func (s *stubStore) SaveBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (Board, error) {
	return s.save(ctx, userID, id, snapshot, version, doc, revision)
}

func (s *stubStore) RenameBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, title string) (Board, error) {
	return s.rename(ctx, userID, id, title)
}

func (s *stubStore) DeleteBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.del(ctx, userID, id)
}

func testBoard(userID uuid.UUID) Board {
	return Board{
		ID:              uuid.New(),
		Title:           "payments",
		Snapshot:        json.RawMessage(`{"store":{}}`),
		SnapshotVersion: 1,
		Doc:             "# payments",
		Revision:        3,
		CreatedAt:       time.Now().UTC().Truncate(time.Second),
		UpdatedAt:       time.Now().UTC().Truncate(time.Second),
	}
}

func testRouter(store Store, userID *uuid.UUID) chi.Router {
	h := &Handler{Store: store}
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		if userID != nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					next.ServeHTTP(w, req.WithContext(auth.WithUserID(req.Context(), *userID, nil)))
				})
			})
		}
		h.Mount(r)
	})
	return r
}

func doRequest(t *testing.T, r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCreateBoard(t *testing.T) {
	userID := uuid.New()
	b := testBoard(userID)
	store := &stubStore{
		create: func(_ context.Context, gotUser uuid.UUID, title string) (Board, error) {
			if gotUser != userID {
				t.Errorf("user = %v, want %v", gotUser, userID)
			}
			if title != "payments" {
				t.Errorf("title = %q", title)
			}
			return b, nil
		},
	}
	rec := doRequest(t, testRouter(store, &userID), http.MethodPost, "/", `{"title":"payments"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var got Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != b.ID || got.Title != "payments" || got.Revision != 3 {
		t.Errorf("board = %+v, want id/title/revision of fixture", got)
	}
}

func TestCreateBoardEmptyTitleRejected(t *testing.T) {
	userID := uuid.New()
	called := false
	store := &stubStore{
		create: func(context.Context, uuid.UUID, string) (Board, error) {
			called = true
			return Board{}, nil
		},
	}
	rec := doRequest(t, testRouter(store, &userID), http.MethodPost, "/", `{"title":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if called {
		t.Errorf("store called with empty title")
	}
}

func TestListBoardsScopedToCaller(t *testing.T) {
	userID := uuid.New()
	mine := testBoard(userID)
	store := &stubStore{
		list: func(_ context.Context, gotUser uuid.UUID, limit, offset int) ([]Board, error) {
			if gotUser != userID {
				t.Errorf("user = %v, want %v", gotUser, userID)
			}
			if limit != 50 || offset != 0 {
				t.Errorf("limit/offset = %d/%d, want defaults 50/0", limit, offset)
			}
			return []Board{mine}, nil
		},
	}
	rec := doRequest(t, testRouter(store, &userID), http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0]["id"] != mine.ID.String() || got[0]["title"] != "payments" {
		t.Errorf("list = %v, want the one board", got)
	}
	if _, hasSnapshot := got[0]["snapshot"]; hasSnapshot {
		t.Errorf("list item carries snapshot, want summary only")
	}
}

func TestGetBoard(t *testing.T) {
	userID := uuid.New()
	b := testBoard(userID)
	store := &stubStore{
		get: func(_ context.Context, gotUser uuid.UUID, id uuid.UUID) (Board, error) {
			if gotUser != userID || id != b.ID {
				return Board{}, ErrNotFound
			}
			return b, nil
		},
	}
	r := testRouter(store, &userID)

	rec := doRequest(t, r, http.MethodGet, "/"+b.ID.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got.Snapshot) != `{"store":{}}` || got.Doc != "# payments" {
		t.Errorf("board snapshot/doc not returned: %+v", got)
	}

	rec = doRequest(t, r, http.MethodGet, "/"+uuid.New().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("other board: status = %d, want 404 (no cross-user leak)", rec.Code)
	}

	rec = doRequest(t, r, http.MethodGet, "/not-a-uuid", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad uuid: status = %d, want 400", rec.Code)
	}
}

func TestSaveBoardRevisionGuard(t *testing.T) {
	userID := uuid.New()
	b := testBoard(userID)
	updated := b
	updated.Revision = 4
	store := &stubStore{
		save: func(_ context.Context, gotUser uuid.UUID, id uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (Board, error) {
			if gotUser != userID || id != b.ID {
				return Board{}, ErrNotFound
			}
			if string(snapshot) != `{"store":{"v":2}}` || version != 2 || doc != "# v2" {
				t.Errorf("save payload mismatch: %s/%d/%s", snapshot, version, doc)
			}
			if revision != 3 {
				return Board{}, ErrRevisionConflict
			}
			return updated, nil
		},
	}
	r := testRouter(store, &userID)

	rec := doRequest(t, r, http.MethodPut, "/"+b.ID.String(),
		`{"snapshot":{"store":{"v":2}},"snapshot_version":2,"doc":"# v2","revision":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Revision != 4 {
		t.Errorf("revision = %d, want 4", got.Revision)
	}

	rec = doRequest(t, r, http.MethodPut, "/"+b.ID.String(),
		`{"snapshot":{"store":{"v":2}},"snapshot_version":2,"doc":"# v2","revision":2}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("stale revision: status = %d, want 409", rec.Code)
	}

	rec = doRequest(t, r, http.MethodPut, "/"+uuid.New().String(),
		`{"snapshot":{},"snapshot_version":1,"doc":"","revision":1}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing board: status = %d, want 404", rec.Code)
	}
}

// A snapshot_version 2 payload is an Excalidraw scene. The server treats it as
// an opaque blob: it never parses the scene shape, storing and returning the
// bytes unchanged.
func TestSnapshotVersionTwoRoundTripsOpaquely(t *testing.T) {
	userID := uuid.New()
	id := uuid.New()
	const scene = `{"elements":[{"type":"rectangle","id":"abc","x":10,"y":20}],"appState":{"viewBackgroundColor":"#ffffff","gridSize":20,"gridModeEnabled":false}}`

	var stored Board
	store := &stubStore{
		get: func(_ context.Context, gotUser, gotID uuid.UUID) (Board, error) {
			if gotUser != userID || gotID != id {
				return Board{}, ErrNotFound
			}
			return stored, nil
		},
		save: func(_ context.Context, gotUser, gotID uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (Board, error) {
			if gotUser != userID || gotID != id {
				return Board{}, ErrNotFound
			}
			stored = Board{
				ID:              id,
				Title:           "payments",
				Snapshot:        json.RawMessage(append([]byte(nil), snapshot...)),
				SnapshotVersion: version,
				Doc:             doc,
				Revision:        revision + 1,
			}
			return stored, nil
		},
	}
	r := testRouter(store, &userID)

	rec := doRequest(t, r, http.MethodPut, "/"+id.String(),
		`{"snapshot":`+scene+`,"snapshot_version":2,"doc":"# x","revision":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, r, http.MethodGet, "/"+id.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status = %d, want 200", rec.Code)
	}
	var got Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SnapshotVersion != 2 {
		t.Errorf("snapshot_version = %d, want 2", got.SnapshotVersion)
	}
	var want, have any
	if err := json.Unmarshal([]byte(scene), &want); err != nil {
		t.Fatalf("decode scene: %v", err)
	}
	if err := json.Unmarshal(got.Snapshot, &have); err != nil {
		t.Fatalf("decode returned snapshot: %v", err)
	}
	if !reflect.DeepEqual(want, have) {
		t.Errorf("snapshot changed in flight:\n got %s\nwant %s", got.Snapshot, scene)
	}
}

func TestRenameBoard(t *testing.T) {
	userID := uuid.New()
	b := testBoard(userID)
	renamed := b
	renamed.Title = "ledger"
	store := &stubStore{
		rename: func(_ context.Context, gotUser uuid.UUID, id uuid.UUID, title string) (Board, error) {
			if gotUser != userID || id != b.ID {
				return Board{}, ErrNotFound
			}
			if title != "ledger" {
				t.Errorf("title = %q", title)
			}
			return renamed, nil
		},
	}
	r := testRouter(store, &userID)

	rec := doRequest(t, r, http.MethodPatch, "/"+b.ID.String(), `{"title":"ledger"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "ledger" {
		t.Errorf("title = %q, want ledger", got.Title)
	}

	rec = doRequest(t, r, http.MethodPatch, "/"+b.ID.String(), `{"title":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty title: status = %d, want 400", rec.Code)
	}
}

func TestDeleteBoard(t *testing.T) {
	userID := uuid.New()
	id := uuid.New()
	store := &stubStore{
		del: func(_ context.Context, gotUser uuid.UUID, gotID uuid.UUID) error {
			if gotUser != userID || gotID != id {
				return ErrNotFound
			}
			return nil
		},
	}
	r := testRouter(store, &userID)

	if rec := doRequest(t, r, http.MethodDelete, "/"+id.String(), ""); rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if rec := doRequest(t, r, http.MethodDelete, "/"+uuid.New().String(), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing: status = %d, want 404", rec.Code)
	}
}

func TestBoardEndpointsRequireIdentity(t *testing.T) {
	store := &stubStore{}
	r := testRouter(store, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodPost, "/"},
		{http.MethodGet, "/" + uuid.New().String()},
		{http.MethodPut, "/" + uuid.New().String()},
		{http.MethodPatch, "/" + uuid.New().String()},
		{http.MethodDelete, "/" + uuid.New().String()},
	} {
		rec := doRequest(t, r, tc.method, tc.path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
