package ai

import (
	"bufio"
	"dev/internal/detector"
	"dev/internal/i18n"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

// CommandType определяет тип команды
type CommandType string

const (
	// CommandTypeAnalysis — команда для анализа (сбор информации)
	CommandTypeAnalysis CommandType = "analysis"
	// CommandTypeSequence — команда из череды последовательных шагов
	CommandTypeSequence CommandType = "sequence"
	// CommandTypeFix — команда, которая сама по себе решает проблему
	CommandTypeFix CommandType = "fix"
)

// CommandAction представляет одну команду от LLM
type CommandAction struct {
	Command     string      `json:"command"`
	Description string      `json:"description,omitempty"`
	Type        CommandType `json:"type,omitempty"`
}

// HistoryEntry представляет запись истории диалога
type HistoryEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatMessage — сообщение в запросе к OpenAI-совместимому API.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RunAI запускает диалог с AI для генерации и выполнения команд.
// info — результат анализа проекта: контекст собирается из него напрямую,
// без повторного вызова CLI-команды dev analyze.
func RunAI(info *detector.ProjectInfo, text string) error {
	cfg, err := LoadConfig()
	if err != nil {
		i18n.Red("Config error: %v", err)
		if err := InteractiveEditConfig(); err != nil {
			return err
		}
		// Пробуем снова после редактирования.
		cfg, err = LoadConfig()
		if err != nil {
			return fmt.Errorf(i18n.T("config still invalid after edit: %w"), err)
		}
	}

	// Собираем контекст проекта из данных детектора.
	contextInfo := buildContext(info)

	history := []HistoryEntry{
		{Role: "system", Content: buildSystemPrompt(contextInfo)},
		{Role: "user", Content: text},
	}

	return interactiveLoop(cfg, history)
}

// buildSystemPrompt создаёт системный промпт с контекстом
func buildSystemPrompt(contextInfo string) string {
	return fmt.Sprintf(`Ты — ассистент для разработчика. Отвечай ТОЛЬКО в формате JSON.
Дай список команд для выполнения в терминале Linux.

Формат ответа:
[
  {"command": "команда", "description": "что делает", "type": "analysis"},
  {"command": "команда", "description": "что делает", "type": "sequence"},
  {"command": "команда", "description": "что делает", "type": "fix"}
]

Поле "type" определяет тип команды:
- "analysis" — команда для сбора диагностической информации. Используй ТОЛЬКО когда пользователь сообщил о проблеме/ошибке, и ты не знаешь её причину. После таких команд будет сделан повторный запрос к LLM для поиска решения.
- "sequence" — команда из череды последовательных шагов. Если за этой командой следуют другие.
- "fix" — команда, которая напрямую отвечает на запрос пользователя, дает пользователю ответ, или решает проблему пользователя. Это финальное действие.

Правила выбора типа:
1. Если пользователь явно просит что-то сделать (проверить место, показать логи, установить пакет, запустить сервер, выполнить код, ответить на вопрос) — используй "fix" или "sequence"
2. "analysis" используй ТОЛЬКО в сценарии "почему не работает / что-то сломалось / ошибка"
3. Если это один из шагов последовательности — используй "sequence"

Контекст текущего проекта:
%s

Доступные команды dev (CLI-утилита):
- dev build — сборка проекта
- dev migrate — запуск миграций БД
- dev migrate status — статус миграций
- dev migrate new [name] — создать новую миграцию
- dev cache — очистка кеша фреймворка
- dev dcr — docker-compose up -d
- dev port [address] — проверка занятости порта
- dev curl [url] [method] — HTTP-запрос

ВАЖНОЕ ОГРАНИЧЕНИЕ: Каждая команда выполняется в ОТДЕЛЬНОМ shell-процессе.
Команда cd НЕ сохраняется между командами. Если нужно выполнить несколько команд в одной директории,
используй полные пути или объединяй команды через && в одной строке для каждой команды из списка 
но ТОЛЬКО если необходимо выполнить команды ВНЕ рабочей директории

Например: "cd /some/dir && make && ./binary"
"./binary" тоже подойдет, если нужно выполнить команду из рабочей директории

Правила:
1. Отвечай ТОЛЬКО JSON-массивом, без пояснений
2. Команды должны быть безопасными и последовательными
3. Если запрос неясен, верни [{"command": "REFINE", "description": "пояснение почему нужны уточнения"}]
4. Для команд, которые требуют подтверждения (rm, dd, format и т.д.), добавь флаги подтверждения
5. Учитывай язык и фреймворк проекта`, contextInfo)
}

// buildContext собирает информацию о проекте из данных детектора —
// аналог вывода dev analyze, но без запуска подпроцесса.
func buildContext(info *detector.ProjectInfo) string {
	cwd, _ := os.Getwd()
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Текущая директория: %s\n", cwd))
	sb.WriteString("\nСодержимое директории:\n")

	entries, _ := os.ReadDir(cwd)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("  📁 %s/\n", name))
		} else {
			sb.WriteString(fmt.Sprintf("  📄 %s\n", name))
		}
	}

	sb.WriteString("\nРезультат dev analyze:\n")
	langLabel := info.Language
	if info.LanguageVersion != "" {
		langLabel = info.Language + " " + info.LanguageVersion
	}
	sb.WriteString(fmt.Sprintf("Язык: %s\n", langLabel))
	sb.WriteString(fmt.Sprintf("Фреймворк: %s\n", info.Framework))
	if info.HasEnv {
		sb.WriteString(".env: присутствует\n")
	} else {
		sb.WriteString(".env: отсутствует\n")
	}
	if info.HasVendor {
		sb.WriteString("Вендорные зависимости: установлены\n")
	} else {
		sb.WriteString("Вендорные зависимости: не установлены\n")
	}
	if len(info.DockerServices) > 0 {
		sb.WriteString("Docker-сервисы: " + strings.Join(info.DockerServices, ", ") + "\n")
	}
	if len(info.MakeCommands) > 0 {
		sb.WriteString("Make-команды: " + strings.Join(info.MakeCommands, ", ") + "\n")
	}
	if len(info.Databases) > 0 {
		sb.WriteString("Базы данных:\n")
		for _, db := range info.Databases {
			if db.URL != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", db.URL))
			}
		}
	}

	return sb.String()
}

