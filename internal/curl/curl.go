package curl

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Run выполняет curl-запрос и обрабатывает результат
func Run(rawURL, method string) error {
	// Добавляем https:// если протокол не указан
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	// Парсим URL для получения имени хоста (для имени файла)
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("неверный URL: %v", err)
	}

	// Приводим метод к верхнему регистру
	method = strings.ToUpper(method)
	switch method {
	case "GET", "POST", "PUT", "DELETE":
		// валидные методы
	default:
		method = "GET" // по умолчанию
	}

	// Создаём HTTP-клиент с insecure (отключаем проверку сертификата)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // разрешаем редиректы
		},
	}

	// Создаём запрос
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %v", err)
	}

	// Выполняем запрос
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %v", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)

	// Читаем тело ответа
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Выводим статус
	statusColor := colorForStatus(resp.StatusCode)
	statusColor.Printf("%s %s\n", method, rawURL)
	statusColor.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Duration: %v\n", elapsed)
	fmt.Printf("Content-Length: %d bytes\n", len(bodyBytes))
	fmt.Println()

	// Спрашиваем пользователя, что делать
	return askUser(rawURL, parsedURL.Host, bodyBytes)
}

// askUser спрашивает, показать вывод или сохранить в файл
func askUser(rawURL, host string, body []byte) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("1. Show output\n2. Save to file\nChoose [1/2]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1", "":
			fmt.Println()
			fmt.Println(string(body))
			return nil

		case "2":
			return saveToFile(host, body)

		default:
			color.Yellow("Unknown option. Use 1 or 2.")
		}
	}
}

// saveToFile сохраняет тело ответа в файл
func saveToFile(host string, body []byte) error {
	// Очищаем имя хоста для имени файла
	filename := sanitizeFilename(host) + ".txt"

	// Проверяем, существует ли файл
	if _, err := os.Stat(filename); err == nil {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("File %s already exists. Overwrite? [y/N]: ", filename)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			color.Yellow("Skipped.")
			return nil
		}
	}

	if err := os.WriteFile(filename, body, 0644); err != nil {
		return fmt.Errorf("ошибка записи файла: %v", err)
	}

	absPath, _ := filepath.Abs(filename)
	color.Green("Saved to %s (%d bytes)", absPath, len(body))
	return nil
}

// colorForStatus возвращает цвет в зависимости от кода ответа
func colorForStatus(code int) *color.Color {
	switch {
	case code >= 200 && code < 300:
		return color.New(color.FgGreen)
	case code >= 300 && code < 400:
		return color.New(color.FgCyan)
	case code >= 400 && code < 500:
		return color.New(color.FgYellow)
	default:
		return color.New(color.FgRed)
	}
}

// sanitizeFilename заменяет недопустимые символы в имени файла
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		"?", "_",
		"*", "_",
		"\"", "_",
		"'", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}
