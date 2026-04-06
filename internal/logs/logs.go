package logs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LogEntry struct {
	Path string
	Type string // "file", "docker"
}

func FindLogs(projectRoot string) ([]LogEntry, error) {
	var entries []LogEntry

	// 1. Find *.log files
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
			rel, _ := filepath.Rel(projectRoot, path)
			entries = append(entries, LogEntry{
				Path: rel,
				Type: "file",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. Docker logs (simplified: check for running containers)
	entries = append(entries, findDockerLogs()...)

	return entries, nil
}

func findDockerLogs() []LogEntry {
	// Try to list containers
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var entries []LogEntry
	for _, name := range lines {
		if name == "" {
			continue
		}
		entries = append(entries, LogEntry{
			Path: name,
			Type: "docker",
		})
	}
	return entries
}

func OpenLogInLnav(path string, logType string) error {
	var cmd *exec.Cmd
	if logType == "docker" {
		cmd = exec.Command("docker", "logs", "-f", path)
	} else {
		cmd = exec.Command("lnav", path)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
