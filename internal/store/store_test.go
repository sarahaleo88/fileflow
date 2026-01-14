package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	t.Run("Config", func(t *testing.T) {
		if err := s.SetConfig("test_key", "test_value"); err != nil {
			t.Fatalf("SetConfig failed: %v", err)
		}

		val, err := s.GetConfig("test_key")
		if err != nil {
			t.Fatalf("GetConfig failed: %v", err)
		}
		if val != "test_value" {
			t.Errorf("GetConfig = %q, want %q", val, "test_value")
		}

		if err := s.SetConfig("test_key", "updated_value"); err != nil {
			t.Fatalf("SetConfig update failed: %v", err)
		}

		val, _ = s.GetConfig("test_key")
		if val != "updated_value" {
			t.Errorf("GetConfig = %q, want %q", val, "updated_value")
		}
	})
}

func TestNewStoreCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	os.MkdirAll(filepath.Dir(dbPath), 0755)

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database file to be created")
	}
}
