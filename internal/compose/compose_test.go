package compose

import "testing"

// TestParseServices проверяет извлечение сервисов и образов из compose-файла.
func TestParseServices(t *testing.T) {
	data := []byte(`version: '3'
services:
  web:
    image: nginx
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: app
  worker:
    build: .
volumes:
  data:
`)
	services := ParseServices(data)
	if len(services) != 3 {
		t.Fatalf("ParseServices вернул %d сервисов, ожидалось 3: %v", len(services), services)
	}
	want := []Service{
		{Name: "web", Image: "nginx"},
		{Name: "db", Image: "postgres:16"},
		{Name: "worker"},
	}
	for i, svc := range services {
		if svc != want[i] {
			t.Errorf("service[%d] = %+v, want %+v", i, svc, want[i])
		}
	}
}

// TestParseServicesWithoutBlock проверяет, что без блока services результат пуст.
func TestParseServicesWithoutBlock(t *testing.T) {
	if services := ParseServices([]byte("version: '3'\nvolumes:\n  data:\n")); services != nil {
		t.Errorf("ParseServices без services = %v, ожидался nil", services)
	}
}

// TestReadServicesMissingFile проверяет, что отсутствие файла не ошибка.
func TestReadServicesMissingFile(t *testing.T) {
	if services := ReadServices(t.TempDir()); services != nil {
		t.Errorf("ReadServices без файла = %v, ожидался nil", services)
	}
}
