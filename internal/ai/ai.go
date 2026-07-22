package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

// CommandAction представляет одну команду от LLM
type CommandAction struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
}

// HistoryEntry представляет запись истории диалога
type HistoryEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatMessage для запроса к OpenAI-compatible API
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// RunAI основная функция для dev ai
func RunAI(text string) error {
	cfg, err := LoadConfig()
	if err != nil {
		color.Red("Config error: %v", err)
		if err := InteractiveEditConfig(); err != nil {
			return err
		}
		// Пробуем снова после редактирования
		cfg, err = LoadConfig()
		if err != nil {
			return fmt.Errorf("config still invalid after edit: %w", err)
		}
	}

	// Собираем контекст проекта
	contextInfo := buildContext()

	history := []HistoryEntry{
		{Role: "system", Content: buildSystemPrompt(contextInfo)},
		{Role: "user", Content: text},
	}

	return interactiveLoop(cfg, history)
}

// buildSystemPrompt создаёт системный промпт с контекстом
func buildSystemPrompt(contextInfo string) string {
	return fmt.Sprintf(`Ты — ассистент для разработчика. Отвечай ТОЛЬКО в формате JSON.
Дай список последовательных команд для выполнения в терминале Linux.

Формат ответа:
[{"command": "команда", "description": "что делает"}, {"command": "следующая команда", "description": "что делает"}]

Контекст текущего проекта:
%s

Доступные команды dev (CLI-утилита):
- dev analyze — анализ проекта (язык, фреймворк, БД, docker)
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
Например: "cd /some/dir && make && ./binary"

Правила:
1. Отвечай ТОЛЬКО JSON-массивом, без пояснений
2. Команды должны быть безопасными и последовательными
3. Если запрос неясен, верни [{"command": "REFINE", "description": "пояснение почему нужны уточнения"}]
4. Для команд, которые требуют подтверждения (rm, dd, format и т.д.), добавь флаги подтверждения
5. Учитывай язык и фреймворк проекта`, contextInfo)
}

// buildContext собирает информацию о проекте
func buildContext() string {
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

	// Пытаемся выполнить dev analyze
	sb.WriteString("\nРезультат dev analyze:\n")
	analyzeOut, err := exec.Command("dev", "analyze").Output()
	if err == nil {
		sb.WriteString(string(analyzeOut))
	} else {
		sb.WriteString("(не удалось выполнить dev analyze)\n")
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
			return fmt.Errorf("LLM query failed: %w", err)
		}

		// Проверяем на REFINE
		if len(commands) == 1 && commands[0].Command == "REFINE" {
			desc := commands[0].Description
			if desc == "" {
				desc = "Request needs clarification"
			}
			color.Yellow("LLM: %s", desc)
			fmt.Print("\nEnter clarification: ")
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
		err = commandLoop(cfg, &history, commands, reader)
		if err != nil {
			return err
		}
		return nil
	}
}

