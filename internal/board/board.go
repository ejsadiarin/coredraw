package board

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ejsadiarin/coredraw/internal/helper"
)

// ErrNotFound is returned when a board does not exist for the calling user.
// Callers must not distinguish "missing" from "belongs to someone else".
var ErrNotFound = errors.New("board not found")

// ErrRevisionConflict is returned when a save carries a stale revision.
var ErrRevisionConflict = errors.New("stale board revision")

// Board is a diagram canvas plus its side doc: the unit coredraw saves
// and syncs.
type Board struct {
	ID              uuid.UUID       `json:"id"`
	Title           string          `json:"title"`
	Snapshot        json.RawMessage `json:"snapshot"`
	SnapshotVersion int32           `json:"snapshot_version"`
	Doc             string          `json:"doc"`
	Revision        int64           `json:"revision"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// BoardSummary is the list-view shape: identity and freshness, no payload.
type BoardSummary struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func summarize(b Board) BoardSummary {
	return BoardSummary{
		ID:        b.ID,
		Title:     b.Title,
		Revision:  b.Revision,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

// Store is the persistence seam for boards. Every method is implicitly
// scoped to the given user.
type Store interface {
	CreateBoard(ctx context.Context, userID uuid.UUID, title string) (Board, error)
	ListBoards(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Board, error)
	GetBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) (Board, error)
	SaveBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, snapshot []byte, version int32, doc string, revision int64) (Board, error)
	RenameBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID, title string) (Board, error)
	DeleteBoard(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
}

// Handler serves the boards API. Mount it under the versioned API group.
type Handler struct {
	Store Store
}

// Mount registers the boards routes on r.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.save)
	r.Patch("/{id}", h.rename)
	r.Delete("/{id}", h.remove)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	limit := helper.ParseQueryInt(r, "limit", 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	offset := helper.ParseQueryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	boards, err := h.Store.ListBoards(r.Context(), userID, limit, offset)
	if err != nil {
		helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to list boards")
		return
	}
	summaries := make([]BoardSummary, 0, len(boards))
	for _, b := range boards {
		summaries = append(summaries, summarize(b))
	}
	helper.RespondJSON(w, http.StatusOK, summaries)
}

type createRequest struct {
	Title string `json:"title"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.RespondErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		helper.RespondErrorJSON(w, http.StatusBadRequest, "title is required")
		return
	}
	b, err := h.Store.CreateBoard(r.Context(), userID, req.Title)
	if err != nil {
		helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to create board")
		return
	}
	helper.RespondJSON(w, http.StatusCreated, b)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	id, ok := helper.ParseUUID(w, r, "id")
	if !ok {
		return
	}
	b, err := h.Store.GetBoard(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			helper.RespondErrorJSON(w, http.StatusNotFound, "board not found")
			return
		}
		helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to fetch board")
		return
	}
	helper.RespondJSON(w, http.StatusOK, b)
}

type saveRequest struct {
	Snapshot        json.RawMessage `json:"snapshot"`
	SnapshotVersion int32           `json:"snapshot_version"`
	Doc             string          `json:"doc"`
	Revision        int64           `json:"revision"`
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	id, ok := helper.ParseUUID(w, r, "id")
	if !ok {
		return
	}
	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.RespondErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b, err := h.Store.SaveBoard(r.Context(), userID, id, req.Snapshot, req.SnapshotVersion, req.Doc, req.Revision)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			helper.RespondErrorJSON(w, http.StatusNotFound, "board not found")
		case errors.Is(err, ErrRevisionConflict):
			helper.RespondErrorJSON(w, http.StatusConflict, "stale revision: refresh and retry")
		default:
			helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to save board")
		}
		return
	}
	helper.RespondJSON(w, http.StatusOK, b)
}

type renameRequest struct {
	Title string `json:"title"`
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	id, ok := helper.ParseUUID(w, r, "id")
	if !ok {
		return
	}
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		helper.RespondErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		helper.RespondErrorJSON(w, http.StatusBadRequest, "title is required")
		return
	}
	b, err := h.Store.RenameBoard(r.Context(), userID, id, req.Title)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			helper.RespondErrorJSON(w, http.StatusNotFound, "board not found")
			return
		}
		helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to rename board")
		return
	}
	helper.RespondJSON(w, http.StatusOK, b)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	userID, ok := helper.GetUserID(w, r)
	if !ok {
		return
	}
	id, ok := helper.ParseUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Store.DeleteBoard(r.Context(), userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			helper.RespondErrorJSON(w, http.StatusNotFound, "board not found")
			return
		}
		helper.RespondErrorJSON(w, http.StatusInternalServerError, "failed to delete board")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
