package store

import (
	"database/sql"
	"errors"
)

// ErrConfigNotFound is returned when a config key doesn't exist.
var ErrConfigNotFound = errors.New("config key not found")

// GetConfig retrieves a configuration value by key.
func (s *Store) GetConfig(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConfigNotFound
	}
	return value, err
}

// SetConfig sets a configuration value, creating or updating as needed.
func (s *Store) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// DeleteConfig removes a configuration key.
func (s *Store) DeleteConfig(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM config WHERE key = ?", key)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrConfigNotFound
	}

	return nil
}

// Config keys used by the application.
const (
	ConfigKeySecretHash = "secret_hash"
	ConfigKeyAppDomain  = "app_domain"
)
