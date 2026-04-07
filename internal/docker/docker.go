package docker

import (
	"dev/internal/colors"
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

	fmt.Println(colors.Cyan("Running docker-compose up -d..."))
	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("docker-compose up failed: %v", err)
	}

	// Check services status
	fmt.Println(colors.Cyan("Checking services..."))
	cmd = exec.Command("docker-compose", "ps", "--services")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}
	services := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(services) == 0 {
		fmt.Println(colors.Cyan("No services found."))
		return nil
	}

	fmt.Println(colors.Green("Services running:"))
	for _, svc := range services {
		if svc == "" {
			continue
		}
		// Get status
		cmd = exec.Command("docker-compose", "ps", "-a", "--filter", "service="+svc, "--format", "{{.Status}}")
		statusOut, _ := cmd.Output()
		status := strings.TrimSpace(string(statusOut))
		if strings.Contains(status, "Up") {
			fmt.Printf("  %s: %s\n", svc, colors.Green("Up"))
		} else {
			fmt.Printf("  %s: %s\n", svc, colors.Red(status))
		}
	}
	return nil
}
