package helper

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ejsadiarin/coredraw/internal/auth"
)

func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			slog.Info("failed to encode response", "error", err.Error())
		}
	}
}

func RespondErrorJSON(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}

func ParseUUID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		RespondErrorJSON(w, http.StatusBadRequest, "invalid "+param)
		return uuid.Nil, false
	}
	return id, true
}

func ParseQueryInt(r *http.Request, key string, defaultVal int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

// GetUserID resolves the verified caller identity. It writes 401 when no
// identity was established and the handler must stop.
func GetUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		RespondErrorJSON(w, http.StatusUnauthorized, "valid internal credentials are required")
		return uuid.Nil, false
	}
	return id, true
}

// ResolveTargetUser resolves act_as delegation: an admin-scoped caller with
// a signed act_as claim reads as the target, everyone else reads as
// themselves. The raw query param is never trusted.
func ResolveTargetUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if target, ok := auth.ActAsFromContext(r.Context()); ok && auth.HasScope(r.Context(), "admin") {
		return target, true
	}
	return GetUserID(w, r)
}
