package job

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("job not found")

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
			_ = db.Close()
			return nil, fmt.Errorf("migrate schema: %w", err)
		}
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(jobType string, payload map[string]any) (*Job, error) {
	if payload == nil {
		payload = map[string]any{}
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}

	now := time.Now().UTC()
	j := &Job{
		ID:          newID(),
		Type:        jobType,
		Payload:     payload,
		Status:      StatusQueued,
		Attempts:    0,
		MaxAttempts: DefaultMaxAttempts,
		CreatedAt:   now,
	}

	_, err = s.db.Exec(
		`INSERT INTO jobs (id, type, payload, status, attempts, max_attempts, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Type, string(payloadJSON), string(j.Status), j.Attempts, j.MaxAttempts, formatTime(j.CreatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	return clone(j), nil
}

const jobColumns = `id, type, payload, status, result, error, attempts, max_attempts, next_retry_at, created_at, started_at, finished_at`

func (s *Store) Get(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id,
	)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (s *Store) List(q ListQuery) (*ListResult, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 2)

	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.Type != "" {
		where = append(where, "type = ?")
		args = append(args, q.Type)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count jobs: %w", err)
	}

	listArgs := append(append([]any{}, args...), q.Limit, q.Offset)
	rows, err := s.db.Query(
		`SELECT `+jobColumns+` FROM jobs WHERE `+whereClause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var out []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []*Job{}
	}

	return &ListResult{
		Jobs:   out,
		Total:  total,
		Limit:  q.Limit,
		Offset: q.Offset,
	}, nil
}

func (s *Store) MarkProcessing(id string) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, started_at = ?
		 WHERE id = ? AND status = ?
		 AND (next_retry_at IS NULL OR next_retry_at <= ?)`,
		string(StatusProcessing), formatTime(now), id, string(StatusQueued), formatTime(now),
	)
	if err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}
	return ensureUpdated(res)
}

func (s *Store) MarkCompleted(id string, result any) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, result = ?, error = NULL, next_retry_at = NULL, finished_at = ? WHERE id = ?`,
		string(StatusCompleted), string(resultJSON), formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	return ensureUpdated(res)
}

func (s *Store) MarkFailed(id string, errMsg string) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, error = ?, next_retry_at = NULL, finished_at = ? WHERE id = ?`,
		string(StatusFailed), errMsg, formatTime(now), id,
	)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return ensureUpdated(res)
}

func (s *Store) ScheduleRetry(id string, errMsg string, nextRetryAt time.Time) error {
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, attempts = attempts + 1, error = ?, next_retry_at = ?, started_at = NULL
		 WHERE id = ? AND status = ?`,
		string(StatusQueued), errMsg, formatTime(nextRetryAt), id, string(StatusProcessing),
	)
	if err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	return ensureUpdated(res)
}

// RestartPending resets interrupted jobs and returns queued job IDs for re-enqueue.
func (s *Store) RestartPending(ctx context.Context) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin restart tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status = ?, started_at = NULL, next_retry_at = NULL WHERE status = ?`,
		string(StatusQueued), string(StatusProcessing),
	); err != nil {
		return nil, fmt.Errorf("reset processing jobs: %w", err)
	}

	now := formatTime(time.Now().UTC())
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM jobs WHERE status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)`,
		string(StatusQueued), now,
	)
	if err != nil {
		return nil, fmt.Errorf("list queued jobs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan queued id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit restart tx: %w", err)
	}
	return ids, nil
}

func (s *Store) ListDelayedRetries(ctx context.Context) ([]DelayedRetry, error) {
	now := formatTime(time.Now().UTC())
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, next_retry_at FROM jobs
		 WHERE status = ? AND next_retry_at IS NOT NULL AND next_retry_at > ?`,
		string(StatusQueued), now,
	)
	if err != nil {
		return nil, fmt.Errorf("list delayed retries: %w", err)
	}
	defer rows.Close()

	var out []DelayedRetry
	for rows.Next() {
		var (
			id      string
			retryAt string
		)
		if err := rows.Scan(&id, &retryAt); err != nil {
			return nil, fmt.Errorf("scan delayed retry: %w", err)
		}
		t, err := parseTime(retryAt)
		if err != nil {
			return nil, fmt.Errorf("parse next_retry_at: %w", err)
		}
		out = append(out, DelayedRetry{ID: id, NextRetryAt: t})
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (*Job, error) {
	var (
		j                       Job
		payloadJSON, resultJSON sql.NullString
		errorText               sql.NullString
		nextRetryAt             sql.NullString
		createdAt               string
		startedAt, finishedAt   sql.NullString
	)

	if err := row.Scan(
		&j.ID, &j.Type, &payloadJSON, &j.Status, &resultJSON, &errorText,
		&j.Attempts, &j.MaxAttempts, &nextRetryAt,
		&createdAt, &startedAt, &finishedAt,
	); err != nil {
		return nil, err
	}

	var err error
	j.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if startedAt.Valid {
		t, err := parseTime(startedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse started_at: %w", err)
		}
		j.StartedAt = &t
	}
	if finishedAt.Valid {
		t, err := parseTime(finishedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse finished_at: %w", err)
		}
		j.FinishedAt = &t
	}
	if errorText.Valid {
		j.Error = errorText.String
	}
	if nextRetryAt.Valid {
		t, err := parseTime(nextRetryAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse next_retry_at: %w", err)
		}
		j.NextRetryAt = &t
	}

	if payloadJSON.Valid && payloadJSON.String != "" {
		if err := json.Unmarshal([]byte(payloadJSON.String), &j.Payload); err != nil {
			return nil, fmt.Errorf("decode payload: %w", err)
		}
	}
	if resultJSON.Valid && resultJSON.String != "" {
		if err := json.Unmarshal([]byte(resultJSON.String), &j.Result); err != nil {
			return nil, fmt.Errorf("decode result: %w", err)
		}
	}

	return clone(&j), nil
}

func ensureUpdated(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
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

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}
