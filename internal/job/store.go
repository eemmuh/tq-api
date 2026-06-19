package job

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("job not found")

type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewStore() *Store {
	return &Store{jobs: make(map[string]*Job)}
}

func (s *Store) Create(jobType string, payload map[string]any) *Job {
	now := time.Now().UTC()
	j := &Job{
		ID:        newID(),
		Type:      jobType,
		Payload:   payload,
		Status:    StatusQueued,
		CreatedAt: now,
	}

	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()

	return clone(j)
}

func (s *Store) Get(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	j, ok := s.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(j), nil
}

func (s *Store) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, clone(j))
	}
	return out
}

func (s *Store) MarkProcessing(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now().UTC()
	j.Status = StatusProcessing
	j.StartedAt = &now
	return nil
}

func (s *Store) MarkCompleted(id string, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now().UTC()
	j.Status = StatusCompleted
	j.Result = result
	j.FinishedAt = &now
	return nil
}

func (s *Store) MarkFailed(id string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now().UTC()
	j.Status = StatusFailed
	j.Error = errMsg
	j.FinishedAt = &now
	return nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func clone(j *Job) *Job {
	cp := *j
	if j.Payload != nil {
		cp.Payload = make(map[string]any, len(j.Payload))
		for k, v := range j.Payload {
			cp.Payload[k] = v
		}
	}
	return &cp
}
