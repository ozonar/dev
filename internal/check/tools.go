package check

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// checkDirName — имя подпапки внутри dev-config, куда скачиваются программы.
const checkDirName = "check"

// availablePhpVersions — версии PHP, доступные в NativePHP/php-bin
// (папка bin/linux/{arch}), от новейшей к старейшей.
var availablePhpVersions = []string{"8.5", "8.4", "8.3"}

// checkDir вернёт абсолютный путь к папке с программами (~/dev-config/check).
func checkDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %v", err)
	}
	return filepath.Join(home, "dev-config", checkDirName), nil
}

// Program описывает одну программу: линтер, анализатор или рантайм (php).
// Name — короткое имя программы, используемое в плейсхолдерах команд
// (например {php}, {phpstan}, {golangci-lint}).
type Program struct {
	Name   string // короткое имя программы
	Binary string // имя исполняемого файла внутри checkDir (и внутри архива)
	URL    string // URL для скачивания
	// Archive — тип архива: "" (простой файл), "tar.gz", "zip".
	Archive string
	// FullCommand — команда для запуска программы. Плейсхолдеры вида {имя}
	// других программ из набора заменяются на их полные пути.
	// Примеры: "{php} {phpstan}", "{golangci-lint}".
	FullCommand string
	Require     []Program
}

// programsFor возвращает список линтеров/анализаторов, которые реально
// запускаются в рамках dev check для данного языка. Каждый линтер несёт
// свои зависимости (Require). Версия php прокидывается в зависимости.
func programsFor(language, version string) []Program {
	switch language {
	case "go":
		return []Program{goLinter()}
	case "php":
		return []Program{phpStanLinter(version), phpCsFixerLinter(version)}
	default:
		return nil
	}
}

// goLinter описывает golangci-lint.
// Скачивается как tar.gz с GitHub releases (бинарник под ОС/архитектуру).
func goLinter() Program {
	return Program{
		Name:        "golangci-lint",
		Binary:      "golangci-lint",
		URL:         fmt.Sprintf("https://github.com/golangci/golangci-lint/releases/download/v1.61.0/golangci-lint-1.61.0-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH),
		Archive:     "tar.gz",
		FullCommand: "{golangci-lint}",
	}
}

// phpProgram описывает готовый php-бинарник из NativePHP/php-bin.
// Скачивается как zip-архив, внутри которого лежит файл php.
func phpProgram(version string) Program {
	// Сопоставляем требуемую версию с доступными в NativePHP/php-bin.
	version = matchAvailableVersion(version, availablePhpVersions)
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	return Program{
		Name:   "php",
		Binary: "php",
		URL: fmt.Sprintf("https://raw.githubusercontent.com/NativePHP/php-bin/%s/bin/linux/%s/php-%s.zip",
			"main", arch, version),
		Archive:     "zip",
		FullCommand: "{php}",
	}
}

// phpStanLinter описывает PHPStan.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php
func phpStanLinter(phpVersion string) Program {
	return Program{
		Name:        "phpstan",
		Binary:      "phpstan.phar",
		URL:         "https://github.com/phpstan/phpstan/releases/download/1.12.0/phpstan.phar",
		FullCommand: "{php} {phpstan}",
		Require:     []Program{phpProgram(phpVersion)},
	}
}

// phpCsFixerLinter описывает PHP CS Fixer.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php
func phpCsFixerLinter(phpVersion string) Program {
	return Program{
		Name:        "php-cs-fixer",
		Binary:      "php-cs-fixer.phar",
		URL:         "https://github.com/PHP-CS-Fixer/PHP-CS-Fixer/releases/download/v3.64.0/php-cs-fixer.phar",
		FullCommand: "{php} {php-cs-fixer}",
		Require:     []Program{phpProgram(phpVersion)},
	}
}

// binaryPath возвращает полный путь к бинарю программы внутри checkDir.
func (p Program) binaryPath(dir string) string {
	return filepath.Join(dir, p.Binary)
}

// isInstalled проверяет, существует ли исполняемый файл программы.
func (p Program) isInstalled(dir string) bool {
	_, err := os.Stat(p.binaryPath(dir))
	return err == nil
}

// download скачивает и подготавливает программу в папку dir.
func (p Program) download(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpName)
	}()

	resp, err := http.Get(p.URL)
	if err != nil {
		return fmt.Errorf("failed to download %s: %v", p.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: HTTP %d", p.Name, resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("error writing %s: %v", p.Name, err)
	}
	tmpFile.Close()

	target := p.binaryPath(dir)

	switch p.Archive {
	case "tar.gz":
		if err := extractTarGz(tmpName, target, p.Binary); err != nil {
			return fmt.Errorf("error extracting %s: %v", p.Name, err)
		}
	case "zip":
		if err := extractZip(tmpName, target, p.Binary); err != nil {
			return fmt.Errorf("error extracting %s: %v", p.Name, err)
		}
	default:
		// Простой файл (phar) — просто перемещаем.
		if err := os.Rename(tmpName, target); err != nil {
			if err := copyFile(tmpName, target); err != nil {
				return fmt.Errorf("error saving %s: %v", p.Name, err)
			}
			os.Remove(tmpName)
		}
	}

	// Даём права на выполнение.
	if err := os.Chmod(target, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %v", target, err)
	}

	return nil
}

