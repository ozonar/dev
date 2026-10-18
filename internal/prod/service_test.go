package prod

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"dev/internal/colors"
)

// TestParseUnitFileLines проверяет извлечение имён юнитов из вывода systemctl.
func TestParseUnitFileLines(t *testing.T) {
	out := "nginx.service\tloaded\tactive\trunning\tNginx\n" +
		"php8.3-fpm.service\tloaded\tactive\trunning\tPHP-FPM\n" +
		"\n" +
		"redis.service\tloaded\tinactive\tdead\tRedis\n"
	got := parseUnitFileLines(out)
	want := []string{"nginx.service", "php8.3-fpm.service", "redis.service"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ожидался %v, получено %v", want, got)
	}
	// Пустой вывод — пустой список.
	if got := parseUnitFileLines(""); len(got) != 0 {
		t.Errorf("ожидался пустой список, получено %v", got)
	}
}

// TestTrimServiceSuffix проверяет удаление суффикса ".service".
func TestTrimServiceSuffix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"php8.3-fpm.service", "php8.3-fpm"},
		{"nginx.service", "nginx"},
		{"nginx", "nginx"},
	}
	for _, tc := range cases {
		if got := trimServiceSuffix(tc.in); got != tc.want {
			t.Errorf("trimServiceSuffix(%q) = %q, ожидалось %q", tc.in, got, tc.want)
		}
	}
}

// TestExcludedService проверяет фильтрацию служб по подстрокам исключения.
func TestExcludedService(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"dbus-org.bluez.service", true},
		{"dbus-org.freedesktop.resolve1.service", true},
		{"ollama.service", false},
		{"redis.service", false},
	}
	for _, tc := range cases {
		if got := excludedService(tc.name); got != tc.want {
			t.Errorf("excludedService(%q) = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

// TestParseCrashedUnits проверяет извлечение имён служб из строк журнала сбоев.
func TestParseCrashedUnits(t *testing.T) {
	lines := []string{
		"2026-10-18T12:00:00+0700 host systemd[1]: foo.service: Main process exited, code=exited, status=1/FAILURE",
		"2026-10-18T12:00:00+0700 host systemd[1]: foo.service: Failed with result 'exit-code'.",
		"2026-10-18T12:00:01+0700 host systemd[1]: Failed to start Foo service.",
		"2026-10-18T12:00:02+0700 host systemd[1]: bar.service: Failed with result 'signal'.",
		"2026-10-18T12:00:03+0700 host systemd[1]: baz.service: Failed with result 'timeout'.",
		"2026-10-18T12:00:03+0700 host systemd[1]: baz.service: Failed with result 'timeout'.",
	}
	got := parseCrashedUnits(lines)
	want := []string{"bar.service", "baz.service", "foo.service"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ожидался %v, получено %v", want, got)
	}
}

// TestParseCrashedUnitsEmpty проверяет, что пустой вход даёт пустой результат.
func TestParseCrashedUnitsEmpty(t *testing.T) {
	if got := parseCrashedUnits(nil); len(got) != 0 {
		t.Errorf("ожидался пустой список, получено %v", got)
	}
	if got := parseCrashedUnits([]string{"no failure here"}); len(got) != 0 {
		t.Errorf("ожидался пустой список, получено %v", got)
	}
}

// TestExtractUnitNames проверяет вычленение имени юнита из строки.
func TestExtractUnitNames(t *testing.T) {
	got := extractUnitNames("2026-10-18T12:00:00+0700 host systemd[1]: nginx.service: Failed with result 'exit-code'.")
	if !reflect.DeepEqual(got, []string{"nginx.service"}) {
		t.Errorf("ожидался [nginx.service], получено %v", got)
	}
	// Строка без ".service:" не даёт имён.
	if got := extractUnitNames("Failed to start Foo service."); len(got) != 0 {
		t.Errorf("ожидался пустой список, получено %v", got)
	}
}

// TestSinceArg проверяет форматирование периода для journalctl --since.
func TestSinceArg(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{6 * time.Hour, "6 hours ago"},
		{2 * time.Hour, "2 hours ago"},
		{30 * time.Minute, "30 minutes ago"},
		{45 * time.Second, "45 seconds ago"},
		{48 * time.Hour, "2 days ago"},
	}
	for _, tc := range cases {
		if got := sinceArg(tc.d); got != tc.want {
			t.Errorf("sinceArg(%v) = %q, ожидалось %q", tc.d, got, tc.want)
		}
	}
}

// TestPeriodHuman проверяет человекочитаемое форматирование периода.
func TestPeriodHuman(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{6 * time.Hour, "6 hours"},
		{2 * time.Hour, "2 hours"},
		{30 * time.Minute, "30 minutes"},
		{45 * time.Second, "45 seconds"},
		{48 * time.Hour, "2 days"},
	}
	for _, tc := range cases {
		if got := periodHuman(tc.d); got != tc.want {
			t.Errorf("periodHuman(%v) = %q, ожидалось %q", tc.d, got, tc.want)
		}
	}
}

// TestServiceUptimeHuman проверяет форматирование времени работы.
func TestServiceUptimeHuman(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
		{3*24*time.Hour + 2*time.Hour, "3d 2h"},
	}
	for _, tc := range cases {
		if got := serviceUptimeHuman(tc.d); got != tc.want {
			t.Errorf("serviceUptimeHuman(%v) = %q, ожидалось %q", tc.d, got, tc.want)
		}
	}
}

