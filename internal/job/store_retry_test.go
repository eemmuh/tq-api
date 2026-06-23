package job

import (
	"context"
	"testing"
	"time"
)

func TestStoreScheduleRetry(t *testing.T) {
	store := openTestStore(t)

	created, err := store.Create("fetch", map[string]any{"url": "http://example.com"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.MarkProcessing(created.ID); err != nil {
		t.Fatalf("MarkProcessing() error = %v", err)
	}

	retryAt := time.Now().UTC().Add(2 * time.Second)
	if err := store.ScheduleRetry(created.ID, "fetch request failed", retryAt); err != nil {
		t.Fatalf("ScheduleRetry() error = %v", err)
	}

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != StatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
	if got.Error != "fetch request failed" {
		t.Fatalf("error = %q", got.Error)
	}
	if got.NextRetryAt == nil || !got.NextRetryAt.Equal(retryAt) {
		t.Fatalf("next_retry_at = %v, want %v", got.NextRetryAt, retryAt)
	}
	if got.StartedAt != nil {
		t.Fatalf("started_at = %v, want nil", got.StartedAt)
	}

	if err := store.MarkProcessing(created.ID); err == nil {
		t.Fatal("MarkProcessing() before retry time should fail")
	}
}

func TestStoreListDelayedRetries(t *testing.T) {
	store := openTestStore(t)

	ready, err := store.Create("hash", map[string]any{"text": "now"})
	if err != nil {
		t.Fatalf("Create ready job: %v", err)
	}
	delayed, err := store.Create("fetch", map[string]any{"url": "http://example.com"})
	if err != nil {
		t.Fatalf("Create delayed job: %v", err)
	}

	if err := store.MarkProcessing(delayed.ID); err != nil {
		t.Fatalf("MarkProcessing() error = %v", err)
	}
	retryAt := time.Now().UTC().Add(time.Minute)
	if err := store.ScheduleRetry(delayed.ID, "temporary", retryAt); err != nil {
		t.Fatalf("ScheduleRetry() error = %v", err)
	}

	pending, err := store.RestartPending(context.Background())
	if err != nil {
		t.Fatalf("RestartPending() error = %v", err)
	}
	if len(pending) != 1 || pending[0] != ready.ID {
		t.Fatalf("RestartPending() = %v, want only ready job", pending)
	}

	retries, err := store.ListDelayedRetries(context.Background())
	if err != nil {
		t.Fatalf("ListDelayedRetries() error = %v", err)
	}
	if len(retries) != 1 || retries[0].ID != delayed.ID {
		t.Fatalf("ListDelayedRetries() = %+v, want delayed job", retries)
	}
}
