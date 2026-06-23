package job

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "jobs.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestStoreCreateGetAndList(t *testing.T) {
	store := openTestStore(t)

	created, err := store.Create("hash", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Type != "hash" || got.Status != StatusQueued {
		t.Fatalf("Get() = %+v, want queued hash job", got)
	}

	if err := store.MarkCompleted(created.ID, map[string]string{"digest": "abc"}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	result, err := store.List(ListQuery{Limit: DefaultListLimit})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("List() len = %d, want 1", len(result.Jobs))
	}
	if result.Jobs[0].Status != StatusCompleted {
		t.Fatalf("List()[0].Status = %q, want completed", result.Jobs[0].Status)
	}
	if result.Total != 1 {
		t.Fatalf("List() total = %d, want 1", result.Total)
	}
}

func TestStoreRestartPending(t *testing.T) {
	store := openTestStore(t)

	queued, err := store.Create("sleep", map[string]any{"seconds": 1})
	if err != nil {
		t.Fatalf("Create queued job: %v", err)
	}
	processing, err := store.Create("hash", map[string]any{"text": "x"})
	if err != nil {
		t.Fatalf("Create processing job: %v", err)
	}
	if err := store.MarkProcessing(processing.ID); err != nil {
		t.Fatalf("MarkProcessing() error = %v", err)
	}
	completed, err := store.Create("hash", map[string]any{"text": "done"})
	if err != nil {
		t.Fatalf("Create completed job: %v", err)
	}
	if err := store.MarkCompleted(completed.ID, map[string]string{"digest": "done"}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	ids, err := store.RestartPending(context.Background())
	if err != nil {
		t.Fatalf("RestartPending() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("RestartPending() len = %d, want 2", len(ids))
	}

	restarted, err := store.Get(processing.ID)
	if err != nil {
		t.Fatalf("Get processing job: %v", err)
	}
	if restarted.Status != StatusQueued {
		t.Fatalf("processing job status = %q, want queued", restarted.Status)
	}
	if restarted.StartedAt != nil {
		t.Fatalf("processing job started_at = %v, want nil", restarted.StartedAt)
	}

	stillQueued, err := store.Get(queued.ID)
	if err != nil {
		t.Fatalf("Get queued job: %v", err)
	}
	if stillQueued.Status != StatusQueued {
		t.Fatalf("queued job status = %q, want queued", stillQueued.Status)
	}

	done, err := store.Get(completed.ID)
	if err != nil {
		t.Fatalf("Get completed job: %v", err)
	}
	if done.Status != StatusCompleted {
		t.Fatalf("completed job status = %q, want completed", done.Status)
	}
}

func TestStoreSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	created, err := store.Create("hash", map[string]any{"text": "persist"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.MarkCompleted(created.ID, map[string]string{"digest": "persisted"}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() after reopen error = %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	result, ok := got.Result.(map[string]any)
	if !ok || result["digest"] != "persisted" {
		t.Fatalf("result = %#v, want persisted digest", got.Result)
	}
}