// interactiveLoop основной цикл взаимодействия с пользователем
func interactiveLoop(cfg *Config, history []HistoryEntry) error {
	reader := bufio.NewReader(os.Stdin)

	for {

		// Получаем ответ от LLM
		commands, err := queryLLM(cfg, history)
		if err != nil {
			return fmt.Errorf(i18n.T("LLM query failed: %w"), err)
		}

		// Проверяем на REFINE
		if len(commands) == 1 && commands[0].Command == "REFINE" {
			desc := commands[0].Description
			if desc == "" {
				desc = i18n.T("Request needs clarification")
			}
			i18n.Yellow("LLM: %s", desc)
			fmt.Println()
			i18n.Printf("Enter clarification: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "exit" || input == "q" {
				return nil
			}
			history = append(history, HistoryEntry{Role: "assistant", Content: formatCommandsJSON(commands)})
			history = append(history, HistoryEntry{Role: "user", Content: input})
			continue
		}

		// Выводим список команд и начинаем цикл выполнения/уточнения
		err = commandLoop(cfg, &history, commands, reader, false, false)
		if err != nil {
			return err
		}
		return nil
	}
}

// runShellCommand обрабатывает ввод пользователя, начинающийся с "!".
// Команда после "!" выполняется как обычная shell-команда, а результат
// добавляется в историю диалога (которая в будущем отправится к LLM).
// Возвращает nil, если команда выполнена (или вход пустой), и ошибку,
// если выполнение невозможно.
func runShellCommand(history *[]HistoryEntry, input string) error {
	shellCmd := strings.TrimSpace(input[1:])
	if shellCmd == "" {
		i18n.Yellow("Empty command after '!'. Usage: !command")
		return nil
	}

	fmt.Println()
	i18n.Cyan("=== Executing shell command: %s ===", shellCmd)
	output, execErr := runCommandStreaming(shellCmd)
	truncatedOutput := truncateOutput(output, MaxMessageLines)
	if execErr != nil {
		i18n.Red("Execution error: %v", execErr)
		*history = append(*history, HistoryEntry{
			Role:    "assistant",
			Content: fmt.Sprintf("Executed shell command: %s\nError: %v\nOutput: %s", shellCmd, execErr, truncatedOutput),
		})
	} else {
		i18n.Green("✓ Shell command executed successfully")
		*history = append(*history, HistoryEntry{
			Role:    "assistant",
			Content: fmt.Sprintf("Executed shell command: %s\nOutput: %s", shellCmd, truncatedOutput),
		})
	}
	return nil
}

