package operator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportedTaskConstructorsReturnTaskRequest(t *testing.T) {
	constructors := map[string]func() TaskRequest{
		"ShellTask":          func() TaskRequest { return ShellTask("session", "whoami") },
		"PowerShellTask":     func() TaskRequest { return PowerShellTask("session", "Get-Process") },
		"DirTask":            func() TaskRequest { return DirTask("session", DirOptions{Path: "."}) },
		"CdTask":             func() TaskRequest { return CdTask("session", "C:\\") },
		"PwdTask":            func() TaskRequest { return PwdTask("session") },
		"CatTask":            func() TaskRequest { return CatTask("session", "C:\\file.txt") },
		"DownloadTask":       func() TaskRequest { return DownloadTask("session", "C:\\file.txt") },
		"PsTask":             func() TaskRequest { return PsTask("session") },
		"KillTask":           func() TaskRequest { return KillTask("session", 42) },
		"SleepTask":          func() TaskRequest { return SleepTask("session", 10, 5) },
		"ExitTask":           func() TaskRequest { return ExitTask("session", "thread") },
		"JobsTask":           func() TaskRequest { return JobsTask("session") },
		"SendRawCommandTask": func() TaskRequest { return SendRawCommandTask("session", 21, map[string]string{}, "job list") },
		"CheckinTask":        func() TaskRequest { return CheckinTask("session") },
		"JobTask":            func() TaskRequest { return JobTask("session", "list", 0) },
		"ProcTask":           func() TaskRequest { return ProcTask("session", ProcOptions{Action: "list"}) },
		"SpawnTask":          func() TaskRequest { return SpawnTask("session", ProcOptions{State: "normal", Program: "cmd.exe"}) },
		"TransferTask":       func() TaskRequest { return TransferTask("session", "list", "") },
		"NetTask":            func() TaskRequest { return NetTask("session", "domain", "") },
		"PivotTask":          func() TaskRequest { return PivotTask("session", "list", "", "", "") },
		"ScreenshotTask":     func() TaskRequest { return ScreenshotTask("session") },
		"SocksTask":          func() TaskRequest { return SocksTask("session", "list", "") },
		"RportfwdTask":       func() TaskRequest { return RportfwdTask("session", RportfwdOptions{Action: "list"}) },
		"TokenTask":          func() TaskRequest { return TokenTask("session", TokenOptions{SubCommand: "privs-list"}) },
		"ShellcodeTask":      func() TaskRequest { return ShellcodeTask("session", "execute", "x64", 0, []byte{1}, "") },
		"DotnetTask":         func() TaskRequest { return DotnetTask("session", "list-versions", nil, "") },
		"InlineExecuteTask":  func() TaskRequest { return InlineExecuteTask("session", []byte{1}, "arg") },
		"DllTask":            func() TaskRequest { return DllTask("session", "spawn", []byte{1}, "arg", 0) },
		"InteractiveTask":    func() TaskRequest { return InteractiveTask("session") },
		"CpTask":             func() TaskRequest { return CpTask("session", "C:\\a", "C:\\b") },
		"MvTask":             func() TaskRequest { return MvTask("session", "C:\\a", "C:\\b") },
		"RemoveTask":         func() TaskRequest { return RemoveTask("session", "C:\\a") },
		"MkdirTask":          func() TaskRequest { return MkdirTask("session", "C:\\a") },
		"UploadTask":         func() TaskRequest { return UploadTask("session", "C:\\a", "Y29udGVudA==") },
		"ConfigTask":         func() TaskRequest { return ConfigTask("session", "implant.verbose", "true") },
		"LuidTask":           func() TaskRequest { return LuidTask("session") },
		"KlistTask":          func() TaskRequest { return KlistTask("session", "/all", "") },
		"PurgeTask":          func() TaskRequest { return PurgeTask("session", "1234") },
		"PttTask":            func() TaskRequest { return PttTask("session", "dGlja2V0", "") },
	}

	if got, want := len(constructors), 39; got != want {
		t.Fatalf("exported task constructor count = %d, want %d", got, want)
	}

	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			request := constructor()
			if request.SessionID != "session" {
				t.Fatalf("SessionID = %q, want session", request.SessionID)
			}
			if request.Info == nil {
				t.Fatal("Info is nil")
			}
			if request.CommandLine == "" {
				t.Fatal("CommandLine is empty")
			}
		})
	}
}

