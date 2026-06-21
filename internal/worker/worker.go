package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/keepcode/api/internal/job"
)

type Pool struct {
	store      *job.Store
	queue      chan string
	workers    int
	httpClient *http.Client
}

func NewPool(store *job.Store, workers int, queueSize int) *Pool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = workers * 8
	}
	return &Pool{
		store:   store,
		queue:   make(chan string, queueSize),
		workers: workers,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Pool) Enqueue(id string) {
	p.queue <- id
}

func (p *Pool) Run(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		go p.loop(ctx, i)
	}
}

func (p *Pool) loop(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-p.queue:
			p.process(ctx, workerID, id)
		}
	}
}

func (p *Pool) process(ctx context.Context, workerID int, id string) {
	if err := p.store.MarkProcessing(id); err != nil {
		log.Printf("worker %d: job %s missing: %v", workerID, id, err)
		return
	}

	j, err := p.store.Get(id)
	if err != nil {
		log.Printf("worker %d: job %s reload failed: %v", workerID, id, err)
		return
	}

	log.Printf("worker %d: processing job %s type=%s", workerID, j.ID, j.Type)

	result, err := execute(ctx, p.httpClient, j)
	if err != nil {
		if markErr := p.store.MarkFailed(id, err.Error()); markErr != nil {
			log.Printf("worker %d: mark failed for %s: %v", workerID, id, markErr)
		}
		log.Printf("worker %d: job %s failed: %v", workerID, id, err)
		return
	}

	if err := p.store.MarkCompleted(id, result); err != nil {
		log.Printf("worker %d: mark completed for %s: %v", workerID, id, err)
		return
	}
	log.Printf("worker %d: job %s completed", workerID, id)
}

func execute(ctx context.Context, client *http.Client, j *job.Job) (any, error) {
	switch j.Type {
	case "sleep":
		return runSleep(ctx, j.Payload)
	case "hash":
		return runHash(j.Payload)
	case "fetch":
		return runFetch(ctx, client, j.Payload)
	default:
		return nil, fmt.Errorf("unknown job type %q (supported: sleep, hash, fetch)", j.Type)
	}
}

func runSleep(ctx context.Context, payload map[string]any) (any, error) {
	seconds, err := payloadFloat(payload, "seconds", 1)
	if err != nil {
		return nil, err
	}
	if seconds <= 0 {
		return nil, fmt.Errorf("sleep.seconds must be positive")
	}
	if seconds > 30 {
		return nil, fmt.Errorf("sleep.seconds must be <= 30")
	}

	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return map[string]any{"slept_seconds": seconds}, nil
	}
}

func runHash(payload map[string]any) (any, error) {
	text, ok := payload["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("hash.text must be a non-empty string")
	}

	sum := sha256.Sum256([]byte(text))
	return map[string]string{
		"algorithm": "sha256",
		"digest":    hex.EncodeToString(sum[:]),
	}, nil
}

func payloadFloat(payload map[string]any, key string, defaultVal float64) (float64, error) {
	raw, ok := payload[key]
	if !ok {
		return defaultVal, nil
	}

	switch v := raw.(type) {
	case float64:
		return v, nil
	case json.Number:
		return v.Float64()
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}