// TestParseSystemdTimestamp проверяет разбор временной метки systemd.
func TestParseSystemdTimestamp(t *testing.T) {
	tm, ok := parseSystemdTimestamp("Mon 2026-10-19 12:00:00 +07")
	if !ok {
		t.Fatal("ожидался успешный разбор")
	}
	if tm.Year() != 2026 || tm.Month() != time.October || tm.Day() != 19 {
		t.Errorf("неожиданная дата: %v", tm)
	}
	if tm.Hour() != 12 || tm.Minute() != 0 {
		t.Errorf("неожиданное время: %v", tm)
	}
	// Пустая строка — ошибка разбора.
	if _, ok := parseSystemdTimestamp(""); ok {
		t.Error("ожидалась ошибка разбора пустой строки")
	}
	// Строка без дня недели тоже разбирается.
	if _, ok := parseSystemdTimestamp("2026-10-19 12:00:00 +07"); !ok {
		t.Error("ожидался успешный разбор без дня недели")
	}
}

// TestServiceColor проверяет выбор цвета по состоянию службы.
func TestServiceColor(t *testing.T) {
	cases := []struct {
		name string
		info ServiceInfo
		want string
	}{
		{"failed", ServiceInfo{State: ServiceFailed}, colors.ColorRed},
		{"inactive", ServiceInfo{State: ServiceInactive}, colors.ColorRed},
		{"crashed", ServiceInfo{State: ServiceActive, CrashedRecently: true}, colors.ColorYellow},
		{"restarted", ServiceInfo{State: ServiceActive, RestartedRecently: true}, colors.ColorWhite},
		{"ok", ServiceInfo{State: ServiceActive}, colors.ColorGreen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceColor(&tc.info)("x")
			if !strings.Contains(got, tc.want) {
				t.Errorf("ожидался цвет %q, получено %q", tc.want, got)
			}
		})
	}
}

// TestProblemServices проверяет объединение упавших и падавших служб без дублей.
func TestProblemServices(t *testing.T) {
	rep := &ServiceReport{
		Failed:  []string{"bar.service", "foo.service"},
		Crashed: []string{"foo.service", "baz.service"},
	}
	got := rep.ProblemServices()
	want := []string{"bar.service", "baz.service", "foo.service"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ожидался %v, получено %v", want, got)
	}
}

// TestUptimeLabel проверяет подпись времени работы для разных состояний.
func TestUptimeLabel(t *testing.T) {
	cases := []struct {
		name string
		info ServiceInfo
		want string
	}{
		{"failed", ServiceInfo{State: ServiceFailed}, "failed"},
		{"inactive", ServiceInfo{State: ServiceInactive}, "stopped"},
		{"unknown", ServiceInfo{State: ServiceUnknown}, "unknown"},
		{"active", ServiceInfo{State: ServiceActive, Uptime: 2 * time.Hour}, "2h 0m"},
	}
	for _, tc := range cases {
		if got := uptimeLabel(&tc.info); got != tc.want {
			t.Errorf("uptimeLabel(%s) = %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}
