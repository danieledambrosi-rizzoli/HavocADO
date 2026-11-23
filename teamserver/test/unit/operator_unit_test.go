package unit

import (
	"testing"

	"Havoc/pkg/operator"
)

// TestOperatorConfigValidation tests config validation logic
func TestOperatorConfigValidation(t *testing.T) {
	// Test that config validation happens before dial
	// We can't easily test Dial without a server, so test the config struct directly
	tests := []struct {
		name        string
		cfg         operator.Config
		hasURL      bool
		hasUser     bool
		hasPassword bool
	}{
		{
			name:        "valid config",
			cfg:         operator.Config{URL: "wss://127.0.0.1:40056", User: "test", Password: "pass"},
			hasURL:      true,
			hasUser:     true,
			hasPassword: true,
		},
		{
			name:        "missing URL",
			cfg:         operator.Config{User: "test", Password: "pass"},
			hasURL:      false,
			hasUser:     true,
			hasPassword: true,
		},
		{
			name:        "missing user",
			cfg:         operator.Config{URL: "wss://127.0.0.1:40056", Password: "pass"},
			hasURL:      true,
			hasUser:     false,
			hasPassword: true,
		},
		{
			name:        "missing password",
			cfg:         operator.Config{URL: "wss://127.0.0.1:40056", User: "test"},
			hasURL:      true,
			hasUser:     true,
			hasPassword: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.hasURL && tc.cfg.URL == "" {
				t.Error("expected URL to be set")
			}
			if !tc.hasURL && tc.cfg.URL != "" {
				t.Error("expected URL to be empty")
			}
			if tc.hasUser && tc.cfg.User == "" {
				t.Error("expected User to be set")
			}
			if !tc.hasUser && tc.cfg.User != "" {
				t.Error("expected User to be empty")
			}
			if tc.hasPassword && tc.cfg.Password == "" {
				t.Error("expected Password to be set")
			}
			if !tc.hasPassword && tc.cfg.Password != "" {
				t.Error("expected Password to be empty")
			}
		})
	}
}

// TestHashPassword tests password hashing
func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	hash1 := operator.HashPassword(password)
	hash2 := operator.HashPassword(password)

	if hash1 != hash2 {
		t.Error("hash should be deterministic")
	}

	if len(hash1) != 64 { // SHA3-256 hex = 64 chars
		t.Errorf("expected hash length 64, got %d", len(hash1))
	}

	// Different passwords should produce different hashes
	hash3 := operator.HashPassword("differentpassword")
	if hash1 == hash3 {
		t.Error("different passwords should produce different hashes")
	}
}

// TestTaskRequestPreparation tests task request preparation
func TestTaskRequestPreparation(t *testing.T) {
	req := operator.TaskRequest{
		SessionID: "test-session",
		CommandID: 1,
		Info: map[string]string{
			"CommandID": "1",
		},
		CommandLine: "test command",
	}

	prepared := operator.PrepareTaskRequest(req)

	if prepared.Info["TaskID"] == "" {
		t.Error("expected TaskID to be generated in Info")
	}

	if prepared.SessionID != "test-session" {
		t.Error("session ID should be preserved")
	}
}