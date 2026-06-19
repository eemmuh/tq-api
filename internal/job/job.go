package job

import "time"

type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Job struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload,omitempty"`
	Status     Status         `json:"status"`
	Result     any            `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}
