package db

import (
	"fmt"
	"time"
)

const (
	RunQueued  = "queued"
	RunRunning = "running"
	RunSuccess = "success"
	RunFailed  = "failed"
)

// nowUnix performs its package-specific operation.
func nowUnix() int64 {
	return time.Now().Unix()
}

// CreateRun inserts a new pipeline run with state=queued.
func (d *DB) CreateRun(siteID int64, commentID string) (int64, error) {
	res, err := d.SQL.Exec(`
INSERT INTO pipeline_runs (
  site_id, trigger_comment_id, state, created_at
) VALUES (?, ?, ?, ?)
`,
		siteID,
		commentID,
		RunQueued,
		nowUnix(),
	)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}

	return res.LastInsertId()
}

// MarkRunRunning sets state=running.
func (d *DB) MarkRunRunning(runID int64) error {
	_, err := d.SQL.Exec(`
UPDATE pipeline_runs
SET state = ?, started_at = ?, step = NULL, error_message = NULL
WHERE id = ?
`,
		RunRunning,
		nowUnix(),
		runID,
	)
	return err
}

// MarkRunStep updates current step (optional helper).
func (d *DB) MarkRunStep(runID int64, step string) error {
	_, err := d.SQL.Exec(`
UPDATE pipeline_runs
SET step = ?
WHERE id = ?
`,
		step,
		runID,
	)
	return err
}

// MarkRunSuccess sets state=success.
func (d *DB) MarkRunSuccess(runID int64) error {
	_, err := d.SQL.Exec(`
UPDATE pipeline_runs
SET state = ?, finished_at = ?, step = NULL
WHERE id = ?
`,
		RunSuccess,
		nowUnix(),
		runID,
	)
	return err
}

// MarkRunFailed sets state=failed and stores error info.
func (d *DB) MarkRunFailed(runID int64, step, msg string) error {
	_, err := d.SQL.Exec(`
UPDATE pipeline_runs
SET state = ?, finished_at = ?, step = ?, error_message = ?
WHERE id = ?
`,
		RunFailed,
		nowUnix(),
		step,
		msg,
		runID,
	)
	return err
}
