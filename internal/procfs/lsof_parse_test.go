package procfs

import "testing"

// TestParseLsofListeners проверяет разбор вывода lsof -Fpcn.
func TestParseLsofListeners(t *testing.T) {
	out := []byte("p1234\ncpostgres\nn127.0.0.1:5432\np5678\ncmain\nn*:8000\n")
	got := parseLsofListeners(out)

	want := []Listener{
		{PID: 1234, Name: "postgres"},
		{PID: 5678, Name: "main"},
	}
	if len(got) != len(want) {
		t.Fatalf("получено %d владельцев, ожидалось %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("владелец %d = %+v, ожидалось %+v", i, got[i], want[i])
		}
	}
}

// TestParseLsofListenersDuplicates проверяет, что несколько сокетов одного
// процесса схлопываются в одного владельца.
func TestParseLsofListenersDuplicates(t *testing.T) {
	out := []byte("p1234\ncpostgres\nn127.0.0.1:5432\nn[::1]:5432\n")
	got := parseLsofListeners(out)

	if len(got) != 1 {
		t.Fatalf("получено %d владельцев, ожидался 1: %+v", len(got), got)
	}
	if got[0].PID != 1234 || got[0].Name != "postgres" {
		t.Errorf("владелец = %+v, ожидалось {1234 postgres}", got[0])
	}
}

// TestParseLsofListenersEmpty проверяет пустой вывод.
func TestParseLsofListenersEmpty(t *testing.T) {
	if got := parseLsofListeners(nil); len(got) != 0 {
		t.Errorf("для пустого вывода получены владельцы: %+v", got)
	}
}
