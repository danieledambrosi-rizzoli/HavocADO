package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"Havoc/pkg/operator"
)

var dedicatedTaskingTools = []string{
	"screenshot",
	"socks",
	"rportfwd",
	"token",
	"shellcode",
	"dotnet",
	"inline_execute",
	"dll",
	"download",
	"checkin",
	"job",
	"proc",
	"spawn",
	"transfer",
	"net",
	"pivot",
}

func TestToolRegistryCountsAndReadOnlyCoverage(t *testing.T) {
	server := NewServer(nil, DefaultConfig())
	if got, want := len(server.tools), 29; got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}

	if got, want := len(dedicatedTaskingTools), 16; got != want {
		t.Fatalf("dedicated tasking tool count = %d, want %d", got, want)
	}
	for _, name := range dedicatedTaskingTools {
		tool, ok := server.tools[name]
		if !ok {
			t.Fatalf("dedicated tool %q is not registered", name)
		}
		if tool.ReadOnly {
			t.Fatalf("dedicated tool %q is marked read-only", name)
		}
	}

	readOnlyServer := NewServer(nil, Config{ReadOnly: true, MaxOutputBytes: 262144})
	if got, want := len(readOnlyServer.tools), 7; got != want {
		t.Fatalf("read-only tool count = %d, want %d", got, want)
	}
	wantReadOnly := []string{
		"get_output",
		"get_session",
		"get_task",
		"list_listeners",
		"list_sessions",
		"teamserver_status",
		"wait_for_output",
	}
	gotReadOnly := make([]string, 0, len(readOnlyServer.tools))
	for name, tool := range readOnlyServer.tools {
		if !tool.ReadOnly {
			t.Fatalf("read-only server exposed tasking tool %q", name)
		}
		gotReadOnly = append(gotReadOnly, name)
	}
	sort.Strings(gotReadOnly)
	if !reflect.DeepEqual(gotReadOnly, wantReadOnly) {
		t.Fatalf("read-only tools = %v, want %v", gotReadOnly, wantReadOnly)
	}
}

func TestSendCommandEnumCoversClassBCommands(t *testing.T) {
	server := NewServer(nil, DefaultConfig())
	tool, ok := server.tools["send_command"]
	if !ok {
		t.Fatal("send_command is not registered")
	}

	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("input schema has no properties object")
	}
	command, ok := properties["command"].(map[string]any)
	if !ok {
		t.Fatal("input schema has no command property")
	}
	values, ok := command["enum"].([]any)
	if !ok {
		t.Fatal("command schema has no enum")
	}

	wantValues := []string{
		"shell", "powershell", "dir", "ls", "cd", "cp", "mv", "move",
		"remove", "rm", "mkdir", "pwd", "cat", "type", "download", "upload",
		"ps", "kill", "sleep", "interactive", "exit", "jobs", "config", "luid",
		"klist", "purge", "ptt", "getprivs", "whoami", "ipconfig", "sysinfo", "env",
	}
	if got, want := len(values), len(wantValues); got != want {
		t.Fatalf("send_command enum count = %d, want %d", got, want)
	}
	gotValues := make([]string, 0, len(values))
	for _, value := range values {
		gotValues = append(gotValues, value.(string))
	}
	if !reflect.DeepEqual(gotValues, wantValues) {
		t.Fatalf("send_command enum = %v, want %v", gotValues, wantValues)
	}
}

func TestSanitizeArgsRedactsTopLevelPassword(t *testing.T) {
	sanitized := sanitizeArgs(json.RawMessage(`{"session_id":"abc","password":"secret"}`))
	if strings.Contains(string(sanitized), "secret") {
		t.Fatalf("sanitized args contain password: %s", sanitized)
	}

	var values map[string]any
	if err := json.Unmarshal(sanitized, &values); err != nil {
		t.Fatalf("decode sanitized args: %v", err)
	}
	if values["password"] != "[REDACTED]" {
		t.Fatalf("password = %v, want [REDACTED]", values["password"])
	}
}

func TestOutputEventMapSeparatesScreenshotPayload(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("BM"))
	text, err := json.Marshal(map[string]any{
		"Type":      "Good",
		"Message":   "Successful took screenshot",
		"MiscType":  "screenshot",
		"MiscData":  payload,
		"MiscData2": "Desktop_2026-11-25.png",
	})
	if err != nil {
		t.Fatalf("encode screenshot envelope: %v", err)
	}

	event := outputEventMap(operator.OutputEvent{Text: string(text)})
	if got := event["text"]; got != "Successful took screenshot" {
		t.Fatalf("text = %v, want screenshot message", got)
	}
	if got := event["payload_b64"]; got != payload {
		t.Fatalf("payload_b64 = %v, want %q", got, payload)
	}
	if got := event["payload_size"]; got != 2 {
		t.Fatalf("payload_size = %v, want 2", got)
	}
	if got := event["file_extension"]; got != ".png" {
		t.Fatalf("file_extension = %v, want .png", got)
	}
	if got := event["content_type"]; got != "image/bmp" {
		t.Fatalf("content_type = %v, want image/bmp", got)
	}
	if strings.Contains(event["text"].(string), payload) {
		t.Fatal("screenshot payload was copied into text")
	}
}