// commandLoop цикл: показывает команды, ждёт ввод (цифра = выполнить, текст = уточнение)
func commandLoop(cfg *Config, history *[]HistoryEntry, commands []CommandAction, reader *bufio.Reader) error {
	for len(commands) > 0 {
		fmt.Println()
		printCommands(commands)

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
				color.Yellow("Invalid command number. Enter a number from 1 to %d.", len(commands))
				continue
			}

			cmd := commands[idx-1]

			// Special "Fix error" command — send to LLM
			if cmd.Command == "FIX_ERROR" {
				color.Cyan("\n=== Sending error to LLM for fix ===")
				*history = append(*history, HistoryEntry{
					Role:    "user",
					Content: "Предыдущая команда завершилась с ошибкой. Исправь её и предложи новые команды.",
				})

				newCommands, err := queryLLM(cfg, *history)
				if err != nil {
					return fmt.Errorf("LLM query failed: %w", err)
				}

				if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
					color.Yellow("LLM: %s", newCommands[0].Description)
					continue
				}

				commands = newCommands
				continue
			}

			// Execute the selected command
			color.Cyan("\n=== Executing: %s ===", cmd.Command)
			if cmd.Description != "" {
				color.White("Description: %s", cmd.Description)
			}

			output, execErr := runCommand(cmd.Command)
			truncatedOutput := truncateOutput(output, 30)
			if execErr != nil {
				color.Red("Execution error: %v", execErr)
				*history = append(*history, HistoryEntry{
					Role:    "assistant",
					Content: fmt.Sprintf("Executed command: %s\nError: %v\nOutput: %s", cmd.Command, execErr, truncatedOutput),
				})
			} else {
				color.Green("✓ Command executed successfully")
				*history = append(*history, HistoryEntry{
					Role:    "assistant",
					Content: fmt.Sprintf("Executed command: %s\nOutput: %s", cmd.Command, truncatedOutput),
				})
			}

			if output != "" {
				fmt.Println()
				fmt.Println(output)
			}

			// Убираем выполненную команду из списка
			commands = append(commands[:idx-1], commands[idx:]...)

			// Если команда завершилась с ошибкой — добавляем "Исправить ошибку" в конец списка
			if execErr != nil {
				fixCmd := CommandAction{
					Command:     "FIX_ERROR",
					Description: fmt.Sprintf("Fix error in command: %s", cmd.Command),
				}
				commands = append(commands, fixCmd)
			}

			if len(commands) > 0 {
				color.Cyan("\nRemaining commands: %d", len(commands))
			}
			continue
		}

		// Если ввод не число — считаем уточнением
		if input == "exit" || input == "q" {
			return nil
		}

		color.Cyan("\n=== Sending refinement to LLM ===")
		*history = append(*history, HistoryEntry{Role: "assistant", Content: formatCommandsJSON(commands)})
		*history = append(*history, HistoryEntry{Role: "user", Content: input})

		newCommands, err := queryLLM(cfg, *history)
		if err != nil {
			return fmt.Errorf("LLM query failed: %w", err)
		}

		// Проверяем на REFINE
		if len(newCommands) == 1 && newCommands[0].Command == "REFINE" {
			desc := newCommands[0].Description
			if desc == "" {
				desc = "Request needs clarification"
			}
			color.Yellow("LLM: %s", desc)
			continue
		}

		commands = newCommands
	}

	color.Green("\n✓ All commands executed!")
	return nil
}

// queryLLM отправляет запрос к OpenAI-совместимому API
func queryLLM(cfg *Config, history []HistoryEntry) ([]CommandAction, error) {
	// Преобразуем историю в формат chatMessage
	messages := make([]chatMessage, len(history))
	for i, entry := range history {
		messages[i] = chatMessage{
			Role:    entry.Role,
			Content: entry.Content,
		}
	}

	reqBody := chatRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: 0.1,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Выполняем curl-запрос
	curlCmd := exec.Command("curl", "-s",
		"-k",
		"-X", "POST",
		cfg.Endpoint,
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer "+cfg.Token,
		"-d", string(jsonData),
	)

	var stdout, stderr bytes.Buffer
	curlCmd.Stdout = &stdout
	curlCmd.Stderr = &stderr

	if err := curlCmd.Run(); err != nil {
		return nil, fmt.Errorf("curl failed: %w\nStderr: %s", err, stderr.String())
	}

	// Парсим ответ
	var resp chatResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nBody: %s", err, stdout.String())
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	content := resp.Choices[0].Message.Content

	// Извлекаем JSON из ответа (может быть обёрнут в ```json ... ```)
	content = extractJSON(content)

	// Парсим команды
	var commands []CommandAction
	if err := json.Unmarshal([]byte(content), &commands); err != nil {
		return nil, fmt.Errorf("failed to parse commands JSON: %w\nContent: %s", err, content)
	}

	return commands, nil
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

// printCommands выводит список команд
func printCommands(commands []CommandAction) {
	// Белый фон для команды
	bg := color.New(color.FgBlack, color.BgWhite)
	for i, cmd := range commands {
		commandStr := bg.Sprintf(" %s ", cmd.Command)
		if cmd.Description != "" {
			fmt.Printf("%d. %s [%s]\n", i+1, commandStr, cmd.Description)
		} else {
			fmt.Printf("%d. %s\n", i+1, commandStr)
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
	return fmt.Sprintf("... (%d lines omitted) ...\n%s", omitted, strings.Join(truncated, "\n"))
}

// runCommand выполняет команду в shell и возвращает вывод и ошибку
func runCommand(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += strings.TrimSpace(stderr.String())
	}

	// Если команда завершилась с ненулевым кодом возврата — это ошибка выполнения
	if err != nil {
		return output, fmt.Errorf("command failed (exit code %d)", cmd.ProcessState.ExitCode())
	}

	return output, nil
}
