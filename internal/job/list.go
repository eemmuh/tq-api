package job

import (
	"errors"
	"fmt"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 100
)

type ListQuery struct {
	Limit  int
	Offset int
	Status string
	Type   string
}

type ListResult struct {
	Jobs   []*Job
	Total  int
	Limit  int
	Offset int
}

func ValidStatus(s Status) bool {
	switch s {
	case StatusQueued, StatusProcessing, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

func ValidateListQuery(q ListQuery) (ListQuery, error) {
	if q.Limit == 0 {
		q.Limit = DefaultListLimit
	}
	if q.Limit < 1 || q.Limit > MaxListLimit {
		return ListQuery{}, fmt.Errorf("limit must be between 1 and %d", MaxListLimit)
	}
	if q.Offset < 0 {
		return ListQuery{}, errors.New("offset must be >= 0")
	}
	if q.Status != "" && !ValidStatus(Status(q.Status)) {
		return ListQuery{}, fmt.Errorf("invalid status %q", q.Status)
	}
	return q, nil
}
