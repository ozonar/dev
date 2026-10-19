package prod

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"dev/internal/common"
	"dev/internal/logs"
	"dev/internal/toolchain"
)

// coreServiceLogGlobs — стандартные места логов для ключевых служб
// (access-лог nginx, slow-лог postgres и т.п.).
var coreServiceLogGlobs = map[string][]string{
	"nginx":           {"/var/log/nginx/*.log"},
	"apache2":         {"/var/log/apache2/*.log"},
	"httpd":           {"/var/log/httpd/*.log"},
	"postgresql":      {"/var/log/postgresql/*.log"},
	"mysql":           {"/var/log/mysql/*.log"},
	"mariadb":         {"/var/log/mysql/*.log"},
	"redis":           {"/var/log/redis/*.log"},
	"php-fpm":         {"/var/log/php*-fpm.log", "/var/log/php-fpm/*.log"},
	"memcached":       {"/var/log/memcached.log"},
	"supervisor":      {"/var/log/supervisor/*.log"},
	"docker":          {"/var/log/docker/*.log"},
	"postfix":         {"/var/log/mail.log", "/var/log/mail/*.log"},
	"cron":            {"/var/log/cron.log"},
	"crond":           {"/var/log/cron.log"},
	"rabbitmq-server": {"/var/log/rabbitmq/*.log"},
}

// ServiceLogSources возвращает источники логов для службы: journalctl первым,
// затем стандартные файлы (для core-служб) или логи в рабочей директории
// (для пользовательских служб).
func ServiceLogSources(s ServiceInfo) []logs.LogEntry {
	entries := []logs.LogEntry{{Path: s.Name, Type: "journal"}}

	// Стандартные места логов для core-служб.
	if globs := coreLogGlobs(s.Name); globs != nil {
		for _, glob := range globs {
			matches, _ := filepath.Glob(glob)
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && !info.IsDir() {
					entries = append(entries, logs.LogEntry{Path: m, Type: "file"})
				}
			}
		}
		return entries
	}

	// Для пользовательских служб — логи в рабочей директории.
	if dir := serviceWorkingDir(s.Name); dir != "" {
		for _, f := range findLogFiles(dir) {
			entries = append(entries, logs.LogEntry{Path: f, Type: "file"})
		}
	}
	return entries
}

// coreLogGlobs возвращает стандартные места логов для core-службы.
// Версионированные php*-fpm используют те же пути, что и php-fpm.
func coreLogGlobs(name string) []string {
	if globs, ok := coreServiceLogGlobs[name]; ok {
		return globs
	}
	if strings.HasPrefix(name, "php") && strings.HasSuffix(name, "-fpm") {
		return coreServiceLogGlobs["php-fpm"]
	}
	return nil
}

// serviceWorkingDir возвращает рабочую директорию службы через systemctl show.
func serviceWorkingDir(unit string) string {
	out := execOut("systemctl", "show", unit, "-p", "WorkingDirectory", "--no-pager")
	val := strings.TrimPrefix(strings.TrimSpace(out), "WorkingDirectory=")
	if val == "" || val == "/" {
		return ""
	}
	return val
}

// findLogFiles ищет *.log файлы в директории рекурсивно (с исключениями
// node_modules, vendor, .git и т.п.).
func findLogFiles(dir string) []string {
	var files []string
	_ = common.WalkWithExclusions(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
			files = append(files, path)
		}
		return nil
	}, nil)
	return files
}

// OpenLog открывает источник логов: файл/docker — через logs.OpenLogInLnav,
// journal — через journalctl в lnav.
func OpenLog(entry logs.LogEntry) error {
	if entry.Type == "journal" {
		return openJournal(entry.Path)
	}
	return logs.OpenLogInLnav(entry.Path, entry.Type)
}

// journalLines — число последних строк журнала для статичного снимка.
const journalLines = 200

// openJournal открывает журнал службы в lnav как статичный снимок последних
// строк. Снимок пишется во временный файл, который lnav открывает как обычный
// файл. Если lnav недоступен — просто journalctl.
func openJournal(unit string) error {
	lnav, err := toolchain.ResolveLnav()
	if err != nil {
		cmd := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(journalLines), "--no-pager")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Снимок журнала во временный файл.
	tmp, err := os.CreateTemp("", "prod-journal-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	cmd := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(journalLines), "--no-pager")
	cmd.Stdout = tmp
	if err := cmd.Run(); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	// Перехватываем Ctrl+C, чтобы lnav успел корректно восстановить терминал,
	// а временный файл был удалён.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		os.Remove(tmp.Name())
	}()

	// Открываем файл в lnav.
	cmd = exec.Command(lnav, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ServiceLabel возвращает цветную строку "name (uptime)" для службы —
// та же индикация, что и в команде service.
func ServiceLabel(s *ServiceInfo) string {
	return serviceColor(s)(s.Name + " (" + uptimeLabel(s) + ")")
}
