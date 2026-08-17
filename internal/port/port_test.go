package port

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// TestHelperServer — вспомогательный процесс-сервер, запускаемый внутри
// тестового бинарника. Служит изолированным процессом, чтобы проверять
// убийство процесса по порту без риска для самого теста.
func TestHelperServer(t *testing.T) {
	// Этот тест запускается только как дочерний процесс
	if os.Getenv("DEV_TEST_HELPER") != "1" {
		return
	}
	port := os.Getenv("DEV_TEST_PORT")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	// ListenAndServe блокирует, удерживая процесс живым до принудительного kill
	_ = http.ListenAndServe("127.0.0.1:"+port, mux)
}

// startHelperServer запускает дочерний процесс-сервер на указанном порту.
func startHelperServer(t *testing.T, port int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperServer")
	cmd.Env = append(os.Environ(),
		"DEV_TEST_HELPER=1",
		"DEV_TEST_PORT="+strconv.Itoa(port),
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("не удалось запустить вспомогательный сервер: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Ждём, пока сервер начнёт слушать порт
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return cmd
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("вспомогательный сервер не начал слушать порт %d", port)
	return cmd
}

// getFreePort получает временный свободный порт для тестов.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось получить свободный порт: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// TestIsPortOccupiedFree проверяет, что свободный порт определяется как незанятый.
func TestIsPortOccupiedFree(t *testing.T) {
	port := getFreePort(t)
	occupied, _ := IsPortOccupied(port)
	if occupied {
		t.Errorf("IsPortOccupied(%d) = true для свободного порта", port)
	}
}

// TestIsPortOccupiedBusy проверяет, что занятый порт определяется как занятый.
func TestIsPortOccupiedBusy(t *testing.T) {
	port := getFreePort(t)
	startHelperServer(t, port)

	occupied, info := IsPortOccupied(port)
	if !occupied {
		t.Errorf("IsPortOccupied(%d) = false для занятого порта", port)
	}
	_ = info
}

// TestKillProcessOnPort проверяет, что KillProcessOnPort убивает процесс на порту.
func TestKillProcessOnPort(t *testing.T) {
	port := getFreePort(t)
	cmd := startHelperServer(t, port)

	// Убеждаемся, что порт занят
	if occupied, _ := IsPortOccupied(port); !occupied {
		t.Fatalf("порт %d должен быть занят перед KillProcessOnPort", port)
	}

	if err := KillProcessOnPort(port); err != nil {
		t.Fatalf("KillProcessOnPort(%d) вернул ошибку: %v", port, err)
	}

	// Проверяем, что дочерний процесс был завершён сигналом
	if err := cmd.Wait(); err == nil {
		t.Error("вспомогательный сервер должен быть завершён сигналом после KillProcessOnPort")
	}

	// Проверяем, что порт освободился
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		occupied, _ := IsPortOccupied(port)
		if !occupied {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("порт %d всё ещё занят после KillProcessOnPort", port)
}

// TestKillProcessOnPortFree проверяет, что вызов на свободном порту не падает.
func TestKillProcessOnPortFree(t *testing.T) {
	port := getFreePort(t)
	if err := KillProcessOnPort(port); err != nil {
		t.Errorf("KillProcessOnPort(%d) на свободном порту вернул ошибку: %v", port, err)
	}
}
