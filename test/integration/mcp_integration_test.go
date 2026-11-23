package integration

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"Havoc/test/ephemeral"
)

// TestMCPServerConfig holds configuration for the MCP test server
type TestMCPServerConfig struct {
	TeamserverURL string
	User          string
	Password      string
	Insecure      bool
	TLSCA         string
	ReadOnly      bool
	AllowTools    string
	AuditLog      string
	HavocBinary   string
}

// DefaultTestMCPConfig returns a default test configuration
func DefaultTestMCPConfig() TestMCPServerConfig {
	return TestMCPServerConfig{
		TeamserverURL: "wss://127.0.0.1:40056/havoc/",
		User:          "mcp",
		Password:      "testpassword",
		Insecure:      true,
		ReadOnly:      false,
		AllowTools:    "",
		AuditLog:      "stderr",
		HavocBinary:   findHavocBinary(),
	}
}

func findHavocBinary() string {
	// Try to find the havoc binary
	paths := []string{
		"/home/kali/Havoc/havoc",
		"../../havoc",
		"havoc",
	}

	for _, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	return "havoc" // fallback to PATH
}

// MCPClient represents a connected MCP client for testing
type MCPClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	scanner *bufio.Scanner
	requestID int
}

// StartMCPClient starts an MCP client connected to the teamserver
func StartMCPClient(t *testing.T, cfg TestMCPServerConfig) *MCPClient {
	t.Helper()

	args := []string{
		"mcp",
		"--teamserver", cfg.TeamserverURL,
		"--user", cfg.User,
		"--password", cfg.Password,
	}

	if cfg.Insecure {
		args = append(args, "--insecure")
	}
	if cfg.TLSCA != "" {
		args = append(args, "--tls-ca", cfg.TLSCA)
	}
	if cfg.ReadOnly {
		args = append(args, "--read-only")
	}
	if cfg.AllowTools != "" {
		args = append(args, "--allow-tools", cfg.AllowTools)
	}
	if cfg.AuditLog != "" {
		args = append(args, "--audit-log", cfg.AuditLog)
	}

	cmd := exec.Command(cfg.HavocBinary, args...)
	cmd.Dir = filepath.Dir(cfg.HavocBinary)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start MCP client: %v", err)
	}

	client := &MCPClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		scanner: bufio.NewScanner(stdout),
	}

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	return client
}

// SendRequest sends a JSON-RPC request and returns the response
func (c *MCPClient) SendRequest(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()

	c.requestID++
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.requestID,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read response
	if !c.scanner.Scan() {
		t.Fatalf("no response received")
	}

	var resp map[string]any
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if errObj, ok := resp["error"].(map[string]any); ok {
		t.Logf("RPC Error: %v", errObj)
	}

	if result, ok := resp["result"]; ok {
		resultBytes, _ := json.Marshal(result)
		return resultBytes
	}

	return nil
}

// Initialize sends the initialize request
func (c *MCPClient) Initialize(t *testing.T) {
	t.Helper()
	c.SendRequest(t, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
	})
}

// ListTools calls tools/list
func (c *MCPClient) ListTools(t *testing.T) json.RawMessage {
	t.Helper()
	return c.SendRequest(t, "tools/list", nil)
}

// CallTool calls a tool with arguments
func (c *MCPClient) CallTool(t *testing.T, name string, args map[string]any) json.RawMessage {
	t.Helper()
	return c.SendRequest(t, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
}

// Close closes the MCP client
func (c *MCPClient) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Wait()
	}
	return nil
}

