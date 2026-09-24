package install

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/i18n"
)

// checkPathDirs возвращает список директорий для выбора установки.
// Возвращает:
//   - первый системный кандидат (/opt/homebrew/bin, /usr/local/bin, /usr/bin, /bin),
//     который есть в PATH. /opt/homebrew/bin — каталог Homebrew на Apple Silicon.
//   - первый пользовательский кандидат (~/.local/bin, ~/bin), который есть в PATH
//
// Если ни одного кандидата нет в PATH, возвращает все уникальные директории из PATH.
func checkPathDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	systemCandidates := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
	userCandidates := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
	}

	pathEnv := os.Getenv("PATH")
	pathDirs := strings.Split(pathEnv, ":")

	// Ищем первого системного кандидата в PATH
	var sysDir, userDir string
	for _, cand := range systemCandidates {
		for _, dir := range pathDirs {
			if dir == cand {
				sysDir = cand
				break
			}
		}
		if sysDir != "" {
			break
		}
	}
	// Ищем первого пользовательского кандидата в PATH
	for _, cand := range userCandidates {
		for _, dir := range pathDirs {
			if dir == cand {
				userDir = cand
				break
			}
		}
		if userDir != "" {
			break
		}
	}

	var result []string
	if sysDir != "" {
		result = append(result, sysDir)
	}
	if userDir != "" {
		result = append(result, userDir)
	}

	// Если нашли хотя бы одного кандидата, возвращаем их
	if len(result) > 0 {
		return result, nil
	}

	// Ни одного кандидата нет в PATH, возвращаем все уникальные директории из PATH
	// (исключаем пустые строки и дубликаты)
	seen := make(map[string]bool)
	var allDirs []string
	for _, dir := range pathDirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		allDirs = append(allDirs, dir)
	}
	if len(allDirs) == 0 {
		return nil, fmt.Errorf(i18n.T("PATH variable is empty"))
	}
	return allDirs, nil
}

// chooseInstallDir возвращает директорию для установки на основе checkPathDirs.
//
// Если больше, показывает интерактивный выбор.
func chooseInstallDir() (string, error) {
	candidates, err := checkPathDirs()
	if err != nil {
		return "", err
	}

	// Показываем список для выбора
	i18n.Printf("Available directories for install:\n")
	for i, dir := range candidates {
		fmt.Printf("%d. %s\n", i+1, dir)
	}
	i18n.Printf("Selection (1): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}
	idx := 0
	if n, err := fmt.Sscanf(input, "%d", &idx); err != nil || n != 1 || idx < 1 || idx > len(candidates) {
		return "", fmt.Errorf(i18n.T("invalid selection"))
	}
	return candidates[idx-1], nil
}

// Install устанавливает указанный файл (или текущий исполняемый) в выбранную директорию.
// Если sourceFile == "", используется os.Executable().
func Install(sourceFile string) error {
	// Определяем исходный файл
	var srcPath string
	if sourceFile == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf(i18n.T("could not determine executable path: %v"), err)
		}
		srcPath = exe
	} else {
		srcPath = sourceFile
		// Проверяем, существует ли файл
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf(i18n.T("source file does not exist: %v"), err)
		}
	}

	// Определяем имя файла для установки (базовое имя исходного файла)
	baseName := filepath.Base(srcPath)

	// Выбираем директорию установки
	targetDir, err := chooseInstallDir()
	if err != nil {
		return err
	}

	// Если директории не существует, создаём её
	if _, err := os.Stat(targetDir); err != nil {
		// Молча создаём (рекурсивно)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf(i18n.T("could not create directory %s: %v"), targetDir, err)
		}
	}

	targetPath := filepath.Join(targetDir, baseName)

	// Проверяем, не пытаемся ли установить файл в самого себя
	srcAbs, err := filepath.Abs(srcPath)
	if err != nil {
		srcAbs = srcPath
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		targetAbs = targetPath
	}
	if srcAbs == targetAbs {
		return fmt.Errorf(i18n.T("Attempting to install %[1]s into itself.\n"), targetPath)
	}

	// Убиваем процесс, использующий целевой файл (если такой есть).
	// Вывод fuser не показываем, чтобы не засорять консоль после промпта выбора.
	// На macOS fuser отсутствует — вызов безопасно пропускается.
	if _, err := exec.LookPath("fuser"); err == nil {
		_ = exec.Command("fuser", "-k", targetPath).Run()
	}

	// Копируем файл
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf(i18n.T("could not open source file %s: %v"), srcPath, err)
	}
	defer srcFile.Close()

	// Пытаемся удалить существующий файл
	os.Remove(targetPath)

	dstFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf(i18n.T("could not create target file %s: %v"), targetPath, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf(i18n.T("copy failed: %v"), err)
	}

	// Устанавливаем права на выполнение (chmod +x)
	if err := os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf(i18n.T("could not set executable permissions: %v"), err)
	}

	i18n.Printf("Installation successful as %s\n", targetPath)
	return nil
}
