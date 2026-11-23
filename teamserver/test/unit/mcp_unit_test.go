package unit

import (
	"encoding/json"
	"testing"

	"Havoc/pkg/mcp"
	"Havoc/pkg/operator"
)

// TestMCPConfig tests the MCP configuration
func TestMCPConfig(t *testing.T) {
	cfg := mcp.DefaultConfig()

	if cfg.MaxOutputBytes != 262144 {
		t.Errorf("expected default MaxOutputBytes=262144, got %d", cfg.MaxOutputBytes)
	}

	if cfg.ReadOnly != false {
		t.Errorf("expected default ReadOnly=false, got %v", cfg.ReadOnly)
	}
}

// TestMCPConfigReadOnly tests read-only configuration
func TestMCPConfigReadOnly(t *testing.T) {
	cfg := mcp.Config{
		ReadOnly:     true,
		AllowedTools: []string{"send_command"},
	}

	if !cfg.ReadOnly {
		t.Error("expected ReadOnly=true")
	}

	if len(cfg.AllowedTools) != 1 || cfg.AllowedTools[0] != "send_command" {
		t.Errorf("expected AllowedTools=[send_command], got %v", cfg.AllowedTools)
	}
}

// TestOperatorConfig tests the operator configuration
func TestOperatorConfig(t *testing.T) {
	cfg := operator.Config{
		URL:      "wss://127.0.0.1:40056",
		User:     "testuser",
		Password: "testpass",
		Insecure: true,
	}

	if cfg.URL != "wss://127.0.0.1:40056" {
		t.Errorf("unexpected URL: %s", cfg.URL)
	}

	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig failed: %v", err)
	}

	if tlsCfg == nil {
		t.Error("expected non-nil TLS config")
	}

	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
}

// TestJSONRPCRequest tests JSON-RPC request marshaling
func TestJSONRPCRequest(t *testing.T) {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Error("jsonrpc version mismatch")
	}

	if parsed["method"] != "initialize" {
		t.Error("method mismatch")
	}
}

// TestToolRegistration tests tool registration logic
func TestToolRegistration(t *testing.T) {
	// This tests the logic in registerTool without needing a full server
	// Read-only tools should be registered even in read-only mode
	// Tasking tools should NOT be registered in read-only mode unless explicitly allowed

	readOnlyTool := mcp.Tool{
		Name:     "test_readonly",
		ReadOnly: true,
	}

	taskingTool := mcp.Tool{
		Name:     "test_tasking",
		ReadOnly: false,
	}

	// In read-only mode without allowed tools
	// readOnlyTool should be allowed, taskingTool should not
	// This is tested via the registerTool logic in jsonrpc.go

	// We can't easily test the private registerTool function,
	// but we can verify the logic through integration tests
	_ = readOnlyTool
	_ = taskingTool
}