//go:build linux

package procfs

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// freePort возвращает свободный TCP-порт.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось получить свободный порт: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// TestParseTCPLine проверяет разбор строк таблиц /proc/net/tcp и tcp6.
func TestParseTCPLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantPort  int
		wantState string
		wantInode string
		wantOK    bool
	}{
		{
			name:   "заголовок",
			line:   "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
			wantOK: false,
		},
		{
			name:      "ipv4 LISTEN 8000",
			line:      "   0: 0100007F:1F40 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 77414218 1",
			wantPort:  8000,
			wantState: "0A",
			wantInode: "77414218",
			wantOK:    true,
		},
		{
			name:      "ipv6 LISTEN 5432",
			line:      "   3: 00000000000000000000000000000000:1538 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4214 1",
			wantPort:  5432,
			wantState: "0A",
			wantInode: "4214",
			wantOK:    true,
		},
		{
			name:      "ESTABLISHED",
			line:      "   1: 0100007F:1F40 0100007F:D431 01 00000000:00000000 00:00000000 00000000  1000        0 111 1",
			wantPort:  8000,
			wantState: "01",
			wantInode: "111",
			wantOK:    true,
		},
		{
			name:   "мусор",
			line:   "not a tcp line",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, state, inode, ok := parseTCPLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, ожидалось %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, ожидалось %d", port, tt.wantPort)
			}
			if state != tt.wantState {
				t.Errorf("state = %q, ожидалось %q", state, tt.wantState)
			}
			if inode != tt.wantInode {
				t.Errorf("inode = %q, ожидалось %q", inode, tt.wantInode)
			}
		})
	}
}

// TestMapsHoldsPath проверяет сравнение пути в строках /proc/<pid>/maps.
func TestMapsHoldsPath(t *testing.T) {
	tests := []struct {
		name string
		line string
		path string
		want bool
	}{
		{
			name: "обычный путь",
			line: "7f00-7f01 r-xp 00000000 08:01 12345 /usr/bin/foo",
			path: "/usr/bin/foo",
			want: true,
		},
		{
			name: "удалённый файл",
			line: "7f00-7f01 r-xp 00000000 08:01 12345 /usr/bin/foo (deleted)",
			path: "/usr/bin/foo",
			want: true,
		},
		{
			name: "путь с пробелом",
			line: "7f00-7f01 r-xp 00000000 08:01 12345 /opt/my app/bin/foo",
			path: "/opt/my app/bin/foo",
			want: true,
		},
		{
			name: "другой путь",
			line: "7f00-7f01 r-xp 00000000 08:01 12345 /usr/bin/bar",
			path: "/usr/bin/foo",
			want: false,
		},
		{
			name: "анонимное отображение",
			line: "7f00-7f01 r-xp 00000000 00:00 0",
			path: "/usr/bin/foo",
			want: false,
		},
		{
			name: "короткая строка",
			line: "7f00-7f01 r-xp",
			path: "/usr/bin/foo",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapsHoldsPath(tt.line, tt.path); got != tt.want {
				t.Errorf("mapsHoldsPath(%q, %q) = %v, ожидалось %v", tt.line, tt.path, got, tt.want)
			}
		})
	}
}

// TestListenersFindsCurrentProcess проверяет, что владелец слушающего порта
// определяется по inode сокета.
func TestListenersFindsCurrentProcess(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось открыть слушающий сокет: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	listeners, err := Listeners(port)
	if err != nil {
		t.Fatalf("Listeners(%d) вернул ошибку: %v", port, err)
	}

	found := false
	for _, x := range listeners {
		if x.PID == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Errorf("Listeners(%d) не нашёл текущий процесс среди владельцев: %+v", port, listeners)
	}
}

// TestListenersFreePort проверяет, что свободный порт не имеет владельцев.
func TestListenersFreePort(t *testing.T) {
	port := freePort(t)
	listeners, err := Listeners(port)
	if err != nil {
		t.Fatalf("Listeners(%d) вернул ошибку: %v", port, err)
	}
	if len(listeners) != 0 {
		t.Errorf("Listeners(%d) вернул владельцев для свободного порта: %+v", port, listeners)
	}
}

// TestHoldersFindsCurrentProcess проверяет поиск держателя файла по fd.
func TestHoldersFindsCurrentProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("не удалось создать файл: %v", err)
	}
	defer f.Close()

	pids, err := Holders(path)
	if err != nil {
		t.Fatalf("Holders(%q) вернул ошибку: %v", path, err)
	}

	found := false
	for _, pid := range pids {
		if pid == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Errorf("Holders(%q) не нашёл текущий процесс: %v", path, pids)
	}
}

// TestKillPortFreePort проверяет, что убийство на свободном порту не является ошибкой.
func TestKillPortFreePort(t *testing.T) {
	port := freePort(t)
	if err := KillPort(port); err != nil {
		t.Errorf("KillPort(%d) на свободном порту вернул ошибку: %v", port, err)
	}
}

// TestHelperHolder — вспомогательный процесс, удерживающий открытый файл.
func TestHelperHolder(t *testing.T) {
	if os.Getenv("DEV_TEST_HOLDER") != "1" {
		return
	}
	f, err := os.Open(os.Getenv("DEV_TEST_HOLDER_FILE"))
	if err != nil {
		os.Exit(1)
	}
	_ = f
	for {
		time.Sleep(time.Hour)
	}
}

// TestKillFile проверяет, что KillFile завершает процесс, удерживающий файл.
func TestKillFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.bin")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("не удалось создать файл: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperHolder")
	cmd.Env = append(os.Environ(),
		"DEV_TEST_HOLDER=1",
		"DEV_TEST_HOLDER_FILE="+path,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("не удалось запустить вспомогательный процесс: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Ждём, пока вспомогательный процесс откроет файл.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pids, err := Holders(path)
		if err != nil {
			t.Fatalf("Holders(%q) вернул ошибку: %v", path, err)
		}
		for _, pid := range pids {
			if pid == cmd.Process.Pid {
				goto found
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("вспомогательный процесс не открыл файл %q", path)

found:
	if err := KillFile(path); err != nil {
		t.Fatalf("KillFile(%q) вернул ошибку: %v", path, err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("вспомогательный процесс должен быть завершён сигналом после KillFile")
	}
}