// commandLoop цикл: показывает команды, ждёт ввод (цифра = выполнить, текст = уточнение).
// analysisExecuted — выполнялись ли analysis-команды (тогда в списке появляется пункт SEND_ANALYSIS);
// pendingAnalysis — есть ли результаты анализа, которые ещё не отправлены в LLM.
func commandLoop(cfg *Config, history *[]HistoryEntry, commands []CommandAction, reader *bufio.Reader, analysisExecuted, pendingAnalysis bool) error {
	for len(commands) > 0 {
		// Добавляем виртуальную команду SEND_ANALYSIS в конец списка, если analysis выполнялись
		commands = appendSendAnalysis(commands, analysisExecuted)

		fmt.Println()
		printCommands(commands, analysisExecuted)

		host, _ := os.Hostname()
		cwd, _ := os.Getwd()
		prompt := color.New(color.FgBlack, color.BgYellow).Sprintf(" %s@%s in dev:%s# ", os.Getenv("USER"), host, cwd)
		fmt.Printf("\nEnter command number to execute or text to refine request (default \"1\")\n%s", prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			input = "1"
		}

		// Проверяем, является ли ввод числом (номер команды)
		if idx, err := strconv.Atoi(input); err == nil {
			if idx < 1 || idx > len(commands) {
				i18n.Yellow("Invalid command number. Enter a number from 1 to %d.", len(commands))
				continue
			}

			cmd := commands[idx-1]

			// Специальная команда SEND_ANALYSIS — отправляем результаты анализа в LLM.
			if cmd.Command == "SEND_ANALYSIS" {
				fmt.Println()
				i18n.Cyan("=== Sending analysis results to LLM ===")
				*history = append(*history, HistoryEntry{
					Role:    "user",
					Content: "Анамнез собран. На основе полученных данных проанализируй проблему и предложи команды для её решения. Используй type: \"fix\" для финальных команд и type: \"sequence\" для промежуточных шагов.",
				})

				newCommands, err := queryLLM(cfg, *history)
				if err != nil {
					return fmt.Errorf(i18n.T("LLM query failed: %w"), err)
				}

				if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
					i18n.Yellow("LLM: %s", newCommands[0].Description)
					continue
				}

				commands = newCommands
				// Анализ только что отправлен вручную — новых неотправленных результатов нет.
				analysisExecuted = true
				pendingAnalysis = false
				continue
			}

			// Специальная команда FIX_ERROR — отправляем ошибку в LLM.
			if cmd.Command == "FIX_ERROR" {
				fmt.Println()
				i18n.Cyan("=== Sending error to LLM for fix ===")
				*history = append(*history, HistoryEntry{
					Role:    "user",
					Content: "Предыдущая команда завершилась с ошибкой. Исправь её и предложи новые команды.",
				})

				newCommands, err := queryLLM(cfg, *history)
				if err != nil {
					return fmt.Errorf(i18n.T("LLM query failed: %w"), err)
				}

				if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
					i18n.Yellow("LLM: %s", newCommands[0].Description)
					continue
				}

				commands = newCommands
				continue
			}

			// Выполняем выбранную команду.
			fmt.Println()
			i18n.Cyan("=== Executing: %s ===", cmd.Command)
			if cmd.Description != "" {
				i18n.White("Description: %s", cmd.Description)
			}

			output, execErr := runCommandStreaming(cmd.Command)
			truncatedOutput := truncateOutput(output, MaxMessageLines)
			if execErr != nil {
				i18n.Red("Execution error: %v", execErr)
				*history = append(*history, HistoryEntry{
					Role:    "assistant",
					Content: fmt.Sprintf("Executed command: %s\nError: %v\nOutput: %s", cmd.Command, execErr, truncatedOutput),
				})
			} else {
				i18n.Green("✓ Command executed successfully")
				*history = append(*history, HistoryEntry{
					Role:    "assistant",
					Content: fmt.Sprintf("Executed command: %s\nOutput: %s", cmd.Command, truncatedOutput),
				})
			}

			// Если выполнили analysis-команду — помечаем, что появились результаты,
			// которые нужно будет отправить в LLM
			if cmd.Type == CommandTypeAnalysis {
				analysisExecuted = true
				pendingAnalysis = true
			}

			// Убираем выполненную команду из списка
			commands = append(commands[:idx-1], commands[idx:]...)

			// Если команда завершилась с ошибкой — добавляем "Исправить ошибку" в конец списка
			if execErr != nil {
				fixCmd := CommandAction{
					Command:     "FIX_ERROR",
					Description: i18n.T("Fix error in command: %s", cmd.Command),
				}
				commands = append(commands, fixCmd)
			}

			if len(commands) > 0 {
				fmt.Println()
				i18n.Cyan("Remaining commands: %d", len(commands))
			}
			continue
		}

		// Если ввод начинается с "!" — выполняем команду как shell-команду,
		// а результат добавляем в историю (она в будущем отправится к LLM)
		if strings.HasPrefix(input, "!") {
			if err := runShellCommand(history, input); err != nil {
				return err
			}
			continue
		}

		// Если ввод не число — считаем уточнением
		if input == "exit" || input == "q" {
			return nil
		}

		fmt.Println()
		i18n.Cyan("=== Sending refinement to LLM ===")
		*history = append(*history, HistoryEntry{Role: "assistant", Content: formatCommandsJSON(commands)})
		*history = append(*history, HistoryEntry{Role: "user", Content: input})

		newCommands, err := queryLLM(cfg, *history)
		if err != nil {
			return fmt.Errorf(i18n.T("LLM query failed: %w"), err)
		}

		// Проверяем на REFINE
		if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
			desc := newCommands[0].Description
			if desc == "" {
				desc = i18n.T("Request needs clarification")
			}
			i18n.Yellow("LLM: %s", desc)
			continue
		}

		commands = newCommands
	}

	// После выполнения всех команд проверяем, остались ли неотправленные
	// результаты анализа — тогда автоматически запрашиваем решение у LLM
	if pendingAnalysis {
		fmt.Println()
		i18n.Cyan("=== Sending analysis results to LLM ===")
		*history = append(*history, HistoryEntry{
			Role:    "user",
			Content: "Анамнез собран. На основе полученных данных проанализируй проблему и предложи команды для её решения. Используй type: \"fix\" для финальных команд и type: \"sequence\" для промежуточных шагов.",
		})

		newCommands, err := queryLLM(cfg, *history)
		if err != nil {
			return fmt.Errorf(i18n.T("LLM query failed: %w"), err)
		}

		if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
			i18n.Yellow("LLM: %s", newCommands[0].Description)
			return nil
		}

		commands = newCommands
		// Рекурсивно запускаем commandLoop для новых команд. Состояние анализа
		// сохраняется, чтобы пункт SEND_ANALYSIS оставался доступным в списке,
		// а повторного автозапроса не происходило (pendingAnalysis=false).
		return commandLoop(cfg, history, commands, reader, true, false)
	}

	fmt.Println()
	i18n.Green("✓ All commands executed!")
	return nil
}