func TestDecodeOutputPreservesOnlyScreenshotEnvelope(t *testing.T) {
	screenshot, err := json.Marshal(map[string]string{
		"Type":     "Good",
		"Message":  "Successful took screenshot",
		"MiscType": "screenshot",
		"MiscData": "Qk0=",
	})
	if err != nil {
		t.Fatalf("encode screenshot output: %v", err)
	}
	decoded := decodeOutput(base64.StdEncoding.EncodeToString(screenshot))
	var envelope map[string]string
	if err := json.Unmarshal([]byte(decoded), &envelope); err != nil {
		t.Fatalf("decode screenshot envelope: %v", err)
	}
	if envelope["MiscData"] != "Qk0=" {
		t.Fatalf("MiscData = %q, want Qk0=", envelope["MiscData"])
	}

	regular, err := json.Marshal(map[string]string{
		"Type":     "Good",
		"Message":  "ordinary output",
		"MiscType": "download",
	})
	if err != nil {
		t.Fatalf("encode regular output: %v", err)
	}
	if got := decodeOutput(base64.StdEncoding.EncodeToString(regular)); got != "ordinary output" {
		t.Fatalf("regular output = %q, want ordinary output", got)
	}
}

func TestSourceBackedTaskPatterns(t *testing.T) {
	t.Run("shell alias", func(t *testing.T) {
		request := ShellTask("session", "whoami")
		if request.CommandID != CommandProc {
			t.Fatalf("CommandID = %d, want %d", request.CommandID, CommandProc)
		}
		wantArgs := "0;FALSE;TRUE;c:\\windows\\system32\\cmd.exe;" + base64.StdEncoding.EncodeToString([]byte("/c whoami"))
		if got := request.Info["Args"]; got != wantArgs {
			t.Fatalf("Args = %q, want %q", got, wantArgs)
		}
	})

	t.Run("upload", func(t *testing.T) {
		request := UploadTask("session", "C:\\remote.txt", "Y29udGVudA==")
		want := base64.StdEncoding.EncodeToString([]byte("C:\\remote.txt")) + ";Y29udGVudA=="
		if got := request.Info["Arguments"]; got != want {
			t.Fatalf("Arguments = %q, want %q", got, want)
		}
	})

	t.Run("token make", func(t *testing.T) {
		request := TokenTask("session", TokenOptions{
			SubCommand: "make",
			Domain:     "DOMAIN",
			Username:   "user",
			Password:   "secret",
			LogonType:  "LOGON_INTERACTIVE",
		})
		want := strings.Join([]string{
			base64.StdEncoding.EncodeToString([]byte("DOMAIN")),
			base64.StdEncoding.EncodeToString([]byte("user")),
			base64.StdEncoding.EncodeToString([]byte("secret")),
			"2",
		}, ";")
		if got := request.Info["Arguments"]; got != want {
			t.Fatalf("Arguments = %q, want %q", got, want)
		}
		if strings.Contains(request.CommandLine, "secret") {
			t.Fatalf("CommandLine contains password: %q", request.CommandLine)
		}
	})

	t.Run("proc create", func(t *testing.T) {
		request := ProcTask("session", ProcOptions{
			Action:    "create",
			State:     "suspended",
			Silent:    true,
			NoPipe:    true,
			Program:   "cmd.exe",
			Arguments: "/c whoami",
		})
		want := "4;FALSE;FALSE;cmd.exe;" + base64.StdEncoding.EncodeToString([]byte(`"cmd.exe" /c whoami`))
		if got := request.Info["Args"]; got != want {
			t.Fatalf("Args = %q, want %q", got, want)
		}
	})

	t.Run("net default target", func(t *testing.T) {
		request := NetTask("session", "logons", "")
		if got := request.Info["Param"]; got != `\\localhost` {
			t.Fatalf("Param = %q, want %q", got, `\\localhost`)
		}
	})

	t.Run("pivot connect", func(t *testing.T) {
		request := PivotTask("session", "connect", "server", "pipe", "")
		if got := request.Info["Param"]; got != `\\server\pipe\pipe` {
			t.Fatalf("Param = %q, want %q", got, `\\server\pipe\pipe`)
		}
	})

	t.Run("shellcode inject", func(t *testing.T) {
		request := ShellcodeTask("session", "inject", "x64", 42, []byte{1, 2}, "arg", "shellcode.bin")
		if request.Info["Way"] != "Inject" || request.Info["PID"] != "42" || request.Info["Arch"] != "x64" {
			t.Fatalf("unexpected shellcode info: %#v", request.Info)
		}
		if got, want := request.Info["Argument"], base64.StdEncoding.EncodeToString([]byte("arg")); got != want {
			t.Fatalf("Argument = %q, want %q", got, want)
		}
		if got, want := request.Info["Arguments"], request.Info["Argument"]; got != want {
			t.Fatalf("Arguments = %q, want compatibility value %q", got, want)
		}
		if got, want := request.CommandLine, "shellcode inject x64 42 shellcode.bin"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("dll spawn", func(t *testing.T) {
		request := DllTask("session", "spawn", []byte{1}, "arg", 0, "payload.dll")
		if request.CommandID != CommandInjectDLLSpawn {
			t.Fatalf("CommandID = %d, want %d", request.CommandID, CommandInjectDLLSpawn)
		}
		if got, want := request.Info["Arguments"], base64.StdEncoding.EncodeToString([]byte("arg")); got != want {
			t.Fatalf("Arguments = %q, want %q", got, want)
		}
		if got, want := request.CommandLine, "dll spawn payload.dll arg"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("proc create command line", func(t *testing.T) {
		request := ProcTask("session", ProcOptions{
			Action:    "create",
			State:     "suspended",
			Silent:    true,
			NoPipe:    true,
			Program:   "cmd.exe",
			Arguments: "/c whoami",
		})
		if got, want := request.CommandLine, "proc create suspended --silent --no-pipe cmd.exe /c whoami"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("spawn uses authoritative proc create command line", func(t *testing.T) {
		request := SpawnTask("session", ProcOptions{State: "normal", Program: "cmd.exe"})
		if got, want := request.CommandLine, "proc create normal cmd.exe"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("rportfwd add command line", func(t *testing.T) {
		request := RportfwdTask("session", RportfwdOptions{
			Action:   "add",
			BindHost: "127.0.0.1",
			BindPort: 8080,
			Host:     "example.com",
			Port:     443,
		})
		if got, want := request.CommandLine, "rportfwd add 127.0.0.1 8080 example.com 443"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("dotnet inline execute command line", func(t *testing.T) {
		request := DotnetTask("session", "inline-execute", []byte{1}, "arg", "assembly.dll")
		if got, want := request.CommandLine, "dotnet inline-execute assembly.dll arg"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("inline execute command line", func(t *testing.T) {
		request := InlineExecuteTask("session", []byte{1}, "arg", "object.bof")
		if got, want := request.CommandLine, "inline-execute object.bof arg"; got != want {
			t.Fatalf("CommandLine = %q, want %q", got, want)
		}
	})

	t.Run("ptt ticket is base64", func(t *testing.T) {
		request := PttTask("session", "dGlja2V0", "")
		decoded, err := base64.StdEncoding.DecodeString(request.Info["Ticket"])
		if err != nil {
			t.Fatalf("decode ticket: %v", err)
		}
		if got, want := string(decoded), "ticket"; got != want {
			t.Fatalf("decoded ticket = %q, want %q", got, want)
		}
	})
}

func TestSendTaskCapturesCursorBeforePacketWrite(t *testing.T) {
	client := &Client{
		cfg:          Config{User: "operator"},
		writeCh:      make(chan writeRequest, 1),
		outputs:      map[string]*ringBuffer{"session": newRingBuffer(16)},
		pendingTasks: make(map[string]*Task),
	}

	go func() {
		request := <-client.writeCh
		client.outputs["session"].Push(OutputEvent{Text: "response"})
		request.done <- nil
	}()

	task, err := client.SendTask(TaskRequest{
		SessionID:   "session",
		CommandID:   CommandProcList,
		Info:        map[string]string{"CommandID": "12"},
		CommandLine: "proc list",
	})
	if err != nil {
		t.Fatalf("send task: %v", err)
	}
	if task.CursorAtSend != 0 {
		t.Fatalf("CursorAtSend = %d, want pre-send cursor 0", task.CursorAtSend)
	}

	page := client.ReadOutput(task.SessionID, task.CursorAtSend, 1024*1024)
	if got, want := len(page.Events), 1; got != want {
		t.Fatalf("output event count = %d, want %d", got, want)
	}
}

func TestWaitOutputUntilReturnsEventsThroughMatch(t *testing.T) {
	client := &Client{outputs: map[string]*ringBuffer{"session": newRingBuffer(16)}}
	client.outputs["session"].Push(OutputEvent{Text: "Started download of file: C:\\file.txt"})
	client.outputs["session"].Push(OutputEvent{Text: "Finished download of file: C:\\file.txt"})

	page := client.WaitOutputUntil(context.Background(), "session", 0, time.Second, func(event OutputEvent) bool {
		return event.Text == "Finished download of file: C:\\file.txt"
	})
	if got, want := len(page.Events), 2; got != want {
		t.Fatalf("output event count = %d, want %d", got, want)
	}
	if got := page.Events[len(page.Events)-1].Text; got != "Finished download of file: C:\\file.txt" {
		t.Fatalf("final event = %q", got)
	}
}
