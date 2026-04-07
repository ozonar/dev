package docker

import (
	"os"
	"strings"
	"testing"
)

// TestComposeUpNoDockerCompose проверяет ошибку при отсутствии docker-compose.yml
func TestComposeUpNoDockerCompose(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	err = ComposeUp()
	if err == nil {
		t.Error("ожидалась ошибка 'docker-compose.yml not found'")
	}
	if !strings.Contains(err.Error(), "docker-compose.yml not found") {
		t.Errorf("сообщение об ошибке должно содержать 'docker-compose.yml not found', получили: %v", err)
	}
}
