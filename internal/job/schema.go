package job

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id          TEXT PRIMARY KEY,
	type        TEXT NOT NULL,
	payload     TEXT NOT NULL DEFAULT '{}',
	status      TEXT NOT NULL,
	result      TEXT,
	error       TEXT,
	created_at  TEXT NOT NULL,
	started_at  TEXT,
	finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
`
