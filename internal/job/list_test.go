package job

import "testing"

func TestStoreListPaginationAndFilters(t *testing.T) {
	store := openTestStore(t)

	for i := 0; i < 5; i++ {
		if _, err := store.Create("hash", map[string]any{"text": "a"}); err != nil {
			t.Fatalf("Create hash: %v", err)
		}
	}
	fetchJob, err := store.Create("fetch", map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Create fetch: %v", err)
	}
	if err := store.MarkCompleted(fetchJob.ID, map[string]any{"status_code": 200}); err != nil {
		t.Fatalf("MarkCompleted fetch: %v", err)
	}

	page, err := store.List(ListQuery{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List page 0: %v", err)
	}
	if len(page.Jobs) != 2 || page.Total != 6 || page.Limit != 2 || page.Offset != 0 {
		t.Fatalf("page 0 = %+v, want 2 jobs of 6 total", page)
	}

	page, err = store.List(ListQuery{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(page.Jobs) != 2 || page.Offset != 2 {
		t.Fatalf("page 1 = %+v, want 2 jobs at offset 2", page)
	}

	byType, err := store.List(ListQuery{Limit: 10, Type: "fetch"})
	if err != nil {
		t.Fatalf("List by type: %v", err)
	}
	if byType.Total != 1 || len(byType.Jobs) != 1 || byType.Jobs[0].Type != "fetch" {
		t.Fatalf("by type = %+v, want one fetch job", byType)
	}

	byStatus, err := store.List(ListQuery{Limit: 10, Status: string(StatusCompleted)})
	if err != nil {
		t.Fatalf("List by status: %v", err)
	}
	if byStatus.Total != 1 || byStatus.Jobs[0].ID != fetchJob.ID {
		t.Fatalf("by status = %+v, want completed fetch job", byStatus)
	}
}

func TestValidateListQuery(t *testing.T) {
	q, err := ValidateListQuery(ListQuery{})
	if err != nil {
		t.Fatalf("ValidateListQuery default: %v", err)
	}
	if q.Limit != DefaultListLimit {
		t.Fatalf("default limit = %d, want %d", q.Limit, DefaultListLimit)
	}

	if _, err := ValidateListQuery(ListQuery{Limit: 101}); err == nil {
		t.Fatal("expected error for limit > max")
	}
	if _, err := ValidateListQuery(ListQuery{Limit: 10, Offset: -1}); err == nil {
		t.Fatal("expected error for negative offset")
	}
	if _, err := ValidateListQuery(ListQuery{Limit: 10, Status: "nope"}); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestStoreListEmptyResult(t *testing.T) {
	store := openTestStore(t)

	result, err := store.List(ListQuery{Limit: 10, Type: "fetch"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Jobs == nil {
		t.Fatal("Jobs = nil, want empty slice")
	}
	if len(result.Jobs) != 0 || result.Total != 0 {
		t.Fatalf("result = %+v, want empty page", result)
	}
}
