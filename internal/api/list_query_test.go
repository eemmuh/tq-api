package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keepcode/api/internal/job"
)

func TestListJobsPaginationAndFilters(t *testing.T) {
	store := openTestStore(t)
	h := NewHandler(store, nil)

	for range 3 {
		if _, err := store.Create("hash", map[string]any{"text": "a"}); err != nil {
			t.Fatalf("Create hash: %v", err)
		}
	}
	if _, err := store.Create("fetch", map[string]any{"url": "https://example.com"}); err != nil {
		t.Fatalf("Create fetch: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	h.ListJobs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Jobs   []job.Job `json:"jobs"`
		Total  int       `json:"total"`
		Limit  int       `json:"limit"`
		Offset int       `json:"offset"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 2 || resp.Total != 4 || resp.Limit != 2 || resp.Offset != 1 {
		t.Fatalf("response = %+v, want 2 jobs at offset 1 of 4 total", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/jobs?type=fetch", nil)
	rec = httptest.NewRecorder()
	h.ListJobs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("filter status = %d, want 200", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode filter response: %v", err)
	}
	if resp.Total != 1 || len(resp.Jobs) != 1 || resp.Jobs[0].Type != "fetch" {
		t.Fatalf("filter response = %+v, want one fetch job", resp)
	}
}

func TestListJobsValidationErrors(t *testing.T) {
	h := NewHandler(openTestStore(t), nil)

	tests := []struct {
		query string
	}{
		{query: "limit=abc"},
		{query: "offset=abc"},
		{query: "limit=101"},
		{query: "offset=-1"},
		{query: "status=unknown"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, "/jobs?"+tc.query, nil)
		rec := httptest.NewRecorder()
		h.ListJobs(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET /jobs?%s status = %d, want 400", tc.query, rec.Code)
		}
	}
}
