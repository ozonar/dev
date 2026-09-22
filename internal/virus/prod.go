package virus

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/ai"
	"dev/internal/prod"
)

// ProdVirus копирует текущий исполняемый файл prod на удалённый сервер через SCP,
// а также переносит прод-конфигурацию: каталог /etc/prod-command (конфиги и
// историю отчётов) и LLM-конфиг, чтобы на удалённом сервере сразу работали
// отчёты, каскадный анализ и команда prod llm.
// Параметр path должен быть в формате "user@ip" или просто "ip".
func ProdVirus(path string) error {
	// Определяем путь к текущему исполняемому файлу.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %v", err)
	}

	// Парсим строку подключения.
	username, host, err := parseTarget(path)
	if err != nil {
		return err
	}

	remotePath := homeDirFor(username)

	// Копируем бинарник на удалённый сервер и ставим права.
	if err := copyBinary(exe, username, host, remotePath); err != nil {
		return err
	}

	// Переносим каталог /etc/prod-command (deps.conf и пр., без истории отчётов).
	if err := copyProdCommand(username, host); err != nil {
		fmt.Printf("Warning: could not copy prod-command files: %v\n", err)
	}

	// Переносим LLM-конфиг, чтобы работала команда prod llm.
	if err := copyLLMConfig(username, host); err != nil {
		fmt.Printf("Warning: could not copy LLM config: %v\n", err)
	}

	fmt.Printf("Successfully copied to %s:%s\n", host, remotePath)
	return nil
}

// copyProdCommand переносит локальный каталог /etc/prod-command в одноимённый
// каталог на удалённом сервере.
func copyProdCommand(username, host string) error {
	return copyProdCommandFrom(prod.CommandDir, username, host)
}

// copyProdCommandFrom переносит указанный каталог конфигов в /etc/prod-command
// на удалённом сервере. Для не-root пользователя копия идёт во временную папку
// домашнего каталога, а затем переносится в /etc через sudo.
func copyProdCommandFrom(localDir, username, host string) error {
	// Проверяем, существует ли локальная папка с конфигами: scpDir молча
	// пропускает отсутствующую папку, а здесь нужно остановиться до выхода
	// в сеть (scp/ssh), чтобы не выполнять лишних команд на удалённом сервере.
	info, err := os.Stat(localDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config directory %s not found, skipping config copy.\n", localDir)
			return nil
		}
		return fmt.Errorf("could not check config directory: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", localDir)
	}

	// История отчётов (reports) на удалённый сервер не переносится: готовим
	// временную копию каталога без неё, чтобы не затирать чужие отчёты на
	// удалённой стороне и не тащить лишние данные по сети.
	staged, err := stageWithoutReports(localDir)
	if err != nil {
		return fmt.Errorf("could not prepare config copy: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(staged))

	// На удалённом сервере prod читает конфиги из того же каталога prod.CommandDir.
	const remoteCommandDir = prod.CommandDir

	if username == "root" {
		// Root пишет в /etc напрямую.
		return scpDir(staged, username, host, "/etc")
	}

	// Обычный пользователь: копируем во временную папку домашнего каталога,
	// затем переносим в /etc/prod-command через sudo и удаляем временную копию.
	tmpDir := "~/" + filepath.Base(staged)
	if err := scpDir(staged, username, host, tmpDir); err != nil {
		return err
	}
	remoteCmd := fmt.Sprintf("sudo mkdir -p %s && sudo cp -r %s/. %s/ && rm -rf %s",
		remoteCommandDir, tmpDir, remoteCommandDir, tmpDir)
	if err := sshRun(username, host, remoteCmd); err != nil {
		return fmt.Errorf("could not install configs to %s: %v", remoteCommandDir, err)
	}
	fmt.Printf("Configs successfully copied to %s@%s:%s\n", username, host, remoteCommandDir)
	return nil
}

// stageWithoutReports копирует каталог src во временную папку, исключая
// подкаталог reports с историей отчётов. Имя итоговой папки совпадает с именем
// исходного каталога, чтобы scp положил её под правильным именем.
func stageWithoutReports(src string) (string, error) {
	parent, err := os.MkdirTemp("", "virus-stage-")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(parent, filepath.Base(src))
	// Корневую папку копии создаём явно: файлы верхнего уровня src
	// записываются прямо в неё.
	if err := os.MkdirAll(dst, 0755); err != nil {
		os.RemoveAll(parent)
		return "", err
	}

	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Корень уже создан как dst.
		if rel == "." {
			return nil
		}
		// Подкаталог reports (история отчётов) исключается целиком.
		if rel == "reports" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		os.RemoveAll(parent)
		return "", err
	}
	return dst, nil
}

// copyLLMConfig переносит LLM-конфиг на удалённый сервер. Источник выбирается
// с тем же приоритетом, что и при загрузке конфига командой prod llm:
// сначала ~/dev-config/main.conf, затем /etc/dev-command/main.conf.
func copyLLMConfig(username, host string) error {
	src := resolveFirstExisting(ai.ConfigPaths())
	if src == "" {
		fmt.Println("LLM config not found, skipping config copy.")
		return nil
	}

	// Копируем конфиг в то же место, откуда он взят: пользовательский путь —
	// в домашний каталог, системный — в /etc (через sudo при необходимости).
	remoteTarget := ai.UserConfigPath
	if strings.HasPrefix(src, "/etc/") {
		remoteTarget = ai.EtcConfigPath
	}

	remoteDir := filepath.Dir(remoteTarget)
	if username != "root" && strings.HasPrefix(remoteTarget, "/etc/") {
		// Системный путь без root-доступа: временная копия + sudo.
		tmpTarget := "~/" + filepath.Base(src)
		if err := scpFile(src, username, host, tmpTarget); err != nil {
			return err
		}
		remoteCmd := fmt.Sprintf("sudo mkdir -p %s && sudo mv %s %s", remoteDir, tmpTarget, remoteTarget)
		if err := sshRun(username, host, remoteCmd); err != nil {
			return fmt.Errorf("could not install LLM config to %s: %v", remoteTarget, err)
		}
		fmt.Printf("LLM config copied to %s@%s:%s\n", username, host, remoteTarget)
		return nil
	}

	// Домашний каталог или root-доступ: создаём родительский каталог и
	// копируем файл напрямую.
	if err := sshRun(username, host, "mkdir -p "+remoteDir); err != nil {
		return err
	}
	if err := scpFile(src, username, host, remoteTarget); err != nil {
		return err
	}
	fmt.Printf("LLM config copied to %s@%s:%s\n", username, host, remoteTarget)
	return nil
}

// scpFile копирует локальный файл на удалённый сервер в указанный путь.
func scpFile(localFile, username, host, remoteTarget string) error {
	cmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no",
		localFile, fmt.Sprintf("%s@%s:%s", username, host, remoteTarget))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// resolveFirstExisting возвращает первый существующий файл из списка путей
// или пустую строку, если ни один не найден. Пути ожидаются абсолютными
// (ai.ConfigPaths уже приводит ~/ к домашнему каталогу).
func resolveFirstExisting(paths []string) string {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