// queryLLM отправляет запрос к OpenAI-совместимому API через общий клиент
// и разбирает ответ в список команд. Сетевые/API-ошибки ретраит клиент;
// здесь повторяются только попытки с невалидным JSON: ответ добавляется
// в историю с просьбой вернуть корректный JSON.
func queryLLM(cfg *Config, history []HistoryEntry) ([]CommandAction, error) {
	client := NewClient(cfg)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		// Ограничиваем историю лимитами: каждое сообщение — MaxMessageLines строк,
		// суммарный объём — MaxRequestChars символов.
		history = prepareHistoryForSend(history)

		rawContent, err := client.Chat(history, 0.1)
		if err != nil {
			return nil, err
		}

		content := extractJSON(rawContent)

		// Парсим команды.
		var commands []CommandAction
		if err := json.Unmarshal([]byte(content), &commands); err != nil {
			i18n.Red("LLM returned invalid JSON (attempt %d/3)", attempt+1)
			// Добавляем в историю ответ LLM и просьбу исправиться.
			history = append(history, HistoryEntry{
				Role:    "assistant",
				Content: rawContent,
			})
			history = append(history, HistoryEntry{
				Role:    "user",
				Content: "Этот ответ содержит невалидный JSON. Верни ТОЛЬКО валидный JSON-массив в формате [{\"command\": \"...\", \"description\": \"...\", \"type\": \"...\"}] без каких-либо пояснений.",
			})
			lastErr = err
			continue
		}

		return commands, nil
	}

	return nil, fmt.Errorf(i18n.T("LLM request failed after 3 attempts: %w"), lastErr)
}

