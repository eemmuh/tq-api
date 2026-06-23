package worker

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/keepcode/api/internal/job"
)

func openTestStore(t *testing.T) *job.Store {
	t.Helper()

	store, err := job.OpenStore(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPoolRetriesFetchUntilSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijack not supported")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	store := openTestStore(t)
	retry := RetryConfig{BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	pool := NewPoolWithRetry(store, 1, 4, retry)
	pool.Run(t.Context())

	created, err := store.Create("fetch", map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	pool.Enqueue(created.ID)

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		switch got.Status {
		case job.StatusCompleted:
			if got.Attempts != 2 {
				t.Fatalf("attempts = %d, want 2", got.Attempts)
			}
			if calls.Load() != 3 {
				t.Fatalf("fetch calls = %d, want 3", calls.Load())
			}
			return
		case job.StatusFailed:
			t.Fatalf("job failed permanently: %s", got.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete, last status=%q attempts=%d", got.Status, got.Attempts)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPoolDoesNotRetryValidationErrors(t *testing.T) {
	store := openTestStore(t)
	pool := NewPoolWithRetry(store, 1, 4, RetryConfig{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	pool.Run(t.Context())

	created, err := store.Create("hash", map[string]any{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	pool.Enqueue(created.ID)

	deadline := time.Now().Add(time.Second)
	for {
		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Status == job.StatusFailed {
			if got.Attempts != 0 {
				t.Fatalf("attempts = %d, want 0", got.Attempts)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not fail, status=%q", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
