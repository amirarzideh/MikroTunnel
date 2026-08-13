package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
)

var ErrNotFound = errors.New("not found")

type SQLiteStore struct{ db *sql.DB }

func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS tunnels (
 id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL,
 local_endpoint TEXT NOT NULL, remote_endpoint TEXT NOT NULL, address TEXT NOT NULL,
 mtu INTEGER NOT NULL, ttl INTEGER NOT NULL, description TEXT NOT NULL,
 desired_state TEXT NOT NULL, actual_state TEXT NOT NULL, last_error TEXT NOT NULL,
 failure_count INTEGER NOT NULL DEFAULT 0, retry_at TEXT,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS operations (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, resource_type TEXT NOT NULL,
 resource_id TEXT, status TEXT NOT NULL, message TEXT NOT NULL,
 created_at TEXT NOT NULL, finished_at TEXT
);
CREATE TABLE IF NOT EXISTS api_keys (
 id TEXT PRIMARY KEY, prefix TEXT NOT NULL UNIQUE, hash TEXT NOT NULL,
 created_at INTEGER NOT NULL, revoked_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations(created_at DESC);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "tunnels", "failure_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return s.ensureColumn(ctx, "tunnels", "retry_at", "TEXT")
}

func (s *SQLiteStore) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primary int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

func (s *SQLiteStore) CreateTunnel(ctx context.Context, t domain.Tunnel) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tunnels (id,name,type,local_endpoint,remote_endpoint,address,mtu,ttl,description,desired_state,actual_state,last_error,failure_count,retry_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.Name, t.Type, t.Local, t.Remote, t.Address, t.MTU, t.TTL, t.Description, t.DesiredState, t.ActualState, t.LastError, t.FailureCount, nullableTime(t.RetryAt), t.CreatedAt.UTC().Format(time.RFC3339Nano), t.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) UpdateTunnel(ctx context.Context, t domain.Tunnel) error {
	res, err := s.db.ExecContext(ctx, `UPDATE tunnels SET name=?, type=?, local_endpoint=?, remote_endpoint=?, address=?, mtu=?, ttl=?, description=?, desired_state=?, actual_state=?, last_error=?, failure_count=?, retry_at=?, updated_at=? WHERE id=?`, t.Name, t.Type, t.Local, t.Remote, t.Address, t.MTU, t.TTL, t.Description, t.DesiredState, t.ActualState, t.LastError, t.FailureCount, nullableTime(t.RetryAt), t.UpdatedAt.UTC().Format(time.RFC3339Nano), t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) DeleteTunnel(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tunnels WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *SQLiteStore) GetTunnel(ctx context.Context, id string) (domain.Tunnel, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,name,type,local_endpoint,remote_endpoint,address,mtu,ttl,description,desired_state,actual_state,last_error,failure_count,retry_at,created_at,updated_at FROM tunnels WHERE id=?`, id)
	return scanTunnel(row)
}
func (s *SQLiteStore) ListTunnels(ctx context.Context) ([]domain.Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,type,local_endpoint,remote_endpoint,address,mtu,ttl,description,desired_state,actual_state,last_error,failure_count,retry_at,created_at,updated_at FROM tunnels ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Tunnel, 0)
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanTunnel(row scanner) (domain.Tunnel, error) {
	var t domain.Tunnel
	var created, updated string
	var retry sql.NullString
	err := row.Scan(&t.ID, &t.Name, &t.Type, &t.Local, &t.Remote, &t.Address, &t.MTU, &t.TTL, &t.Description, &t.DesiredState, &t.ActualState, &t.LastError, &t.FailureCount, &retry, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if retry.Valid {
		value, _ := time.Parse(time.RFC3339Nano, retry.String)
		t.RetryAt = &value
	}
	return t, nil
}

func (s *SQLiteStore) CreateOperation(ctx context.Context, o domain.Operation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO operations VALUES(?,?,?,?,?,?,?,?)`, o.ID, o.Action, o.ResourceType, o.ResourceID, o.Status, o.Message, o.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(o.FinishedAt))
	return err
}
func (s *SQLiteStore) ListOperations(ctx context.Context, limit int) ([]domain.Operation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,action,resource_type,resource_id,status,message,created_at,finished_at FROM operations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Operation, 0)
	for rows.Next() {
		var o domain.Operation
		var created string
		var finished sql.NullString
		if err := rows.Scan(&o.ID, &o.Action, &o.ResourceType, &o.ResourceID, &o.Status, &o.Message, &created, &finished); err != nil {
			return nil, err
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if finished.Valid {
			v, _ := time.Parse(time.RFC3339Nano, finished.String)
			o.FinishedAt = &v
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) MarkOperationsRunning(ctx context.Context, resourceID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET status=? WHERE resource_id=? AND status=?`, domain.OperationRunning, resourceID, domain.OperationQueued)
	return err
}

func (s *SQLiteStore) CompleteOperations(ctx context.Context, resourceID string, status domain.OperationStatus, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET status=?, message=?, finished_at=? WHERE resource_id=? AND status IN (?, ?)`, status, message, now, resourceID, domain.OperationQueued, domain.OperationRunning)
	return err
}

func (s *SQLiteStore) RequeueInterruptedOperations(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET status=? WHERE status=?`, domain.OperationQueued, domain.OperationRunning)
	return err
}
func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UTC().Format(time.RFC3339Nano)
}

func (s *SQLiteStore) HasAPIKeys(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL`).Scan(&n)
	return n > 0, err
}
func (s *SQLiteStore) CreateAPIKey(ctx context.Context, k domain.APIKey) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_keys VALUES(?,?,?,?,NULL)`, k.ID, k.Prefix, k.Hash, k.CreatedAt)
	return err
}
func (s *SQLiteStore) FindAPIKey(ctx context.Context, prefix string) (domain.APIKey, error) {
	var k domain.APIKey
	var revoked sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id,prefix,hash,created_at,revoked_at FROM api_keys WHERE prefix=?`, prefix).Scan(&k.ID, &k.Prefix, &k.Hash, &k.CreatedAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return k, ErrNotFound
	}
	if err != nil {
		return k, err
	}
	if revoked.Valid {
		k.RevokedAt = &revoked.Int64
	}
	return k, nil
}

func NewID() string { return uuid.NewString() }
