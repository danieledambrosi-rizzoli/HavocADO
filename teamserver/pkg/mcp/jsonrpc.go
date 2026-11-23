package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

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
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (no ID).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	ReadOnly    bool            `json:"-"`
	Handler     func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

// Server is the MCP server.
type Server struct {
	opClient *operator.Client
	cfg      Config
	tools    map[string]*Tool
	mu       sync.RWMutex
	auditLog io.Writer
}

// NewServer creates a new MCP server.
func NewServer(opClient *operator.Client, cfg Config) *Server {
	s := &Server{
		opClient: opClient,
		cfg:      cfg,
		tools:    make(map[string]*Tool),
		auditLog: nil, // Will be set in Serve
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
			"type": "object",
			"properties": map[string]any{},
		}),
		ReadOnly: true,
		Handler:  s.handleTeamserverStatus,
	})

	s.registerTool(&Tool{
		Name:        "list_sessions",
		Description: "List all active agent sessions",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
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
			"type": "object",
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
					"enum": []string{"shell", "powershell", "dir", "ls", "cd", "pwd", "cat", "download", "ps", "kill", "sleep", "exit", "jobs"},
				},
				"args": map[string]any{"type": "object"},
			},
			"required": []string{"session_id", "command"},
		}),
		ReadOnly: false,
		Handler:  s.handleSendCommand,
	})

	s.registerTool(&Tool{
		Name:        "send_raw_command",
		Description: "Send a raw command to an agent (advanced)",
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"command_id": map[string]any{"type": "string"},
				"info":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
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
				"session_id":    map[string]any{"type": "string"},
				"cursor":        map[string]any{"type": "integer", "minimum": 0},
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
	if len(s.cfg.AllowedTools) > 0 {
		allowed := false
		for _, name := range s.cfg.AllowedTools {
			if name == t.Name {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
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
	// Set up audit log
	if s.cfg.AuditLog != "" && s.cfg.AuditLog != "stderr" {
		// TODO: open file for audit log
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
		json.Unmarshal(req.Params, &params)
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

	// Audit log
	s.auditToolCall(params.Name, params.Arguments)

	// Call handler
	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		return s.errorResponse(req.ID, InternalError, "Tool execution failed", err.Error())
	}

	// Format result as MCP tool result
	content := []map[string]any{
		{"type": "text", "text": fmt.Sprintf("%v", result)},
	}

	return s.successResponse(req.ID, map[string]any{
		"content":  content,
		"isError":  false,
	})
}

func (s *Server) handlePing(req *Request) *Response {
	return s.successResponse(req.ID, map[string]any{})
}

func (s *Server) successResponse(id json.RawMessage, result any) *Response {
	data, _ := json.Marshal(result)
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
	enc.Encode(resp)
}

func (s *Server) auditToolCall(name string, args json.RawMessage) {
	// TODO: implement audit logging
	_ = name
	_ = args
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
			"name":       l.Name,
			"protocol":   l.Protocol,
			"status":     l.Status,
			"hosts":      l.Hosts,
			"host_bind":  l.HostBind,
			"port_bind":  l.PortBind,
			"port_conn":  l.PortConn,
			"secure":     l.Secure,
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

	if params.MaxBytes == 0 {
		params.MaxBytes = s.cfg.MaxOutputBytes
	}
	if params.MaxBytes > 1048576 {
		params.MaxBytes = 1048576
	}

	page := s.opClient.ReadOutput(params.SessionID, params.Cursor, params.MaxBytes)

	events := make([]map[string]any, len(page.Events))
	for i, e := range page.Events {
		events[i] = map[string]any{
			"seq":        e.Seq,
			"time":       e.Time,
			"command_id": e.CommandID,
			"text":       e.Text,
			"task_id":    e.TaskID,
		}
	}

	return map[string]any{
		"events":      events,
		"next_cursor": page.NextCursor,
		"truncated":   page.Truncated,
	}, nil
}

func (s *Server) handleWaitForOutput(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionID     string `json:"session_id"`
		Cursor        uint64 `json:"cursor"`
		TimeoutSeconds int   `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	if params.TimeoutSeconds == 0 {
		params.TimeoutSeconds = 30
	}
	if params.TimeoutSeconds > 60 {
		params.TimeoutSeconds = 60
	}

	timeout := time.Duration(params.TimeoutSeconds) * time.Second
	page := s.opClient.WaitOutput(ctx, params.SessionID, params.Cursor, timeout)

	events := make([]map[string]any, len(page.Events))
	for i, e := range page.Events {
		events[i] = map[string]any{
			"seq":        e.Seq,
			"time":       e.Time,
			"command_id": e.CommandID,
			"text":       e.Text,
			"task_id":    e.TaskID,
		}
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
		events[i] = map[string]any{
			"seq":        e.Seq,
			"time":       e.Time,
			"command_id": e.CommandID,
			"text":       e.Text,
			"task_id":    e.TaskID,
		}
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
		req = operator.ShellTask(params.SessionID, cmd)
	case "powershell":
		cmd, _ := params.Args["command"].(string)
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
		req = operator.DirTask(params.SessionID, opts)
	case "cd":
		path, _ := params.Args["path"].(string)
		req = operator.CdTask(params.SessionID, path)
	case "pwd":
		req = operator.PwdTask(params.SessionID)
	case "cat":
		path, _ := params.Args["path"].(string)
		req = operator.CatTask(params.SessionID, path)
	case "download":
		path, _ := params.Args["path"].(string)
		req = operator.DownloadTask(params.SessionID, path)
	case "ps":
		req = operator.PsTask(params.SessionID)
	case "kill":
		pid := getInt(params.Args, "pid")
		req = operator.KillTask(params.SessionID, pid)
	case "sleep":
		delay := getInt(params.Args, "delay")
		jitter := getInt(params.Args, "jitter")
		req = operator.SleepTask(params.SessionID, delay, jitter)
	case "exit":
		method, _ := params.Args["method"].(string)
		req = operator.ExitTask(params.SessionID, method)
	case "jobs":
		req = operator.JobsTask(params.SessionID)
	default:
		return nil, fmt.Errorf("unknown command: %s", params.Command)
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
		SessionID   string         `json:"session_id"`
		CommandID   json.RawMessage `json:"command_id"`
		Info        map[string]string `json:"info"`
		CommandLine string         `json:"command_line"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Parse command_id (can be int or string)
	var commandID interface{}
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

func (s *Server) handleStartListener(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Name     string         `json:"name"`
		Protocol string         `json:"protocol"`
		Config   map[string]any `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Convert config to string map
	info := make(map[string]string)
	for k, v := range params.Config {
		info[k] = fmt.Sprintf("%v", v)
	}

	// Map protocol to teamserver protocol
	protocol := params.Protocol
	switch protocol {
	case "http":
		protocol = "HTTP"
	case "https":
		protocol = "HTTPS"
	case "smb":
		protocol = "SMB"
	case "external":
		protocol = "External"
	}

	if err := s.opClient.StartListener(params.Name, protocol, info); err != nil {
		return nil, fmt.Errorf("start listener: %w", err)
	}

	return map[string]any{
		"name":     params.Name,
		"protocol": protocol,
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

	info := make(map[string]string)
	for k, v := range params.Config {
		info[k] = fmt.Sprintf("%v", v)
	}

	if err := s.opClient.EditListener(params.Name, info); err != nil {
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

	if err := s.opClient.GeneratePayload(params.AgentType, params.Listener, params.Arch, params.Format, params.Config); err != nil {
		return nil, fmt.Errorf("generate payload: %w", err)
	}

	return map[string]any{
		"agent_type": params.AgentType,
		"listener":   params.Listener,
		"arch":       params.Arch,
		"format":     params.Format,
		"status":     "generation_started",
		"note":       "Payload will be delivered via Gate.Stageless response. Use get_output to retrieve.",
	}, nil
}