// runCommand строит имя команды и список аргументов для запуска программы,
// заменяя плейсхолдеры {имя} на полные пути бинарей всех программ набора.
func (p Program) runCommand(dir string, args []string) (string, []string) {
	full := p.FullCommand
	// Подставляем сам линтер.
	full = strings.ReplaceAll(full, "{"+p.Name+"}", p.binaryPath(dir))
	// Подставляем вендоры (Require) линтера.
	for _, v := range p.Require {
		full = strings.ReplaceAll(full, "{"+v.Name+"}", v.binaryPath(dir))
	}
	parts := strings.Fields(full)
	if len(parts) == 0 {
		parts = []string{p.binaryPath(dir)}
	}
	parts = append(parts, args...)
	return parts[0], parts[1:]
}

// collectPrograms возвращает все программы из переданного списка вместе
// с их вендорами.
func collectPrograms(programs []Program) []Program {
	var result []Program
	seen := make(map[string]bool)
	var walk func([]Program)
	walk = func(list []Program) {
		for _, p := range list {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			result = append(result, p)
			walk(p.Require)
		}
	}
	walk(programs)
	return result
}

// ensurePrograms гарантирует, что линтеры и их вендоры скачаны.
// Возвращает путь к папке с программами и линтеры для запуска.
func ensurePrograms(language, phpVersion string) (string, []Program, error) {
	dir, err := checkDir()
	if err != nil {
		return "", nil, err
	}

	linters := programsFor(language, phpVersion)
	if len(linters) == 0 {
		return "", nil, fmt.Errorf("no linters configured for language %q", language)
	}

	for _, p := range collectPrograms(linters) {
		if p.isInstalled(dir) {
			continue
		}
		if err := p.download(dir); err != nil {
			return "", nil, err
		}
	}

	return dir, linters, nil
}

// extractTarGz распаковывает tar.gz архив и кладёт файл, имя которого
// оканчивается на fileName, в target.
func extractTarGz(src, target, fileName string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !strings.HasSuffix(hdr.Name, fileName) {
			continue
		}
		if err := copyFromReader(tr, target); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("file %q not found in archive", fileName)
}

// extractZip распаковывает zip архив и кладёт файл, имя которого
// оканчивается на fileName, в target.
func extractZip(src, target, fileName string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name, fileName) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return copyFromReader(rc, target)
	}
	return fmt.Errorf("file %q not found in archive", fileName)
}

// copyFromReader копирует данные из reader в файл target (создавая его).
func copyFromReader(r io.Reader, target string) error {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

// copyFile копирует файл src в dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// matchAvailableVersion сопоставляет требуемую версию (major.minor) с ближайшей
// доступной в списке available (от новейшей к старейшей). Если required пуста,
// возвращается новейшая доступная. Если required выше всех доступных — также
// новейшая.
func matchAvailableVersion(required string, available []string) string {
	if required == "" {
		return available[0]
	}
	// available отсортирован от новейшей к старейшей.
	// Выбираем наименьшую доступную версию, которая >= required:
	// идём от новейшей к старейшей и запоминаем последнюю подходящую.
	best := ""
	for _, v := range available {
		if compareMajorMinor(v, required) >= 0 {
			best = v
			continue
		}
		break
	}
	if best != "" {
		return best
	}
	// Все доступные версии меньше required — возвращаем новейшую (ближайшую).
	return available[0]
}

// compareMajorMinor сравнивает две версии вида "major.minor".
// Возвращает 1, если a > b; -1 если a < b; 0 если равны.
func compareMajorMinor(a, b string) int {
	ai := parseMajorMinor(a)
	bi := parseMajorMinor(b)
	for i := 0; i < 2; i++ {
		if ai[i] > bi[i] {
			return 1
		}
		if ai[i] < bi[i] {
			return -1
		}
	}
	return 0
}

// parseMajorMinor разбирает версию "major.minor" на два целых числа.
func parseMajorMinor(v string) [2]int {
	parts := strings.SplitN(v, ".", 2)
	var res [2]int
	if len(parts) > 0 {
		res[0], _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	if len(parts) > 1 {
		res[1], _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return res
}