// extractJSON извлекает JSON из markdown-блока если есть
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Убираем ```json ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}

// formatCommandsJSON форматирует команды в JSON для истории
func formatCommandsJSON(commands []CommandAction) string {
	data, _ := json.Marshal(commands)
	return string(data)
}

// appendSendAnalysis добавляет виртуальную команду SEND_ANALYSIS в конец списка,
// если выполнялись analysis-команды и такого пункта ещё нет.
func appendSendAnalysis(commands []CommandAction, analysisExecuted bool) []CommandAction {
	if !analysisExecuted {
		return commands
	}
	for _, c := range commands {
		if c.Command == "SEND_ANALYSIS" {
			return commands
		}
	}
	return append(commands, CommandAction{
		Command:     "SEND_ANALYSIS",
		Description: "Send analysis results to LLM for solution",
	})
}

// printCommands выводит список команд, разбитый на три блока:
// 1. Analysis — диагностические команды
// 2. Action — команды для выполнения (sequence/fix)
// 3. LLM — команды для обращения к LLM (FIX_ERROR, SEND_ANALYSIS)
func printCommands(commands []CommandAction, analysisExecuted bool) {
	// Белый фон для обычных команд
	bg := color.New(color.FgBlack, color.BgWhite)
	// Голубой фон для analysis-команд
	analysisBg := color.New(color.FgBlack, color.BgCyan)
	// Жёлтый фон для LLM-команд
	llmBg := color.New(color.FgBlack, color.BgYellow)

	// Разделяем команды на три группы
	var analysisCmds, actionCmds, llmCmds []CommandAction
	for _, cmd := range commands {
		switch {
		case cmd.Type == CommandTypeAnalysis:
			analysisCmds = append(analysisCmds, cmd)
		case cmd.Command == "FIX_ERROR" || cmd.Command == "SEND_ANALYSIS":
			llmCmds = append(llmCmds, cmd)
		default:
			actionCmds = append(actionCmds, cmd)
		}
	}

	// Если есть analysis-команды — пишем пояснение перед всеми блоками
	if len(analysisCmds) > 0 {
		i18n.White("LLM doesn't understand what happened, so it suggests an analysis")
	}

	// Блок 1: Analysis commands
	if len(analysisCmds) > 0 {
		fmt.Println()
		i18n.Cyan("── Analysis commands ──")
		for i, cmd := range analysisCmds {
			commandStr := analysisBg.Sprintf(" %s ", cmd.Command)
			if cmd.Description != "" {
				fmt.Printf("%d. %s [%s]\n", i+1, commandStr, cmd.Description)
			} else {
				fmt.Printf("%d. %s\n", i+1, commandStr)
			}
		}
	}

	// Блок 2: Action commands
	if len(actionCmds) > 0 {
		if len(analysisCmds) > 0 {
			fmt.Println()
			i18n.Cyan("── Action commands ──")
		}
		for i, cmd := range actionCmds {
			idx := len(analysisCmds) + i + 1
			commandStr := bg.Sprintf(" %s ", cmd.Command)

			if cmd.Description != "" {
				fmt.Printf("%d. %s [%s]", idx, commandStr, cmd.Description)
			} else {
				fmt.Printf("%d. %s", idx, commandStr)
			}

			// Рисуем стрелку вниз, если команда типа sequence и это не последняя action-команда
			// и следующая команда не FIX_ERROR
			isLast := (i == len(actionCmds)-1)
			nextIsFixError := false
			if !isLast && actionCmds[i+1].Command == "FIX_ERROR" {
				nextIsFixError = true
			}
			if cmd.Type == CommandTypeSequence && !isLast && !nextIsFixError {
				fmt.Print("  ↓")
			}
			fmt.Println()
		}
	}

	// Блок 3: LLM commands (FIX_ERROR, SEND_ANALYSIS)
	if len(llmCmds) > 0 {
		fmt.Println()
		i18n.Cyan("── LLM commands ──")
		for i, cmd := range llmCmds {
			idx := len(analysisCmds) + len(actionCmds) + i + 1
			commandStr := llmBg.Sprintf(" %s ", cmd.Command)
			if cmd.Description != "" {
				fmt.Printf("%d. %s [%s]\n", idx, commandStr, cmd.Description)
			} else {
				fmt.Printf("%d. %s\n", idx, commandStr)
			}
		}
	}
}

// truncateOutput обрезает вывод до последних n строк
func truncateOutput(output string, n int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= n {
		return output
	}

	truncated := lines[len(lines)-n:]
	omitted := len(lines) - n
	return i18n.T("... (%d lines omitted) ...\n%s", omitted, strings.Join(truncated, "\n"))
}

// runCommandStreaming выполняет команду в shell и выводит результат построчно по мере поступления.
// Возвращает полный вывод (как runCommand) для сохранения в историю.
func runCommandStreaming(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf(i18n.T("failed to create stdout pipe: %w"), err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf(i18n.T("failed to create stderr pipe: %w"), err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf(i18n.T("failed to start command: %w"), err)
	}

	var outputBuf strings.Builder
	outputBuf.Grow(4096)

	// Канал для сбора завершения чтения потоков
	done := make(chan error, 2)

	// Читаем stdout построчно
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			outputBuf.WriteString(line)
			outputBuf.WriteByte('\n')
		}
		done <- scanner.Err()
	}()

	// Читаем stderr построчно
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			// stderr выводим в красном цвете, чтобы было видно, что это ошибка
			color.Red(line)
			outputBuf.WriteString(line)
			outputBuf.WriteByte('\n')
		}
		done <- scanner.Err()
	}()

	// Ждём завершения чтения обоих потоков
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			// Ждём завершения процесса перед возвратом ошибки сканера
			cmd.Wait()
			return strings.TrimSpace(outputBuf.String()), fmt.Errorf(i18n.T("error reading command output: %w"), err)
		}
	}

	// Ждём завершения команды
	err = cmd.Wait()
	output := strings.TrimSpace(outputBuf.String())

	if err != nil {
		return output, fmt.Errorf(i18n.T("command failed (exit code %d)"), cmd.ProcessState.ExitCode())
	}

	return output, nil
}
