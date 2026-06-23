package job

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id            TEXT PRIMARY KEY,
	type          TEXT NOT NULL,
	payload       TEXT NOT NULL DEFAULT '{}',
	status        TEXT NOT NULL,
	result        TEXT,
	error         TEXT,
	attempts      INTEGER NOT NULL DEFAULT 0,
	max_attempts  INTEGER NOT NULL DEFAULT 3,
	next_retry_at TEXT,
	created_at    TEXT NOT NULL,
	started_at    TEXT,
	finished_at   TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
`

var migrations = []string{
	`ALTER TABLE jobs ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE jobs ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3`,
	`ALTER TABLE jobs ADD COLUMN next_retry_at TEXT`,
}
