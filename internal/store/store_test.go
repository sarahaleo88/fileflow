package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	t.Run("AddAndGetDevice", func(t *testing.T) {
		device := &Device{
			DeviceID:   "test-device-1",
			PubJWKJSON: `{"kty":"EC","crv":"P-256"}`,
			Label:      "Test Device",
			CreatedAt:  time.Now().UnixMilli(),
		}

		if err := s.AddDevice(device); err != nil {
			t.Fatalf("AddDevice failed: %v", err)
		}

		got, err := s.GetDevice("test-device-1")
		if err != nil {
			t.Fatalf("GetDevice failed: %v", err)
		}

		if got.DeviceID != device.DeviceID {
			t.Errorf("DeviceID = %q, want %q", got.DeviceID, device.DeviceID)
		}
		if got.Label != device.Label {
			t.Errorf("Label = %q, want %q", got.Label, device.Label)
		}
	})

	t.Run("AddDuplicateDevice", func(t *testing.T) {
		device := &Device{
			DeviceID:   "test-device-1",
			PubJWKJSON: `{}`,
			CreatedAt:  time.Now().UnixMilli(),
		}

		err := s.AddDevice(device)
		if err != ErrDeviceExists {
			t.Errorf("Expected ErrDeviceExists, got %v", err)
		}
	})

	t.Run("GetNonexistentDevice", func(t *testing.T) {
		_, err := s.GetDevice("nonexistent")
		if err != ErrDeviceNotFound {
			t.Errorf("Expected ErrDeviceNotFound, got %v", err)
		}
	})

	t.Run("IsWhitelisted", func(t *testing.T) {
		ok, err := s.IsWhitelisted("test-device-1")
		if err != nil {
			t.Fatalf("IsWhitelisted failed: %v", err)
		}
		if !ok {
			t.Error("Expected device to be whitelisted")
		}

		ok, err = s.IsWhitelisted("unknown-device")
		if err != nil {
			t.Fatalf("IsWhitelisted failed: %v", err)
		}
		if ok {
			t.Error("Expected device to not be whitelisted")
		}
	})

	t.Run("UpdateLastSeen", func(t *testing.T) {
		if err := s.UpdateLastSeen("test-device-1"); err != nil {
			t.Fatalf("UpdateLastSeen failed: %v", err)
		}

		device, _ := s.GetDevice("test-device-1")
		if device.LastSeenAt == nil {
			t.Error("Expected LastSeenAt to be set")
		}
	})

	t.Run("ListDevices", func(t *testing.T) {
		devices, err := s.ListDevices()
		if err != nil {
			t.Fatalf("ListDevices failed: %v", err)
		}

		if len(devices) != 1 {
			t.Errorf("Expected 1 device, got %d", len(devices))
		}
	})

	t.Run("DeleteDevice", func(t *testing.T) {
		if err := s.DeleteDevice("test-device-1"); err != nil {
			t.Fatalf("DeleteDevice failed: %v", err)
		}

		_, err := s.GetDevice("test-device-1")
		if err != ErrDeviceNotFound {
			t.Errorf("Expected ErrDeviceNotFound after delete, got %v", err)
		}
	})

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
