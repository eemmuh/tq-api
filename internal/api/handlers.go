package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/keepcode/api/internal/job"
	"github.com/keepcode/api/internal/worker"
)

type Handler struct {
	store *job.Store
	pool  *worker.Pool
}

func NewHandler(store *job.Store, pool *worker.Pool) *Handler {
	return &Handler{store: store, pool: pool}
}

type createJobRequest struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}

	j, err := h.store.Create(req.Type, req.Payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}
	h.pool.Enqueue(j.ID)

	writeJSON(w, http.StatusAccepted, j)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, err := h.store.Get(id)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load job")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