func TestLocateTeamserverDownloadIn(t *testing.T) {
	root := t.TempDir()

	t.Run("nested agent download path", func(t *testing.T) {
		run := filepath.Join(root, "2026.11.25_12.00.00")
		downloadRoot := filepath.Join(run, "agents", "session", "Download", "C:", "dir")
		if err := os.MkdirAll(downloadRoot, 0o755); err != nil {
			t.Fatalf("create download directory: %v", err)
		}
		remotePath := filepath.Join(downloadRoot, "file.txt")
		if err := os.WriteFile(remotePath, []byte("data"), 0o600); err != nil {
			t.Fatalf("write downloaded file: %v", err)
		}

		got := locateTeamserverDownloadIn(root, "session", `C:\dir\file.txt`)
		want, err := filepath.Abs(remotePath)
		if err != nil {
			t.Fatalf("absolute path: %v", err)
		}
		if got != want {
			t.Fatalf("download path = %q, want %q", got, want)
		}
	})

	t.Run("callback full-path filename", func(t *testing.T) {
		run := filepath.Join(root, "2026.11.26_12.00.00")
		downloadRoot := filepath.Join(run, "agents", "session", "Download")
		if err := os.MkdirAll(downloadRoot, 0o755); err != nil {
			t.Fatalf("create download directory: %v", err)
		}
		remotePath := filepath.Join(downloadRoot, `C:\dir\literal.txt`)
		if err := os.WriteFile(remotePath, []byte("data"), 0o600); err != nil {
			t.Fatalf("write downloaded file: %v", err)
		}

		got := locateTeamserverDownloadIn(root, "session", `C:\dir\literal.txt`)
		want, err := filepath.Abs(remotePath)
		if err != nil {
			t.Fatalf("absolute path: %v", err)
		}
		if got != want {
			t.Fatalf("download path = %q, want %q", got, want)
		}
	})

	t.Run("missing file is not fabricated", func(t *testing.T) {
		if got := locateTeamserverDownloadIn(root, "session", `C:\dir\missing.txt`); got != "" {
			t.Fatalf("download path = %q, want empty", got)
		}
	})
}

