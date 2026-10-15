package store

import (
	"database/sql"
	"fmt"
	"time"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"internship-tracker/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id              TEXT PRIMARY KEY,   -- "{source}:{company}:{raw_id}"
	source          TEXT NOT NULL,      -- "greenhouse" | "lever" | "workday"
	fetched_at   DATETIME NOT NULL,   -- first seen; never updated
	last_seen_at DATETIME NOT NULL,
	closed_at    DATETIME,            -- NULL = still open
	title           TEXT NOT NULL,
	company         TEXT NOT NULL,
	apply_url       TEXT NOT NULL,
	location_raw    TEXT NOT NULL,
	workplace_type  TEXT NOT NULL,
	department      TEXT NOT NULL,
	commitment      TEXT NOT NULL,
	description     TEXT NOT NULL,
	posted_at       DATETIME,           -- NULL when source doesn't expose it
	updated_at      DATETIME
);

-- seq is a monotonically increasing rowid alias used by the server
-- poller to efficiently fetch only rows it hasn't seen yet.
-- SQLite assigns rowid automatically; we expose it via this index.

`

// Store wraps a SQLite database for reading and writing Job records.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path and
// applies the schema. Call Close when done.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("store.Open: %w", err)
	}

	// WAL mode allows the crawler (writer) and server (reader) to operate
	// on the same file concurrently without blocking each other.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Insert writes a job to the database.
// If a job with the same ID already exists, it is ignored (idempotent).
func (s *Store) Insert(job model.Job) error {
	const q = `
	INSERT INTO jobs (
		id, source, fetched_at, last_seen_at, title, company, apply_url,
		location_raw, workplace_type, department, commitment,
		description, posted_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		last_seen_at = excluded.last_seen_at,
		closed_at    = NULL,
		title        = excluded.title,
		company      = excluded.company,
		location_raw = excluded.location_raw,
		description  = excluded.description,
		updated_at   = excluded.updated_at`

	_, err := s.db.Exec(q,
		job.ID, string(job.Source),
		job.FetchedAt.UTC(),  // first seen: never overwritten on conflict
		time.Now().UTC(),     // last_seen_at
		job.Title, job.Company, job.ApplyURL, job.LocationRaw,
		string(job.WorkplaceType), job.Department, job.Commitment,
		job.Description, nullTime(job.PostedAt), nullTime(job.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store.Insert: %w", err)
	}
	return nil
}

func (s *Store) MarkClosed(runStart time.Time, sources []string) (int64, error) {
	if len(sources) == 0 { return 0, nil }
	ph := strings.TrimSuffix(strings.Repeat("?,", len(sources)), ",")
	args := []any{time.Now().UTC(), runStart.UTC()}
	for _, src := range sources { args = append(args, src) }
	res, err := s.db.Exec(`UPDATE jobs SET closed_at = ?
		WHERE closed_at IS NULL AND last_seen_at < ? AND source IN (`+ph+`)`, args...)
	if err != nil { return 0, err }
	return res.RowsAffected()
}

func (s *Store) ClosedSince(after time.Time) (ids []string, latest time.Time, err error) {
	latest = after
	rows, err := s.db.Query(`SELECT id, closed_at FROM jobs WHERE closed_at > ? ORDER BY closed_at`, after.UTC())
	if err != nil { return nil, after, err }
	defer rows.Close()
	for rows.Next() {
		var id string; var t time.Time
		if err := rows.Scan(&id, &t); err != nil { return nil, after, err }
		ids = append(ids, id)
		if t.After(latest) { latest = t }
	}
	return ids, latest, rows.Err()
}

// PollSince returns all jobs with a rowid greater than afterRowID,
// along with the highest rowid seen in the result set.
//
// The server poller calls this on a ticker, passing back the last
// maxRowID each time, so it only ever fetches rows it hasn't pushed
// to WebSocket clients yet.
func (s *Store) PollSince(afterRowID int64) (jobs []model.Job, maxRowID int64, err error) {
	const q = `
	SELECT
		rowid, id, source, fetched_at, title, company, apply_url,
		location_raw, workplace_type, department, commitment,
		description, posted_at, updated_at
	FROM jobs
	WHERE rowid > ?
	ORDER BY rowid ASC`

	rows, err := s.db.Query(q, afterRowID)
	if err != nil {
		return nil, afterRowID, fmt.Errorf("store.PollSince: %w", err)
	}
	defer rows.Close()

	maxRowID = afterRowID

	for rows.Next() {
		var (
			rowID    int64
			job      model.Job
			source   string
			wpType   string
			postedAt sql.NullTime
			updatedAt sql.NullTime
		)

		if err := rows.Scan(
			&rowID,
			&job.ID,
			&source,
			&job.FetchedAt,
			&job.Title,
			&job.Company,
			&job.ApplyURL,
			&job.LocationRaw,
			&wpType,
			&job.Department,
			&job.Commitment,
			&job.Description,
			&postedAt,
			&updatedAt,
		); err != nil {
			return nil, afterRowID, fmt.Errorf("store.PollSince scan: %w", err)
		}

		job.Source = model.Source(source)
		job.WorkplaceType = model.WorkplaceType(wpType)
		if postedAt.Valid {
			job.PostedAt = postedAt.Time
		}
		if updatedAt.Valid {
			job.UpdatedAt = updatedAt.Time
		}

		if rowID > maxRowID {
			maxRowID = rowID
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, afterRowID, fmt.Errorf("store.PollSince rows: %w", err)
	}

	return jobs, maxRowID, nil
}

// Count returns the total number of jobs currently in the database.
// Useful for logging at the end of a crawl run.
func (s *Store) Count() (int64, error) {
	var n int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&n)
	return n, err
}

// nullTime converts a time.Time to a sql.NullTime.
// A zero time is stored as NULL so the DB schema stays honest about
// which sources actually expose a posted date.
func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
