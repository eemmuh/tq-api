package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/keepcode/api/internal/job"
	"github.com/keepcode/api/internal/worker"
)

func TestCreateAndGetJob(t *testing.T) {
	store := job.NewStore()
	pool := worker.NewPool(store, 1, 4)
	pool.Run(t.Context())

	h := NewHandler(store, pool)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", h.CreateJob)
	mux.HandleFunc("GET /jobs/{id}", h.GetJob)

	body := []byte(`{"type":"hash","payload":{"text":"hello"}}`)
	createReq := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want %d body=%s", createRec.Code, http.StatusAccepted, createRec.Body.String())
	}

	var created job.Job
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Status != job.StatusQueued {
		t.Fatalf("initial status = %q, want queued", created.Status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := getJob(mux, created.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if got.Status == job.StatusCompleted {
			result, ok := got.Result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map", got.Result)
			}
			if result["digest"] != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
				t.Fatalf("unexpected digest: %v", result["digest"])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete, last status=%q", got.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCreateAndGetFetchJob(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	store := job.NewStore()
	pool := worker.NewPool(store, 1, 4)
	pool.Run(t.Context())

	h := NewHandler(store, pool)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", h.CreateJob)
	mux.HandleFunc("GET /jobs/{id}", h.GetJob)

	body := []byte(fmt.Sprintf(`{"type":"fetch","payload":{"url":%q}}`, target.URL))
	createReq := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want %d body=%s", createRec.Code, http.StatusAccepted, createRec.Body.String())
	}

	var created job.Job
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := getJob(mux, created.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if got.Status == job.StatusCompleted {
			result, ok := got.Result.(map[string]any)
			if !ok {
				t.Fatalf("result type = %T, want map", got.Result)
			}
			if result["status_code"] != float64(http.StatusOK) && result["status_code"] != http.StatusOK {
				t.Fatalf("status_code = %v, want 200", result["status_code"])
			}
			if result["body_preview"] != `{"ok":true}` {
				t.Fatalf("body_preview = %q", result["body_preview"])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete, last status=%q", got.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestGetJobNotFound(t *testing.T) {
	store := job.NewStore()
	pool := worker.NewPool(store, 1, 1)
	h := NewHandler(store, pool)

	req := httptest.NewRequest(http.MethodGet, "/jobs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	h.GetJob(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func getJob(mux *http.ServeMux, id string) (*job.Job, error) {
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var got job.Job
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		return nil, err
	}
	return &got, nil
}
