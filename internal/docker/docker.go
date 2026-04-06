package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ComposeUp() error {
	// Check if docker-compose.yml exists
	if _, err := os.Stat("docker-compose.yml"); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yml not found")
	}

	colorCyan := "\033[36m"
	colorGreen := "\033[32m"
	colorReset := "\033[0m"

	fmt.Printf("%sRunning docker-compose up -d...%s\n", colorCyan, colorReset)
	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("docker-compose up failed: %v", err)
	}

	// Check services status
	fmt.Printf("%sChecking services...%s\n", colorCyan, colorReset)
	cmd = exec.Command("docker-compose", "ps", "--services")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}
	services := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(services) == 0 {
		fmt.Printf("%sNo services found.%s\n", colorCyan, colorReset)
		return nil
	}

	fmt.Printf("%sServices running:%s\n", colorGreen, colorReset)
	for _, svc := range services {
		if svc == "" {
			continue
		}
		// Get status
		cmd = exec.Command("docker-compose", "ps", "-a", "--filter", "service="+svc, "--format", "{{.Status}}")
		statusOut, _ := cmd.Output()
		status := strings.TrimSpace(string(statusOut))
		if strings.Contains(status, "Up") {
			fmt.Printf("  %s: %sUp%s\n", svc, colorGreen, colorReset)
		} else {
			fmt.Printf("  %s: %s%s%s\n", svc, "\033[31m", status, colorReset)
		}
	}
	return nil
}
