package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from fetch"))
	}))
	defer srv.Close()

	client := srv.Client()
	result, err := runFetch(t.Context(), client, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("runFetch() error = %v", err)
	}

	got, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	if got["status_code"] != http.StatusOK {
		t.Fatalf("status_code = %v, want 200", got["status_code"])
	}
	if got["bytes"] != len("hello from fetch") {
		t.Fatalf("bytes = %v, want %d", got["bytes"], len("hello from fetch"))
	}
	if got["body_preview"] != "hello from fetch" {
		t.Fatalf("body_preview = %q", got["body_preview"])
	}
	if got["truncated"] != false {
		t.Fatalf("truncated = %v, want false", got["truncated"])
	}
}

func TestRunFetchInvalidURL(t *testing.T) {
	client := &http.Client{Timeout: time.Second}

	_, err := runFetch(t.Context(), client, map[string]any{"url": "ftp://example.com"})
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("runFetch() error = %v, want scheme validation error", err)
	}
}

func TestRunFetchRequestFailure(t *testing.T) {
	client := &http.Client{Timeout: time.Second}

	_, err := runFetch(t.Context(), client, map[string]any{"url": "http://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "fetch request failed") {
		t.Fatalf("runFetch() error = %v, want request failure", err)
	}
}

func TestRunFetchTruncatesLargeBody(t *testing.T) {
	largeBody := strings.Repeat("a", maxFetchBodyBytes+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(largeBody))
	}))
	defer srv.Close()

	client := srv.Client()
	result, err := runFetch(context.Background(), client, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("runFetch() error = %v", err)
	}

	got := result.(map[string]any)
	if got["truncated"] != true {
		t.Fatalf("truncated = %v, want true", got["truncated"])
	}
	if got["bytes"] != maxFetchBodyBytes {
		t.Fatalf("bytes = %v, want %d", got["bytes"], maxFetchBodyBytes)
	}
}

func TestRunFetchRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	client := srv.Client()
	_, err := runFetch(ctx, client, map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("runFetch() expected context error, got nil")
	}
}
