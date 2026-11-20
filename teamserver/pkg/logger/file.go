package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	fileLogFileName   = "crypto.log"
	fileLogTimeLayout = "2006-01-02 15:04:05"
)

// fileLog is the process-wide handle for the crypto/payload audit log.
// A single file is shared by every caller and guarded by a mutex so
// multi-line hex dumps never interleave.
var fileLog struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// resolveTeamserverDir returns the teamserver directory that owns the log.
//
// It prefers the directory next to the executable (the normal layout is
// <repo>/havoc plus <repo>/teamserver). If the binary is not in the repository
// (for example `go run`), it walks up from the working directory looking for a
// sibling `teamserver` directory.
func resolveTeamserverDir() string {
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		if isDirectory(filepath.Join(executableDir, "teamserver")) {
			return filepath.Join(executableDir, "teamserver")
		}
	}

	if workingDir, err := os.Getwd(); err == nil {
		for {
			if isDirectory(filepath.Join(workingDir, "teamserver")) {
				return filepath.Join(workingDir, "teamserver")
			}

			parent := filepath.Dir(workingDir)
			if parent == workingDir {
				break
			}
			workingDir = parent
		}
	}

	return "teamserver"
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileLogPath() string {
	return filepath.Join(resolveTeamserverDir(), "logs", fileLogFileName)
}

// FileLog writes one timestamped entry to <teamserver>/logs/crypto.log.
// It is safe for concurrent use. component identifies the caller ("aes" or
// "agent"); format and args follow fmt.Sprintf.
//
// The file and directory are created on first use. The file is append-only and
// mode 0600 because it contains live session keys.
func FileLog(component string, format string, args ...interface{}) {
	entry := fmt.Sprintf("%s [%s] %s\n",
		time.Now().Format(fileLogTimeLayout),
		component,
		fmt.Sprintf(format, args...),
	)

	fileLog.mu.Lock()
	defer fileLog.mu.Unlock()

	if fileLog.file == nil {
		if err := openFileLogLocked(); err != nil {
			fmt.Fprintf(os.Stderr, "logger: cannot open file log: %v\n", err)
			return
		}
	}

	if _, err := fileLog.file.WriteString(entry); err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot write file log: %v\n", err)
	}
}

// FileLogPath reports the absolute path of the audit log without creating it.
func FileLogPath() string {
	fileLog.mu.Lock()
	path := fileLog.path
	fileLog.mu.Unlock()

	if path == "" {
		path = fileLogPath()
	}

	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}

	return path
}

func openFileLogLocked() error {
	path := fileLogPath()

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	fileLog.file = file
	fileLog.path = path

	return nil
}