func TestDedicatedHandlersRejectUnknownActions(t *testing.T) {
	server := NewServer(nil, DefaultConfig())
	ctx := context.Background()

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{"socks", func() (any, error) {
			return server.handleSocks(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"rportfwd", func() (any, error) {
			return server.handleRportfwd(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"token", func() (any, error) {
			return server.handleToken(ctx, json.RawMessage(`{"session_id":"session","subcommand":"invalid"}`))
		}},
		{"shellcode", func() (any, error) {
			return server.handleShellcode(ctx, json.RawMessage(`{"session_id":"session","action":"invalid","arch":"x64","file_path":"missing.bin"}`))
		}},
		{"dotnet", func() (any, error) {
			return server.handleDotnet(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"dll", func() (any, error) {
			return server.handleDll(ctx, json.RawMessage(`{"session_id":"session","action":"invalid","file_path":"missing.dll"}`))
		}},
		{"job", func() (any, error) {
			return server.handleJob(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"proc", func() (any, error) {
			return server.handleProc(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"transfer", func() (any, error) {
			return server.handleTransfer(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"net", func() (any, error) {
			return server.handleNet(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"pivot", func() (any, error) {
			return server.handlePivot(ctx, json.RawMessage(`{"session_id":"session","action":"invalid"}`))
		}},
		{"token logon type", func() (any, error) {
			return server.handleToken(ctx, json.RawMessage(`{"session_id":"session","subcommand":"make","domain":"DOMAIN","username":"user","password":"secret","logon_type":"invalid"}`))
		}},
		{"proc protection", func() (any, error) {
			return server.handleProc(ctx, json.RawMessage(`{"session_id":"session","action":"memory","pid":1,"protection":"invalid"}`))
		}},
		{"proc state", func() (any, error) {
			return server.handleProc(ctx, json.RawMessage(`{"session_id":"session","action":"create","state":"invalid","program":"cmd.exe"}`))
		}},
		{"rportfwd port", func() (any, error) {
			return server.handleRportfwd(ctx, json.RawMessage(`{"session_id":"session","action":"add","bind_host":"127.0.0.1","bind_port":-1,"host":"example.com","port":443}`))
		}},
		{"shellcode arch", func() (any, error) {
			return server.handleShellcode(ctx, json.RawMessage(`{"session_id":"session","action":"execute","arch":"arm","file_path":"missing.bin"}`))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.call(); err == nil {
				t.Fatal("unknown action was accepted")
			}
		})
	}
}

func TestSendCommandValidatesArguments(t *testing.T) {
	server := NewServer(nil, DefaultConfig())
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
		args    string
	}{
		{"shell", "shell", `{}`},
		{"cp", "cp", `{"source":"C:\\a"}`},
		{"dir filters", "dir", `{"files_only":true,"dirs_only":true}`},
		{"remove", "rm", `{}`},
		{"mkdir", "mkdir", `{}`},
		{"cat", "cat", `{}`},
		{"upload", "upload", `{}`},
		{"download", "download", `{}`},
		{"kill", "kill", `{"pid":0}`},
		{"sleep", "sleep", `{"delay":10,"jitter":101}`},
		{"exit", "exit", `{"method":"invalid"}`},
		{"config", "config", `{"key":"","value":"true"}`},
		{"klist", "klist", `{"argument1":"invalid"}`},
		{"purge", "purge", `{}`},
		{"ptt", "ptt", `{"ticket":"not-base64"}`},
		{"unknown", "unknown", `{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := json.RawMessage(`{"session_id":"session","command":"` + test.command + `","args":` + test.args + `}`)
			if _, err := server.handleSendCommand(ctx, args); err == nil {
				t.Fatal("invalid command arguments were accepted")
			}
		})
	}
}

func TestSendRawCommandRequiresInfo(t *testing.T) {
	server := NewServer(nil, DefaultConfig())
	_, err := server.handleSendRawCommand(context.Background(), json.RawMessage(`{"session_id":"session","command_id":"21"}`))
	if err == nil {
		t.Fatal("raw command without info was accepted")
	}
}

func TestToolsCallUsesCentralAuditPath(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	server := NewServer(&operator.Client{}, Config{AuditLog: auditPath, MaxOutputBytes: 262144})
	server.tools["audit_probe"] = &Tool{
		Name:     "audit_probe",
		ReadOnly: true,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}
	if err := server.openAuditLog(); err != nil {
		t.Fatalf("open audit log: %v", err)
	}

	request := &Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"audit_probe","arguments":{"password":"secret"}}`),
	}
	response := server.handleToolsCall(context.Background(), request)
	server.closeAuditLog()
	if response.Error != nil {
		t.Fatalf("tool call failed: %#v", response.Error)
	}

	contents, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	text := string(contents)
	for _, expected := range []string{"tool=audit_probe status=start", "tool=audit_probe status=success", "[REDACTED]"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("audit log does not contain %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "secret") {
		t.Fatalf("audit log contains password: %s", text)
	}
}

func TestDownloadResultContract(t *testing.T) {
	root := t.TempDir()
	downloadRoot := filepath.Join(root, "run", "agents", "session", "Download", "C:", "dir")
	if err := os.MkdirAll(downloadRoot, 0o755); err != nil {
		t.Fatalf("create download directory: %v", err)
	}
	artifact := filepath.Join(downloadRoot, "file.txt")
	if err := os.WriteFile(artifact, []byte("data"), 0o600); err != nil {
		t.Fatalf("write downloaded file: %v", err)
	}

	server := NewServer(nil, Config{LootRoot: root, MaxOutputBytes: 262144})
	result := server.populateDownloadResult(map[string]any{
		"events": []map[string]any{
			{"text": `Started download of file: C:\dir\file.txt`},
			{"text": `Finished download of file: C:\dir\file.txt`},
		},
	}, "session")

	if got, want := result["text"], `Finished download of file: C:\dir\file.txt`; got != want {
		t.Fatalf("text = %v, want %q", got, want)
	}
	if got := result["completed"]; got != true {
		t.Fatalf("completed = %v, want true", got)
	}
	wantPath, err := filepath.Abs(artifact)
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if got := result["teamserver_path"]; got != wantPath {
		t.Fatalf("teamserver_path = %v, want %q", got, wantPath)
	}
	if got, ok := result["limitation"].(string); !ok || got == "" {
		t.Fatalf("limitation = %v, want explicit text", result["limitation"])
	}
	if _, ok := result["payload_b64"]; ok {
		t.Fatal("download result fabricated payload_b64")
	}
}