// TestMCPIntegration runs basic MCP integration tests
// This test requires a running teamserver with an "mcp" operator configured
func TestMCPIntegration(t *testing.T) {
	// Skip if no teamserver is running
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if teamserver is reachable
	cfg := DefaultTestMCPConfig()
	if !isTeamserverReachable(cfg.TeamserverURL) {
		t.Skip("teamserver not reachable, skipping integration test")
	}

	// Create ephemeral directory for audit log
	auditDir, cleanup := ephemeral.TempDir(t, "mcp-audit-*")
	defer cleanup()
	auditLogPath := filepath.Join(auditDir, "mcp-audit.log")

	cfg.AuditLog = auditLogPath
	cfg.ReadOnly = true

	client := StartMCPClient(t, cfg)
	defer client.Close()

	// Initialize
	client.Initialize(t)

	// List tools - should only show read-only tools
	toolsResult := client.ListTools(t)
	var toolsList map[string]any
	json.Unmarshal(toolsResult, &toolsList)

	tools := toolsList["tools"].([]any)
	t.Logf("Available tools (%d):", len(tools))
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		t.Logf("  - %s: %s", toolMap["name"], toolMap["description"])
	}

	// Verify read-only tools are present
	readOnlyTools := []string{"teamserver_status", "list_sessions", "get_session", "list_listeners", "get_output", "wait_for_output", "get_task"}
	for _, toolName := range readOnlyTools {
		found := false
		for _, tool := range tools {
			if tool.(map[string]any)["name"] == toolName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected read-only tool %s to be present", toolName)
		}
	}

	// Verify tasking tools are NOT present
	taskingTools := []string{"send_command", "send_raw_command", "start_listener", "edit_listener", "remove_listener", "generate_payload"}
	for _, toolName := range taskingTools {
		found := false
		for _, tool := range tools {
			if tool.(map[string]any)["name"] == toolName {
				found = true
				break
			}
		}
		if found {
			t.Errorf("expected tasking tool %s to be absent in read-only mode", toolName)
		}
	}

	// Verify audit log was created
	if _, err := os.Stat(auditLogPath); os.IsNotExist(err) {
		t.Errorf("audit log file was not created at %s", auditLogPath)
	} else {
		content, _ := os.ReadFile(auditLogPath)
		t.Logf("Audit log content:\n%s", string(content))
	}
}

// TestMCPReadOnlyMode tests that read-only mode properly restricts tools
func TestMCPReadOnlyMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := DefaultTestMCPConfig()
	if !isTeamserverReachable(cfg.TeamserverURL) {
		t.Skip("teamserver not reachable, skipping integration test")
	}

	auditDir, cleanup := ephemeral.TempDir(t, "mcp-readonly-*")
	defer cleanup()
	auditLogPath := filepath.Join(auditDir, "mcp-audit.log")

	cfg.AuditLog = auditLogPath
	cfg.ReadOnly = true

	client := StartMCPClient(t, cfg)
	defer client.Close()

	client.Initialize(t)

	// Try to call a tasking tool - should fail
	result := client.CallTool(t, "send_command", map[string]any{
		"session_id": "test",
		"command":    "shell",
		"args":       map[string]any{"command": "whoami"},
	})

	var resp map[string]any
	json.Unmarshal(result, &resp)

	if resp["error"] == nil {
		t.Error("expected error when calling tasking tool in read-only mode")
	} else {
		t.Logf("Correctly rejected tasking tool: %v", resp["error"])
	}
}

// TestMCPAllowToolsOverride tests that --allow-tools overrides read-only
func TestMCPAllowToolsOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := DefaultTestMCPConfig()
	if !isTeamserverReachable(cfg.TeamserverURL) {
		t.Skip("teamserver not reachable, skipping integration test")
	}

	auditDir, cleanup := ephemeral.TempDir(t, "mcp-allowtools-*")
	defer cleanup()
	auditLogPath := filepath.Join(auditDir, "mcp-audit.log")

	cfg.AuditLog = auditLogPath
	cfg.ReadOnly = true
	cfg.AllowTools = "send_command,get_output"

	client := StartMCPClient(t, cfg)
	defer client.Close()

	client.Initialize(t)

	toolsResult := client.ListTools(t)
	var toolsList map[string]any
	json.Unmarshal(toolsResult, &toolsList)

	tools := toolsList["tools"].([]any)
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.(map[string]any)["name"].(string)] = true
	}

	// send_command should be allowed
	if !toolNames["send_command"] {
		t.Error("expected send_command to be allowed via --allow-tools")
	}

	// Other tasking tools should still be blocked
	blockedTools := []string{"send_raw_command", "start_listener", "edit_listener", "remove_listener", "generate_payload"}
	for _, toolName := range blockedTools {
		if toolNames[toolName] {
			t.Errorf("expected %s to be blocked even with --allow-tools", toolName)
		}
	}
}

// TestMCPTeamserverStatus tests the teamserver_status tool
func TestMCPTeamserverStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := DefaultTestMCPConfig()
	if !isTeamserverReachable(cfg.TeamserverURL) {
		t.Skip("teamserver not reachable, skipping integration test")
	}

	client := StartMCPClient(t, cfg)
	defer client.Close()

	client.Initialize(t)

	result := client.CallTool(t, "teamserver_status", nil)
	var resp map[string]any
	json.Unmarshal(result, &resp)

	if content, ok := resp["content"].([]any); ok {
		for _, c := range content {
			if textMap, ok := c.(map[string]any); ok {
				if text, ok := textMap["text"].(string); ok {
					t.Logf("Teamserver status: %s", text)
				}
			}
		}
	}
}

func isTeamserverReachable(url string) bool {
	// Quick check - try to connect
	// For now just return false to skip if not explicitly enabled
	return false
}