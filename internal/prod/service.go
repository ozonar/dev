package prod

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"dev/internal/colors"
)

// ServiceState — состояние systemd-службы.
type ServiceState string

const (
	ServiceActive   ServiceState = "active"
	ServiceInactive ServiceState = "inactive"
	ServiceFailed   ServiceState = "failed"
	ServiceUnknown  ServiceState = "unknown"
)

// ServiceInfo — информация об одной пользовательской службе.
type ServiceInfo struct {
	Name              string
	State             ServiceState
	Uptime            time.Duration // время работы (0 — не работает)
	CrashedRecently   bool          // падала за анализируемый период
	RestartedRecently bool          // перезапускалась за анализируемый период
}

// ServiceReport — результат анализа служб.
type ServiceReport struct {
	Services []ServiceInfo // все пользовательские службы
	Core     []ServiceInfo // ключевые службы веб-продакшена (если есть)
	Failed   []string      // упавшие сейчас (systemctl --failed)
	Crashed  []string      // падавшие за период (journalctl)
}

// CollectServices собирает отчёт о службах: список пользовательских служб и
// ключевых служб веб-продакшена с состоянием и временем работы, упавшие
// сейчас и падавшие за период.
func CollectServices(period time.Duration) *ServiceReport {
	rep := &ServiceReport{}
	rep.Failed = failedServices()
	rep.Crashed = crashedServices(period)

	// Множество падавших служб для быстрой проверки при раскраске списка.
	crashedSet := make(map[string]bool, len(rep.Crashed))
	for _, n := range rep.Crashed {
		crashedSet[n] = true
	}

	now := time.Now()
	for _, name := range listUserServices() {
		rep.Services = append(rep.Services, buildServiceInfo(name, crashedSet, now, period))
	}
	for _, name := range coreServiceNames() {
		if !serviceExists(name) {
			continue
		}
		rep.Core = append(rep.Core, buildServiceInfo(name, crashedSet, now, period))
	}
	return rep
}

// buildServiceInfo собирает информацию о службе: состояние, время работы,
// флаги падения и перезапуска за период.
func buildServiceInfo(name string, crashedSet map[string]bool, now time.Time, period time.Duration) ServiceInfo {
	info := ServiceInfo{Name: name}
	info.State = serviceState(name)
	info.Uptime = serviceUptime(name, now)
	info.CrashedRecently = crashedSet[name]
	// Перезапускалась за период: активна и стартовала недавно.
	info.RestartedRecently = info.State == ServiceActive &&
		info.Uptime > 0 && info.Uptime <= period
	return info
}

// coreServiceCandidates — фиксированные кандидаты в ключевые службы
// веб-продакшена. Показываются в блоке "Core services", если присутствуют.
var coreServiceCandidates = []string{
	"nginx",
	"php-fpm",
	"postgresql",
	"redis",
	"mysql",
	"mariadb",
	"apache2",
	"httpd",
	"memcached",
	"rabbitmq-server",
	"supervisor",
	"cron",
	"crond",
	"docker",
	"postfix",
}

// coreServiceNames возвращает полный список кандидатов в ключевые службы:
// фиксированный набор плюс обнаруженные php*-fpm.
func coreServiceNames() []string {
	names := append([]string{}, coreServiceCandidates...)
	names = append(names, discoverPhpFpmServices()...)
	return names
}

// discoverPhpFpmServices находит службы php*-fpm через systemctl list-unit-files
// (покрывает любые версии PHP, например php8.3-fpm).
func discoverPhpFpmServices() []string {
	out := execOut("systemctl", "list-unit-files", "php*-fpm.service", "--no-legend", "--no-pager")
	names := parseUnitFileLines(out)
	for i := range names {
		names[i] = trimServiceSuffix(names[i])
	}
	return names
}

