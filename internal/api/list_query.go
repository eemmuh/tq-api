package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/keepcode/api/internal/job"
)

func parseListQuery(r *http.Request) (job.ListQuery, error) {
	q := job.ListQuery{
		Status: r.URL.Query().Get("status"),
		Type:   r.URL.Query().Get("type"),
	}

	limitRaw := r.URL.Query().Get("limit")
	if limitRaw == "" {
		q.Limit = 0
	} else {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil {
			return job.ListQuery{}, fmt.Errorf("limit must be an integer")
		}
		q.Limit = limit
	}

	offsetRaw := r.URL.Query().Get("offset")
	if offsetRaw == "" {
		q.Offset = 0
	} else {
		offset, err := strconv.Atoi(offsetRaw)
		if err != nil {
			return job.ListQuery{}, fmt.Errorf("offset must be an integer")
		}
		q.Offset = offset
	}

	return job.ValidateListQuery(q)
}
