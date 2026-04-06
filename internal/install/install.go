package install

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// checkPathDirs возвращает список директорий для выбора установки.
// Возвращает:
// - первый системный кандидат (/usr/local/bin, /usr/bin, /bin), который есть в PATH
// - первый пользовательский кандидат (~/.local/bin, ~/bin), который есть в PATH
// Если ни одного кандидата нет в PATH, возвращает все уникальные директории из PATH.
func checkPathDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	systemCandidates := []string{"/usr/local/bin", "/usr/bin", "/bin"}
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
		return nil, fmt.Errorf("переменная PATH пуста")
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
	fmt.Println("Доступные директории для установки:")
	for i, dir := range candidates {
		fmt.Printf("%d. %s\n", i+1, dir)
	}
	fmt.Print("Выбор (1): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}
	idx := 0
	if n, err := fmt.Sscanf(input, "%d", &idx); err != nil || n != 1 || idx < 1 || idx > len(candidates) {
		return "", fmt.Errorf("неверный выбор")
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
			return fmt.Errorf("не удалось определить путь к исполняемому файлу: %v", err)
		}
		srcPath = exe
	} else {
		srcPath = sourceFile
		// Проверяем, существует ли файл
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("исходный файл не существует: %v", err)
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
			return fmt.Errorf("не удалось создать директорию %s: %v", targetDir, err)
		}
	}

	targetPath := filepath.Join(targetDir, baseName)

	// Копируем файл
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("не удалось открыть исходный файл %s: %v", srcPath, err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("не удалось создать целевой файл %s: %v", targetPath, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("ошибка копирования: %v", err)
	}

	// Устанавливаем права на выполнение (chmod +x)
	if err := os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf("не удалось установить права на выполнение: %v", err)
	}

	fmt.Printf("Успешно установлено в %s\n", targetPath)
	// Проверяем, находится ли директория в PATH
	pathEnv := os.Getenv("PATH")
	pathDirs := strings.Split(pathEnv, ":")
	inPath := false
	for _, dir := range pathDirs {
		if dir == targetDir {
			inPath = true
			break
		}
	}
	if !inPath {
		fmt.Printf("Внимание: директория %s не находится в PATH. Возможно, команда не будет доступна из оболочки.\n", targetDir)
	}
	return nil
}
