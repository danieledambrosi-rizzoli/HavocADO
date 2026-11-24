package operator

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// CommandID constants matching teamserver/pkg/agent/demons.go
const (
	CommandProc       = 0x1010 // 4112
	CommandFs         = 15
	CommandSleep      = 11
	CommandProcList   = 12
	CommandJob        = 21
	CommandExit       = 92
	CommandToken      = 40
	CommandTeamserver = -1 // Special marker for "Teamserver" string
)

// DirOptions holds options for dir/ls command.
type DirOptions struct {
	Path      string
	Recursive bool
	FilesOnly bool
	DirsOnly  bool
	ListOnly  bool
	Starts    string
	Contains  string
	Ends      string
}

// TaskRequest represents a task to send to an agent.
type TaskRequest struct {
	SessionID   string
	CommandID   int
	Info        map[string]string
	CommandLine string
}

// ShellTask creates a shell command task.
func ShellTask(sessionID, command string) TaskRequest {
	encodedCmd := base64.StdEncoding.EncodeToString([]byte("/c " + command))
	args := fmt.Sprintf("0;FALSE;TRUE;c:\\windows\\system32\\cmd.exe;%s", encodedCmd)

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProc,
		Info: map[string]string{
			"DemonID":     sessionID,
			"CommandID":   strconv.Itoa(CommandProc),
			"ProcCommand": "4",
			"Args":        args,
		},
		CommandLine: "shell " + command,
	}
}

// PowerShellTask creates a PowerShell command task.
func PowerShellTask(sessionID, command string) TaskRequest {
	encodedCmd := base64.StdEncoding.EncodeToString([]byte("-C " + command))
	args := fmt.Sprintf("0;FALSE;TRUE;C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe;%s", encodedCmd)

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProc,
		Info: map[string]string{
			"DemonID":     sessionID,
			"CommandID":   strconv.Itoa(CommandProc),
			"ProcCommand": "4",
			"Args":        args,
		},
		CommandLine: "powershell " + command,
	}
}

// DirTask creates a directory listing task.
func DirTask(sessionID string, opts DirOptions) TaskRequest {
	path := opts.Path
	if path == "" {
		path = "."
	}

	// Arguments: path;subdirs;filesOnly;dirsOnly;listOnly;starts;contains;ends
	args := fmt.Sprintf("%s;%v;%v;%v;%v;%s;%s;%s",
		path,
		opts.Recursive,
		opts.FilesOnly,
		opts.DirsOnly,
		opts.ListOnly,
		opts.Starts,
		opts.Contains,
		opts.Ends,
	)

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "dir",
			"Arguments":  args,
		},
		CommandLine: fmt.Sprintf("ls %s", path),
	}
}

// CdTask creates a change directory task.
func CdTask(sessionID, path string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "cd",
			"Arguments":  path,
		},
		CommandLine: "cd " + path,
	}
}

// PwdTask creates a print working directory task.
func PwdTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "pwd",
			"Arguments":  "",
		},
		CommandLine: "pwd",
	}
}

// CatTask creates a file read task.
func CatTask(sessionID, path string) TaskRequest {
	encodedPath := base64.StdEncoding.EncodeToString([]byte(path))
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "cat",
			"Arguments":  encodedPath,
		},
		CommandLine: "cat " + path,
	}
}

// DownloadTask creates a file download task.
func DownloadTask(sessionID, path string) TaskRequest {
	encodedPath := base64.StdEncoding.EncodeToString([]byte(path))
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "download",
			"Arguments":  encodedPath,
		},
		CommandLine: "download " + path,
	}
}

// PsTask creates a process list task.
func PsTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProcList,
		Info: map[string]string{
			"DemonID":            sessionID,
			"CommandID":          strconv.Itoa(CommandProcList),
			"FromProcessManager": "false",
		},
		CommandLine: "ps",
	}
}

// KillTask creates a process kill task.
func KillTask(sessionID string, pid int) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProc,
		Info: map[string]string{
			"DemonID":     sessionID,
			"CommandID":   strconv.Itoa(CommandProc),
			"ProcCommand": "7",
			"Args":        strconv.Itoa(pid),
		},
		CommandLine: fmt.Sprintf("kill %d", pid),
	}
}

// SleepTask creates a sleep modification task.
func SleepTask(sessionID string, delay, jitter int) TaskRequest {
	args := fmt.Sprintf("%d;%d", delay, jitter)
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandSleep,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandSleep),
			"Arguments": args,
		},
		CommandLine: fmt.Sprintf("sleep %d %d", delay, jitter),
	}
}

// ExitTask creates an agent exit task.
func ExitTask(sessionID, method string) TaskRequest {
	if method == "" {
		method = "thread"
	}
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandExit,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandExit),
			"ExitMethod": method,
		},
		CommandLine: "exit " + method,
	}
}

// JobsTask creates a job list task.
func JobsTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandJob,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandJob),
			"Command":   "list",
			"Param":     "0",
		},
		CommandLine: "jobs",
	}
}

// SendRawCommandTask creates a raw command task (escape hatch).
func SendRawCommandTask(sessionID string, commandID interface{}, info map[string]string, commandLine string) TaskRequest {
	var cmdIDStr string
	switch v := commandID.(type) {
	case int:
		cmdIDStr = strconv.Itoa(v)
	case string:
		cmdIDStr = v
	default:
		cmdIDStr = "0"
	}

	// Ensure DemonID and CommandID are in info
	if _, ok := info["DemonID"]; !ok {
		info["DemonID"] = sessionID
	}
	if _, ok := info["CommandID"]; !ok {
		info["CommandID"] = cmdIDStr
	}

	return TaskRequest{
		SessionID:   sessionID,
		CommandID:   0, // Will be parsed from info
		Info:        info,
		CommandLine: commandLine,
	}
}

// generateTaskID generates an 8-hex uppercase task ID.
func generateTaskID() string {
	return fmt.Sprintf("%08X", time.Now().UnixNano()&0xFFFFFFFF)
}

// PrepareTaskRequest prepares a task request with generated TaskID.
func PrepareTaskRequest(req TaskRequest) TaskRequest {
	taskID := generateTaskID()
	req.Info["TaskID"] = taskID
	return req
}