// parseUnitFileLines извлекает имена юнитов из вывода systemctl/docker
// (по одной строке на юнит). Перед разбором убираются юникод-маркеры "●",
// которые systemctl --failed выводит перед именами упавших служб.
func parseUnitFileLines(out string) []string {
	out = strings.ReplaceAll(out, "●", "")
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// trimServiceSuffix убирает суффикс ".service" из имени юнита.
func trimServiceSuffix(name string) string {
	return strings.TrimSuffix(name, ".service")
}

// serviceExists проверяет, присутствует ли служба в системе (LoadState).
func serviceExists(name string) bool {
	out := execOut("systemctl", "show", name, "-p", "LoadState", "--no-pager")
	switch strings.TrimSpace(out) {
	case "LoadState=loaded", "LoadState=masked":
		return true
	}
	return false
}

// ProblemServices возвращает объединённый список проблемных служб
// (упавшие сейчас + падавшие за период) без дубликатов, отсортированный.
func (rep *ServiceReport) ProblemServices() []string {
	seen := make(map[string]bool)
	var names []string
	for _, n := range rep.Failed {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, n := range rep.Crashed {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// excludedServiceSubstrings — подстроки, по которым службы исключаются из
// списка пользовательских служб (системные алиасы и служебные юниты).
var excludedServiceSubstrings = []string{
	"dbus-org",
}

// listUserServices возвращает имена пользовательских служб из
// /etc/systemd/system (аналог find -maxdepth 1 -name '*.service'),
// за вычетом исключённых по подстрокам служб.
func listUserServices() []string {
	entries, err := os.ReadDir("/etc/systemd/system")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		if excludedService(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// excludedService проверяет, попадает ли имя службы под исключение.
func excludedService(name string) bool {
	for _, sub := range excludedServiceSubstrings {
		if strings.Contains(name, sub) {
			return true
		}
	}
	return false
}

// serviceState возвращает текущее состояние службы через systemctl is-active.
// systemctl is-active возвращает ненулевой код для неактивных/упавших служб,
// но вывод при этом содержит состояние — используем его, а не execOut.
func serviceState(name string) ServiceState {
	cmd := exec.Command("systemctl", "is-active", name)
	out, err := cmd.Output()
	state := strings.TrimSpace(string(out))
	if err != nil && state == "" {
		return ServiceUnknown
	}
	switch state {
	case "active":
		return ServiceActive
	case "inactive":
		return ServiceInactive
	case "failed":
		return ServiceFailed
	default:
		return ServiceUnknown
	}
}

// serviceUptime возвращает время работы службы (с момента последней активации)
// через systemctl show ActiveEnterTimestamp.
func serviceUptime(name string, now time.Time) time.Duration {
	out := execOut("systemctl", "show", "-p", "ActiveEnterTimestamp", name)
	val := strings.TrimPrefix(strings.TrimSpace(out), "ActiveEnterTimestamp=")
	t, ok := parseSystemdTimestamp(val)
	if !ok {
		return 0
	}
	d := now.Sub(t)
	if d < 0 {
		return 0
	}
	return d
}

// failedServices возвращает список служб, упавших на данный момент
// (systemctl --failed --type=service).
func failedServices() []string {
	out := execOut("systemctl", "--failed", "--type=service", "--no-legend", "--no-pager")
	return parseUnitFileLines(out)
}

// crashedServices возвращает список служб, падавших за период, по журналу
// systemd (journalctl -p err..alert с фильтром по ключевым словам сбоя).
func crashedServices(period time.Duration) []string {
	since := sinceArg(period)
	cmdStr := fmt.Sprintf(
		"journalctl --since %q -p err..alert -o short-iso --no-pager 2>/dev/null | "+
			"grep -Ei 'systemd.*(failed|failure)|Failed to start|Failed with result' || true",
		since)
	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseCrashedUnits(strings.Split(string(out), "\n"))
}

// parseCrashedUnits извлекает имена служб из строк журнала сбоев.
// Имя юнита ищем по паттерну "<unit>.service:" перед ключевыми словами.
func parseCrashedUnits(lines []string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, line := range lines {
		for _, u := range extractUnitNames(line) {
			if !seen[u] {
				seen[u] = true
				names = append(names, u)
			}
		}
	}
	sort.Strings(names)
	return names
}

// extractUnitNames вычленяет имена юнитов из одной строки журнала.
func extractUnitNames(line string) []string {
	var res []string
	idx := strings.Index(line, ".service:")
	if idx < 0 {
		return res
	}
	start := strings.LastIndex(line[:idx], " ")
	if start < 0 {
		start = 0
	} else {
		start++
	}
	res = append(res, line[start:idx]+".service")
	return res
}

// ServiceLogs возвращает последние n строк журнала службы с временными метками.
func ServiceLogs(name string, n int) []string {
	if !procExists("/run/systemd/system") {
		return nil
	}
	cmd := exec.Command("journalctl", "-u", name, "-n", strconv.Itoa(n),
		"--no-pager", "-o", "short-iso")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines
}

// sinceArg форматирует период в аргумент journalctl --since.
func sinceArg(period time.Duration) string {
	return periodHuman(period) + " ago"
}

// periodHuman форматирует период в человекочитаемый вид ("6 hours", "30 minutes").
func periodHuman(period time.Duration) string {
	switch {
	case period >= 24*time.Hour:
		return fmt.Sprintf("%d days", int(period.Hours()/24))
	case period >= time.Hour:
		return fmt.Sprintf("%d hours", int(period.Hours()))
	case period >= time.Minute:
		return fmt.Sprintf("%d minutes", int(period.Minutes()))
	default:
		return fmt.Sprintf("%d seconds", int(period.Seconds()))
	}
}

// parseSystemdTimestamp разбирает временную метку systemd вида
// "Mon 2026-10-18 12:00:00 +07". День недели может не совпадать с локалью,
// поэтому он отбрасывается перед разбором.
func parseSystemdTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	fields := strings.Fields(s)
	if len(fields) > 1 && isWeekday(fields[0]) {
		s = strings.Join(fields[1:], " ")
	}
	layouts := []string{
		"2006-01-02 15:04:05 -07",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// isWeekday проверяет, является ли токен сокращением дня недели.
func isWeekday(s string) bool {
	switch strings.ToLower(s) {
	case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
		return true
	}
	return false
}

// serviceUptimeHuman форматирует время работы в человекочитаемый вид.
func serviceUptimeHuman(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// uptimeLabel возвращает подпись времени работы для строки службы.
func uptimeLabel(s *ServiceInfo) string {
	switch s.State {
	case ServiceFailed:
		return "failed"
	case ServiceInactive:
		return "stopped"
	case ServiceUnknown:
		return "unknown"
	default:
		return serviceUptimeHuman(s.Uptime)
	}
}

// serviceColor возвращает цветную функцию для состояния службы:
// красный — упала/остановлена, жёлтый — падала за период, белый —
// перезапускалась за период, зелёный — работает стабильно.
func serviceColor(s *ServiceInfo) func(string) string {
	switch {
	case s.State == ServiceFailed || s.State == ServiceInactive:
		return colors.Red
	case s.CrashedRecently:
		return colors.Yellow
	case s.RestartedRecently:
		return colors.White
	default:
		return colors.Green
	}
}

// RenderServices выводит отчёт о службах: ключевые и пользовательские службы
// с цветовой индикацией состояния, упавшие сейчас и падавшие за период.
func RenderServices(rep *ServiceReport, period time.Duration) {
	var sb strings.Builder

	if len(rep.Core) > 0 {
		sb.WriteString(colors.Cyan("Core services:") + "\n")
		for i := range rep.Core {
			s := &rep.Core[i]
			col := serviceColor(s)
			sb.WriteString("  " + col(s.Name+" ("+uptimeLabel(s)+")") + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(colors.Cyan("User services:") + "\n")
	for i := range rep.Services {
		s := &rep.Services[i]
		col := serviceColor(s)
		sb.WriteString("  " + col(s.Name+" ("+uptimeLabel(s)+")") + "\n")
	}

	sb.WriteString("\n" + colors.Cyan("Failed services now:") + "\n")
	if len(rep.Failed) == 0 {
		sb.WriteString("  " + colors.Green("none") + "\n")
	} else {
		for _, name := range rep.Failed {
			sb.WriteString("  " + colors.Red(name) + "\n")
		}
	}

	sb.WriteString("\n" + colors.Cyan("Services crashed in the last "+periodHuman(period)+":") + "\n")
	if len(rep.Crashed) == 0 {
		sb.WriteString("  " + colors.Green("none") + "\n")
	} else {
		for _, name := range rep.Crashed {
			sb.WriteString("  " + colors.Yellow(name) + "\n")
		}
	}

	fmt.Print(sb.String())
}
