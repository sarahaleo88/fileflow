// Package store provides database access for FileFlow.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	sqlite "modernc.org/sqlite"
	lib "modernc.org/sqlite/lib"
)

// Store wraps the SQLite database connection.
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// New creates a new Store and initializes the database schema.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

var (
	ErrDeviceExists   = fmt.Errorf("device already exists")
	ErrDeviceNotFound = errors.New("device not found")
)

type Device struct {
	DeviceID   string `json:"device_id"`
	PubJWKJSON string `json:"pub_jwk_json"`
	Label      string `json:"label"`
	CreatedAt  int64  `json:"created_at"`
}

func (s *Store) AddDevice(d *Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stmt := `INSERT INTO devices (device_id, pub_jwk_json, label, created_at) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(stmt, d.DeviceID, d.PubJWKJSON, d.Label, d.CreatedAt)
	if err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code() == lib.SQLITE_CONSTRAINT_PRIMARYKEY ||
				sqliteErr.Code() == lib.SQLITE_CONSTRAINT_UNIQUE {
				return ErrDeviceExists
			}
		}
		return err
	}
	return nil
}

func (s *Store) GetDevice(deviceID string) (*Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var d Device
	err := s.db.QueryRow("SELECT device_id, pub_jwk_json, label, created_at FROM devices WHERE device_id = ?", deviceID).
		Scan(&d.DeviceID, &d.PubJWKJSON, &d.Label, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	return &d, nil
}

// migrate creates the database schema if it doesn't exist.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS devices (
		device_id TEXT PRIMARY KEY,
		pub_jwk_json TEXT NOT NULL,
		label TEXT,
		created_at INTEGER NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	return err
}
